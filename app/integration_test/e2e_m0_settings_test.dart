import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:integration_test/integration_test.dart';

import 'package:keenbench/app_keys.dart';

import 'support/e2e_screenshots.dart';
import 'support/e2e_utils.dart';

void main() {
  IntegrationTestWidgetsFlutterBinding.ensureInitialized();
  final screenshooter = E2eScreenshooter();
  final pauseOnFailure = screenshooter.pauseOnFailure;

  testWidgets(
    'm0 settings key lifecycle + persistence',
    (tester) async {
      final harness = E2eHarness(tester, screenshooter);
      addTearDown(() async {
        await harness.stopEngine();
        await cleanupE2eConfig();
      });

      try {
        await harness.launchApp();
        await screenshooter.capture('home');

        await harness.openSettings();
        await screenshooter.capture('settings');
        final initialStatus = tester
            .widget<Text>(find.byKey(AppKeys.settingsProviderStatus))
            .data;
        expect(initialStatus, 'Not configured');

        await enterTextAndEnsure(
          tester,
          find.byKey(AppKeys.settingsApiKeyField),
          'invalid-key',
        );
        await tester.tap(find.byKey(AppKeys.settingsSaveButton));
        await tester.pump();
        final invalidToast = await waitForSnackBarText(tester);
        expect(invalidToast, 'PROVIDER_AUTH_FAILED');
        await clearSnackBars(tester);
        expectTextFieldValue(
          tester,
          find.byKey(AppKeys.settingsApiKeyField),
          'invalid-key',
        );
        final invalidStatus = tester
            .widget<Text>(find.byKey(AppKeys.settingsProviderStatus))
            .data;
        expect(invalidStatus, 'Not configured');

        final validKey = requireOpenAIKeyForTests();
        await enterTextAndEnsure(
          tester,
          find.byKey(AppKeys.settingsApiKeyField),
          validKey,
        );
        expectTextFieldValue(
          tester,
          find.byKey(AppKeys.settingsApiKeyField),
          validKey,
        );
        await tester.tap(find.byKey(AppKeys.settingsSaveButton));
        await tester.pump();
        final successToast = await waitForSnackBarText(
          tester,
          timeout: const Duration(seconds: 30),
        );
        expect(successToast, 'Key saved and validated.');
        var configuredStatus = tester
            .widget<Text>(find.byKey(AppKeys.settingsProviderStatus))
            .data;
        if (configuredStatus != 'Configured') {
          await harness.backToHome();
          await harness.openSettings();
          configuredStatus = tester
              .widget<Text>(find.byKey(AppKeys.settingsProviderStatus))
              .data;
        }
        expect(configuredStatus, 'Configured');

        await harness.restartApp();
        await harness.openSettings();
        final persistedStatus = tester
            .widget<Text>(find.byKey(AppKeys.settingsProviderStatus))
            .data;
        expect(persistedStatus, 'Configured');
      } catch (error) {
        await screenshooter.capture('failure_settings');
        await screenshooter.pauseIfEnabled(error.toString());
        rethrow;
      }
    },
    timeout: pauseOnFailure
        ? Timeout.none
        : const Timeout(Duration(minutes: 3)),
  );
}
