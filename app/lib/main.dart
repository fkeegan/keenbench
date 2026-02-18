import 'package:flutter/material.dart';
import 'package:provider/provider.dart';
import 'package:window_manager/window_manager.dart';

import 'app_env.dart';
import 'engine/engine_client.dart';
import 'logging.dart';
import 'screens/home_screen.dart';
import 'theme.dart';
import 'window_state.dart';

Future<void> main() async {
  WidgetsFlutterBinding.ensureInitialized();
  await windowManager.ensureInitialized();
  AppEnv.load();
  AppLog.init();

  final windowState = await WindowStateStore.load();

  final windowOptions = WindowOptions(
    size: Size(windowState.width, windowState.height),
    minimumSize: const Size(WindowState.minWidth, WindowState.minHeight),
    center: windowState.isFirstRun,
    title: 'KeenBench',
  );

  await windowManager.waitUntilReadyToShow(windowOptions, () async {
    if (!windowState.isFirstRun) {
      await windowManager.setPosition(Offset(windowState.x, windowState.y));
    }
    if (windowState.isMaximized) {
      await windowManager.maximize();
    }
    await windowManager.show();
  });

  await windowManager.setPreventClose(true);
  windowManager.addListener(WindowStateListener());

  final engine = EngineClient();
  runApp(Provider<EngineApi>.value(value: engine, child: const KeenBenchApp()));
}

class KeenBenchApp extends StatelessWidget {
  const KeenBenchApp({super.key});

  @override
  Widget build(BuildContext context) {
    return MaterialApp(
      title: 'KeenBench',
      debugShowCheckedModeBanner: false,
      theme: KeenBenchTheme.theme(),
      builder: (context, child) {
        final mediaQuery = MediaQuery.of(context);
        final scopedMediaQuery = mediaQuery.copyWith(
          textScaler: mediaQuery.textScaler.clamp(
            minScaleFactor: 1.0,
            maxScaleFactor: 2.0,
          ),
        );
        final appChild = child ?? const SizedBox.shrink();
        return MediaQuery(
          data: scopedMediaQuery,
          child: TickerMode(
            enabled: !scopedMediaQuery.disableAnimations,
            child: appChild,
          ),
        );
      },
      home: const HomeScreen(),
    );
  }
}
