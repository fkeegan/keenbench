import 'dart:convert';
import 'dart:io';

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
    'm0 workshop consent -> auto-apply -> publish flow',
    (tester) async {
      final harness = E2eHarness(tester, screenshooter);
      final useFakeOpenAI = isFakeOpenAIEnabled();
      final draftTimeout = useFakeOpenAI
          ? const Duration(seconds: 10)
          : const Duration(seconds: 45);
      final binding = tester.binding;
      await binding.setSurfaceSize(const Size(1600, 1000));
      addTearDown(() async {
        await binding.setSurfaceSize(null);
        await harness.stopEngine();
        await cleanupE2eConfig();
      });

      final fixtures = await E2eFixtures.create();
      try {
        await harness.launchApp();

        await _configureValidKey(tester, harness);
        await harness.backToHome();

        final workbenchName = await harness.createWorkbench(
          name: 'E2E Workshop ${DateTime.now().millisecondsSinceEpoch}',
        );
        await screenshooter.capture('workbench');

        final state = harness.workbenchState();
        await tester.runAsync(() async {
          await state.addFiles([fixtures.notes.path, fixtures.dataCsv.path]);
        });
        await tester.pumpAndSettle();

        await tester.runAsync(() async {
          final status =
              await harness.engineApi().call('EgressGetConsentStatus', {
                    'workbench_id': state.workbenchId,
                  })
                  as Map<String, dynamic>;
          expect(status['consented'], isFalse);
        });

        await _sendMessageWithConsent(
          tester,
          text: 'Summarize staffing decisions from the roster and notes.',
          acceptConsent: false,
        );
        expect(
          find.textContaining('Summarize staffing decisions from the roster'),
          findsNothing,
        );

        await _sendMessageWithConsent(
          tester,
          harness: harness,
          text: 'Summarize staffing decisions from the roster and notes.',
          acceptConsent: true,
          waitForAssistant: true,
        );
        final summarizeReply = _lastAssistantText(harness);
        if (useFakeOpenAI) {
          expect(
            summarizeReply,
            contains(
              'Assistant response: Summarize staffing decisions from the roster and notes.',
            ),
          );
        } else {
          expect(summarizeReply.trim().isNotEmpty, isTrue);
        }
        await screenshooter.capture('workbench_after_summary');

        await _sendMessageWithConsent(
          tester,
          harness: harness,
          text: 'List any open roles or staffing gaps.',
          expectConsent: false,
          waitForAssistant: true,
        );
        final secondReply = _lastAssistantText(harness);
        if (useFakeOpenAI) {
          expect(
            secondReply,
            contains(
              'Assistant response: List any open roles or staffing gaps.',
            ),
          );
        } else {
          expect(secondReply.trim().isNotEmpty, isTrue);
        }

        expect(find.byKey(AppKeys.consentDialog), findsNothing);

        final extra = await fixtures.createExtraFile(
          'scope_change.txt',
          'scope change',
        );
        await tester.runAsync(() async {
          await state.addFiles([extra.path]);
        });
        await tester.pumpAndSettle();

        await _sendMessageWithConsent(
          tester,
          harness: harness,
          text:
              'Create exactly two files with these exact names:\n'
              '- org_chart.md\n'
              '- project_assignments.csv\n'
              '\n'
              'org_chart.md requirements:\n'
              '- Include project headers "Atlas" and "Beacon".\n'
              '- List each employee by full name under the correct project.\n'
              '\n'
              'project_assignments.csv requirements:\n'
              '- Header: name,project,role\n'
              '- Include one row per employee using full names.\n'
              '\n'
              'Use only the roster CSV and meeting notes.',
          acceptConsent: true,
          waitForAssistant: true,
        );
        final scopeReply = _lastAssistantText(harness);
        if (useFakeOpenAI) {
          expect(
            scopeReply,
            contains(
              'Assistant response: Create exactly two files with these exact names:',
            ),
          );
        } else {
          expect(scopeReply.trim().isNotEmpty, isTrue);
        }
        await screenshooter.capture('workbench_after_instruction');

        final messageCountBefore = harness.workbenchState().messages.length;
        await harness.backToHome();
        await harness.openWorkbenchByName(workbenchName);
        expect(
          harness.workbenchState().messages.length,
          greaterThanOrEqualTo(messageCountBefore),
        );

        await pumpUntilFound(
          find.byKey(AppKeys.reviewScreen),
          tester: tester,
          timeout: draftTimeout,
        );

        await tester.runAsync(() async {
          await harness.engineApi().call('ProvidersSetEnabled', {
            'provider_id': 'openai',
            'enabled': false,
          });
        });

        await screenshooter.capture('review');
        if (useFakeOpenAI) {
          expect(find.text('ADDED'), findsWidgets);
          expect(find.text('org_chart.md'), findsWidgets);
          expect(find.text('project_assignments.csv'), findsWidgets);
        }
        expect(find.text('Summary'), findsOneWidget);

        await _tapVisible(tester, find.byKey(AppKeys.reviewPublishButton));
        await pumpUntilFound(
          find.byKey(AppKeys.workbenchScreen),
          tester: tester,
        );
        expect(find.byKey(AppKeys.workbenchDraftBanner), findsNothing);
        if (useFakeOpenAI) {
          expect(
            find.byKey(AppKeys.workbenchFileRow('org_chart.md')),
            findsOneWidget,
          );
          expect(
            find.byKey(AppKeys.workbenchFileRow('project_assignments.csv')),
            findsOneWidget,
          );
        }

        await tester.runAsync(() async {
          await harness.engineApi().call('ProvidersSetEnabled', {
            'provider_id': 'openai',
            'enabled': true,
          });
        });

        await tester.runAsync(() async {
          await _assertPublishedStaffingFiles(
            workbenchId: state.workbenchId,
            dataDir: requireDataDir(),
          );
        });
      } catch (error) {
        await screenshooter.capture('failure_workshop_flow');
        await screenshooter.pauseIfEnabled(error.toString());
        rethrow;
      }
    },
    timeout: pauseOnFailure
        ? Timeout.none
        : const Timeout(Duration(minutes: 5)),
  );

  testWidgets(
    'm0 workshop errors, discard, and draft persistence',
    (tester) async {
      final harness = E2eHarness(tester, screenshooter);
      final useFakeOpenAI = isFakeOpenAIEnabled();
      final draftTimeout = useFakeOpenAI
          ? const Duration(seconds: 10)
          : const Duration(seconds: 45);
      final binding = tester.binding;
      await binding.setSurfaceSize(const Size(1600, 1000));
      addTearDown(() async {
        await binding.setSurfaceSize(null);
        await harness.stopEngine();
        await cleanupE2eConfig();
      });

      final fixtures = await E2eFixtures.create();
      try {
        await harness.launchApp();
        await _configureValidKey(tester, harness);
        await harness.backToHome();

        final discardName = await harness.createWorkbench(
          name: 'E2E Discard ${DateTime.now().millisecondsSinceEpoch}',
        );
        final discardState = harness.workbenchState();
        await tester.runAsync(() async {
          await discardState.addFiles([
            fixtures.notes.path,
            fixtures.dataCsv.path,
          ]);
        });
        await tester.pumpAndSettle();

        await _sendMessageWithConsent(
          tester,
          harness: harness,
          text:
              'Draft org_chart.md and project_assignments.csv for the staffing plan.\n'
              'Use full names and include Atlas + Beacon assignments.\n'
              'CSV header must be: name,project,role.',
          acceptConsent: true,
          waitForAssistant: true,
        );
        final prepareReply = _lastAssistantText(harness);
        if (useFakeOpenAI) {
          expect(
            prepareReply,
            contains(
              'Assistant response: Draft org_chart.md and project_assignments.csv for the staffing plan.',
            ),
          );
        } else {
          expect(prepareReply.trim().isNotEmpty, isTrue);
        }

        await pumpUntilFound(
          find.byKey(AppKeys.reviewScreen),
          tester: tester,
          timeout: draftTimeout,
        );
        if (useFakeOpenAI) {
          expect(find.text('org_chart.md'), findsOneWidget);
        } else {
          expect(find.text('Summary'), findsOneWidget);
        }
        await _tapVisible(tester, find.byKey(AppKeys.reviewDiscardButton));
        await pumpUntilFound(
          find.byKey(AppKeys.workbenchScreen),
          tester: tester,
        );
        expect(find.byKey(AppKeys.workbenchDraftBanner), findsNothing);
        if (useFakeOpenAI) {
          expect(
            find.byKey(AppKeys.workbenchFileRow('org_chart.md')),
            findsNothing,
          );
          expect(
            find.byKey(AppKeys.workbenchFileRow('project_assignments.csv')),
            findsNothing,
          );
        }

        await harness.backToHome();

        final persistName = await harness.createWorkbench(
          name: 'E2E Persist ${DateTime.now().millisecondsSinceEpoch}',
        );
        final persistState = harness.workbenchState();
        await tester.runAsync(() async {
          await persistState.addFiles([
            fixtures.notes.path,
            fixtures.dataCsv.path,
          ]);
        });
        await tester.pumpAndSettle();
        await _sendMessageWithConsent(
          tester,
          harness: harness,
          text:
              'Persist org_chart.md and project_assignments.csv for the staffing plan.',
          acceptConsent: true,
          waitForAssistant: true,
        );
        final persistWaitEnd = DateTime.now().add(draftTimeout);
        while (DateTime.now().isBefore(persistWaitEnd)) {
          if (find.byKey(AppKeys.reviewScreen).evaluate().isNotEmpty ||
              find.byKey(AppKeys.workbenchDraftBanner).evaluate().isNotEmpty) {
            break;
          }
          await tester.pump(const Duration(milliseconds: 100));
        }
        expect(
          find.byKey(AppKeys.reviewScreen).evaluate().isNotEmpty ||
              find.byKey(AppKeys.workbenchDraftBanner).evaluate().isNotEmpty,
          isTrue,
        );

        await harness.restartApp();
        await harness.openWorkbenchByName(persistName);
        await pumpUntilFound(
          find.byKey(AppKeys.reviewScreen),
          tester: tester,
          timeout: draftTimeout,
        );
        await _tapVisible(tester, find.byKey(AppKeys.reviewDiscardButton));
        await pumpUntilFound(
          find.byKey(AppKeys.workbenchScreen),
          tester: tester,
        );
        expect(find.byKey(AppKeys.workbenchDraftBanner), findsNothing);

        await harness.backToHome();

        if (useFakeOpenAI) {
          await harness.createWorkbench(
            name: 'E2E Delete ${DateTime.now().millisecondsSinceEpoch}',
          );
          final deleteState = harness.workbenchState();
          await tester.runAsync(() async {
            await deleteState.addFiles([fixtures.notes.path]);
          });
          await tester.pumpAndSettle();
          await _sendMessageWithConsent(
            tester,
            harness: harness,
            text: 'Please [delete] notes.txt',
            acceptConsent: true,
            waitForAssistant: true,
          );
          await tester.pumpAndSettle();
          expect(find.byKey(AppKeys.workbenchDraftBanner), findsNothing);
          expect(find.byKey(AppKeys.workbenchComposerField), findsOneWidget);

          await harness.backToHome();
        }

        await harness.createWorkbench(
          name: 'E2E Sandbox ${DateTime.now().millisecondsSinceEpoch}',
        );
        final sandboxState = harness.workbenchState();
        await tester.runAsync(() async {
          await sandboxState.addFiles([fixtures.notes.path]);
        });
        await tester.pumpAndSettle();
        final workbenchId = sandboxState.workbench?.id ?? '';
        final dataDir = requireDataDir();
        late String proposalId;
        await tester.runAsync(() async {
          final consent =
              await harness.engineApi().call('EgressGetConsentStatus', {
                    'workbench_id': workbenchId,
                  })
                  as Map<String, dynamic>;
          final scopeHash = consent['scope_hash'] as String? ?? '';
          await harness.engineApi().call('EgressGrantWorkshopConsent', {
            'workbench_id': workbenchId,
            'provider_id': 'openai',
            'model_id': consent['model_id'],
            'scope_hash': scopeHash,
          });
          final messageResp =
              await harness.engineApi().call('WorkshopSendUserMessage', {
                    'workbench_id': workbenchId,
                    'text':
                        'Sandbox test: draft org_chart.md and project_assignments.csv.',
                  })
                  as Map<String, dynamic>;
          final messageId = messageResp['message_id'] as String? ?? '';
          await harness.engineApi().call('WorkshopStreamAssistantReply', {
            'workbench_id': workbenchId,
            'message_id': messageId,
          });
          final proposalResp =
              await harness.engineApi().call('WorkshopProposeChanges', {
                    'workbench_id': workbenchId,
                  })
                  as Map<String, dynamic>;
          proposalId = proposalResp['proposal_id'] as String? ?? '';
          final file = File(proposalPath(dataDir, workbenchId, proposalId));
          final payload =
              jsonDecode(await file.readAsString()) as Map<String, dynamic>;
          final writes = (payload['writes'] as List<dynamic>)
              .cast<Map<String, dynamic>>();
          if (writes.isEmpty) {
            throw StateError('No writes in proposal.');
          }
          writes[0]['path'] = '../outside.txt';
          payload['writes'] = writes;
          await file.writeAsString(jsonEncode(payload));
        });

        await tester.runAsync(() async {
          try {
            await harness.engineApi().call('WorkshopApplyProposal', {
              'workbench_id': workbenchId,
              'proposal_id': proposalId,
            });
            fail('Expected proposal apply to fail.');
          } catch (err) {
            expect(
              err.toString(),
              anyOf(
                contains('VALIDATION_FAILED'),
                contains('SANDBOX_VIOLATION'),
              ),
            );
          }
        });

        await tester.runAsync(() async {
          final filesResponse =
              await harness.engineApi().call('WorkbenchFilesList', {
                    'workbench_id': workbenchId,
                  })
                  as Map<String, dynamic>;
          final files = (filesResponse['files'] as List<dynamic>)
              .cast<Map<String, dynamic>>()
              .map((entry) => entry['path'] as String)
              .toList();
          expect(files.contains('outside.txt'), isFalse);
        });

        await tester.runAsync(() async {
          try {
            await harness.engineApi().call('DraftDiscard', {
              'workbench_id': workbenchId,
            });
          } catch (_) {}
        });

        await harness.backToHome();
        expect(find.text(discardName), findsOneWidget);
      } catch (error) {
        await screenshooter.capture('failure_workshop_errors');
        await screenshooter.pauseIfEnabled(error.toString());
        rethrow;
      }
    },
    timeout: pauseOnFailure
        ? Timeout.none
        : const Timeout(Duration(minutes: 6)),
  );
}

