import 'dart:async';
import 'dart:io';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:integration_test/integration_test.dart';
import 'package:path/path.dart' as path;

import 'package:keenbench/app_env.dart';
import 'package:keenbench/app_keys.dart';
import 'package:keenbench/engine/engine_client.dart';
import 'package:keenbench/state/workbench_state.dart';

import 'support/e2e_screenshots.dart';
import 'support/e2e_utils.dart';

void main() {
  IntegrationTestWidgetsFlutterBinding.ensureInitialized();
  final screenshooter = E2eScreenshooter();

  testWidgets(
    'rpi workflow creates categorized workbook with summary-only chat output',
    (tester) async {
      final harness = E2eHarness(tester, screenshooter);
      final binding = tester.binding;
      await binding.setSurfaceSize(const Size(1600, 1000));

      addTearDown(() async {
        await binding.setSurfaceSize(null);
        await harness.stopEngine();
        await cleanupE2eConfig();
      });

      try {
        AppEnv.setOverride('KEENBENCH_FAKE_OPENAI', '0');
        AppEnv.setOverride('KEENBENCH_FAKE_TOOL_WORKER', '0');

        await harness.launchApp();
        await _configureValidKey(tester, harness);
        await _setOpenAIReasoningEffort(tester, harness);
        await harness.backToHome();

        final sourceXlsx = _realFixturePath(
          'cuentas_octubre_2024_anonymized_draft.xlsx',
        );
        final prompt =
            'Break down expenses by category into separate sheets in a new Excel file named expenses_by_category.xlsx. '
            'Add a Summary sheet with category totals and a Grand Total row. '
            'Use values only and do not inspect or preserve formatting/styles.';

        WorkbenchState? state;
        Object? lastTransientError;
        for (var attempt = 1; attempt <= 2; attempt++) {
          await harness.createWorkbench(
            name:
                'E2E RPI ${DateTime.now().millisecondsSinceEpoch} attempt-$attempt',
          );
          state = harness.workbenchState();

          await tester.runAsync(() async {
            await state!.addFiles([sourceXlsx]);
          });
          await tester.pumpAndSettle();

          try {
            await _sendMessageWithConsentRetries(
              tester,
              harness: harness,
              state: state,
              prompt: prompt,
            );
            break;
          } catch (error) {
            if (!_isTransientAgentFailure(error) || attempt == 2) {
              rethrow;
            }
            lastTransientError = error;
            await harness.backToHome();
          }
        }

        final activeState = state;
        if (activeState == null) {
          throw StateError(
            'Failed to create RPI workbench: $lastTransientError',
          );
        }

        await pumpUntilFound(
          find.byKey(AppKeys.reviewScreen),
          tester: tester,
          timeout: const Duration(seconds: 300),
        );

        await tester.runAsync(() async {
          expect(
            activeState.files.any((file) => file.path.contains('_rpi')),
            isFalse,
            reason: 'Internal RPI artifacts must not appear in file list.',
          );

          final conversation =
              await harness.engineApi().call('WorkshopGetConversation', {
                    'workbench_id': activeState.workbenchId,
                  })
                  as Map<String, dynamic>;
          final messages = (conversation['messages'] as List<dynamic>? ?? [])
              .cast<Map<String, dynamic>>();
          final assistantMessages = messages
              .where(
                (message) =>
                    message['role'] == 'assistant' &&
                    message['type'] == 'assistant_message',
              )
              .toList();
          expect(
            assistantMessages.length >= 1,
            isTrue,
            reason: 'Expected at least one summary assistant output.',
          );

          final changeSet =
              await harness.engineApi().call('ReviewGetChangeSet', {
                    'workbench_id': activeState.workbenchId,
                  })
                  as Map<String, dynamic>;
          final changes = (changeSet['changes'] as List<dynamic>? ?? [])
              .cast<Map<String, dynamic>>();
          final xlsxChanges = changes
              .where(
                (change) => (change['path'] as String? ?? '').endsWith('.xlsx'),
              )
              .toList();
          expect(xlsxChanges, isNotEmpty);

          String targetPath = 'expenses_by_category.xlsx';
          if (!xlsxChanges.any((change) => change['path'] == targetPath)) {
            targetPath = xlsxChanges.first['path'] as String;
          }

          final preview =
              await harness.engineApi().call('ReviewGetXlsxPreviewGrid', {
                    'workbench_id': activeState.workbenchId,
                    'path': targetPath,
                    'version': 'draft',
                    'sheet': 'Summary',
                    'row_start': 0,
                    'row_count': 200,
                    'col_start': 0,
                    'col_count': 12,
                  })
                  as Map<String, dynamic>;

          final sheets = (preview['sheets'] as List<dynamic>? ?? [])
              .cast<String>();
          expect(
            sheets.length >= 3,
            isTrue,
            reason: 'Expected multiple category sheets plus Summary.',
          );

          final cells = (preview['cells'] as List<dynamic>? ?? [])
              .cast<List<dynamic>>()
              .expand((row) => row)
              .cast<Map<String, dynamic>>();
          final values = cells
              .map((cell) => _toDouble(cell['value']))
              .whereType<double>()
              .toList();
          const expectedGrandTotalAbs = 12257.08;
          final hasExpectedTotal = values.any(
            (value) => (value.abs() - expectedGrandTotalAbs).abs() < 0.5,
          );
          expect(
            hasExpectedTotal,
            isTrue,
            reason:
                'Expected to find grand total with absolute value 12257.08 in Summary sheet.',
          );
        });
      } catch (error) {
        await screenshooter.capture('failure_rpi_workflow');
        rethrow;
      }
    },
    timeout: const Timeout(Duration(minutes: 20)),
  );
}

