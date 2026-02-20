import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:provider/provider.dart';

import 'package:keenbench/app_keys.dart';
import 'package:keenbench/engine/engine_client.dart';
import 'package:keenbench/egress_consent.dart';
import 'package:keenbench/screens/settings_screen.dart';
import 'package:keenbench/theme.dart';

class _FakeSettingsEngine implements EngineApi {
  _FakeSettingsEngine({
    this.openAICodexConnected = false,
    this.openAICodexExpired = false,
    this.openAICodexAccountLabel,
    this.openAICodexExpiresAt,
    this.oauthCallbackListening = true,
    List<Map<String, dynamic>>? oauthStatusResponses,
    Map<String, dynamic>? oauthStatusDefault,
    this.openAIResearchEffort = 'medium',
    this.openAIPlanEffort = 'medium',
    this.openAIImplementEffort = 'medium',
    this.openAICodexResearchEffort = 'medium',
    this.openAICodexPlanEffort = 'medium',
    this.openAICodexImplementEffort = 'medium',
    this.anthropicResearchEffort = 'medium',
    this.anthropicPlanEffort = 'medium',
    this.anthropicImplementEffort = 'medium',
  }) : oauthStatusResponses = List<Map<String, dynamic>>.from(
         oauthStatusResponses ?? const <Map<String, dynamic>>[],
       ),
       oauthStatusDefault = Map<String, dynamic>.from(
         oauthStatusDefault ?? const {'status': 'pending'},
       );

  final _notifications = StreamController<EngineNotification>.broadcast();
  final List<String> calls = [];
  final Map<String, List<Map<String, dynamic>?>> paramsHistory = {};

  bool openAIConfigured = false;
  bool anthropicConfigured = false;
  bool mistralConfigured = false;
  bool openAICodexConnected;
  bool openAICodexExpired;
  String? openAICodexAccountLabel;
  String? openAICodexExpiresAt;
  bool oauthCallbackListening;
  final List<Map<String, dynamic>> oauthStatusResponses;
  final Map<String, dynamic> oauthStatusDefault;
  String openAIResearchEffort;
  String openAIPlanEffort;
  String openAIImplementEffort;
  String openAICodexResearchEffort;
  String openAICodexPlanEffort;
  String openAICodexImplementEffort;
  String anthropicResearchEffort;
  String anthropicPlanEffort;
  String anthropicImplementEffort;
  String defaultModelId = 'openai/gpt-4o-mini';
  String userConsentMode = consentModeAsk;

  int _nextFlow = 1;
  String? lastFlowId;

  @override
  Stream<EngineNotification> get notifications => _notifications.stream;

  int callCount(String method) =>
      calls.where((entry) => entry == method).length;

  Map<String, dynamic>? lastParams(String method) {
    final entries = paramsHistory[method];
    if (entries == null || entries.isEmpty) {
      return null;
    }
    return entries.last;
  }

