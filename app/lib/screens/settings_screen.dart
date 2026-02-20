import 'dart:async';
import 'dart:io';

import 'package:flutter/material.dart';
import 'package:provider/provider.dart';

import '../app_keys.dart';
import '../egress_consent.dart';
import '../engine/engine_client.dart';
import '../logging.dart';
import '../models/models.dart';
import '../theme.dart';
import '../widgets/centered_content.dart';
import '../widgets/dialog_keyboard_shortcuts.dart';
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

const List<_ReasoningEffortOption> _anthropicReasoningEffortOptions = [
  _ReasoningEffortOption(label: 'Low', value: 'low'),
  _ReasoningEffortOption(label: 'Medium', value: 'medium'),
  _ReasoningEffortOption(label: 'High', value: 'high'),
  _ReasoningEffortOption(label: 'Max (Opus only)', value: 'max'),
];

const Duration _oauthStatusPollIntervalDefault = Duration(seconds: 1);
const Duration _oauthStatusWaitTimeoutDefault = Duration(minutes: 2);
const _oauthFlowStatusCodeReceived = 'code_received';
const _oauthFlowStatusCompleted = 'completed';
const _oauthFlowStatusFailed = 'failed';
const _oauthFlowStatusExpired = 'expired';

class _OAuthWaitResult {
  const _OAuthWaitResult._({
    required this.codeCaptured,
    required this.cancelled,
    required this.timedOut,
    this.error,
  });

  const _OAuthWaitResult.captured()
    : this._(codeCaptured: true, cancelled: false, timedOut: false);

  const _OAuthWaitResult.cancelled()
    : this._(codeCaptured: false, cancelled: true, timedOut: false);

  const _OAuthWaitResult.timedOut()
    : this._(codeCaptured: false, cancelled: false, timedOut: true);

  const _OAuthWaitResult.failed(String error)
    : this._(
        codeCaptured: false,
        cancelled: false,
        timedOut: false,
        error: error,
      );

  final bool codeCaptured;
  final bool cancelled;
  final bool timedOut;
  final String? error;
}

class SettingsScreen extends StatefulWidget {
  const SettingsScreen({
    super.key,
    this.urlLauncher,
    this.oauthStatusPollInterval = _oauthStatusPollIntervalDefault,
    this.oauthStatusWaitTimeout = _oauthStatusWaitTimeoutDefault,
  });

  final ExternalUrlLauncher? urlLauncher;
  final Duration oauthStatusPollInterval;
  final Duration oauthStatusWaitTimeout;

  @override
  State<SettingsScreen> createState() => _SettingsScreenState();
}

