import 'dart:io';

import 'package:path/path.dart' as path;

class AppEnv {
  static final Map<String, String> _fileEnv = {};
  static final Map<String, String> _runtimeEnv = {};
  static bool _loaded = false;

  static void load() {
    if (_loaded) {
      return;
    }
    _loaded = true;
    final override = Platform.environment['KEENBENCH_ENV_PATH'];
    final envPath = override != null && override.trim().isNotEmpty
        ? override.trim()
        : _findEnvPath();
    if (envPath == null) {
      return;
    }
    final file = File(envPath);
    if (!file.existsSync()) {
      return;
    }
    final lines = file.readAsLinesSync();
    for (final raw in lines) {
      var line = raw.trim();
      if (line.isEmpty || line.startsWith('#')) {
        continue;
      }
      if (line.startsWith('export ')) {
        line = line.substring(7).trim();
      }
      final idx = line.indexOf('=');
      if (idx <= 0) {
        continue;
      }
      final key = line.substring(0, idx).trim();
      if (key.isEmpty) {
        continue;
      }
      var value = line.substring(idx + 1).trim();
      value = _stripQuotes(value);
      if (Platform.environment.containsKey(key)) {
        continue;
      }
      _fileEnv[key] = value;
    }
  }

  static void applyTo(Map<String, String> target) {
    load();
    _runtimeEnv.forEach((key, value) {
      target[key] = value;
    });
    _fileEnv.forEach((key, value) {
      target.putIfAbsent(key, () => value);
    });
  }

  static String? get(String key) {
    load();
    return _runtimeEnv[key] ?? Platform.environment[key] ?? _fileEnv[key];
  }

  static void setOverride(String key, String value) {
    if (key.trim().isEmpty) {
      return;
    }
    _runtimeEnv[key] = value;
  }

  static void setOverrides(Map<String, String> values) {
    values.forEach((key, value) {
      setOverride(key, value);
    });
  }

  static void clearOverrides() {
    _runtimeEnv.clear();
  }

  static bool get isDebug => _parseBool(get('KEENBENCH_DEBUG'));

  static String? _findEnvPath() {
    var dir = Directory.current;
    while (true) {
      final candidate = File(path.join(dir.path, '.env'));
      if (candidate.existsSync()) {
        return candidate.path;
      }
      final parent = dir.parent;
      if (parent.path == dir.path) {
        return null;
      }
      dir = parent;
    }
  }

  static String _stripQuotes(String value) {
    if (value.length < 2) {
      return value;
    }
    final first = value[0];
    final last = value[value.length - 1];
    if ((first == '"' && last == '"') || (first == "'" && last == "'")) {
      return value.substring(1, value.length - 1);
    }
    return value;
  }

  static bool _parseBool(String? value) {
    if (value == null) {
      return false;
    }
    switch (value.trim().toLowerCase()) {
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
}
