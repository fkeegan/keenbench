import 'dart:async';
import 'dart:io';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:path/path.dart' as path;
import 'package:provider/provider.dart';

import 'package:keenbench/app_keys.dart';
import 'package:keenbench/app_env.dart';
import 'package:keenbench/engine/engine_client.dart';
import 'package:keenbench/state/workbench_state.dart';
import 'package:keenbench/main.dart' as app;

import 'e2e_screenshots.dart';

const Duration _pollInterval = Duration(milliseconds: 100);

class E2eHarness {
  E2eHarness(this.tester, this.screenshooter);

  final WidgetTester tester;
  final E2eScreenshooter screenshooter;
  WorkbenchState? _cachedWorkbenchState;

  Future<void> launchApp() async {
    ensureE2eEnv();
    app.main();
    await tester.pumpAndSettle();
    await pumpUntilFound(find.byKey(AppKeys.homeScreen), tester: tester);
  }

  Future<void> restartApp() async {
    await stopEngine();
    await tester.pumpWidget(const SizedBox());
    await tester.pumpAndSettle();
    ensureE2eEnv();
    app.main();
    await tester.pumpAndSettle();
    await pumpUntilFound(find.byKey(AppKeys.homeScreen), tester: tester);
  }

  Future<void> openSettings() async {
    await tester.tap(find.byKey(AppKeys.homeSettingsButton));
    await tester.pumpAndSettle();
    await pumpUntilFound(find.byKey(AppKeys.settingsScreen), tester: tester);
  }

  Future<void> backToHome() async {
    final homeFinder = find.byKey(AppKeys.homeScreen);
    final end = DateTime.now().add(const Duration(seconds: 10));
    while (DateTime.now().isBefore(end)) {
      if (homeFinder.evaluate().isNotEmpty) {
        return;
      }
      await tester.pageBack();
      await tester.pumpAndSettle();
    }
    await pumpUntilFound(homeFinder, tester: tester);
  }

  Future<String> createWorkbench({String? name}) async {
    final workbenchName =
        name ?? 'E2E Workbench ${DateTime.now().millisecondsSinceEpoch}';
    await tester.tap(find.byKey(AppKeys.homeNewWorkbenchButton));
    await tester.pumpAndSettle();
    await pumpUntilFound(
      find.byKey(AppKeys.newWorkbenchDialog),
      tester: tester,
    );

    await tester.enterText(
      find.byKey(AppKeys.newWorkbenchNameField),
      workbenchName,
    );
    await tester.tap(find.byKey(AppKeys.newWorkbenchCreateButton));
    await tester.pumpAndSettle();
    await pumpUntilFound(find.byKey(AppKeys.workbenchScreen), tester: tester);
    return workbenchName;
  }

  Future<void> openWorkbenchByName(String name) async {
    await pumpUntilFound(find.text(name), tester: tester);
    await tester.tap(find.text(name));
    await tester.pumpAndSettle();
    final end = DateTime.now().add(const Duration(seconds: 10));
    while (DateTime.now().isBefore(end)) {
      if (tester.any(find.byKey(AppKeys.workbenchScreen)) ||
          tester.any(find.byKey(AppKeys.reviewScreen))) {
        return;
      }
      await tester.pump(_pollInterval);
    }
    throw TimeoutException(
      'Did not open workbench/review for workbench "$name" within 10 seconds.',
    );
  }

  WorkbenchState workbenchState() {
    if (tester.any(find.byKey(AppKeys.workbenchScreen))) {
      final context = tester.element(find.byKey(AppKeys.workbenchScreen));
      _cachedWorkbenchState = Provider.of<WorkbenchState>(
        context,
        listen: false,
      );
    }
    if (_cachedWorkbenchState != null) {
      return _cachedWorkbenchState!;
    }
    throw StateError(
      'WorkbenchState is not available before opening a workbench.',
    );
  }

  EngineApi engineApi() {
    final context = _rootContext();
    return Provider.of<EngineApi>(context, listen: false);
  }

  Future<void> stopEngine() async {
    EngineApi? engine;
    try {
      engine = engineApi();
    } catch (_) {
      engine = null;
    }
    if (engine is EngineClient) {
      await engine.stop();
    }
  }

  BuildContext _rootContext() {
    if (tester.any(find.byKey(AppKeys.workbenchScreen))) {
      return tester.element(find.byKey(AppKeys.workbenchScreen));
    }
    if (tester.any(find.byKey(AppKeys.reviewScreen))) {
      return tester.element(find.byKey(AppKeys.reviewScreen));
    }
    if (tester.any(find.byKey(AppKeys.settingsScreen))) {
      return tester.element(find.byKey(AppKeys.settingsScreen));
    }
    return tester.element(find.byKey(AppKeys.homeScreen));
  }
}

