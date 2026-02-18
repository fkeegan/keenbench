import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:integration_test/integration_test.dart';

import 'package:keenbench/app_keys.dart';
import 'package:keenbench/app_env.dart';

import 'support/e2e_screenshots.dart';
import 'support/e2e_utils.dart';

void main() {
  IntegrationTestWidgetsFlutterBinding.ensureInitialized();
  final screenshooter = E2eScreenshooter();

  testWidgets('m1 office ops -> review previews', (tester) async {
    final useReal = AppEnv.get('KEENBENCH_REAL_OPENAI') == '1';
    if (!useReal) {
      AppEnv.setOverride('KEENBENCH_FAKE_OPENAI', '1');
    }
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

      await tester.runAsync(() async {
        await state.sendMessage('Update the office files. [proposal_ops]');
      });
      await tester.pumpAndSettle();
      await pumpUntilFound(
        find.byKey(AppKeys.reviewScreen),
        tester: tester,
        timeout: const Duration(seconds: 30),
      );

      final isFake = isFakeOpenAIEnabled();
      String? targetPath;
      await tester.runAsync(() async {
        final changeResp =
            await harness.engineApi().call('ReviewGetChangeSet', {
                  'workbench_id': state.workbenchId,
                })
                as Map<String, dynamic>;
        final changes = (changeResp['changes'] as List<dynamic>? ?? [])
            .cast<Map<String, dynamic>>();
        if (changes.isNotEmpty) {
          targetPath = changes.first['path'] as String?;
        }
      });
      if (isFake) {
        await pumpUntilFound(
          find.text('report.docx'),
          tester: tester,
          timeout: const Duration(seconds: 30),
        );
        expect(find.text('DOCX'), findsWidgets);
        expect(find.text('XLSX'), findsWidgets);
        expect(find.text('PPTX'), findsWidgets);
        targetPath ??= 'report.docx';
      } else {
        if (targetPath == null || targetPath!.isEmpty) {
          fail('Expected at least one review change with real OpenAI.');
        }
        await pumpUntilFound(
          find.text(targetPath!),
          tester: tester,
          timeout: const Duration(seconds: 30),
        );
      }

      await _tapVisible(tester, find.text(targetPath!));
      if (isFake) {
        await pumpUntilFound(find.text('Draft'), tester: tester);
      } else {
        await pumpUntilFound(find.text('Summary'), tester: tester);
      }

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
