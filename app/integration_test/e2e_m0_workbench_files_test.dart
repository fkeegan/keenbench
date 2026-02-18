import 'dart:io';

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
    'm0 workbench file add semantics',
    (tester) async {
      final harness = E2eHarness(tester, screenshooter);
      addTearDown(() async {
        await harness.stopEngine();
        await cleanupE2eConfig();
      });

      final fixtures = await E2eFixtures.create();
      try {
        await harness.launchApp();

        await tester.tap(find.byKey(AppKeys.homeNewWorkbenchButton));
        await tester.pumpAndSettle();
        await pumpUntilFound(
          find.byKey(AppKeys.newWorkbenchDialog),
          tester: tester,
        );
        await screenshooter.capture('new_workbench_dialog');

        final workbenchName =
            'E2E Files ${DateTime.now().millisecondsSinceEpoch}';
        await tester.enterText(
          find.byKey(AppKeys.newWorkbenchNameField),
          workbenchName,
        );
        await tester.tap(find.byKey(AppKeys.newWorkbenchCreateButton));
        await tester.pumpAndSettle();
        await pumpUntilFound(
          find.byKey(AppKeys.workbenchScreen),
          tester: tester,
        );
        await screenshooter.capture('workbench_empty');

        expect(find.textContaining('Originals stay untouched'), findsOneWidget);

        final state = harness.workbenchState();
        await tester.runAsync(() async {
          await state.addFiles([fixtures.notes.path, fixtures.dataCsv.path]);
        });
        await tester.pumpAndSettle();
        expect(
          find.byKey(AppKeys.workbenchFileRow('notes.txt')),
          findsOneWidget,
        );
        expect(
          find.byKey(AppKeys.workbenchFileRow('data.csv')),
          findsOneWidget,
        );

        final dataDir = requireDataDir();
        final workbenchId = state.workbench?.id ?? '';
        final originalNotes = await fixtures.notes.readAsString();
        await fixtures.notes.writeAsString('Modified outside the app.\n');
        final copiedNotes = await File(
          workbenchPublishedPath(dataDir, workbenchId, 'notes.txt'),
        ).readAsString();
        expect(copiedNotes, originalNotes);

        final fileCountAfterAdd = state.files.length;
        await tester.runAsync(() async {
          await state.addFiles([fixtures.duplicateNotes.path]);
        });
        await tester.pumpAndSettle();
        expect(state.files.length, fileCountAfterAdd);
        expect(state.files.any((file) => file.path == 'notes.txt'), isTrue);

        await tester.runAsync(() async {
          await state.addFiles([fixtures.opaque.path]);
        });
        await tester.pumpAndSettle();
        expect(state.files.length, fileCountAfterAdd + 1);
        expect(state.files.any((file) => file.isOpaque), isTrue);
        final fileCountAfterOpaque = state.files.length;

        await tester.runAsync(() async {
          await state.addFiles([fixtures.linkToNotes.path]);
        });
        await tester.pumpAndSettle();
        expect(state.files.length, fileCountAfterOpaque);

        final extra = await fixtures.createExtraFile(
          'extra.txt',
          'extra content',
        );
        await tester.runAsync(() async {
          await state.addFiles([fixtures.bigCsv.path, extra.path]);
        });
        await tester.pumpAndSettle();
        expect(state.files.any((file) => file.path == 'extra.txt'), isTrue);
        expect(state.files.any((file) => file.path == 'big.csv'), isFalse);

        await harness.backToHome();
        await tester.tap(find.byKey(AppKeys.homeNewWorkbenchButton));
        await tester.pumpAndSettle();
        await pumpUntilFound(
          find.byKey(AppKeys.newWorkbenchDialog),
          tester: tester,
        );
        final limitWorkbenchName =
            'E2E Limit ${DateTime.now().millisecondsSinceEpoch}';
        await tester.enterText(
          find.byKey(AppKeys.newWorkbenchNameField),
          limitWorkbenchName,
        );
        await tester.tap(find.byKey(AppKeys.newWorkbenchCreateButton));
        await tester.pumpAndSettle();
        await pumpUntilFound(
          find.byKey(AppKeys.workbenchScreen),
          tester: tester,
        );

        final limitState = harness.workbenchState();
        final firstBatch = await fixtures.createSmallFiles(9, prefix: 'limit');
        await tester.runAsync(() async {
          await limitState.addFiles(firstBatch.map((f) => f.path).toList());
        });
        await tester.pumpAndSettle();
        expect(limitState.files.length, 9);

        final overflowBatch = await fixtures.createSmallFiles(
          2,
          prefix: 'overflow',
        );
        await tester.runAsync(() async {
          try {
            await limitState.addFiles(
              overflowBatch.map((f) => f.path).toList(),
            );
            fail('Expected file limit error.');
          } catch (err) {
            expect(err.toString(), contains('file limit'));
          }
        });
        await tester.pumpAndSettle();
        expect(limitState.files.length, 9);
      } catch (error) {
        await screenshooter.capture('failure_workbench_files');
        await screenshooter.pauseIfEnabled(error.toString());
        rethrow;
      }
    },
    timeout: pauseOnFailure
        ? Timeout.none
        : const Timeout(Duration(minutes: 4)),
  );
}
