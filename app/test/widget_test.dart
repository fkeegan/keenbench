import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:provider/provider.dart';

import 'package:keenbench/engine/engine_client.dart';
import 'package:keenbench/screens/home_screen.dart';
import 'package:keenbench/theme.dart';

class FakeEngine implements EngineApi {
  final _controller = StreamController<EngineNotification>.broadcast();

  @override
  Stream<EngineNotification> get notifications => _controller.stream;

  @override
  Future<dynamic> call(String method, [Map<String, dynamic>? params]) async {
    if (method == 'WorkbenchList') {
      return {
        'workbenches': [
          {
            'id': 'wb-1',
            'name': 'Sample Workbench',
            'created_at': '2026-01-01T00:00:00Z',
            'updated_at': '2026-01-02T00:00:00Z',
          },
        ],
      };
    }
    if (method == 'WorkbenchCreate') {
      return {'workbench_id': 'wb-2'};
    }
    return {};
  }
}

void main() {
  testWidgets('Home screen renders workbenches', (tester) async {
    await tester.pumpWidget(
      Provider<EngineApi>.value(
        value: FakeEngine(),
        child: MaterialApp(
          theme: KeenBenchTheme.theme(),
          home: const HomeScreen(),
        ),
      ),
    );
    await tester.pumpAndSettle();

    expect(find.text('Workbenches'), findsOneWidget);
    expect(find.text('Sample Workbench'), findsOneWidget);
  });
}