class _SettingsScreenState extends State<SettingsScreen> {
  bool _loading = true;
  bool _updatingConsentMode = false;
  List<ProviderStatus> _providers = [];
  List<ModelInfo> _models = [];
  String? _userDefaultModelId;
  String _userConsentMode = consentModeAsk;
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
    if (!mounted) {
      return;
    }
    final engine = context.read<EngineApi>();
    final providersResponse = await engine.call('ProvidersGetStatus');
    final providerList =
        (providersResponse['providers'] as List<dynamic>? ?? [])
            .cast<Map<String, dynamic>>();
    final modelsResponse = await engine.call('ModelsListSupported');
    final modelList = (modelsResponse['models'] as List<dynamic>? ?? [])
        .cast<Map<String, dynamic>>();
    final userDefault = await engine.call('UserGetDefaultModel');
    final consentModeResponse = await engine.call('UserGetConsentMode');
    final consentMode =
        (consentModeResponse['mode'] as String? ?? consentModeAsk).trim();
    if (!mounted) {
      return;
    }
    setState(() {
      _providers = providerList.map(ProviderStatus.fromJson).toList();
      _models = modelList.map(ModelInfo.fromJson).toList();
      _userDefaultModelId = userDefault['model_id'] as String?;
      _userConsentMode = consentMode == consentModeAllowAll
          ? consentModeAllowAll
          : consentModeAsk;
      for (final provider in _providers) {
        if (provider.authMode == 'api_key' ||
            provider.authMode == 'setup_token') {
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
    final credentialLabel = provider.authMode == 'setup_token'
        ? 'setup token'
        : 'API key';
    final savedLabel = provider.authMode == 'setup_token' ? 'token' : 'key';
    if (key.isEmpty) {
      ScaffoldMessenger.of(
        context,
      ).showSnackBar(SnackBar(content: Text('Enter a $credentialLabel.')));
      return;
    }
    try {
      AppLog.info('settings.save_api_key', {'provider_id': provider.id});
      await engine.call('ProvidersSetApiKey', {
        'provider_id': provider.id,
        'api_key': key,
      });
      await engine.call('ProvidersValidate', {'provider_id': provider.id});
      if (!mounted) {
        return;
      }
      await _load();
      if (!mounted) {
        return;
      }
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(content: Text('${provider.displayName} $savedLabel saved.')),
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

  Future<void> _clearCredential(ProviderStatus provider) async {
    if (provider.authMode == 'oauth') {
      await _disconnectOAuth(provider);
      return;
    }

    final engine = context.read<EngineApi>();
    final clearedLabel = provider.authMode == 'setup_token' ? 'token' : 'key';
    try {
      AppLog.info('settings.clear_api_key', {'provider_id': provider.id});
      await engine.call('ProvidersClearApiKey', {'provider_id': provider.id});
      _controllers[provider.id]?.clear();
      await _load();
      if (!mounted) {
        return;
      }
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(
          content: Text('${provider.displayName} $clearedLabel cleared.'),
        ),
      );
    } on EngineError catch (err) {
      AppLog.warn('settings.clear_api_key_failed', {
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
      provider.id == 'openai' ||
      provider.id == 'openai-codex' ||
      provider.id == 'anthropic' ||
      provider.id == 'anthropic-claude';

  List<_ReasoningEffortOption> _reasoningEffortOptionsForProvider(
    String providerId,
  ) {
    switch (providerId) {
      case 'openai':
        return _openAIReasoningEffortOptions;
      case 'openai-codex':
        return _openAICodexReasoningEffortOptions;
      case 'anthropic':
      case 'anthropic-claude':
        return _anthropicReasoningEffortOptions;
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

  Future<bool> _confirmEnableGlobalConsentMode() async {
    final confirmed = await showDialog<bool>(
      context: context,
      builder: (dialogContext) {
        void cancel() => Navigator.of(dialogContext).pop(false);

        void submit() => Navigator.of(dialogContext).pop(true);

        return DialogKeyboardShortcuts(
          onCancel: cancel,
          onSubmit: submit,
          child: AlertDialog(
            key: AppKeys.settingsConsentAllowAllDialog,
            title: const Text('Enable consent-all mode?'),
            content: const Text(
              'When enabled, KeenBench will not prompt for egress consent before Workshop or Context model actions across all Workbenches. You can disable this any time in Settings.',
            ),
            actions: [
              OutlinedButton(
                key: AppKeys.settingsConsentAllowAllCancel,
                onPressed: cancel,
                child: const Text('Cancel'),
              ),
              ElevatedButton(
                key: AppKeys.settingsConsentAllowAllConfirm,
                onPressed: submit,
                child: const Text('Enable'),
              ),
            ],
          ),
        );
      },
    );
    return confirmed == true;
  }

  Future<void> _setConsentModeAllowAll(bool enabled) async {
    if (_updatingConsentMode) {
      return;
    }
    final engine = context.read<EngineApi>();
    setState(() {
      _updatingConsentMode = true;
    });
    try {
      if (enabled) {
        final confirmed = await _confirmEnableGlobalConsentMode();
        if (!confirmed) {
          return;
        }
        await engine.call('UserSetConsentMode', {
          'mode': consentModeAllowAll,
          'approved': true,
        });
        if (!mounted) {
          return;
        }
        setState(() {
          _userConsentMode = consentModeAllowAll;
        });
        return;
      }
      await engine.call('UserSetConsentMode', {'mode': consentModeAsk});
      if (!mounted) {
        return;
      }
      setState(() {
        _userConsentMode = consentModeAsk;
      });
    } on EngineError catch (err) {
      AppLog.warn('settings.set_consent_mode_failed', {
        'error_code': err.errorCode,
        'message': err.message,
      });
      if (!mounted) {
        return;
      }
      ScaffoldMessenger.of(
        context,
      ).showSnackBar(SnackBar(content: Text(err.message)));
    } finally {
      if (mounted) {
        setState(() {
          _updatingConsentMode = false;
        });
      } else {
        _updatingConsentMode = false;
      }
    }
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
    String authorizeUrl, {
    String? statusMessage,
  }) async {
    final message = statusMessage?.trim() ?? '';
    final controller = TextEditingController();
    final redirectUrl = await showDialog<String>(
      context: context,
      builder: (dialogContext) {
        void cancel() => Navigator.of(dialogContext).pop();

        void submit() =>
            Navigator.of(dialogContext).pop(controller.text.trim());

        return DialogKeyboardShortcuts(
          onCancel: cancel,
          onSubmit: submit,
          child: AlertDialog(
            title: Text('Connect ${provider.displayName}'),
            content: SizedBox(
              width: 520,
              child: SingleChildScrollView(
                child: Column(
                  mainAxisSize: MainAxisSize.min,
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(
                      message.isEmpty
                          ? 'Automatic callback capture is unavailable. '
                                'Authorize in your browser, then paste the full redirect URL.'
                          : message,
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
                      textInputAction: TextInputAction.done,
                      onSubmitted: (_) => submit(),
                      decoration: const InputDecoration(
                        labelText: 'Redirect URL',
                      ),
                    ),
                  ],
                ),
              ),
            ),
            actions: [
              TextButton(onPressed: cancel, child: const Text('Cancel')),
              ElevatedButton(
                key: AppKeys.settingsOAuthCompleteButton(provider.id),
                onPressed: submit,
                child: const Text('Complete'),
              ),
            ],
          ),
        );
      },
    );
    return redirectUrl;
  }

  Future<_OAuthWaitResult> _showOAuthProgressDialog(
    ProviderStatus provider,
    String flowId,
  ) async {
    final engine = context.read<EngineApi>();
    final stopPolling = Completer<void>();
    final waitResult = Completer<_OAuthWaitResult>();
    BuildContext? dialogContext;
    var pollingStarted = false;

    void resolve(_OAuthWaitResult result) {
      if (!waitResult.isCompleted) {
        waitResult.complete(result);
      }
      if (!stopPolling.isCompleted) {
        stopPolling.complete();
      }
      if (dialogContext != null && dialogContext!.mounted) {
        Navigator.of(dialogContext!).pop(result);
      }
    }

    Future<void> pollStatus() async {
      final deadline = DateTime.now().add(widget.oauthStatusWaitTimeout);
      while (!stopPolling.isCompleted && DateTime.now().isBefore(deadline)) {
        Map<String, dynamic> status;
        try {
          final statusRaw = await engine.call('ProvidersOAuthStatus', {
            'provider_id': provider.id,
            'flow_id': flowId,
          });
          status = Map<String, dynamic>.from(statusRaw as Map);
        } on EngineError catch (err) {
          resolve(_OAuthWaitResult.failed(err.message));
          return;
        } catch (_) {
          resolve(
            _OAuthWaitResult.failed('Could not read OAuth callback status.'),
          );
          return;
        }

        final flowStatus = (status['status'] as String? ?? '')
            .trim()
            .toLowerCase();
        final codeCaptured =
            status['code_captured'] == true ||
            flowStatus == _oauthFlowStatusCodeReceived ||
            flowStatus == _oauthFlowStatusCompleted;
        if (codeCaptured) {
          resolve(const _OAuthWaitResult.captured());
          return;
        }

        final statusError = (status['error'] as String? ?? '').trim();
        if (flowStatus == _oauthFlowStatusFailed) {
          resolve(
            _OAuthWaitResult.failed(
              statusError.isNotEmpty
                  ? statusError
                  : 'Authorization failed before completion.',
            ),
          );
          return;
        }
        if (flowStatus == _oauthFlowStatusExpired) {
          resolve(const _OAuthWaitResult.timedOut());
          return;
        }
        if (statusError.isNotEmpty) {
          resolve(_OAuthWaitResult.failed(statusError));
          return;
        }

        await Future.any<void>([
          Future<void>.delayed(widget.oauthStatusPollInterval),
          stopPolling.future,
        ]);
      }
      if (!stopPolling.isCompleted) {
        resolve(const _OAuthWaitResult.timedOut());
      }
    }

    final dialogResult = await showDialog<_OAuthWaitResult>(
      context: context,
      barrierDismissible: false,
      builder: (dialogCtx) {
        dialogContext = dialogCtx;
        if (!pollingStarted) {
          pollingStarted = true;
          unawaited(pollStatus());
        }
        void cancel() => resolve(const _OAuthWaitResult.cancelled());
        return DialogKeyboardShortcuts(
          onCancel: cancel,
          onSubmit: cancel,
          child: AlertDialog(
            key: AppKeys.settingsOAuthProgressDialog(provider.id),
            title: Text('Connect ${provider.displayName}'),
            content: SizedBox(
              width: 420,
              child: Row(
                children: [
                  const SizedBox(
                    height: 20,
                    width: 20,
                    child: CircularProgressIndicator(strokeWidth: 2),
                  ),
                  const SizedBox(width: 12),
                  Expanded(
                    child: Text(
                      'Waiting for browser authorization callback...',
                      style: Theme.of(context).textTheme.bodyMedium,
                    ),
                  ),
                ],
              ),
            ),
            actions: [
              TextButton(
                key: AppKeys.settingsOAuthProgressCancelButton(provider.id),
                onPressed: cancel,
                child: const Text('Cancel'),
              ),
            ],
          ),
        );
      },
    );

    if (!stopPolling.isCompleted) {
      stopPolling.complete();
    }
    if (dialogResult != null) {
      return dialogResult;
    }
    if (waitResult.isCompleted) {
      return waitResult.future;
    }
    return const _OAuthWaitResult.cancelled();
  }

  Future<void> _completeOAuthConnection(
    ProviderStatus provider,
    String flowId, {
    String? redirectUrl,
  }) async {
    final engine = context.read<EngineApi>();
    final payload = <String, dynamic>{
      'provider_id': provider.id,
      'flow_id': flowId,
    };
    final redirect = redirectUrl?.trim() ?? '';
    if (redirect.isNotEmpty) {
      payload['redirect_url'] = redirect;
    }
    AppLog.info('settings.oauth_complete', {
      'provider_id': provider.id,
      'manual_redirect': redirect.isNotEmpty,
    });
    await engine.call('ProvidersOAuthComplete', payload);
  }

  Future<bool> _completeOAuthWithManualRedirect(
    ProviderStatus provider,
    String flowId,
    String authorizeUrl, {
    String? statusMessage,
  }) async {
    final redirectUrl = await _showOAuthCompleteDialog(
      provider,
      authorizeUrl,
      statusMessage: statusMessage,
    );
    if (!mounted || redirectUrl == null) {
      return false;
    }
    await _completeOAuthConnection(
      provider,
      flowId,
      redirectUrl: redirectUrl.trim(),
    );
    return true;
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
      final callbackListening = start['callback_listening'] == true;
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

      var connected = false;
      if (callbackListening) {
        final waitResult = await _showOAuthProgressDialog(provider, flowId);
        if (!mounted || waitResult.cancelled) {
          return;
        }
        if (waitResult.codeCaptured) {
          await _completeOAuthConnection(provider, flowId);
          connected = true;
        } else {
          final fallbackMessage = waitResult.timedOut
              ? 'Automatic callback capture timed out. Authorize in your browser, then paste the full redirect URL.'
              : waitResult.error?.trim().isNotEmpty == true
              ? 'Automatic callback capture failed (${waitResult.error}). '
                    'Authorize in your browser, then paste the full redirect URL.'
              : 'Automatic callback capture failed. Authorize in your browser, then paste the full redirect URL.';
          connected = await _completeOAuthWithManualRedirect(
            provider,
            flowId,
            authorizeUrl,
            statusMessage: fallbackMessage,
          );
        }
      } else {
        connected = await _completeOAuthWithManualRedirect(
          provider,
          flowId,
          authorizeUrl,
          statusMessage:
              'Automatic callback capture is unavailable on this device. '
              'Authorize in your browser, then paste the full redirect URL.',
        );
      }
      if (!mounted || !connected) {
        return;
      }

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
    final isSetupToken = provider.authMode == 'setup_token';
    final credentialLabel = isSetupToken ? 'Setup Token' : 'API Key';
    final clearLabel = isSetupToken ? 'Clear Token' : 'Clear Key';
    final helperText = isSetupToken
        ? 'Setup tokens are stored locally and encrypted at rest by the engine.'
        : 'Keys are stored locally and encrypted at rest by the engine.';
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        TextField(
          key: provider.id == 'openai' ? AppKeys.settingsApiKeyField : null,
          controller: controller,
          obscureText: true,
          decoration: InputDecoration(
            labelText: '${provider.displayName} $credentialLabel',
          ),
        ),
        const SizedBox(height: 12),
        Wrap(
          spacing: 8,
          runSpacing: 8,
          children: [
            ElevatedButton(
              key: provider.id == 'openai' ? AppKeys.settingsSaveButton : null,
              onPressed: () => _saveKey(provider),
              child: const Text('Save & Validate'),
            ),
            OutlinedButton(
              key: AppKeys.settingsClearCredentialButton(provider.id),
              onPressed: () => _clearCredential(provider),
              child: Text(clearLabel),
            ),
          ],
        ),
        const SizedBox(height: 8),
        Text(
          helperText,
          style: Theme.of(context).textTheme.bodySmall?.copyWith(
            color: KeenBenchTheme.colorTextSecondary,
          ),
        ),
        if (isSetupToken) ...[
          const SizedBox(height: 12),
          Container(
            padding: const EdgeInsets.all(12),
            decoration: BoxDecoration(
              color: KeenBenchTheme.colorInfoBackground,
              borderRadius: BorderRadius.circular(6),
              border: Border.all(color: KeenBenchTheme.colorInfoBorder),
            ),
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text(
                  'How to connect',
                  style: Theme.of(context).textTheme.bodyMedium?.copyWith(
                    color: KeenBenchTheme.colorInfoText,
                  ),
                ),
                const SizedBox(height: 8),
                Text(
                  '1. Run this command on any machine where Claude Code is signed in.\n'
                  '2. Copy the setup token it prints.\n'
                  '3. Paste it above and click Save & Validate.',
                  style: Theme.of(context).textTheme.bodySmall?.copyWith(
                    color: KeenBenchTheme.colorInfoText,
                  ),
                ),
                const SizedBox(height: 8),
                Container(
                  width: double.infinity,
                  padding: const EdgeInsets.symmetric(
                    horizontal: 12,
                    vertical: 10,
                  ),
                  decoration: BoxDecoration(
                    color: KeenBenchTheme.colorSurfaceSubtle,
                    borderRadius: BorderRadius.circular(6),
                    border: Border.all(
                      color: KeenBenchTheme.colorBorderDefault,
                    ),
                  ),
                  child: Text(
                    'claude setup-token',
                    style: KeenBenchTheme.mono.copyWith(
                      color: KeenBenchTheme.colorTextPrimary,
                    ),
                  ),
                ),
              ],
            ),
          ),
        ],
      ],
    );
  }

  Widget _buildOAuthControls(ProviderStatus provider) {
    final connected = provider.oauthConnected == true;
    return Wrap(
      spacing: 8,
      runSpacing: 8,
      children: [
        if (!connected)
          ElevatedButton(
            key: AppKeys.settingsOAuthConnectButton(provider.id),
            onPressed: () => _connectOAuth(provider),
            child: const Text('Connect'),
          )
        else
          OutlinedButton(
            key: AppKeys.settingsOAuthDisconnectButton(provider.id),
            onPressed: () => _disconnectOAuth(provider),
            child: const Text('Disconnect'),
          ),
        OutlinedButton(
          key: AppKeys.settingsClearCredentialButton(provider.id),
          onPressed: () => _clearCredential(provider),
          child: const Text('Clear Token'),
        ),
      ],
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
    final isSetupTokenProvider = provider.authMode == 'setup_token';
    final oauthLabel = provider.oauthAccountLabel?.trim() ?? '';
    final oauthConnected = provider.oauthConnected == true;
    final oauthStatusText = oauthConnected
        ? (oauthLabel.isEmpty ? 'Connected' : 'Connected as $oauthLabel')
        : 'Not connected';
    final oauthHint = _oauthStatusHint(provider);
    final tokenLabel = provider.tokenAccountLabel?.trim() ?? '';
    final tokenConnected =
        provider.tokenConnected == true || provider.configured;
    final tokenStatusText = tokenConnected
        ? (tokenLabel.isEmpty ? 'Connected' : 'Connected as $tokenLabel')
        : 'Not connected';

    return Container(
      padding: const EdgeInsets.all(24),
      margin: const EdgeInsets.only(bottom: 24),
      decoration: BoxDecoration(
        color: KeenBenchTheme.colorBackgroundElevated,
        borderRadius: BorderRadius.circular(8),
        border: Border.all(color: KeenBenchTheme.colorBorderSubtle),
        boxShadow: const [
          BoxShadow(
            color: Color.fromRGBO(100, 90, 80, 0.08),
            blurRadius: 4,
            offset: Offset(0, 2),
          ),
        ],
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
          ] else if (isSetupTokenProvider) ...[
            Text(
              tokenStatusText,
              style: Theme.of(context).textTheme.bodySmall?.copyWith(
                color: tokenConnected
                    ? KeenBenchTheme.colorSuccessText
                    : KeenBenchTheme.colorWarningText,
              ),
            ),
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
                      'Egress Consent',
                      style: Theme.of(context).textTheme.headlineSmall,
                    ),
                    const SizedBox(height: 8),
                    SwitchListTile(
                      key: AppKeys.settingsConsentModeToggle,
                      value: _userConsentMode == consentModeAllowAll,
                      onChanged: _updatingConsentMode
                          ? null
                          : _setConsentModeAllowAll,
                      contentPadding: EdgeInsets.zero,
                      title: const Text('Allow all model actions'),
                      subtitle: Text(
                        _userConsentMode == consentModeAllowAll
                            ? 'Enabled: no consent prompts for Workshop or Context model actions.'
                            : 'Default: prompt for consent before model actions.',
                      ),
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