  @override
  Future<dynamic> call(String method, [Map<String, dynamic>? params]) async {
    calls.add(method);
    paramsHistory
        .putIfAbsent(method, () => [])
        .add(params == null ? null : Map<String, dynamic>.from(params));

    switch (method) {
      case 'ProvidersGetStatus':
        return {
          'providers': [
            {
              'provider_id': 'openai',
              'display_name': 'OpenAI',
              'enabled': true,
              'configured': openAIConfigured,
              'models': ['openai/gpt-4o-mini'],
              'auth_mode': 'api_key',
              'rpi_reasoning': {
                'research_effort': openAIResearchEffort,
                'plan_effort': openAIPlanEffort,
                'implement_effort': openAIImplementEffort,
              },
            },
            {
              'provider_id': 'openai-codex',
              'display_name': 'OpenAI Codex',
              'enabled': true,
              'configured': openAICodexConnected,
              'models': ['openai-codex/gpt-5-codex'],
              'auth_mode': 'oauth',
              'rpi_reasoning': {
                'research_effort': openAICodexResearchEffort,
                'plan_effort': openAICodexPlanEffort,
                'implement_effort': openAICodexImplementEffort,
              },
              'oauth_connected': openAICodexConnected,
              'oauth_expired': openAICodexExpired,
              if (openAICodexAccountLabel != null)
                'oauth_account_label': openAICodexAccountLabel,
              if (openAICodexExpiresAt != null)
                'oauth_expires_at': openAICodexExpiresAt,
            },
            {
              'provider_id': 'anthropic',
              'display_name': 'Anthropic',
              'enabled': true,
              'configured': anthropicConfigured,
              'models': [
                'anthropic:claude-sonnet-4-6',
                'anthropic:claude-opus-4-6',
              ],
              'auth_mode': 'api_key',
              'rpi_reasoning': {
                'research_effort': anthropicResearchEffort,
                'plan_effort': anthropicPlanEffort,
                'implement_effort': anthropicImplementEffort,
              },
            },
            {
              'provider_id': 'mistral',
              'display_name': 'Mistral',
              'enabled': true,
              'configured': mistralConfigured,
              'models': ['mistral:mistral-large'],
              'auth_mode': 'api_key',
            },
          ],
        };
      case 'ModelsListSupported':
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
            {
              'model_id': 'openai-codex/gpt-5-codex',
              'provider_id': 'openai-codex',
              'display_name': 'GPT-5 Codex',
              'context_tokens_estimate': 200000,
              'supports_file_read': true,
              'supports_file_write': true,
            },
            {
              'model_id': 'mistral:mistral-large',
              'provider_id': 'mistral',
              'display_name': 'Mistral Large',
              'context_tokens_estimate': 128000,
              'supports_file_read': true,
              'supports_file_write': true,
            },
            {
              'model_id': 'anthropic:claude-sonnet-4-6',
              'provider_id': 'anthropic',
              'display_name': 'Anthropic Claude Sonnet 4.6',
              'context_tokens_estimate': 200000,
              'supports_file_read': true,
              'supports_file_write': true,
            },
            {
              'model_id': 'anthropic:claude-opus-4-6',
              'provider_id': 'anthropic',
              'display_name': 'Anthropic Claude Opus 4.6',
              'context_tokens_estimate': 200000,
              'supports_file_read': true,
              'supports_file_write': true,
            },
          ],
        };
      case 'UserGetDefaultModel':
        return {'model_id': defaultModelId};
      case 'UserSetDefaultModel':
        defaultModelId = params?['model_id'] as String? ?? defaultModelId;
        return {};
      case 'UserGetConsentMode':
        return {'mode': userConsentMode};
      case 'UserSetConsentMode':
        final mode = (params?['mode'] as String? ?? '').trim();
        if (mode == consentModeAllowAll && params?['approved'] != true) {
          throw EngineError(
            'explicit approval required for allow_all mode',
            const {'error_code': 'VALIDATION_FAILED'},
          );
        }
        userConsentMode = mode;
        return {};
      case 'ProvidersSetEnabled':
        return {};
      case 'ProvidersSetApiKey':
        final providerId = params?['provider_id'] as String? ?? '';
        if (providerId == 'openai') {
          openAIConfigured = true;
        } else if (providerId == 'anthropic') {
          anthropicConfigured = true;
        } else if (providerId == 'mistral') {
          mistralConfigured = true;
        }
        return {};
      case 'ProvidersSetReasoningEffort':
        final providerId = params?['provider_id'] as String? ?? '';
        final research = params?['research_effort'] as String?;
        final plan = params?['plan_effort'] as String?;
        final implement = params?['implement_effort'] as String?;
        if (providerId == 'openai') {
          if (research != null) {
            openAIResearchEffort = research;
          }
          if (plan != null) {
            openAIPlanEffort = plan;
          }
          if (implement != null) {
            openAIImplementEffort = implement;
          }
        } else if (providerId == 'openai-codex') {
          if (research != null) {
            openAICodexResearchEffort = research;
          }
          if (plan != null) {
            openAICodexPlanEffort = plan;
          }
          if (implement != null) {
            openAICodexImplementEffort = implement;
          }
        } else if (providerId == 'anthropic') {
          if (research != null) {
            anthropicResearchEffort = research;
          }
          if (plan != null) {
            anthropicPlanEffort = plan;
          }
          if (implement != null) {
            anthropicImplementEffort = implement;
          }
        }
        return {};
      case 'ProvidersValidate':
        return {'ok': true};
      case 'ProvidersOAuthStart':
        final flowId = 'flow-${_nextFlow++}';
        lastFlowId = flowId;
        return {
          'provider_id': 'openai-codex',
          'flow_id': flowId,
          'authorize_url': 'https://example.test/oauth/start?flow_id=$flowId',
          'status': 'pending',
          'expires_at': '2026-02-17T12:00:00Z',
          'callback_listening': oauthCallbackListening,
        };
      case 'ProvidersOAuthStatus':
        if (oauthStatusResponses.isNotEmpty) {
          return Map<String, dynamic>.from(oauthStatusResponses.removeAt(0));
        }
        return Map<String, dynamic>.from(oauthStatusDefault);
      case 'ProvidersOAuthComplete':
        openAICodexConnected = true;
        openAICodexExpired = false;
        openAICodexAccountLabel = 'acct_test';
        openAICodexExpiresAt = '2026-02-17T13:00:00Z';
        return {
          'provider_id': 'openai-codex',
          'oauth_connected': true,
          'oauth_account_label': openAICodexAccountLabel,
          'oauth_expires_at': openAICodexExpiresAt,
        };
      case 'ProvidersOAuthDisconnect':
        openAICodexConnected = false;
        openAICodexExpired = false;
        openAICodexAccountLabel = null;
        openAICodexExpiresAt = null;
        return {};
      default:
        return {};
    }
  }
}

