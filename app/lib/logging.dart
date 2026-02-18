import 'package:logger/logger.dart';

import 'app_env.dart';

class AppLog {
  static Logger _logger = _build(Level.info);

  static void init() {
    _logger = _build(AppEnv.isDebug ? Level.debug : Level.info);
  }

  static void info(String event, [Map<String, dynamic>? fields]) {
    _logger.i(_format(event, fields));
  }

  static void debug(String event, [Map<String, dynamic>? fields]) {
    _logger.d(_format(event, fields));
  }

  static void warn(
    String event, [
    Map<String, dynamic>? fields,
    Object? error,
  ]) {
    _logger.w(_format(event, fields), error: error);
  }

  static void error(
    String event, [
    Map<String, dynamic>? fields,
    Object? error,
    StackTrace? stackTrace,
  ]) {
    _logger.e(_format(event, fields), error: error, stackTrace: stackTrace);
  }

  static Logger _build(Level level) {
    return Logger(
      level: level,
      printer: PrettyPrinter(
        methodCount: 0,
        errorMethodCount: 5,
        printTime: true,
        colors: false,
      ),
    );
  }

  static String _format(String event, Map<String, dynamic>? fields) {
    if (fields == null || fields.isEmpty) {
      return event;
    }
    final redacted = _redactValue(fields);
    return '$event $redacted';
  }

  static dynamic _redactValue(dynamic value) {
    if (value is Map) {
      final Map<dynamic, dynamic> out = {};
      value.forEach((key, val) {
        final keyStr = key.toString();
        if (_isSecretKey(keyStr)) {
          out[keyStr] = _redactString(val?.toString() ?? '');
        } else {
          out[keyStr] = _redactValue(val);
        }
      });
      return out;
    }
    if (value is List) {
      return value.map(_redactValue).toList();
    }
    return value;
  }

  static bool _isSecretKey(String key) {
    switch (key.trim().toLowerCase()) {
      case 'api_key':
      case 'apikey':
      case 'authorization':
      case 'openai_api_key':
      case 'keenbench_openai_api_key':
      case 'token':
      case 'secret':
        return true;
      default:
        return false;
    }
  }

  static String _redactString(String value) {
    final trimmed = value.trim();
    if (trimmed.isEmpty) {
      return '';
    }
    if (trimmed.toLowerCase().startsWith('bearer ')) {
      return 'Bearer ${_mask(trimmed.substring(7))}';
    }
    return _mask(trimmed);
  }

  static String _mask(String value) {
    if (value.length <= 4) {
      return '****';
    }
    return '****${value.substring(value.length - 4)}';
  }
}