Future<void> pumpUntilFound(
  Finder finder, {
  Duration timeout = const Duration(seconds: 10),
  required WidgetTester tester,
}) async {
  final end = DateTime.now().add(timeout);
  while (DateTime.now().isBefore(end)) {
    if (finder.evaluate().isNotEmpty) {
      return;
    }
    await tester.pump(_pollInterval);
  }
  throw TimeoutException('Finder not found within $timeout: $finder');
}

Future<void> pumpUntilGone(
  Finder finder, {
  Duration timeout = const Duration(seconds: 10),
  required WidgetTester tester,
}) async {
  final end = DateTime.now().add(timeout);
  while (DateTime.now().isBefore(end)) {
    if (finder.evaluate().isEmpty) {
      return;
    }
    await tester.pump(_pollInterval);
  }
  throw TimeoutException('Finder still present after $timeout: $finder');
}

Future<String> waitForSnackBarText(
  WidgetTester tester, {
  Duration timeout = const Duration(seconds: 10),
}) async {
  final end = DateTime.now().add(timeout);
  while (DateTime.now().isBefore(end)) {
    final snackFinder = find.byType(SnackBar);
    if (snackFinder.evaluate().isNotEmpty) {
      final snack = tester.widget<SnackBar>(snackFinder.first);
      final content = snack.content;
      if (content is Text && content.data != null) {
        return content.data!;
      }
    }
    await tester.pump(_pollInterval);
  }
  throw TimeoutException('SnackBar not shown within $timeout.');
}

Future<void> clearSnackBars(
  WidgetTester tester, {
  Duration timeout = const Duration(seconds: 10),
}) async {
  if (find.byType(SnackBar).evaluate().isEmpty) {
    return;
  }
  await pumpUntilGone(find.byType(SnackBar), tester: tester, timeout: timeout);
}

Future<void> enterTextAndEnsure(
  WidgetTester tester,
  Finder finder,
  String text,
) async {
  await tester.tap(finder);
  await tester.pumpAndSettle();
  await tester.enterText(finder, text);
  await tester.pumpAndSettle();
  final field = tester.widget<TextField>(finder);
  final controller = field.controller;
  if (controller == null) {
    throw TestFailure('TextField has no controller for $finder');
  }
  if (controller.text != text) {
    controller.text = text;
    await tester.pumpAndSettle();
  }
  if (controller.text != text) {
    throw TestFailure('TextField did not update for $finder');
  }
}

void expectTextFieldValue(WidgetTester tester, Finder finder, String expected) {
  final field = tester.widget<TextField>(finder);
  final controller = field.controller;
  if (controller == null) {
    throw TestFailure('TextField has no controller for $finder');
  }
  expect(controller.text, expected);
}

bool isFakeOpenAIEnabled() {
  final raw = AppEnv.get('KEENBENCH_FAKE_OPENAI');
  if (raw == null) {
    return false;
  }
  switch (raw.trim().toLowerCase()) {
    case '1':
    case 'true':
    case 't':
    case 'yes':
    case 'y':
    case 'on':
      return true;
    default:
      return false;
  }
}

String requireOpenAIKeyForTests() {
  if (isFakeOpenAIEnabled()) {
    return 'sk-valid';
  }
  final key = AppEnv.get('KEENBENCH_OPENAI_API_KEY');
  if (key == null || key.trim().isEmpty) {
    throw StateError(
      'KEENBENCH_OPENAI_API_KEY must be set when KEENBENCH_FAKE_OPENAI != 1.',
    );
  }
  return key.trim();
}

int assistantMessageCount(WorkbenchState state) {
  return state.messages
      .where((m) => m.role == 'assistant' && m.text.trim().isNotEmpty)
      .length;
}

Future<void> pumpUntilAssistantMessageCount(
  WorkbenchState state, {
  required int minCount,
  Duration timeout = const Duration(seconds: 45),
  required WidgetTester tester,
}) async {
  final end = DateTime.now().add(timeout);
  while (DateTime.now().isBefore(end)) {
    if (!state.isSending && assistantMessageCount(state) >= minCount) {
      return;
    }
    await tester.pump(_pollInterval);
  }
  throw TimeoutException('Assistant response not received within $timeout.');
}

String requireDataDir() {
  return ensureE2eDataDir();
}

Future<void> cleanupE2eConfig() async {
  final dir = AppEnv.get('KEENBENCH_DATA_DIR') ?? _cachedDataDir;
  if (dir == null || dir.isEmpty) {
    return;
  }
  final settings = File(path.join(dir, 'settings.json'));
  final secrets = File(path.join(dir, 'secrets.enc'));
  final masterKey = File(path.join(dir, 'master.key'));
  for (final file in [settings, secrets, masterKey]) {
    if (await file.exists()) {
      await file.delete();
    }
  }
}

String? _cachedDataDir;

void ensureE2eEnv() {
  ensureE2eDataDir();
  if (AppEnv.get('KEENBENCH_FAKE_TOOL_WORKER') == null) {
    AppEnv.setOverride('KEENBENCH_FAKE_TOOL_WORKER', '1');
  }
}

