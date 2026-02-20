import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:integration_test/integration_test.dart';

import 'package:keenbench/app_keys.dart';
import 'package:keenbench/app_env.dart';
import 'package:keenbench/engine/engine_client.dart';

import 'support/e2e_screenshots.dart';
import 'support/e2e_utils.dart';

void main() {
  IntegrationTestWidgetsFlutterBinding.ensureInitialized();
  final screenshooter = E2eScreenshooter();

  testWidgets('m1 office ops -> review previews', (tester) async {
    AppEnv.setOverride('KEENBENCH_FAKE_OPENAI', '0');
    AppEnv.setOverride('KEENBENCH_FAKE_TOOL_WORKER', '0');
    final harness = E2eHarness(tester, screenshooter);
    final binding = tester.binding;
    await binding.setSurfaceSize(const Size(1600, 1000));
    try {
      await harness.launchApp();
      await _configureValidKey(tester, harness);
      await harness.backToHome();

      await harness.createWorkbench(name: 'E2E M1 File Ops');

      final state = harness.workbenchState();
      final docx = officeFixturePath('simple.docx');
      final xlsx = officeFixturePath('multi-sheet.xlsx');
      final pptx = officeFixturePath('slides.pptx');
      final pdf = officeFixturePath('report.pdf');
      final png = officeFixturePath('chart.png');
      final svg = officeFixturePath('logo.svg');
      final bin = officeFixturePath('unknown.bin');

      await tester.runAsync(() async {
        await state.addFiles([docx, xlsx, pptx, pdf, png, svg, bin]);
      });
      await tester.pumpAndSettle();
      expect(state.files.any((file) => file.isOpaque), isTrue);

      await tester.runAsync(() async {
        final status =
            await harness.engineApi().call('EgressGetConsentStatus', {
                  'workbench_id': state.workbenchId,
                })
                as Map<String, dynamic>;
        await harness.engineApi().call('EgressGrantWorkshopConsent', {
          'workbench_id': state.workbenchId,
          'provider_id': 'openai',
          'model_id': status['model_id'],
          'scope_hash': status['scope_hash'],
        });
      });

      await _sendMessageWithRetries(
        tester,
        state,
        'Create exactly one draft change by editing simple.docx and changing a single sentence. Do not modify any other files.',
      );
      await tester.pumpAndSettle();
      await pumpUntilFound(
        find.byKey(AppKeys.reviewScreen),
        tester: tester,
        timeout: const Duration(seconds: 90),
      );

      String? targetPath;
      await tester.runAsync(() async {
        final changeResp =
            await harness.engineApi().call('ReviewGetChangeSet', {
                  'workbench_id': state.workbenchId,
                })
                as Map<String, dynamic>;
        final changes = (changeResp['changes'] as List<dynamic>? ?? [])
            .cast<Map<String, dynamic>>();
        if (changes.isEmpty) {
          fail('Expected at least one review change.');
        }
        targetPath = changes.first['path'] as String?;
      });
      if (targetPath == null || targetPath!.isEmpty) {
        fail('Expected at least one review change with a valid path.');
      }
      await pumpUntilFound(
        find.text(targetPath!),
        tester: tester,
        timeout: const Duration(seconds: 30),
      );

      await _tapVisible(tester, find.text(targetPath!));
      await pumpUntilFound(find.text('Summary'), tester: tester);

      await _tapVisible(tester, find.byKey(AppKeys.reviewPublishButton));
      await pumpUntilFound(find.byKey(AppKeys.workbenchScreen), tester: tester);
      await tester.pumpAndSettle();
      expect(find.byKey(AppKeys.workbenchDraftBanner), findsNothing);

      await tester.runAsync(() async {
        final stateResp =
            await harness.engineApi().call('DraftGetState', {
                  'workbench_id': state.workbenchId,
                })
                as Map<String, dynamic>;
        expect(stateResp['has_draft'], isFalse);
      });
    } finally {
      await binding.setSurfaceSize(null);
      await harness.stopEngine();
      await cleanupE2eConfig();
    }
  });
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

Future<void> _tapVisible(WidgetTester tester, Finder finder) async {
  await tester.ensureVisible(finder);
  await tester.pumpAndSettle();
  await tester.tap(finder);
  await tester.pumpAndSettle();
}

Future<void> _sendMessageWithRetries(
  WidgetTester tester,
  dynamic state,
  String message, {
  int maxAttempts = 2,
}) async {
  Object? lastError;
  for (var attempt = 1; attempt <= maxAttempts; attempt++) {
    try {
      await tester.runAsync(() async {
        await state.sendMessage(message);
      });
      return;
    } catch (error) {
      lastError = error;
      if (!_isTransientAgentError(error) || attempt == maxAttempts) {
        rethrow;
      }
      await tester.pumpAndSettle();
    }
  }
  throw StateError('message send failed after retries: $lastError');
}

bool _isTransientAgentError(Object error) {
  if (error is EngineError) {
    return error.errorCode == 'AGENT_LOOP_DETECTED' ||
        error.errorCode == 'VALIDATION_FAILED';
  }
  final text = error.toString();
  return text.contains('AGENT_LOOP_DETECTED') ||
      text.contains('VALIDATION_FAILED');
}
