import 'dart:async';
import 'dart:convert';
import 'dart:io';

import 'package:path/path.dart' as p;
import 'package:window_manager/window_manager.dart';

import 'logging.dart';

// ---------------------------------------------------------------------------
// WindowState — data class for persisted window geometry
// ---------------------------------------------------------------------------

class WindowState {
  const WindowState({
    required this.x,
    required this.y,
    required this.width,
    required this.height,
    required this.isMaximized,
  });

  final double x;
  final double y;
  final double width;
  final double height;
  final bool isMaximized;

  static const double defaultWidth = 1280;
  static const double defaultHeight = 720;
  static const double minWidth = 800;
  static const double minHeight = 500;

  static const WindowState defaults = WindowState(
    x: -1,
    y: -1,
    width: defaultWidth,
    height: defaultHeight,
    isMaximized: false,
  );

  bool get isFirstRun => x == -1 && y == -1;

  Map<String, dynamic> toJson() => {
    'x': x,
    'y': y,
    'width': width,
    'height': height,
    'isMaximized': isMaximized,
  };

  factory WindowState.fromJson(Map<String, dynamic> json) {
    final x = (json['x'] as num?)?.toDouble();
    final y = (json['y'] as num?)?.toDouble();
    final width = (json['width'] as num?)?.toDouble();
    final height = (json['height'] as num?)?.toDouble();
    final isMaximized = json['isMaximized'] as bool? ?? false;

    if (x == null || y == null || width == null || height == null) {
      return defaults;
    }
    if (width < minWidth || height < minHeight) {
      return defaults;
    }
    if (width > 10000 || height > 10000) {
      return defaults;
    }

    return WindowState(
      x: x,
      y: y,
      width: width,
      height: height,
      isMaximized: isMaximized,
    );
  }
}

// ---------------------------------------------------------------------------
// WindowStateStore — load/save to JSON file on disk
// ---------------------------------------------------------------------------

class WindowStateStore {
  static const _fileName = 'window_state.json';
  static const _appDir = 'keenbench';

  static String _configDir() {
    if (Platform.isMacOS) {
      final home = Platform.environment['HOME'] ?? '';
      return p.join(home, 'Library', 'Application Support', _appDir);
    }
    if (Platform.isWindows) {
      final appData = Platform.environment['APPDATA'] ?? '';
      return p.join(appData, _appDir);
    }
    // Linux and others
    final xdgConfig =
        Platform.environment['XDG_CONFIG_HOME'] ??
        p.join(Platform.environment['HOME'] ?? '', '.config');
    return p.join(xdgConfig, _appDir);
  }

  static String _filePath() => p.join(_configDir(), _fileName);

  static Future<WindowState> load() async {
    try {
      final file = File(_filePath());
      if (!await file.exists()) {
        return WindowState.defaults;
      }
      final content = await file.readAsString();
      if (content.trim().isEmpty) {
        return WindowState.defaults;
      }
      final json = jsonDecode(content) as Map<String, dynamic>;
      return WindowState.fromJson(json);
    } catch (e) {
      AppLog.warn('window_state.load_failed', {'error': e.toString()});
      return WindowState.defaults;
    }
  }

  static Future<void> save(WindowState state) async {
    try {
      final dir = Directory(_configDir());
      if (!await dir.exists()) {
        await dir.create(recursive: true);
      }
      final file = File(_filePath());
      await file.writeAsString(jsonEncode(state.toJson()));
    } catch (e) {
      AppLog.warn('window_state.save_failed', {'error': e.toString()});
    }
  }
}

// ---------------------------------------------------------------------------
// WindowStateListener — saves state on close/move/resize
// ---------------------------------------------------------------------------

class WindowStateListener extends WindowListener {
  Timer? _debounceTimer;

  Future<void> _saveCurrentState() async {
    final isMaximized = await windowManager.isMaximized();
    final bounds = await windowManager.getBounds();
    final state = WindowState(
      x: bounds.left,
      y: bounds.top,
      width: bounds.width,
      height: bounds.height,
      isMaximized: isMaximized,
    );
    await WindowStateStore.save(state);
  }

  void _debounceSave() {
    _debounceTimer?.cancel();
    _debounceTimer = Timer(const Duration(milliseconds: 500), () {
      _saveCurrentState();
    });
  }

  @override
  void onWindowClose() async {
    _debounceTimer?.cancel();
    await _saveCurrentState();
    await windowManager.setPreventClose(false);
    await windowManager.close();
  }

  @override
  void onWindowResize() {
    _debounceSave();
  }

  @override
  void onWindowMove() {
    _debounceSave();
  }

  @override
  void onWindowMaximize() {
    _debounceSave();
  }

  @override
  void onWindowUnmaximize() {
    _debounceSave();
  }
}