Future<void> _configureValidKey(WidgetTester tester, E2eHarness harness) async {
  await harness.openSettings();
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
  final toast = await waitForSnackBarText(
    tester,
    timeout: const Duration(seconds: 30),
  );
  if (toast != 'Key saved and validated.' && toast != 'OpenAI key saved.') {
    fail('OpenAI key validation failed: $toast');
  }
  await clearSnackBars(tester);
  final status = tester
      .widget<Text>(find.byKey(AppKeys.settingsProviderStatus))
      .data;
  expect(status, 'Configured');
}

Future<void> _sendMessageWithConsent(
  WidgetTester tester, {
  E2eHarness? harness,
  required String text,
  bool acceptConsent = true,
  bool expectConsent = true,
  bool waitForAssistant = false,
}) async {
  final state = waitForAssistant && harness != null
      ? harness.workbenchState()
      : null;
  final assistantBefore = state != null ? assistantMessageCount(state) : null;
  await pumpUntilFound(
    find.byKey(AppKeys.workbenchComposerField),
    tester: tester,
  );
  await tester.tap(find.byKey(AppKeys.workbenchComposerField));
  await tester.pumpAndSettle();
  await tester.enterText(find.byKey(AppKeys.workbenchComposerField), text);
  await tester.pumpAndSettle();
  final field = tester.widget<TextField>(
    find.byKey(AppKeys.workbenchComposerField),
  );
  if (field.controller?.text != text) {
    field.controller?.text = text;
    await tester.pumpAndSettle();
  }
  final sendButton = tester.widget<ElevatedButton>(
    find.byKey(AppKeys.workbenchSendButton),
  );
  expect(sendButton.onPressed, isNotNull);
  await tester.tap(find.byKey(AppKeys.workbenchSendButton));
  await tester.pumpAndSettle();
  if (expectConsent) {
    final dialogFinder = find.byKey(AppKeys.consentDialog);
    final titleFinder = find.text('Consent required');
    try {
      await pumpUntilFound(dialogFinder, tester: tester);
    } catch (_) {
      await pumpUntilFound(titleFinder, tester: tester);
    }
    expect(find.byKey(AppKeys.consentFileList), findsOneWidget);
    expect(find.byKey(AppKeys.consentScopeHash), findsOneWidget);
    expect(find.textContaining('bytes'), findsWidgets);
    if (acceptConsent) {
      await tester.tap(find.byKey(AppKeys.consentContinueButton));
    } else {
      await tester.tap(find.byKey(AppKeys.consentCancelButton));
    }
    await tester.pumpAndSettle();
  }
  final shouldWaitForAssistant =
      waitForAssistant && state != null && !(expectConsent && !acceptConsent);
  if (shouldWaitForAssistant) {
    await pumpUntilAssistantMessageCount(
      state,
      minCount: (assistantBefore ?? 0) + 1,
      tester: tester,
    );
  }
}

