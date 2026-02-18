import 'dart:io';

import 'package:flutter/material.dart';
import 'package:provider/provider.dart';

import '../app_keys.dart';
import '../engine/engine_client.dart';
import '../logging.dart';
import '../models/models.dart';
import '../theme.dart';
import '../widgets/centered_content.dart';
import '../widgets/keenbench_app_bar.dart';

typedef ExternalUrlLauncher = Future<void> Function(String url);

class _ReasoningEffortOption {
  const _ReasoningEffortOption({required this.label, required this.value});

  final String label;
  final String value;
}

const List<_ReasoningEffortOption> _openAIReasoningEffortOptions = [
  _ReasoningEffortOption(label: 'None', value: 'none'),
  _ReasoningEffortOption(label: 'Low', value: 'low'),
  _ReasoningEffortOption(label: 'Medium', value: 'medium'),
  _ReasoningEffortOption(label: 'High', value: 'high'),
];

const List<_ReasoningEffortOption> _openAICodexReasoningEffortOptions = [
  _ReasoningEffortOption(label: 'Low', value: 'low'),
  _ReasoningEffortOption(label: 'Medium', value: 'medium'),
  _ReasoningEffortOption(label: 'High', value: 'high'),
  _ReasoningEffortOption(label: 'Extra high', value: 'xhigh'),
];

class SettingsScreen extends StatefulWidget {
  const SettingsScreen({super.key, this.urlLauncher});

  final ExternalUrlLauncher? urlLauncher;

  @override
  State<SettingsScreen> createState() => _SettingsScreenState();
}

class _SettingsScreenState extends State<SettingsScreen> {
  bool _loading = true;
  List<ProviderStatus> _providers = [];
  List<ModelInfo> _models = [];
  String? _userDefaultModelId;
  final Map<String, TextEditingController> _controllers = {};

  @override
  void initState() {
    super.initState();
    _load();
  }

  @override
  void dispose() {
    for (final controller in _controllers.values) {
      controller.dispose();
    }
    super.dispose();
  }

  Future<void> _load() async {
    final engine = context.read<EngineApi>();
    final providersResponse = await engine.call('ProvidersGetStatus');
    final providerList =
        (providersResponse['providers'] as List<dynamic>? ?? [])
            .cast<Map<String, dynamic>>();
    final modelsResponse = await engine.call('ModelsListSupported');
    final modelList = (modelsResponse['models'] as List<dynamic>? ?? [])
        .cast<Map<String, dynamic>>();
    final userDefault = await engine.call('UserGetDefaultModel');
    if (!mounted) {
      return;
    }
    setState(() {
      _providers = providerList.map(ProviderStatus.fromJson).toList();
      _models = modelList.map(ModelInfo.fromJson).toList();
      _userDefaultModelId = userDefault['model_id'] as String?;
      for (final provider in _providers) {
        if (provider.authMode == 'api_key') {
          _controllers.putIfAbsent(provider.id, () => TextEditingController());
        }
      }
      _loading = false;
    });
    AppLog.debug('settings.loaded', {
      'providers': _providers.length,
      'models': _models.length,
    });
  }

