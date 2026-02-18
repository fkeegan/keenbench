import 'dart:io';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:integration_test/integration_test.dart';
import 'package:path/path.dart' as path;

import 'package:keenbench/app_env.dart';
import 'package:keenbench/app_keys.dart';

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
        await harness.backToHome();

        await harness.createWorkbench(
          name: 'E2E RPI ${DateTime.now().millisecondsSinceEpoch}',
        );
        final state = harness.workbenchState();
        final sourceXlsx = _realFixturePath(
          'cuentas_octubre_2024_anonymized_draft.xlsx',
        );

        await tester.runAsync(() async {
          await state.addFiles([sourceXlsx]);
        });
        await tester.pumpAndSettle();

        await _sendMessageWithConsent(
          tester,
          prompt:
              'Break down expenses by category into separate sheets in a new Excel file named expenses_by_category.xlsx. '
              'Add a Summary sheet with category totals and a Grand Total row.',
        );

        await pumpUntilFound(
          find.byKey(AppKeys.reviewScreen),
          tester: tester,
          timeout: const Duration(seconds: 300),
        );

        await tester.runAsync(() async {
          expect(
            state.files.any((file) => file.path.contains('_rpi')),
            isFalse,
            reason: 'Internal RPI artifacts must not appear in file list.',
          );

          final conversation =
              await harness.engineApi().call('WorkshopGetConversation', {
                    'workbench_id': state.workbenchId,
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
            assistantMessages.length,
            1,
            reason: 'Only summary assistant output should be persisted.',
          );

          final changeSet =
              await harness.engineApi().call('ReviewGetChangeSet', {
                    'workbench_id': state.workbenchId,
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
                    'workbench_id': state.workbenchId,
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
          final hasExpectedTotal = values.any(
            (value) => (value - (-12257.08)).abs() < 0.5,
          );
          expect(
            hasExpectedTotal,
            isTrue,
            reason: 'Expected to find grand total -12257.08 in Summary sheet.',
          );
        });
      } catch (error) {
        await screenshooter.capture('failure_rpi_workflow');
        rethrow;
      }
    },
    timeout: const Timeout(Duration(minutes: 6)),
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

Future<void> _sendMessageWithConsent(
  WidgetTester tester, {
  required String prompt,
}) async {
  await enterTextAndEnsure(
    tester,
    find.byKey(AppKeys.workbenchComposerField),
    prompt,
  );
  await tester.tap(find.byKey(AppKeys.workbenchSendButton));
  await tester.pumpAndSettle();
  if (find.byKey(AppKeys.consentDialog).evaluate().isNotEmpty) {
    await tester.tap(find.byKey(AppKeys.consentContinueButton));
    await tester.pumpAndSettle();
  }
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
