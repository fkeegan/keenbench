import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:integration_test/integration_test.dart';
import 'package:keenbench/main.dart' as app;

import 'support/e2e_screenshots.dart';

void main() {
  IntegrationTestWidgetsFlutterBinding.ensureInitialized();
  final screenshooter = E2eScreenshooter();
  final pauseOnFailure = screenshooter.pauseOnFailure;

  testWidgets(
    'm0 e2e smoke',
    (tester) async {
      try {
        app.main();
        await tester.pumpAndSettle();
        await screenshooter.capture('home');

        await tester.tap(find.text('Settings'));
        await tester.pumpAndSettle();
        await screenshooter.capture('settings');

        await tester.pageBack();
        await tester.pumpAndSettle();
        await screenshooter.capture('home_after_settings');

        await tester.tap(find.text('New Workbench'));
        await tester.pumpAndSettle();
        await screenshooter.capture('new_workbench_dialog');

        final workbenchName =
            'E2E Workbench ${DateTime.now().millisecondsSinceEpoch}';
        await tester.enterText(find.byType(TextField), workbenchName);
        await tester.tap(find.text('Create'));
        await tester.pumpAndSettle();
        await screenshooter.capture('workbench');

        final reviewButton = find.text('Review');
        if (reviewButton.evaluate().isNotEmpty) {
          await tester.tap(reviewButton);
          await tester.pumpAndSettle();
          await screenshooter.capture('review');
          await tester.pageBack();
          await tester.pumpAndSettle();
        }
      } catch (error) {
        await screenshooter.capture('failure');
        await screenshooter.pauseIfEnabled(error.toString());
        rethrow;
      }
    },
    timeout: pauseOnFailure
        ? Timeout.none
        : const Timeout(Duration(minutes: 2)),
    // Superseded by the M0 E2E suite; keep for manual smoke runs.
    skip: true,
  );
}
