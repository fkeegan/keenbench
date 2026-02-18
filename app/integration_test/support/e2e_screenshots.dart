import 'dart:io';

import 'package:path/path.dart' as path;

import 'package:keenbench/app_env.dart';

class E2eScreenshooter {
  E2eScreenshooter() : _repoRoot = _resolveRepoRoot();

  final String _repoRoot;

  String get outputDir =>
      AppEnv.get('KEENBENCH_E2E_SCREENSHOTS_DIR') ??
      path.join(_repoRoot, 'artifacts', 'screenshots');

  String get scriptPath =>
      AppEnv.get('KEENBENCH_E2E_CAPTURE_SCRIPT') ??
      path.join(_repoRoot, 'scripts', 'e2e', 'capture_window.sh');

  bool get screenshotsEnabled => AppEnv.get('KEENBENCH_E2E_SCREENSHOTS') != '0';

  bool get pauseOnFailure =>
      AppEnv.get('KEENBENCH_E2E_PAUSE_ON_FAILURE') == '1';

  String get pauseFile =>
      AppEnv.get('KEENBENCH_E2E_PAUSE_FILE') ??
      path.join(outputDir, '.e2e_resume');

  Future<void> capture(String label) async {
    if (!screenshotsEnabled) {
      return;
    }

    await Future.delayed(const Duration(milliseconds: 150));

    final output = Directory(outputDir);
    if (!output.existsSync()) {
      output.createSync(recursive: true);
    }

    final env = Map<String, String>.from(Platform.environment);
    env['KEENBENCH_E2E_PID'] = pid.toString();
    env['KEENBENCH_E2E_SCREENSHOTS_DIR'] = outputDir;
    env['KEENBENCH_E2E_WINDOW_TITLE'] = 'KeenBench';

    final result = await Process.run(scriptPath, [label], environment: env);

    if (result.exitCode != 0) {
      stderr.writeln(result.stderr);
      throw StateError('Screenshot capture failed: ${result.stderr}');
    }

    final stdoutText = (result.stdout as String).trim();
    if (stdoutText.isNotEmpty) {
      stdout.writeln('Saved screenshot: $stdoutText');
    }
  }

  Future<void> pauseIfEnabled(String reason) async {
    if (!pauseOnFailure) {
      return;
    }

    final output = Directory(outputDir);
    if (!output.existsSync()) {
      output.createSync(recursive: true);
    }

    stdout.writeln('E2E paused: $reason');
    stdout.writeln('Create file to resume: $pauseFile');

    while (!File(pauseFile).existsSync()) {
      await Future.delayed(const Duration(seconds: 1));
    }
  }

  static String _resolveRepoRoot() {
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
}