  Future<void> _saveKey(ProviderStatus provider) async {
    final engine = context.read<EngineApi>();
    final controller = _controllers[provider.id];
    final key = controller?.text.trim() ?? '';
    if (key.isEmpty) {
      ScaffoldMessenger.of(
        context,
      ).showSnackBar(const SnackBar(content: Text('Enter an API key.')));
      return;
    }
    try {
      AppLog.info('settings.save_api_key', {'provider_id': provider.id});
      await engine.call('ProvidersSetApiKey', {
        'provider_id': provider.id,
        'api_key': key,
      });
      await engine.call('ProvidersValidate', {'provider_id': provider.id});
      await _load();
      if (!mounted) {
        return;
      }
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(content: Text('${provider.displayName} key saved.')),
      );
    } on EngineError catch (err) {
      AppLog.warn('settings.save_api_key_failed', {
        'provider_id': provider.id,
        'error_code': err.errorCode,
        'message': err.message,
      });
      if (!mounted) {
        return;
      }
      ScaffoldMessenger.of(
        context,
      ).showSnackBar(SnackBar(content: Text(err.message)));
    }
  }

  Future<void> _toggleEnabled(ProviderStatus provider, bool value) async {
    final engine = context.read<EngineApi>();
    await engine.call('ProvidersSetEnabled', {
      'provider_id': provider.id,
      'enabled': value,
    });
    await _load();
  }

  bool _supportsReasoningEffort(ProviderStatus provider) =>
      provider.id == 'openai' || provider.id == 'openai-codex';

  List<_ReasoningEffortOption> _reasoningEffortOptionsForProvider(
    String providerId,
  ) {
    switch (providerId) {
      case 'openai':
        return _openAIReasoningEffortOptions;
      case 'openai-codex':
        return _openAICodexReasoningEffortOptions;
      default:
        return const [];
    }
  }

  String _resolvedReasoningEffort(
    String? value,
    List<_ReasoningEffortOption> options,
  ) {
    final normalized = value?.trim().toLowerCase() ?? '';
    for (final option in options) {
      if (option.value == normalized) {
        return option.value;
      }
    }
    for (final option in options) {
      if (option.value == 'medium') {
        return option.value;
      }
    }
    return options.isNotEmpty ? options.first.value : 'medium';
  }

  Future<void> _setReasoningEffort(
    ProviderStatus provider, {
    required String researchEffort,
    required String planEffort,
    required String implementEffort,
  }) async {
    final engine = context.read<EngineApi>();
    try {
      await engine.call('ProvidersSetReasoningEffort', {
        'provider_id': provider.id,
        'research_effort': researchEffort,
        'plan_effort': planEffort,
        'implement_effort': implementEffort,
      });
      await _load();
    } on EngineError catch (err) {
      AppLog.warn('settings.set_reasoning_effort_failed', {
        'provider_id': provider.id,
        'error_code': err.errorCode,
        'message': err.message,
      });
      if (!mounted) {
        return;
      }
      ScaffoldMessenger.of(
        context,
      ).showSnackBar(SnackBar(content: Text(err.message)));
    }
  }

  Future<void> _setDefaultModel(String? value) async {
    if (value == null || value.isEmpty) {
      return;
    }
    final engine = context.read<EngineApi>();
    await engine.call('UserSetDefaultModel', {'model_id': value});
    setState(() {
      _userDefaultModelId = value;
    });
  }

  List<ModelInfo> _availableModels() {
    final enabledProviders = <String, ProviderStatus>{
      for (final provider in _providers) provider.id: provider,
    };
    return _models.where((model) {
      final provider = enabledProviders[model.providerId];
      if (provider == null) {
        return false;
      }
      return provider.enabled && provider.configured;
    }).toList();
  }

  String? _oauthStatusHint(ProviderStatus provider) {
    if (provider.oauthConnected != true) {
      return null;
    }
    if (provider.oauthExpired == true) {
      return 'Session expired. Reconnect to continue.';
    }
    final expiresAt = provider.oauthExpiresAt?.trim() ?? '';
    if (expiresAt.isNotEmpty) {
      return 'Expires at $expiresAt';
    }
    return null;
  }

  Future<void> _openAuthorizeUrl(String authorizeUrl) async {
    final url = authorizeUrl.trim();
    if (url.isEmpty) {
      return;
    }
    final launcher = widget.urlLauncher ?? _defaultUrlLauncher;
    try {
      await launcher(url);
    } catch (err) {
      AppLog.warn('settings.oauth_open_url_failed', {
        'url': url,
        'error': err.toString(),
      });
      if (!mounted) {
        return;
      }
      ScaffoldMessenger.of(context).showSnackBar(
        const SnackBar(
          content: Text(
            'Could not open browser automatically. Use the URL in the dialog.',
          ),
        ),
      );
    }
  }

  Future<void> _defaultUrlLauncher(String url) async {
    if (Platform.isMacOS) {
      await Process.start('open', [url], mode: ProcessStartMode.detached);
      return;
    }
    if (Platform.isWindows) {
      await Process.start('cmd', [
        '/c',
        'start',
        '',
        url,
      ], mode: ProcessStartMode.detached);
      return;
    }
    await Process.start('xdg-open', [url], mode: ProcessStartMode.detached);
  }

  Future<String?> _showOAuthCompleteDialog(
    ProviderStatus provider,
    String authorizeUrl,
  ) async {
    final controller = TextEditingController();
    final redirectUrl = await showDialog<String>(
      context: context,
      builder: (dialogContext) => AlertDialog(
        title: Text('Connect ${provider.displayName}'),
        content: SizedBox(
          width: 520,
          child: SingleChildScrollView(
            child: Column(
              mainAxisSize: MainAxisSize.min,
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                const Text(
                  'Authorize in your browser, then paste the full redirect URL.',
                ),
                if (authorizeUrl.trim().isNotEmpty) ...[
                  const SizedBox(height: 8),
                  Text(
                    authorizeUrl,
                    maxLines: 4,
                    overflow: TextOverflow.ellipsis,
                    style: Theme.of(context).textTheme.bodySmall?.copyWith(
                      color: KeenBenchTheme.colorTextSecondary,
                    ),
                  ),
                ],
                const SizedBox(height: 12),
                TextField(
                  key: AppKeys.settingsOAuthRedirectField(provider.id),
                  controller: controller,
                  decoration: const InputDecoration(labelText: 'Redirect URL'),
                ),
              ],
            ),
          ),
        ),
        actions: [
          TextButton(
            onPressed: () => Navigator.of(dialogContext).pop(),
            child: const Text('Cancel'),
          ),
          ElevatedButton(
            key: AppKeys.settingsOAuthCompleteButton(provider.id),
            onPressed: () =>
                Navigator.of(dialogContext).pop(controller.text.trim()),
            child: const Text('Complete'),
          ),
        ],
      ),
    );
    return redirectUrl;
  }

  Future<void> _connectOAuth(ProviderStatus provider) async {
    final engine = context.read<EngineApi>();
    try {
      AppLog.info('settings.oauth_start', {'provider_id': provider.id});
      final startRaw = await engine.call('ProvidersOAuthStart', {
        'provider_id': provider.id,
      });
      final start = Map<String, dynamic>.from(startRaw as Map);
      final flowId = (start['flow_id'] as String? ?? '').trim();
      final authorizeUrl = (start['authorize_url'] as String? ?? '').trim();
      if (flowId.isEmpty) {
        if (!mounted) {
          return;
        }
        ScaffoldMessenger.of(context).showSnackBar(
          const SnackBar(content: Text('OAuth start failed. Missing flow ID.')),
        );
        return;
      }
      await _openAuthorizeUrl(authorizeUrl);
      if (!mounted) {
        return;
      }
      final redirectUrl = await _showOAuthCompleteDialog(
        provider,
        authorizeUrl,
      );
      if (!mounted || redirectUrl == null) {
        return;
      }
      AppLog.info('settings.oauth_complete', {'provider_id': provider.id});
      await engine.call('ProvidersOAuthComplete', {
        'provider_id': provider.id,
        'flow_id': flowId,
        'redirect_url': redirectUrl.trim(),
      });
      await _load();
      if (!mounted) {
        return;
      }
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(content: Text('${provider.displayName} connected.')),
      );
    } on EngineError catch (err) {
      AppLog.warn('settings.oauth_connect_failed', {
        'provider_id': provider.id,
        'error_code': err.errorCode,
        'message': err.message,
      });
      if (!mounted) {
        return;
      }
      ScaffoldMessenger.of(
        context,
      ).showSnackBar(SnackBar(content: Text(err.message)));
    }
  }

  Future<void> _disconnectOAuth(ProviderStatus provider) async {
    final engine = context.read<EngineApi>();
    try {
      AppLog.info('settings.oauth_disconnect', {'provider_id': provider.id});
      await engine.call('ProvidersOAuthDisconnect', {
        'provider_id': provider.id,
      });
      await _load();
      if (!mounted) {
        return;
      }
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(content: Text('${provider.displayName} disconnected.')),
      );
    } on EngineError catch (err) {
      AppLog.warn('settings.oauth_disconnect_failed', {
        'provider_id': provider.id,
        'error_code': err.errorCode,
        'message': err.message,
      });
      if (!mounted) {
        return;
      }
      ScaffoldMessenger.of(
        context,
      ).showSnackBar(SnackBar(content: Text(err.message)));
    }
  }

  Widget _buildApiKeyControls(ProviderStatus provider) {
    final controller = _controllers.putIfAbsent(
      provider.id,
      () => TextEditingController(),
    );
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        TextField(
          key: provider.id == 'openai' ? AppKeys.settingsApiKeyField : null,
          controller: controller,
          obscureText: true,
          decoration: InputDecoration(
            labelText: '${provider.displayName} API Key',
          ),
        ),
        const SizedBox(height: 12),
        ElevatedButton(
          key: provider.id == 'openai' ? AppKeys.settingsSaveButton : null,
          onPressed: () => _saveKey(provider),
          child: const Text('Save & Validate'),
        ),
        const SizedBox(height: 8),
        Text(
          'Keys are stored locally and encrypted at rest by the engine.',
          style: Theme.of(context).textTheme.bodySmall?.copyWith(
            color: KeenBenchTheme.colorTextSecondary,
          ),
        ),
      ],
    );
  }

  Widget _buildOAuthControls(ProviderStatus provider) {
    final connected = provider.oauthConnected == true;
    if (!connected) {
      return ElevatedButton(
        key: AppKeys.settingsOAuthConnectButton(provider.id),
        onPressed: () => _connectOAuth(provider),
        child: const Text('Connect'),
      );
    }
    return OutlinedButton(
      key: AppKeys.settingsOAuthDisconnectButton(provider.id),
      onPressed: () => _disconnectOAuth(provider),
      child: const Text('Disconnect'),
    );
  }

  Widget _buildReasoningEffortDropdown({
    required String label,
    required Key dropdownKey,
    required String value,
    required List<_ReasoningEffortOption> options,
    required ValueChanged<String?> onChanged,
  }) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Text(
          label,
          style: Theme.of(context).textTheme.bodySmall?.copyWith(
            color: KeenBenchTheme.colorTextSecondary,
          ),
        ),
        const SizedBox(height: 4),
        DropdownButton<String>(
          key: dropdownKey,
          value: value,
          items: options
              .map(
                (option) => DropdownMenuItem<String>(
                  value: option.value,
                  child: Text(option.label),
                ),
              )
              .toList(),
          onChanged: onChanged,
        ),
      ],
    );
  }

  Widget _buildReasoningEffortControls(ProviderStatus provider) {
    final options = _reasoningEffortOptionsForProvider(provider.id);
    if (options.isEmpty) {
      return const SizedBox.shrink();
    }
    final current = provider.rpiReasoning;
    final researchEffort = _resolvedReasoningEffort(
      current?.researchEffort,
      options,
    );
    final planEffort = _resolvedReasoningEffort(current?.planEffort, options);
    final implementEffort = _resolvedReasoningEffort(
      current?.implementEffort,
      options,
    );

    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Text('Reasoning effort', style: Theme.of(context).textTheme.bodyMedium),
        const SizedBox(height: 8),
        _buildReasoningEffortDropdown(
          label: 'Research reasoning effort',
          dropdownKey: AppKeys.settingsReasoningResearchDropdown(provider.id),
          value: researchEffort,
          options: options,
          onChanged: (value) {
            if (value == null) {
              return;
            }
            _setReasoningEffort(
              provider,
              researchEffort: value,
              planEffort: planEffort,
              implementEffort: implementEffort,
            );
          },
        ),
        const SizedBox(height: 8),
        _buildReasoningEffortDropdown(
          label: 'Plan reasoning effort',
          dropdownKey: AppKeys.settingsReasoningPlanDropdown(provider.id),
          value: planEffort,
          options: options,
          onChanged: (value) {
            if (value == null) {
              return;
            }
            _setReasoningEffort(
              provider,
              researchEffort: researchEffort,
              planEffort: value,
              implementEffort: implementEffort,
            );
          },
        ),
        const SizedBox(height: 8),
        _buildReasoningEffortDropdown(
          label: 'Implement reasoning effort',
          dropdownKey: AppKeys.settingsReasoningImplementDropdown(provider.id),
          value: implementEffort,
          options: options,
          onChanged: (value) {
            if (value == null) {
              return;
            }
            _setReasoningEffort(
              provider,
              researchEffort: researchEffort,
              planEffort: planEffort,
              implementEffort: value,
            );
          },
        ),
      ],
    );
  }

  Widget _buildProviderCard(ProviderStatus provider) {
    final configuredText = provider.configured
        ? 'Configured'
        : 'Not configured';
    final isOAuthProvider = provider.authMode == 'oauth';
    final oauthLabel = provider.oauthAccountLabel?.trim() ?? '';
    final oauthConnected = provider.oauthConnected == true;
    final oauthStatusText = oauthConnected
        ? (oauthLabel.isEmpty ? 'Connected' : 'Connected as $oauthLabel')
        : 'Not connected';
    final oauthHint = _oauthStatusHint(provider);

    return Container(
      padding: const EdgeInsets.all(16),
      margin: const EdgeInsets.only(bottom: 16),
      decoration: BoxDecoration(
        color: KeenBenchTheme.colorBackgroundElevated,
        borderRadius: BorderRadius.circular(8),
        border: Border.all(color: KeenBenchTheme.colorBorderSubtle),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            children: [
              Expanded(
                child: Text(
                  provider.displayName,
                  style: Theme.of(context).textTheme.headlineSmall,
                ),
              ),
              Switch(
                key: provider.id == 'openai'
                    ? AppKeys.settingsProviderToggle
                    : null,
                value: provider.enabled,
                onChanged: (value) => _toggleEnabled(provider, value),
              ),
            ],
          ),
          if (isOAuthProvider) ...[
            Text(
              oauthStatusText,
              key: AppKeys.settingsOAuthStatusText(provider.id),
              style: Theme.of(context).textTheme.bodySmall?.copyWith(
                color: oauthConnected
                    ? KeenBenchTheme.colorSuccessText
                    : KeenBenchTheme.colorWarningText,
              ),
            ),
            if (oauthHint != null) ...[
              const SizedBox(height: 4),
              Text(
                oauthHint,
                style: Theme.of(context).textTheme.bodySmall?.copyWith(
                  color: provider.oauthExpired == true
                      ? KeenBenchTheme.colorWarningText
                      : KeenBenchTheme.colorTextSecondary,
                ),
              ),
            ],
          ] else
            Text(
              configuredText,
              key: provider.id == 'openai'
                  ? AppKeys.settingsProviderStatus
                  : null,
              style: Theme.of(context).textTheme.bodySmall?.copyWith(
                color: provider.configured
                    ? KeenBenchTheme.colorSuccessText
                    : KeenBenchTheme.colorWarningText,
              ),
            ),
          const SizedBox(height: 12),
          isOAuthProvider
              ? _buildOAuthControls(provider)
              : _buildApiKeyControls(provider),
          if (_supportsReasoningEffort(provider)) ...[
            const SizedBox(height: 16),
            _buildReasoningEffortControls(provider),
          ],
        ],
      ),
    );
  }

  @override
  Widget build(BuildContext context) {
    final availableModels = _availableModels();
    return Scaffold(
      key: AppKeys.settingsScreen,
      appBar: const KeenBenchAppBar(title: 'Settings', showBack: true),
      body: _loading
          ? const Center(child: CircularProgressIndicator())
          : CenteredContent(
              child: SingleChildScrollView(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(
                      'Default Model',
                      style: Theme.of(context).textTheme.headlineSmall,
                    ),
                    const SizedBox(height: 8),
                    if (availableModels.isEmpty)
                      Text(
                        'Configure a provider to select a default model.',
                        style: Theme.of(context).textTheme.bodySmall?.copyWith(
                          color: KeenBenchTheme.colorTextSecondary,
                        ),
                      )
                    else
                      DropdownButton<String>(
                        value:
                            availableModels.any(
                              (m) => m.id == _userDefaultModelId,
                            )
                            ? _userDefaultModelId
                            : availableModels.first.id,
                        items: availableModels
                            .map(
                              (model) => DropdownMenuItem<String>(
                                value: model.id,
                                child: Text(model.displayName),
                              ),
                            )
                            .toList(),
                        onChanged: _setDefaultModel,
                      ),
                    const SizedBox(height: 24),
                    Text(
                      'Model Providers',
                      style: Theme.of(context).textTheme.headlineSmall,
                    ),
                    const SizedBox(height: 12),
                    ..._providers.map(_buildProviderCard),
                  ],
                ),
              ),
            ),
    );
  }
}