Future<void> _pumpSettingsScreen(
  WidgetTester tester,
  _FakeSettingsEngine engine, {
  ExternalUrlLauncher? urlLauncher,
  Duration oauthStatusPollInterval = const Duration(milliseconds: 10),
  Duration oauthStatusWaitTimeout = const Duration(milliseconds: 300),
}) async {
  await tester.pumpWidget(
    Provider<EngineApi>.value(
      value: engine,
      child: MaterialApp(
        theme: KeenBenchTheme.theme(),
        home: SettingsScreen(
          urlLauncher: urlLauncher,
          oauthStatusPollInterval: oauthStatusPollInterval,
          oauthStatusWaitTimeout: oauthStatusWaitTimeout,
        ),
      ),
    ),
  );
  await tester.pumpAndSettle();
}

List<String> _dropdownLabels(WidgetTester tester, Key key) {
  final dropdown = tester.widget<DropdownButton<String>>(find.byKey(key));
  final items = dropdown.items ?? const <DropdownMenuItem<String>>[];
  return items.map((item) {
    final child = item.child;
    if (child is Text) {
      return child.data ?? '';
    }
    return '';
  }).toList();
}

List<String> _dropdownValues(WidgetTester tester, Key key) {
  final dropdown = tester.widget<DropdownButton<String>>(find.byKey(key));
  final items = dropdown.items ?? const <DropdownMenuItem<String>>[];
  return items.map((item) => item.value ?? '').toList();
}

Future<void> _selectDropdownOption(
  WidgetTester tester,
  Key key,
  String optionLabel,
) async {
  final dropdown = find.byKey(key);
  await tester.ensureVisible(dropdown);
  await tester.tap(dropdown);
  await tester.pumpAndSettle();
  await tester.tap(find.text(optionLabel).last);
  await tester.pumpAndSettle();
}