Future<void> _tapVisible(WidgetTester tester, Finder finder) async {
  await tester.ensureVisible(finder);
  await tester.pumpAndSettle();
  await tester.tap(finder);
  await tester.pumpAndSettle();
}

Future<void> _assertPublishedStaffingFiles({
  required String workbenchId,
  required String dataDir,
}) async {
  final orgPath = workbenchPublishedPath(dataDir, workbenchId, 'org_chart.md');
  final assignmentsPath = workbenchPublishedPath(
    dataDir,
    workbenchId,
    'project_assignments.csv',
  );
  final orgChart = await File(orgPath).readAsString();
  final assignments = await File(assignmentsPath).readAsString();
  final orgLower = orgChart.toLowerCase();
  final assignmentsLower = assignments.toLowerCase();
  expect(orgLower.contains('atlas'), isTrue);
  expect(orgLower.contains('beacon'), isTrue);
  expect(assignmentsLower.contains('name,project,role'), isTrue);

  const names = [
    'Alice Kim',
    'Bruno Silva',
    'Chloe Nguyen',
    'Diego Patel',
    'Elena Rossi',
  ];
  for (final name in names) {
    final needle = name.toLowerCase();
    expect(orgLower.contains(needle), isTrue);
    expect(assignmentsLower.contains(needle), isTrue);
  }
}

String _lastAssistantText(E2eHarness harness) {
  final messages = harness
      .workbenchState()
      .messages
      .where((m) => m.role == 'assistant')
      .toList();
  if (messages.isEmpty) {
    return '';
  }
  return messages.last.text;
}
