import 'dart:async';
import 'dart:convert';
import 'dart:io';

import 'package:path/path.dart' as path;

import '../app_env.dart';
import '../logging.dart';

class EngineError implements Exception {
  EngineError(this.message, this.data);

  final String message;
  final Map<String, dynamic> data;

  String get errorCode => data['error_code'] as String? ?? '';
  String? get providerId => data['provider_id'] as String?;
  String? get modelId => data['model_id'] as String?;
  String? get scopeHash => data['scope_hash'] as String?;
  List<String> get actions => (data['actions'] as List<dynamic>? ?? [])
      .map((e) => e.toString())
      .toList();

  @override
  String toString() => 'EngineError($message, $errorCode)';
}

class EngineNotification {
  EngineNotification(this.method, this.params);

  final String method;
  final Map<String, dynamic> params;
}

abstract class EngineApi {
  Stream<EngineNotification> get notifications;
  Future<dynamic> call(String method, [Map<String, dynamic>? params]);
}

class EngineClient implements EngineApi {
  EngineClient();

  final _notificationController =
      StreamController<EngineNotification>.broadcast();
  @override
  Stream<EngineNotification> get notifications =>
      _notificationController.stream;

  Process? _process;
  int _nextId = 1;
  final Map<int, Completer<dynamic>> _pending = {};

  Future<void> start() async {
    if (_process != null) {
      return;
    }
    final env = Map<String, String>.from(Platform.environment);
    AppEnv.applyTo(env);

    final bundledToolWorker = await _resolveBundledBinary(
      'keenbench-tool-worker',
    );
    if ((env['KEENBENCH_TOOL_WORKER_PATH'] ?? '').isEmpty &&
        bundledToolWorker != null) {
      env['KEENBENCH_TOOL_WORKER_PATH'] = bundledToolWorker;
    }

    AppLog.info('engine.start', {
      'debug': AppEnv.isDebug,
      'env_path': env['KEENBENCH_ENV_PATH'],
      'tool_worker_path': env['KEENBENCH_TOOL_WORKER_PATH'],
    });
    final enginePath = await _resolveEnginePath();
    if (enginePath != null) {
      _process = await Process.start(
        enginePath,
        [],
        environment: env,
        mode: ProcessStartMode.detachedWithStdio,
      );
      AppLog.info('engine.started', {'path': enginePath, 'pid': _process?.pid});
    } else {
      final repoRoot = path.normalize(path.join(Directory.current.path, '..'));
      _process = await Process.start(
        'go',
        ['run', './engine/cmd/keenbench-engine'],
        workingDirectory: repoRoot,
        environment: env,
        mode: ProcessStartMode.detachedWithStdio,
      );
      AppLog.info('engine.started', {'mode': 'go run', 'pid': _process?.pid});
    }

    _process?.stdout
        .transform(utf8.decoder)
        .transform(const LineSplitter())
        .listen(_handleLine);
    _process?.stderr
        .transform(utf8.decoder)
        .transform(const LineSplitter())
        .listen((line) {
          if (line.trim().isEmpty) {
            return;
          }
          AppLog.warn('engine.stderr', {'message': line});
          _notificationController.add(
            EngineNotification('EngineError', {'message': line}),
          );
        });
  }

  Future<void> stop() async {
    AppLog.info('engine.stop');
    await _process?.kill();
    _process = null;
    await _notificationController.close();
  }

  @override
  Future<dynamic> call(String method, [Map<String, dynamic>? params]) async {
    if (_process == null) {
      await start();
    }
    final id = _nextId++;
    AppLog.debug('rpc.call', {
      'id': id,
      'method': method,
      if (params != null) 'params': params,
    });
    final payload = <String, dynamic>{
      'jsonrpc': '2.0',
      'id': id,
      'method': method,
      'api_version': '1',
      if (params != null) 'params': params,
    };
    final completer = Completer<dynamic>();
    _pending[id] = completer;
    _process?.stdin.writeln(jsonEncode(payload));
    return completer.future;
  }

  void _handleLine(String line) {
    if (line.trim().isEmpty) {
      return;
    }
    final payload = jsonDecode(line) as Map<String, dynamic>;
    if (payload.containsKey('method') && !payload.containsKey('id')) {
      AppLog.debug('rpc.notify', {
        'method': payload['method'],
        'params': payload['params'],
      });
      _notificationController.add(
        EngineNotification(
          payload['method'] as String,
          (payload['params'] as Map<String, dynamic>? ?? {}),
        ),
      );
      return;
    }
    final id = payload['id'] as int?;
    if (id == null) {
      return;
    }
    final completer = _pending.remove(id);
    if (completer == null) {
      return;
    }
    if (payload['error'] != null) {
      final error = payload['error'] as Map<String, dynamic>;
      final data = (error['data'] as Map<String, dynamic>? ?? {});
      AppLog.warn('rpc.error', {
        'id': id,
        'message': error['message'],
        'data': data,
      });
      completer.completeError(
        EngineError(error['message'] as String? ?? 'engine error', data),
      );
      return;
    }
    AppLog.debug('rpc.response', {'id': id, 'result': payload['result']});
    completer.complete(payload['result']);
  }

  Future<String?> _resolveEnginePath() async {
    final envPath = AppEnv.get('KEENBENCH_ENGINE_PATH');
    if (envPath != null && envPath.isNotEmpty) {
      final file = File(envPath);
      if (await file.exists()) {
        return envPath;
      }
    }

    final bundled = await _resolveBundledBinary('keenbench-engine');
    if (bundled != null) {
      return bundled;
    }

    final candidate = File(
      path.normalize(
        path.join(
          Directory.current.path,
          '..',
          'engine',
          'bin',
          'keenbench-engine',
        ),
      ),
    );
    if (await candidate.exists()) {
      return candidate.path;
    }
    final localCandidate = File(
      path.normalize(
        path.join(Directory.current.path, 'engine', 'bin', 'keenbench-engine'),
      ),
    );
    if (await localCandidate.exists()) {
      return localCandidate.path;
    }
    return null;
  }

  Future<String?> _resolveBundledBinary(String name) async {
    final resolved = Platform.resolvedExecutable;
    if (resolved.isEmpty) {
      return null;
    }
    final exe = File(resolved);
    final candidate = File(path.join(exe.parent.path, name));
    if (await candidate.exists()) {
      return candidate.path;
    }
    return null;
  }
}