Future<void> _configureValidKey(WidgetTester tester, E2eHarness harness) async {
  await harness.openSettings();
  final key = requireOpenAIKeyForTests();
  await enterTextAndEnsure(
    tester,
    find.byKey(AppKeys.settingsApiKeyField),
    key,
  );
  await tester.tap(find.byKey(AppKeys.settingsSaveButton));
  await tester.pumpAndSettle();
}

Future<void> _setOpenAIReasoningEffort(
  WidgetTester tester,
  E2eHarness harness,
) async {
  await tester.runAsync(() async {
    await harness.engineApi().call('ProvidersSetReasoningEffort', {
      'provider_id': 'openai',
      'research_effort': 'medium',
      'plan_effort': 'low',
      'implement_effort': 'low',
    });
  });
}

Future<void> _sendMessageWithConsentRetries(
  WidgetTester tester, {
  required E2eHarness harness,
  required WorkbenchState state,
  required String prompt,
  int maxAttempts = 2,
}) async {
  Object? lastError;
  for (var attempt = 1; attempt <= maxAttempts; attempt++) {
    try {
      final status =
          await harness.engineApi().call('EgressGetConsentStatus', {
                'workbench_id': state.workbenchId,
              })
              as Map<String, dynamic>;
      if (status['consented'] != true) {
        await harness.engineApi().call('EgressGrantWorkshopConsent', {
          'workbench_id': state.workbenchId,
          'provider_id': status['provider_id'] ?? 'openai',
          'model_id': status['model_id'],
          'scope_hash': status['scope_hash'],
        });
      }
      await _sendMessageWithPumping(tester, state: state, prompt: prompt);
      return;
    } catch (error) {
      lastError = error;
      if (!_isTransientAgentFailure(error) || attempt == maxAttempts) {
        rethrow;
      }
      await tester.pumpAndSettle();
    }
  }
  throw StateError('RPI send failed after retries: $lastError');
}

Future<void> _sendMessageWithPumping(
  WidgetTester tester, {
  required WorkbenchState state,
  required String prompt,
}) async {
  const maxWait = Duration(minutes: 19, seconds: 30);
  final deadline = DateTime.now().add(maxWait);
  Object? error;
  StackTrace? stackTrace;
  var done = false;
  unawaited(() async {
    try {
      await state.sendMessage(prompt);
    } catch (err, st) {
      error = err;
      stackTrace = st;
    } finally {
      done = true;
    }
  }());

  while (!done) {
    if (DateTime.now().isAfter(deadline)) {
      try {
        await state.cancelRun();
      } catch (_) {
        // Ignore cancellation errors; timeout is the primary failure signal.
      }
      throw TimeoutException(
        'Timed out waiting for sendMessage to complete.',
        maxWait,
      );
    }
    await tester.pump(const Duration(milliseconds: 100));
  }
  if (error != null) {
    Error.throwWithStackTrace(error!, stackTrace ?? StackTrace.current);
  }
}

bool _isTransientAgentFailure(Object error) {
  if (error is EngineError) {
    return error.errorCode == 'AGENT_LOOP_DETECTED' ||
        error.errorCode == 'VALIDATION_FAILED';
  }
  final text = error.toString();
  return text.contains('AGENT_LOOP_DETECTED') ||
      text.contains('VALIDATION_FAILED');
}

String _realFixturePath(String name) {
  final root = _resolveRepoRoot();
  return path.join(root, 'engine', 'testdata', 'real', name);
}

String _resolveRepoRoot() {
  var dir = Directory.current;
  for (var i = 0; i < 6; i++) {
    final makefile = File(path.join(dir.path, 'Makefile'));
    if (makefile.existsSync()) {
      return dir.path;
    }
    dir = dir.parent;
  }
  return Directory.current.path;
}

double? _toDouble(dynamic value) {
  if (value is num) {
    return value.toDouble();
  }
  if (value is! String) {
    return null;
  }
  final normalized = value
      .replaceAll(',', '')
      .replaceAll('\$', '')
      .replaceAll('€', '')
      .trim();
  if (normalized.isEmpty) {
    return null;
  }
  return double.tryParse(normalized);
}