void main() {
  testWidgets('OAuth provider card renders disconnected state', (tester) async {
    final engine = _FakeSettingsEngine();

    await _pumpSettingsScreen(tester, engine);

    expect(
      find.byKey(AppKeys.settingsOAuthStatusText('openai-codex')),
      findsOneWidget,
    );
    expect(find.text('Not connected'), findsOneWidget);
    expect(
      find.byKey(AppKeys.settingsOAuthConnectButton('openai-codex')),
      findsOneWidget,
    );
  });

  testWidgets('OAuth connect flow auto-completes after callback capture', (
    tester,
  ) async {
    final engine = _FakeSettingsEngine(
      oauthStatusResponses: [
        {'status': 'pending'},
        {'status': 'code_received', 'code_captured': true},
      ],
    );
    final launchedUrls = <String>[];

    await _pumpSettingsScreen(
      tester,
      engine,
      urlLauncher: (url) async => launchedUrls.add(url),
      oauthStatusPollInterval: const Duration(milliseconds: 10),
      oauthStatusWaitTimeout: const Duration(milliseconds: 300),
    );

    final connectButton = find.byKey(
      AppKeys.settingsOAuthConnectButton('openai-codex'),
    );
    await tester.ensureVisible(connectButton);
    await tester.tap(connectButton);
    await tester.pumpAndSettle();

    expect(engine.callCount('ProvidersOAuthStart'), 1);
    expect(launchedUrls, hasLength(1));
    expect(engine.callCount('ProvidersOAuthStatus') >= 1, isTrue);

    expect(
      find.byKey(AppKeys.settingsOAuthRedirectField('openai-codex')),
      findsNothing,
    );

    expect(engine.callCount('ProvidersOAuthComplete'), 1);

    final completeParams = engine.lastParams('ProvidersOAuthComplete');
    expect(completeParams?['provider_id'], 'openai-codex');
    expect(completeParams?['flow_id'], engine.lastFlowId);
    expect(completeParams?.containsKey('redirect_url'), isFalse);

    final startIndex = engine.calls.indexOf('ProvidersOAuthStart');
    final completeIndex = engine.calls.indexOf('ProvidersOAuthComplete');
    expect(startIndex >= 0, isTrue);
    expect(completeIndex > startIndex, isTrue);
  });

  testWidgets(
    'OAuth connect falls back to manual entry when callback is unavailable',
    (tester) async {
      final engine = _FakeSettingsEngine(oauthCallbackListening: false);
      final launchedUrls = <String>[];

      await _pumpSettingsScreen(
        tester,
        engine,
        urlLauncher: (url) async => launchedUrls.add(url),
      );

      final connectButton = find.byKey(
        AppKeys.settingsOAuthConnectButton('openai-codex'),
      );
      await tester.ensureVisible(connectButton);
      await tester.tap(connectButton);
      await tester.pumpAndSettle();

      expect(engine.callCount('ProvidersOAuthStart'), 1);
      expect(engine.callCount('ProvidersOAuthStatus'), 0);
      expect(launchedUrls, hasLength(1));
      expect(
        find.byKey(AppKeys.settingsOAuthRedirectField('openai-codex')),
        findsOneWidget,
      );

      await tester.enterText(
        find.byKey(AppKeys.settingsOAuthRedirectField('openai-codex')),
        'http://localhost:1455/auth/callback?code=code-1&state=state-1',
      );
      await tester.tap(
        find.byKey(AppKeys.settingsOAuthCompleteButton('openai-codex')),
      );
      await tester.pumpAndSettle();

      expect(engine.callCount('ProvidersOAuthComplete'), 1);
      expect(
        engine.lastParams('ProvidersOAuthComplete')?['redirect_url'],
        'http://localhost:1455/auth/callback?code=code-1&state=state-1',
      );
    },
  );

  testWidgets('OAuth connect falls back to manual entry after timeout', (
    tester,
  ) async {
    final engine = _FakeSettingsEngine(
      oauthStatusDefault: {'status': 'pending'},
    );
    final launchedUrls = <String>[];

    await _pumpSettingsScreen(
      tester,
      engine,
      urlLauncher: (url) async => launchedUrls.add(url),
      oauthStatusPollInterval: const Duration(milliseconds: 10),
      oauthStatusWaitTimeout: const Duration(milliseconds: 40),
    );

    final connectButton = find.byKey(
      AppKeys.settingsOAuthConnectButton('openai-codex'),
    );
    await tester.ensureVisible(connectButton);
    await tester.tap(connectButton);
    await tester.pumpAndSettle();

    expect(engine.callCount('ProvidersOAuthStart'), 1);
    expect(engine.callCount('ProvidersOAuthStatus') >= 1, isTrue);
    expect(launchedUrls, hasLength(1));
    expect(
      find.byKey(AppKeys.settingsOAuthRedirectField('openai-codex')),
      findsOneWidget,
    );

    await tester.enterText(
      find.byKey(AppKeys.settingsOAuthRedirectField('openai-codex')),
      'http://localhost:1455/auth/callback?code=code-1&state=state-1',
    );
    await tester.tap(
      find.byKey(AppKeys.settingsOAuthCompleteButton('openai-codex')),
    );
    await tester.pumpAndSettle();

    expect(engine.callCount('ProvidersOAuthComplete'), 1);
    expect(
      engine.lastParams('ProvidersOAuthComplete')?['redirect_url'],
      'http://localhost:1455/auth/callback?code=code-1&state=state-1',
    );
  });

  testWidgets('OAuth connect cancel stops without completion', (tester) async {
    final engine = _FakeSettingsEngine();
    final launchedUrls = <String>[];

    await _pumpSettingsScreen(
      tester,
      engine,
      urlLauncher: (url) async => launchedUrls.add(url),
      oauthStatusPollInterval: const Duration(milliseconds: 10),
      oauthStatusWaitTimeout: const Duration(seconds: 1),
    );

    final connectButton = find.byKey(
      AppKeys.settingsOAuthConnectButton('openai-codex'),
    );
    await tester.ensureVisible(connectButton);
    await tester.tap(connectButton);
    await tester.pump();
    await tester.pump(const Duration(milliseconds: 20));

    expect(
      find.byKey(AppKeys.settingsOAuthProgressDialog('openai-codex')),
      findsOneWidget,
    );
    await tester.tap(
      find.byKey(AppKeys.settingsOAuthProgressCancelButton('openai-codex')),
    );
    await tester.pumpAndSettle();

    expect(engine.callCount('ProvidersOAuthStart'), 1);
    expect(engine.callCount('ProvidersOAuthComplete'), 0);
    expect(launchedUrls, hasLength(1));
    expect(find.text('Not connected'), findsOneWidget);
  });

  testWidgets('OAuth disconnect calls disconnect RPC', (tester) async {
    final engine = _FakeSettingsEngine(
      openAICodexConnected: true,
      openAICodexExpired: false,
      openAICodexAccountLabel: 'acct_test',
      openAICodexExpiresAt: '2026-02-17T13:00:00Z',
    );

    await _pumpSettingsScreen(tester, engine);

    expect(
      find.byKey(AppKeys.settingsOAuthDisconnectButton('openai-codex')),
      findsOneWidget,
    );

    final disconnectButton = find.byKey(
      AppKeys.settingsOAuthDisconnectButton('openai-codex'),
    );
    await tester.ensureVisible(disconnectButton);
    await tester.tap(disconnectButton);
    await tester.pumpAndSettle();

    expect(engine.callCount('ProvidersOAuthDisconnect'), 1);
    expect(
      engine.lastParams('ProvidersOAuthDisconnect')?['provider_id'],
      'openai-codex',
    );
    expect(find.text('Not connected'), findsOneWidget);
  });

  testWidgets('OpenAI reasoning effort dropdowns expose expected options', (
    tester,
  ) async {
    final engine = _FakeSettingsEngine();

    await _pumpSettingsScreen(tester, engine);

    final researchKey = AppKeys.settingsReasoningResearchDropdown('openai');
    final planKey = AppKeys.settingsReasoningPlanDropdown('openai');
    final implementKey = AppKeys.settingsReasoningImplementDropdown('openai');

    expect(find.byKey(researchKey), findsOneWidget);
    expect(find.byKey(planKey), findsOneWidget);
    expect(find.byKey(implementKey), findsOneWidget);

    const expectedLabels = ['None', 'Low', 'Medium', 'High'];
    const expectedValues = ['none', 'low', 'medium', 'high'];

    expect(_dropdownLabels(tester, researchKey), expectedLabels);
    expect(_dropdownLabels(tester, planKey), expectedLabels);
    expect(_dropdownLabels(tester, implementKey), expectedLabels);

    expect(_dropdownValues(tester, researchKey), expectedValues);
    expect(_dropdownValues(tester, planKey), expectedValues);
    expect(_dropdownValues(tester, implementKey), expectedValues);
  });

  testWidgets(
    'OpenAI Codex reasoning effort dropdowns expose expected options',
    (tester) async {
      final engine = _FakeSettingsEngine();

      await _pumpSettingsScreen(tester, engine);

      final researchKey = AppKeys.settingsReasoningResearchDropdown(
        'openai-codex',
      );
      final planKey = AppKeys.settingsReasoningPlanDropdown('openai-codex');
      final implementKey = AppKeys.settingsReasoningImplementDropdown(
        'openai-codex',
      );

      expect(find.byKey(researchKey), findsOneWidget);
      expect(find.byKey(planKey), findsOneWidget);
      expect(find.byKey(implementKey), findsOneWidget);

      const expectedLabels = ['Low', 'Medium', 'High', 'Extra high'];
      const expectedValues = ['low', 'medium', 'high', 'xhigh'];

      expect(_dropdownLabels(tester, researchKey), expectedLabels);
      expect(_dropdownLabels(tester, planKey), expectedLabels);
      expect(_dropdownLabels(tester, implementKey), expectedLabels);

      expect(_dropdownValues(tester, researchKey), expectedValues);
      expect(_dropdownValues(tester, planKey), expectedValues);
      expect(_dropdownValues(tester, implementKey), expectedValues);
    },
  );

  testWidgets('Anthropic reasoning effort dropdowns expose expected options', (
    tester,
  ) async {
    final engine = _FakeSettingsEngine();

    await _pumpSettingsScreen(tester, engine);

    final researchKey = AppKeys.settingsReasoningResearchDropdown('anthropic');
    final planKey = AppKeys.settingsReasoningPlanDropdown('anthropic');
    final implementKey = AppKeys.settingsReasoningImplementDropdown(
      'anthropic',
    );

    expect(find.byKey(researchKey), findsOneWidget);
    expect(find.byKey(planKey), findsOneWidget);
    expect(find.byKey(implementKey), findsOneWidget);

    const expectedLabels = ['Low', 'Medium', 'High', 'Max (Opus only)'];
    const expectedValues = ['low', 'medium', 'high', 'max'];

    expect(_dropdownLabels(tester, researchKey), expectedLabels);
    expect(_dropdownLabels(tester, planKey), expectedLabels);
    expect(_dropdownLabels(tester, implementKey), expectedLabels);

    expect(_dropdownValues(tester, researchKey), expectedValues);
    expect(_dropdownValues(tester, planKey), expectedValues);
    expect(_dropdownValues(tester, implementKey), expectedValues);
  });

  testWidgets('OpenAI reasoning effort change sends all phase literals', (
    tester,
  ) async {
    final engine = _FakeSettingsEngine(
      openAIResearchEffort: 'low',
      openAIPlanEffort: 'medium',
      openAIImplementEffort: 'high',
    );

    await _pumpSettingsScreen(tester, engine);

    await _selectDropdownOption(
      tester,
      AppKeys.settingsReasoningResearchDropdown('openai'),
      'None',
    );

    expect(engine.callCount('ProvidersSetReasoningEffort'), 1);
    final payload = engine.lastParams('ProvidersSetReasoningEffort');
    expect(payload?['provider_id'], 'openai');
    expect(payload?['research_effort'], 'none');
    expect(payload?['plan_effort'], 'medium');
    expect(payload?['implement_effort'], 'high');
  });

  testWidgets('OpenAI Codex reasoning effort change sends all phase literals', (
    tester,
  ) async {
    final engine = _FakeSettingsEngine(
      openAICodexResearchEffort: 'low',
      openAICodexPlanEffort: 'medium',
      openAICodexImplementEffort: 'high',
    );

    await _pumpSettingsScreen(tester, engine);

    await _selectDropdownOption(
      tester,
      AppKeys.settingsReasoningPlanDropdown('openai-codex'),
      'Extra high',
    );

    expect(engine.callCount('ProvidersSetReasoningEffort'), 1);
    final payload = engine.lastParams('ProvidersSetReasoningEffort');
    expect(payload?['provider_id'], 'openai-codex');
    expect(payload?['research_effort'], 'low');
    expect(payload?['plan_effort'], 'xhigh');
    expect(payload?['implement_effort'], 'high');
  });

  testWidgets('Anthropic reasoning effort change sends all phase literals', (
    tester,
  ) async {
    final engine = _FakeSettingsEngine(
      anthropicResearchEffort: 'low',
      anthropicPlanEffort: 'medium',
      anthropicImplementEffort: 'high',
    );

    await _pumpSettingsScreen(tester, engine);

    await _selectDropdownOption(
      tester,
      AppKeys.settingsReasoningImplementDropdown('anthropic'),
      'Max (Opus only)',
    );

    expect(engine.callCount('ProvidersSetReasoningEffort'), 1);
    final payload = engine.lastParams('ProvidersSetReasoningEffort');
    expect(payload?['provider_id'], 'anthropic');
    expect(payload?['research_effort'], 'low');
    expect(payload?['plan_effort'], 'medium');
    expect(payload?['implement_effort'], 'max');
  });

  testWidgets(
    'consent mode toggle enables allow_all after explicit confirmation',
    (tester) async {
      final engine = _FakeSettingsEngine();
      await _pumpSettingsScreen(tester, engine);

      final toggleFinder = find.byKey(AppKeys.settingsConsentModeToggle);
      expect(toggleFinder, findsOneWidget);
      expect(tester.widget<SwitchListTile>(toggleFinder).value, isFalse);

      await tester.tap(toggleFinder);
      await tester.pumpAndSettle();

      expect(find.byKey(AppKeys.settingsConsentAllowAllDialog), findsOneWidget);
      await tester.tap(find.byKey(AppKeys.settingsConsentAllowAllConfirm));
      await tester.pumpAndSettle();

      expect(engine.callCount('UserSetConsentMode'), 1);
      expect(engine.lastParams('UserSetConsentMode')?['mode'], 'allow_all');
      expect(engine.lastParams('UserSetConsentMode')?['approved'], true);
      expect(tester.widget<SwitchListTile>(toggleFinder).value, isTrue);
    },
  );

  testWidgets('consent mode enable can be canceled', (tester) async {
    final engine = _FakeSettingsEngine();
    await _pumpSettingsScreen(tester, engine);

    final toggleFinder = find.byKey(AppKeys.settingsConsentModeToggle);
    await tester.tap(toggleFinder);
    await tester.pumpAndSettle();

    expect(find.byKey(AppKeys.settingsConsentAllowAllDialog), findsOneWidget);
    await tester.tap(find.byKey(AppKeys.settingsConsentAllowAllCancel));
    await tester.pumpAndSettle();

    expect(engine.callCount('UserSetConsentMode'), 0);
    expect(tester.widget<SwitchListTile>(toggleFinder).value, isFalse);
  });

  testWidgets('consent mode toggle disables allow_all directly', (
    tester,
  ) async {
    final engine = _FakeSettingsEngine()..userConsentMode = consentModeAllowAll;
    await _pumpSettingsScreen(tester, engine);

    final toggleFinder = find.byKey(AppKeys.settingsConsentModeToggle);
    expect(tester.widget<SwitchListTile>(toggleFinder).value, isTrue);

    await tester.tap(toggleFinder);
    await tester.pumpAndSettle();

    expect(engine.callCount('UserSetConsentMode'), 1);
    expect(engine.lastParams('UserSetConsentMode')?['mode'], 'ask');
    expect(tester.widget<SwitchListTile>(toggleFinder).value, isFalse);
  });

  testWidgets('OpenAI API key controls still render and save', (tester) async {
    final engine = _FakeSettingsEngine();

    await _pumpSettingsScreen(tester, engine);

    expect(find.byKey(AppKeys.settingsApiKeyField), findsOneWidget);
    expect(find.byKey(AppKeys.settingsSaveButton), findsOneWidget);

    await tester.enterText(find.byKey(AppKeys.settingsApiKeyField), 'sk-test');
    await tester.tap(find.byKey(AppKeys.settingsSaveButton));
    await tester.pumpAndSettle();

    expect(engine.callCount('ProvidersSetApiKey'), 1);
    expect(engine.lastParams('ProvidersSetApiKey')?['provider_id'], 'openai');
    expect(engine.lastParams('ProvidersSetApiKey')?['api_key'], 'sk-test');
    expect(engine.callCount('ProvidersValidate'), 1);
    expect(engine.lastParams('ProvidersValidate')?['provider_id'], 'openai');
  });

  testWidgets('Mistral API key controls render and save', (tester) async {
    final engine = _FakeSettingsEngine();

    await _pumpSettingsScreen(tester, engine);

    expect(find.text('Mistral'), findsOneWidget);

    final mistralField = find.byWidgetPredicate(
      (widget) =>
          widget is TextField &&
          widget.decoration?.labelText == 'Mistral API Key',
    );
    expect(mistralField, findsOneWidget);

    await tester.enterText(mistralField, 'mistral-test-key');
    final mistralSaveButton = find.text('Save & Validate').last;
    await tester.ensureVisible(mistralSaveButton);
    await tester.tap(mistralSaveButton);
    await tester.pumpAndSettle();

    expect(engine.callCount('ProvidersSetApiKey'), 1);
    expect(engine.lastParams('ProvidersSetApiKey')?['provider_id'], 'mistral');
    expect(
      engine.lastParams('ProvidersSetApiKey')?['api_key'],
      'mistral-test-key',
    );
    expect(engine.callCount('ProvidersValidate'), 1);
    expect(engine.lastParams('ProvidersValidate')?['provider_id'], 'mistral');
  });
}
