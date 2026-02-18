import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:provider/provider.dart';

import 'package:keenbench/app_keys.dart';
import 'package:keenbench/engine/engine_client.dart';
import 'package:keenbench/screens/home_screen.dart';
import 'package:keenbench/theme.dart';

class FakeEngine implements EngineApi {
  final _controller = StreamController<EngineNotification>.broadcast();
  final List<String> calls = [];

  @override
  Stream<EngineNotification> get notifications => _controller.stream;

  int callCount(String method) => calls.where((m) => m == method).length;

  @override
  Future<dynamic> call(String method, [Map<String, dynamic>? params]) async {
    calls.add(method);
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
    if (method == 'WorkbenchOpen') {
      return {
        'workbench': {
          'id': params?['workbench_id'] as String? ?? 'wb-2',
          'name': 'New Workbench',
          'created_at': '2026-01-01T00:00:00Z',
          'updated_at': '2026-01-02T00:00:00Z',
          'default_model_id': 'openai/gpt-4o-mini',
        },
      };
    }
    if (method == 'WorkshopGetState') {
      return {
        'active_model_id': 'openai/gpt-4o-mini',
        'default_model_id': 'openai/gpt-4o-mini',
        'has_draft': false,
        'pending_proposal_id': '',
      };
    }
    if (method == 'ProvidersGetStatus') {
      return {
        'providers': [
          {
            'provider_id': 'openai',
            'display_name': 'OpenAI',
            'enabled': true,
            'configured': true,
            'models': ['openai/gpt-4o-mini'],
          },
        ],
      };
    }
    if (method == 'ModelsListSupported') {
      return {
        'models': [
          {
            'model_id': 'openai/gpt-4o-mini',
            'provider_id': 'openai',
            'display_name': 'GPT-4o mini',
            'context_tokens_estimate': 128000,
            'supports_file_read': true,
            'supports_file_write': true,
          },
        ],
      };
    }
    if (method == 'WorkbenchFilesList') {
      return {'files': const []};
    }
    if (method == 'WorkbenchGetScope') {
      return {
        'limits': {'max_files': 10, 'max_file_bytes': 26214400},
        'supported_types': ['txt', 'csv', 'md'],
        'sandbox_root': '/tmp/workbench',
      };
    }
    if (method == 'ContextList') {
      return {'items': const []};
    }
    if (method == 'WorkshopGetConversation') {
      return {'messages': const []};
    }
    if (method == 'DraftGetState') {
      return {'has_draft': false};
    }
    if (method == 'WorkbenchGetClutter') {
      return {
        'score': 0.0,
        'level': 'Light',
        'model_id': 'openai/gpt-4o-mini',
        'context_items_weight': 0.0,
        'context_share': 0.0,
        'context_warning': false,
      };
    }
    return {};
  }
}

void main() {
  Future<void> pumpHome(WidgetTester tester, FakeEngine engine) async {
    await tester.pumpWidget(
      Provider<EngineApi>.value(
        value: engine,
        child: MaterialApp(
          theme: KeenBenchTheme.theme(),
          home: const HomeScreen(),
        ),
      ),
    );
    await tester.pumpAndSettle();
  }

  testWidgets('Home screen renders workbenches', (tester) async {
    final engine = FakeEngine();
    await pumpHome(tester, engine);
    expect(find.text('Workbenches'), findsOneWidget);
    expect(find.text('Sample Workbench'), findsOneWidget);
  });

  testWidgets('new workbench dialog Escape cancels creation', (tester) async {
    final engine = FakeEngine();
    await pumpHome(tester, engine);

    await tester.tap(find.byKey(AppKeys.homeNewWorkbenchButton));
    await tester.pumpAndSettle();
    expect(find.byKey(AppKeys.newWorkbenchDialog), findsOneWidget);

    await tester.sendKeyEvent(LogicalKeyboardKey.escape);
    await tester.pumpAndSettle();

    expect(find.byKey(AppKeys.newWorkbenchDialog), findsNothing);
    expect(engine.callCount('WorkbenchCreate'), 0);
  });

  testWidgets('new workbench dialog Enter submits creation', (tester) async {
    final engine = FakeEngine();
    await pumpHome(tester, engine);

    await tester.tap(find.byKey(AppKeys.homeNewWorkbenchButton));
    await tester.pumpAndSettle();
    expect(find.byKey(AppKeys.newWorkbenchDialog), findsOneWidget);

    await tester.enterText(find.byKey(AppKeys.newWorkbenchNameField), 'WB 2');
    await tester.sendKeyEvent(LogicalKeyboardKey.enter);
    await tester.pumpAndSettle();

    expect(engine.callCount('WorkbenchCreate'), 1);
    expect(find.byKey(AppKeys.workbenchScreen), findsOneWidget);
  });
}