String ensureE2eDataDir() {
  final existing = AppEnv.get('KEENBENCH_DATA_DIR');
  if (existing != null && existing.trim().isNotEmpty) {
    return existing;
  }
  if (_cachedDataDir != null) {
    return _cachedDataDir!;
  }
  final repoRoot = _resolveRepoRoot();
  final dirName = DateTime.now().millisecondsSinceEpoch.toString();
  final dataDir = path.join(repoRoot, 'artifacts', 'e2e_data', dirName);
  Directory(dataDir).createSync(recursive: true);
  AppEnv.setOverride('KEENBENCH_DATA_DIR', dataDir);
  _cachedDataDir = dataDir;
  return dataDir;
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

String officeFixturePath(String name) {
  final root = _resolveRepoRoot();
  return path.join(root, 'engine', 'testdata', 'office', name);
}

class E2eFixtures {
  E2eFixtures._({
    required this.root,
    required this.notes,
    required this.dataCsv,
    required this.notesUpdated,
    required this.bigCsv,
    required this.duplicateNotes,
    required this.opaque,
    required this.linkToNotes,
  });

  final Directory root;
  final File notes;
  final File dataCsv;
  final File notesUpdated;
  final File bigCsv;
  final File duplicateNotes;
  final File opaque;
  final Link linkToNotes;

  static Future<E2eFixtures> create() async {
    final root = await Directory.systemTemp.createTemp('keenbench-e2e-');
    final notes = File(path.join(root.path, 'notes.txt'));
    final notesUpdated = File(path.join(root.path, 'notes_updated.txt'));
    final dataCsv = File(path.join(root.path, 'data.csv'));
    final bigCsv = File(path.join(root.path, 'big.csv'));
    final opaque = File(path.join(root.path, 'unknown.bin'));

    await notes.writeAsString(
      'Staffing meeting notes (Jan 30, 2026)\n'
      'Projects:\n'
      '- Atlas: customer onboarding revamp (needs PM, backend, design)\n'
      '- Beacon: usage analytics refresh (needs analyst, frontend, design)\n'
      'Decisions:\n'
      '- Alice Kim will lead Atlas as PM.\n'
      '- Chloe Nguyen will take backend for Atlas.\n'
      '- Bruno Silva will cover Beacon analytics.\n'
      '- Elena Rossi will handle Beacon frontend.\n'
      '- Diego Patel will support design across both projects (80/20 Beacon/Atlas).\n'
      'Open item: QA support is still unassigned.\n',
    );
    await notesUpdated.writeAsString(
      'Staffing update: QA support assigned to Contract QA (pending).\n',
    );
    await dataCsv.writeAsString(
      'name,role,location,availability\n'
      'Alice Kim,Product Manager,Remote,0.6\n'
      'Bruno Silva,Data Analyst,NYC,0.8\n'
      'Chloe Nguyen,Engineer,Remote,1.0\n'
      'Diego Patel,Designer,Austin,0.7\n'
      'Elena Rossi,Engineer,Berlin,0.9\n',
    );
    await opaque.writeAsBytes([0, 1, 2, 3, 4, 5]);

    final raf = await bigCsv.open(mode: FileMode.write);
    await raf.truncate(25 * 1024 * 1024 + 1);
    await raf.close();

    final dupDir = Directory(path.join(root.path, 'dup'));
    await dupDir.create();
    final duplicateNotes = File(path.join(dupDir.path, 'notes.txt'));
    await duplicateNotes.writeAsString('Duplicate notes file.\n');

    final linkToNotes = Link(path.join(root.path, 'link_to_notes'));
    await linkToNotes.create(notes.path);

    return E2eFixtures._(
      root: root,
      notes: notes,
      dataCsv: dataCsv,
      notesUpdated: notesUpdated,
      bigCsv: bigCsv,
      duplicateNotes: duplicateNotes,
      opaque: opaque,
      linkToNotes: linkToNotes,
    );
  }

  Future<List<File>> createSmallFiles(
    int count, {
    String prefix = 'file',
  }) async {
    final files = <File>[];
    for (var i = 0; i < count; i++) {
      final file = File(path.join(root.path, '$prefix-$i.txt'));
      await file.writeAsString('data $i');
      files.add(file);
    }
    return files;
  }

  Future<File> createExtraFile(String name, String content) async {
    final file = File(path.join(root.path, name));
    await file.writeAsString(content);
    return file;
  }
}

String workbenchPublishedPath(
  String dataDir,
  String workbenchId,
  String fileName,
) {
  return path.join(dataDir, 'workbenches', workbenchId, 'published', fileName);
}

String proposalPath(String dataDir, String workbenchId, String proposalId) {
  return path.join(
    dataDir,
    'workbenches',
    workbenchId,
    'meta',
    'workshop',
    'proposals',
    '$proposalId.json',
  );
}
