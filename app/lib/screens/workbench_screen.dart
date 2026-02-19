import 'package:file_selector/file_selector.dart';
import 'package:flutter/material.dart';
import 'package:provider/provider.dart';
import 'package:flutter/services.dart';
import 'package:flutter_markdown_plus/flutter_markdown_plus.dart';

import '../accessibility/a11y_announcer.dart';
import '../accessibility/a11y_focus.dart';
import '../accessibility/a11y_shortcuts.dart';
import '../accessibility/skip_links.dart';
import '../app_keys.dart';
import '../engine/engine_client.dart';
import '../logging.dart';
import '../models/models.dart';
import '../state/workbench_state.dart';
import '../theme.dart';
import '../widgets/clutter_bar.dart';
import '../widgets/dialog_keyboard_shortcuts.dart';
import 'review_screen.dart';
import 'settings_screen.dart';
import 'checkpoints_screen.dart';
import 'workbench_context_screen.dart';
import '../widgets/keenbench_app_bar.dart';

class WorkbenchScreen extends StatelessWidget {
  const WorkbenchScreen({super.key, required this.workbenchId});

  final String workbenchId;

  @override
  Widget build(BuildContext context) {
    final engine = context.read<EngineApi>();
    return ChangeNotifierProvider(
      create: (_) =>
          WorkbenchState(engine: engine, workbenchId: workbenchId)..load(),
      child: const _WorkbenchView(),
    );
  }
}

class _ConsentDecision {
  const _ConsentDecision({required this.granted, required this.persist});

  final bool granted;
  final bool persist;
}

enum _ErrorActionChoice {
  dismiss,
  retry,
  openSettings,
  reviewDraft,
  discardDraft,
}

class _WorkbenchView extends StatefulWidget {
  const _WorkbenchView();

  @override
  State<_WorkbenchView> createState() => _WorkbenchViewState();
}

class _WorkbenchViewState extends State<_WorkbenchView> {
  final _composerController = TextEditingController();
  final _messageScrollController = ScrollController();
  final _mainContentFocusNode = FocusNode(debugLabel: 'workbench_main_content');
  final _fileListFocusNode = FocusNode(debugLabel: 'workbench_file_list');
  final _composerFocusNode = FocusNode(debugLabel: 'workbench_composer');
  final _modelSelectorFocusNode = FocusNode(
    debugLabel: 'workbench_model_selector',
  );
  final _errorSummaryFocusNode = FocusNode(
    debugLabel: 'workbench_error_summary',
  );

  String _lastScrollTrigger = '';
  bool _reviewRouteOpen = false;
  bool _autoReviewScheduled = false;
  bool? _lastHasDraft;
  String? _lastDraftToken;
  String? _dismissedDraftToken;
  String? _restoringCheckpointId;
  String? _errorSummary;

  String? _lastAnnouncedPhaseLabel;
  bool _lastToolExecutingAnnounced = false;
  String _lastToolName = '';
  bool _lastConversationBusy = false;
  String? _lastAnnouncedModelId;
  String _lastClutterLevel = '';
  String? _lastAnnouncedDraftReadyToken;

  @override
  void dispose() {
    _composerController.dispose();
    _messageScrollController.dispose();
    _mainContentFocusNode.dispose();
    _fileListFocusNode.dispose();
    _composerFocusNode.dispose();
    _modelSelectorFocusNode.dispose();
    _errorSummaryFocusNode.dispose();
    super.dispose();
  }

  void _scrollToBottom() {
    WidgetsBinding.instance.addPostFrameCallback((_) {
      if (!mounted || !_messageScrollController.hasClients) {
        return;
      }
      final maxScroll = _messageScrollController.position.maxScrollExtent;
      if (MediaQuery.maybeOf(context)?.disableAnimations == true) {
        _messageScrollController.jumpTo(maxScroll);
        return;
      }
      _messageScrollController.animateTo(
        maxScroll,
        duration: const Duration(milliseconds: 150),
        curve: Curves.easeOut,
      );
    });
  }

  String _draftToken(WorkbenchState state) {
    final draftId = state.draftId?.trim() ?? '';
    if (draftId.isNotEmpty) {
      return draftId;
    }
    return state.hasDraft ? '__draft__' : '';
  }

  void _announce(String message, {bool force = false}) {
    if (!mounted) {
      return;
    }
    A11yAnnouncer.instance.announce(context, message, force: force);
  }

  void _showMessage(String message, {bool isError = false}) {
    if (!mounted) {
      return;
    }
    ScaffoldMessenger.of(
      context,
    ).showSnackBar(SnackBar(content: Text(message)));
    if (isError) {
      _setErrorSummary(message, announce: true);
      return;
    }
    _announce(message);
  }

  void _setErrorSummary(String message, {bool announce = true}) {
    final normalized = message.trim();
    if (normalized.isEmpty || !mounted) {
      return;
    }
    setState(() {
      _errorSummary = normalized;
    });
    requestFocusSafely(_errorSummaryFocusNode);
    if (announce) {
      _announce('Error: $normalized');
    }
  }

  void _clearErrorSummary() {
    if (_errorSummary == null) {
      return;
    }
    setState(() {
      _errorSummary = null;
    });
  }

  String _modelDisplayNameFor(String modelId, List<ModelInfo> models) {
    for (final model in models) {
      if (model.id == modelId) {
        return model.displayName;
      }
    }
    return modelId;
  }

  void _announceStateTransitions(
    WorkbenchState state,
    List<ModelInfo> availableModels,
  ) {
    final phaseLabel = _phaseStatusLabel(state);
    if (phaseLabel != null && phaseLabel != _lastAnnouncedPhaseLabel) {
      _announce(phaseLabel);
    }
    if (phaseLabel == null && _lastAnnouncedPhaseLabel != null) {
      _announce('Phase complete.');
    }
    _lastAnnouncedPhaseLabel = phaseLabel;

    if (state.isToolExecuting) {
      final toolName = state.currentToolName ?? '';
      if (!_lastToolExecutingAnnounced || toolName != _lastToolName) {
        _announce(_toolStatusLabel(toolName));
      }
      _lastToolExecutingAnnounced = true;
      _lastToolName = toolName;
    } else if (_lastToolExecutingAnnounced) {
      _announce('Tool run complete.');
      _lastToolExecutingAnnounced = false;
      _lastToolName = '';
    }

    if (state.isConversationBusy && !_lastConversationBusy) {
      _announce('Generating response.');
    } else if (!state.isConversationBusy && _lastConversationBusy) {
      _announce('Response complete.');
    }
    _lastConversationBusy = state.isConversationBusy;

    final modelId = state.activeModelId?.trim() ?? '';
    if (_lastAnnouncedModelId != null &&
        _lastAnnouncedModelId!.isNotEmpty &&
        modelId.isNotEmpty &&
        _lastAnnouncedModelId != modelId) {
      final label = _modelDisplayNameFor(modelId, availableModels);
      _announce('Model changed to $label.');
    }
    if (modelId.isNotEmpty) {
      _lastAnnouncedModelId = modelId;
    }

    final draftToken = _draftToken(state);
    if (state.hasDraft &&
        draftToken.isNotEmpty &&
        draftToken != _lastAnnouncedDraftReadyToken) {
      _announce('Review ready. Draft changes available.');
      _lastAnnouncedDraftReadyToken = draftToken;
    }
    if (!state.hasDraft) {
      _lastAnnouncedDraftReadyToken = null;
    }

    final clutterLevel = state.clutter?.level.trim().toLowerCase() ?? '';
    if (clutterLevel == 'heavy' && _lastClutterLevel != 'heavy') {
      _announce('Clutter level heavy.');
    }
    _lastClutterLevel = clutterLevel;
  }

  void _scheduleAutoOpenReviewIfNeeded(WorkbenchState state) {
    final hasDraft = state.hasDraft;
    final draftToken = _draftToken(state);
    final firstObservation = _lastHasDraft == null;
    final becameDraft = (_lastHasDraft == false) && hasDraft;
    final draftChangedWhileOpen =
        (_lastHasDraft == true) &&
        hasDraft &&
        _lastDraftToken != null &&
        _lastDraftToken != draftToken;

    if (!hasDraft) {
      _dismissedDraftToken = null;
    }

    final shouldOpen =
        hasDraft &&
        !_reviewRouteOpen &&
        !_autoReviewScheduled &&
        draftToken != _dismissedDraftToken &&
        (firstObservation && hasDraft || becameDraft || draftChangedWhileOpen);

    if (shouldOpen) {
      _autoReviewScheduled = true;
      WidgetsBinding.instance.addPostFrameCallback((_) async {
        _autoReviewScheduled = false;
        if (!mounted) {
          return;
        }
        await _openReview(autoOpened: true);
      });
    }

    _lastHasDraft = hasDraft;
    _lastDraftToken = draftToken;
  }

  Future<void> _openReview({required bool autoOpened}) async {
    final state = context.read<WorkbenchState>();
    if (_reviewRouteOpen || !state.hasDraft || !mounted) {
      return;
    }
    _reviewRouteOpen = true;
    final result = await Navigator.of(context).push(
      MaterialPageRoute(
        builder: (_) => ReviewScreen(workbenchId: state.workbenchId),
      ),
    );
    _reviewRouteOpen = false;
    if (!mounted) {
      return;
    }
    if (result != null) {
      await state.load();
      return;
    }
    if (autoOpened && state.hasDraft) {
      _dismissedDraftToken = _draftToken(state);
    }
  }

  Future<void> _restorePublishCheckpoint(ChatMessage message) async {
    final checkpointId = message.checkpointId;
    if (checkpointId == null || checkpointId.isEmpty) {
      return;
    }
    final state = context.read<WorkbenchState>();
    if (state.hasDraft || _restoringCheckpointId != null) {
      return;
    }
    final confirm = await showDialog<bool>(
      context: context,
      barrierColor: KeenBenchTheme.colorSurfaceOverlay,
      builder: (dialogContext) {
        void cancel() => Navigator.of(dialogContext).pop(false);

        void submit() => Navigator.of(dialogContext).pop(true);

        return DialogKeyboardShortcuts(
          onCancel: cancel,
          onSubmit: submit,
          child: AlertDialog(
            title: const Text('Restore checkpoint'),
            content: Text(
              'Restore publish checkpoint $checkpointId? This reverts Published files and Workbench history.',
            ),
            actions: [
              OutlinedButton(onPressed: cancel, child: const Text('Cancel')),
              ElevatedButton(onPressed: submit, child: const Text('Restore')),
            ],
          ),
        );
      },
    );
    if (confirm != true) {
      return;
    }
    setState(() {
      _restoringCheckpointId = checkpointId;
    });
    try {
      await state.restoreCheckpoint(checkpointId);
      if (!mounted) {
        return;
      }
      _showMessage('Checkpoint restored.');
    } on EngineError catch (err) {
      await _handleEngineError(
        err,
        onRetry: () => state.restoreCheckpoint(checkpointId),
      );
    } finally {
      if (mounted) {
        setState(() {
          _restoringCheckpointId = null;
        });
      }
    }
  }

  Future<void> _submitComposer() async {
    final state = context.read<WorkbenchState>();
    if (state.isConversationBusy) {
      return;
    }
    if (state.hasDraft) {
      return;
    }
    final text = _composerController.text.trim();
    if (text.isEmpty) {
      return;
    }
    _clearErrorSummary();
    final providerReady = await _ensureProviderReady();
    if (!providerReady) {
      return;
    }
    final consented = await _ensureConsent();
    if (!consented) {
      return;
    }
    _composerController.clear();
    try {
      await state.sendMessage(text);
    } on EngineError catch (err) {
      await _handleEngineError(err, onRetry: () => state.sendMessage(text));
    }
  }

  Future<void> _cancelActiveRun() async {
    final state = context.read<WorkbenchState>();
    if (!state.isConversationBusy) {
      return;
    }
    try {
      await state.cancelRun();
    } on EngineError catch (err) {
      await _handleEngineError(err);
    }
  }

  Future<void> _rewindToMessage(ChatMessage message) async {
    final messageId = message.id.trim();
    if (messageId.isEmpty) {
      return;
    }
    final state = context.read<WorkbenchState>();
    if (state.isConversationBusy) {
      return;
    }
    final confirm = await showDialog<bool>(
      context: context,
      barrierColor: KeenBenchTheme.colorSurfaceOverlay,
      builder: (dialogContext) {
        void cancel() => Navigator.of(dialogContext).pop(false);

        void submit() => Navigator.of(dialogContext).pop(true);

        return DialogKeyboardShortcuts(
          onCancel: cancel,
          onSubmit: submit,
          child: AlertDialog(
            title: const Text('Rewind conversation'),
            content: const Text(
              'Rewinding will discard all messages after this point and revert Draft changes. If publish checkpoints are crossed, Published files are also restored.',
            ),
            actions: [
              OutlinedButton(onPressed: cancel, child: const Text('Cancel')),
              ElevatedButton(onPressed: submit, child: const Text('Rewind')),
            ],
          ),
        );
      },
    );
    if (confirm != true) {
      return;
    }
    try {
      await state.undoToMessage(messageId);
    } on EngineError catch (err) {
      await _handleEngineError(
        err,
        onRetry: () => state.undoToMessage(messageId),
      );
    }
  }

  Future<void> _regenerateFromMessage(ChatMessage message) async {
    final messageId = message.id.trim();
    if (messageId.isEmpty) {
      return;
    }
    final state = context.read<WorkbenchState>();
    if (state.isConversationBusy) {
      return;
    }
    try {
      await state.regenerate(messageId: messageId);
    } on EngineError catch (err) {
      await _handleEngineError(
        err,
        onRetry: () => state.regenerate(messageId: messageId),
      );
    }
  }

  Future<bool> _ensureConsent() async {
    final state = context.read<WorkbenchState>();
    final engine = context.read<EngineApi>();
    final status = await engine.call('EgressGetConsentStatus', {
      'workbench_id': state.workbenchId,
    });
    final consented = status['consented'] == true;
    final scopeHash = status['scope_hash'] as String? ?? '';
    final providerId = status['provider_id'] as String? ?? '';
    final modelId = status['model_id'] as String? ?? '';
    AppLog.debug('workbench.consent_status', {
      'workbench_id': state.workbenchId,
      'consented': consented,
      'scope_hash': scopeHash,
    });
    if (consented) {
      return true;
    }
    final decision = await _showConsentDialog(
      state.files,
      scopeHash,
      providerId,
      modelId,
    );
    if (decision == null || !decision.granted) {
      return false;
    }
    await engine.call('EgressGrantWorkshopConsent', {
      'workbench_id': state.workbenchId,
      'provider_id': providerId,
      'model_id': modelId,
      'scope_hash': scopeHash,
      'persist': decision.persist,
    });
    return true;
  }

  Future<bool> _ensureProviderReady() async {
    final state = context.read<WorkbenchState>();
    final engine = context.read<EngineApi>();
    final workshopState = await engine.call('WorkshopGetState', {
      'workbench_id': state.workbenchId,
    });
    final activeModelId =
        workshopState['active_model_id'] as String? ??
        state.activeModelId ??
        '';
    final providersResponse = await engine.call('ProvidersGetStatus');
    final providerList =
        (providersResponse['providers'] as List<dynamic>? ?? [])
            .cast<Map<String, dynamic>>();
    final providers = providerList.map(ProviderStatus.fromJson).toList();

    String providerId = '';
    for (final model in state.models) {
      if (model.id == activeModelId) {
        providerId = model.providerId;
        break;
      }
    }
    if (providerId.isEmpty) {
      for (final provider in providers) {
        if (provider.models.contains(activeModelId)) {
          providerId = provider.id;
          break;
        }
      }
    }

    ProviderStatus? targetProvider;
    for (final provider in providers) {
      if (provider.id == providerId) {
        targetProvider = provider;
        break;
      }
    }

    if (targetProvider != null &&
        targetProvider.enabled &&
        targetProvider.configured) {
      return true;
    }

    final providerName = targetProvider?.displayName.trim().isNotEmpty == true
        ? targetProvider!.displayName
        : _providerNameFor(providerId);
    await _showProviderRequiredDialog(providerName, providerId: providerId);
    return false;
  }

  String _providerNameFor(String providerId) {
    final normalizedId = providerId.trim();
    if (normalizedId.isEmpty) {
      return 'Provider';
    }
    final providers = context.read<WorkbenchState>().providers;
    for (final provider in providers) {
      if (provider.id == normalizedId) {
        final displayName = provider.displayName.trim();
        if (displayName.isNotEmpty) {
          return displayName;
        }
        break;
      }
    }
    return normalizedId;
  }

  String _providerAuthModeFor(String providerId) {
    final normalizedId = providerId.trim();
    if (normalizedId.isEmpty) return 'api_key';
    final providers = context.read<WorkbenchState>().providers;
    for (final provider in providers) {
      if (provider.id == normalizedId) {
        final authMode = provider.authMode.trim();
        if (authMode.isNotEmpty) return authMode;
        break;
      }
    }
    if (normalizedId == 'openai-codex') return 'oauth';
    return 'api_key';
  }

  Future<void> _showProviderRequiredDialog(
    String providerName, {
    String providerId = '',
  }) async {
    final label = providerName.trim().isEmpty
        ? 'Provider'
        : providerName.trim();
    final isOAuth = _providerAuthModeFor(providerId) == 'oauth';
    final title = isOAuth
        ? '$label authentication required'
        : '$label key required';
    final content = isOAuth
        ? 'Connect $label in Settings before continuing.'
        : 'Configure a valid $label API key before continuing.';
    _setErrorSummary(content, announce: true);
    final open = await showDialog<bool>(
      context: context,
      barrierColor: KeenBenchTheme.colorSurfaceOverlay,
      builder: (dialogContext) {
        void cancel() => Navigator.of(dialogContext).pop(false);

        void submit() => Navigator.of(dialogContext).pop(true);

        return DialogKeyboardShortcuts(
          onCancel: cancel,
          onSubmit: submit,
          child: AlertDialog(
            key: AppKeys.providerRequiredDialog,
            title: Text(title),
            content: Text(content),
            actions: [
              OutlinedButton(
                key: AppKeys.providerRequiredCancel,
                onPressed: cancel,
                child: const Text('Cancel'),
              ),
              ElevatedButton(
                key: AppKeys.providerRequiredOpenSettings,
                onPressed: submit,
                child: const Text('Open Settings'),
              ),
            ],
          ),
        );
      },
    );
    if (open == true && mounted) {
      _clearErrorSummary();
      await _openSettings();
    }
  }

  Future<void> _handleEngineError(
    EngineError err, {
    Future<void> Function()? onRetry,
  }) async {
    if (!mounted) return;
    if (err.errorCode == 'USER_CANCELED') {
      AppLog.info('workbench.run_canceled', {
        'workbench_id': context.read<WorkbenchState>().workbenchId,
      });
      _showMessage('Run canceled.');
      return;
    }
    if (err.errorCode == 'PROVIDER_NOT_CONFIGURED' ||
        err.errorCode == 'PROVIDER_AUTH_FAILED') {
      AppLog.warn('workbench.provider_error', {
        'error_code': err.errorCode,
        'message': err.message,
      });
      await _showProviderRequiredDialog(
        _providerNameFor(err.providerId ?? ''),
        providerId: err.providerId ?? '',
      );
      return;
    }
    if (err.errorCode == 'EGRESS_CONSENT_REQUIRED') {
      AppLog.warn('workbench.consent_required', {
        'workbench_id': context.read<WorkbenchState>().workbenchId,
        'scope_hash': err.scopeHash ?? '',
      });
      final decision = await _showConsentDialog(
        context.read<WorkbenchState>().files,
        err.scopeHash ?? '',
        err.providerId ?? '',
        err.modelId ?? '',
      );
      if (decision != null && decision.granted) {
        await context.read<EngineApi>().call('EgressGrantWorkshopConsent', {
          'workbench_id': context.read<WorkbenchState>().workbenchId,
          'provider_id': err.providerId ?? '',
          'model_id': err.modelId ?? '',
          'scope_hash': err.scopeHash ?? '',
          'persist': decision.persist,
        });
        if (onRetry != null) {
          await _runRetry(onRetry);
        }
      }
      return;
    }
    if (await _handleActionableError(err, onRetry: onRetry)) {
      return;
    }
    AppLog.warn('workbench.engine_error', {
      'workbench_id': context.read<WorkbenchState>().workbenchId,
      'error_code': err.errorCode,
      'message': err.message,
    });
    _showMessage(err.message, isError: true);
  }

  Future<void> _openSettings() async {
    if (!mounted) {
      return;
    }
    await Navigator.of(
      context,
    ).push(MaterialPageRoute(builder: (_) => const SettingsScreen()));
  }

  Future<void> _runRetry(Future<void> Function() onRetry) async {
    try {
      await onRetry();
    } on EngineError catch (retryErr) {
      await _handleEngineError(retryErr);
    } catch (retryErr) {
      if (!mounted) {
        return;
      }
      _showMessage(retryErr.toString(), isError: true);
    }
  }

  Future<bool> _handleActionableError(
    EngineError err, {
    Future<void> Function()? onRetry,
  }) async {
    final actions = err.actions.toSet();
    final canRetry = actions.contains('retry') && onRetry != null;
    final hasKnownActions =
        canRetry ||
        actions.contains('open_settings') ||
        actions.contains('review_draft') ||
        actions.contains('discard_draft');
    if (!hasKnownActions) {
      return false;
    }
    _setErrorSummary(err.message, announce: true);
    final defaultChoice = canRetry
        ? _ErrorActionChoice.retry
        : actions.contains('review_draft')
        ? _ErrorActionChoice.reviewDraft
        : actions.contains('discard_draft')
        ? _ErrorActionChoice.discardDraft
        : actions.contains('open_settings')
        ? _ErrorActionChoice.openSettings
        : _ErrorActionChoice.dismiss;
    final choice = await showDialog<_ErrorActionChoice>(
      context: context,
      barrierColor: KeenBenchTheme.colorSurfaceOverlay,
      builder: (dialogContext) {
        void cancel() =>
            Navigator.of(dialogContext).pop(_ErrorActionChoice.dismiss);

        void submit() => Navigator.of(dialogContext).pop(defaultChoice);

        return DialogKeyboardShortcuts(
          onCancel: cancel,
          onSubmit: submit,
          child: AlertDialog(
            title: const Text('Action required'),
            content: Text(err.message),
            actions: [
              OutlinedButton(onPressed: cancel, child: const Text('Dismiss')),
              if (canRetry)
                ElevatedButton(
                  onPressed: () =>
                      Navigator.of(dialogContext).pop(_ErrorActionChoice.retry),
                  child: const Text('Retry'),
                ),
              if (actions.contains('review_draft'))
                TextButton(
                  onPressed: () => Navigator.of(
                    dialogContext,
                  ).pop(_ErrorActionChoice.reviewDraft),
                  child: const Text('Open review'),
                ),
              if (actions.contains('discard_draft'))
                TextButton(
                  onPressed: () => Navigator.of(
                    dialogContext,
                  ).pop(_ErrorActionChoice.discardDraft),
                  style: TextButton.styleFrom(
                    foregroundColor: KeenBenchTheme.colorErrorText,
                  ),
                  child: const Text('Discard Draft'),
                ),
              if (actions.contains('open_settings'))
                TextButton(
                  onPressed: () => Navigator.of(
                    dialogContext,
                  ).pop(_ErrorActionChoice.openSettings),
                  child: const Text('Open Settings'),
                ),
            ],
          ),
        );
      },
    );
    switch (choice ?? _ErrorActionChoice.dismiss) {
      case _ErrorActionChoice.retry:
        if (onRetry != null) {
          await _runRetry(onRetry);
        }
        return true;
      case _ErrorActionChoice.openSettings:
        await _openSettings();
        return true;
      case _ErrorActionChoice.reviewDraft:
        await _openReview(autoOpened: false);
        return true;
      case _ErrorActionChoice.discardDraft:
        final confirm = await _confirmDiscardDraft();
        if (confirm) {
          try {
            await context.read<WorkbenchState>().discardDraft();
          } on EngineError catch (discardErr) {
            await _handleEngineError(discardErr);
          }
        }
        return true;
      case _ErrorActionChoice.dismiss:
        return true;
    }
  }

  Future<bool> _confirmDiscardDraft() async {
    final confirm = await showDialog<bool>(
      context: context,
      barrierColor: KeenBenchTheme.colorSurfaceOverlay,
      builder: (dialogContext) {
        void cancel() => Navigator.of(dialogContext).pop(false);

        void submit() => Navigator.of(dialogContext).pop(true);

        return DialogKeyboardShortcuts(
          onCancel: cancel,
          onSubmit: submit,
          child: AlertDialog(
            title: const Text('Discard Draft'),
            content: const Text(
              'Discarding removes all unpublished changes in this Draft. Published files remain unchanged.',
            ),
            actions: [
              OutlinedButton(onPressed: cancel, child: const Text('Cancel')),
              ElevatedButton(
                onPressed: submit,
                style: ElevatedButton.styleFrom(
                  backgroundColor: KeenBenchTheme.colorErrorText,
                ),
                child: const Text('Discard'),
              ),
            ],
          ),
        );
      },
    );
    return confirm == true;
  }

  Future<void> _confirmRemoveFile(WorkbenchFile file) async {
    final state = context.read<WorkbenchState>();
    if (state.hasDraft) {
      return;
    }
    final confirm = await showDialog<bool>(
      context: context,
      barrierColor: KeenBenchTheme.colorSurfaceOverlay,
      builder: (dialogContext) {
        void cancel() => Navigator.of(dialogContext).pop(false);

        void submit() => Navigator.of(dialogContext).pop(true);

        return DialogKeyboardShortcuts(
          onCancel: cancel,
          onSubmit: submit,
          child: AlertDialog(
            key: AppKeys.workbenchRemoveFileDialog,
            title: const Text('Remove file'),
            content: Text(
              'Remove "${file.path}" from this Workbench? Originals remain untouched.',
            ),
            actions: [
              OutlinedButton(
                key: AppKeys.workbenchRemoveFileCancel,
                onPressed: cancel,
                child: const Text('Cancel'),
              ),
              ElevatedButton(
                key: AppKeys.workbenchRemoveFileConfirm,
                onPressed: submit,
                style: ElevatedButton.styleFrom(
                  backgroundColor: KeenBenchTheme.colorErrorText,
                ),
                child: const Text('Remove'),
              ),
            ],
          ),
        );
      },
    );
    if (confirm != true) {
      return;
    }
    try {
      await state.removeFiles([file.path]);
    } on EngineError catch (err) {
      if (!mounted) return;
      _showMessage(err.message, isError: true);
    }
  }

  Future<void> _openContextOverview() async {
    final state = context.read<WorkbenchState>();
    await state.loadContextItems(notify: false);
    if (!mounted) {
      return;
    }
    await Navigator.of(context).push(
      MaterialPageRoute(
        builder: (_) => ChangeNotifierProvider<WorkbenchState>.value(
          value: state,
          child: const WorkbenchContextScreen(),
        ),
      ),
    );
  }

  Future<void> _extractFile(WorkbenchFile file) async {
    final state = context.read<WorkbenchState>();
    if (state.hasDraft) {
      return;
    }
    final destinationDir = await getDirectoryPath(
      confirmButtonText: 'Extract here',
    );
    if (destinationDir == null || destinationDir.trim().isEmpty) {
      return;
    }
    try {
      final results = await state.extractFiles(
        destinationDir.trim(),
        paths: [file.path],
      );
      if (!mounted) {
        return;
      }
      final result = results.isNotEmpty ? results.first : null;
      var message = 'No files extracted.';
      if (result != null) {
        if (result.isExtracted) {
          message = 'Extracted "${file.path}".';
        } else if (result.reason == 'destination_exists') {
          message = '"${file.path}" already exists in the destination.';
        } else if (result.isSkipped) {
          message = 'Skipped "${file.path}" (${result.reason}).';
        } else if (result.isFailed) {
          message = 'Failed to extract "${file.path}" (${result.reason}).';
        }
      }
      _showMessage(message);
    } on EngineError catch (err) {
      if (!mounted) {
        return;
      }
      _showMessage(err.message, isError: true);
    }
  }

  Future<_ConsentDecision?> _showConsentDialog(
    List<WorkbenchFile> files,
    String scopeHash,
    String providerId,
    String modelId,
  ) async {
    final state = context.read<WorkbenchState>();
    final providerName = state.providers
        .firstWhere(
          (p) => p.id == providerId,
          orElse: () => ProviderStatus(
            id: providerId,
            displayName: providerId,
            enabled: true,
            configured: true,
            models: const [],
          ),
        )
        .displayName;
    final modelName = state.models
        .firstWhere(
          (m) => m.id == modelId,
          orElse: () => ModelInfo(
            id: modelId,
            providerId: providerId,
            displayName: modelId,
            contextTokensEstimate: 0,
            supportsFileRead: true,
            supportsFileWrite: true,
          ),
        )
        .displayName;
    bool persist = true;
    return await showDialog<_ConsentDecision>(
      context: context,
      barrierColor: KeenBenchTheme.colorSurfaceOverlay,
      builder: (context) => StatefulBuilder(
        builder: (dialogContext, setState) {
          void cancel() => Navigator.of(
            dialogContext,
          ).pop(const _ConsentDecision(granted: false, persist: false));

          void submit() => Navigator.of(
            dialogContext,
          ).pop(_ConsentDecision(granted: true, persist: persist));

          return DialogKeyboardShortcuts(
            onCancel: cancel,
            onSubmit: submit,
            child: AlertDialog(
              key: AppKeys.consentDialog,
              title: const Text('Consent required'),
              content: SizedBox(
                width: 420,
                child: Column(
                  mainAxisSize: MainAxisSize.min,
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(
                      'KeenBench will send Workbench content to $providerName ($modelName) to generate responses.',
                      style: Theme.of(dialogContext).textTheme.bodyMedium,
                    ),
                    const SizedBox(height: 12),
                    Text(
                      'Files in scope',
                      style: Theme.of(dialogContext).textTheme.bodyMedium,
                    ),
                    const SizedBox(height: 8),
                    SizedBox(
                      height: 180,
                      child: ListView.builder(
                        key: AppKeys.consentFileList,
                        itemCount: files.length,
                        itemBuilder: (context, index) {
                          final file = files[index];
                          return Padding(
                            padding: const EdgeInsets.symmetric(vertical: 4),
                            child: Text(
                              '${file.path} (${file.size} bytes)',
                              style: Theme.of(
                                dialogContext,
                              ).textTheme.bodySmall,
                            ),
                          );
                        },
                      ),
                    ),
                    const SizedBox(height: 8),
                    Text(
                      key: AppKeys.consentScopeHash,
                      'Scope hash: $scopeHash',
                      style: Theme.of(dialogContext).textTheme.bodySmall,
                    ),
                    const SizedBox(height: 12),
                    CheckboxListTile(
                      value: persist,
                      onChanged: (value) =>
                          setState(() => persist = value ?? true),
                      contentPadding: EdgeInsets.zero,
                      title: const Text("Don't ask again for this Workbench"),
                    ),
                  ],
                ),
              ),
              actions: [
                OutlinedButton(
                  key: AppKeys.consentCancelButton,
                  onPressed: cancel,
                  child: const Text('Cancel'),
                ),
                ElevatedButton(
                  key: AppKeys.consentContinueButton,
                  onPressed: submit,
                  child: const Text('Continue'),
                ),
              ],
            ),
          );
        },
      ),
    );
  }

  @override
  Widget build(BuildContext context) {
    return Consumer<WorkbenchState>(
      builder: (context, state, _) {
        _scheduleAutoOpenReviewIfNeeded(state);
        final scrollTrigger =
            '${state.messages.length}:'
            '${state.messages.isNotEmpty ? state.messages.last.text.length : 0}';
        if (scrollTrigger != _lastScrollTrigger) {
          _lastScrollTrigger = scrollTrigger;
          _scrollToBottom();
        }
        final workbench = state.workbench;
        final providerMap = {
          for (final provider in state.providers) provider.id: provider,
        };
        final availableModelMap = <String, ModelInfo>{};
        for (final model in state.models) {
          final provider = providerMap[model.providerId];
          if (provider == null) {
            continue;
          }
          if (provider.enabled && provider.configured) {
            availableModelMap.putIfAbsent(model.id, () => model);
          }
        }
        final availableModels = availableModelMap.values.toList();
        final selectedModelId =
            state.activeModelId != null &&
                availableModels.any((model) => model.id == state.activeModelId)
            ? state.activeModelId
            : availableModels.isNotEmpty
            ? availableModels.first.id
            : null;
        final scope = state.scope;
        final scopeLimitsText = _buildScopeLimitsText(
          scope,
          state.files.length,
        );
        final draftMetadataText = _buildDraftMetadataText(state);
        final canRunConversationAction =
            !state.isApplyingDraft && !state.isConversationBusy;
        WidgetsBinding.instance.addPostFrameCallback((_) {
          if (!mounted) {
            return;
          }
          _announceStateTransitions(state, availableModels);
        });

        return Shortcuts(
          shortcuts: workbenchShortcutMap(),
          child: Actions(
            actions: <Type, Action<Intent>>{
              FocusComposerIntent: CallbackAction<Intent>(
                onInvoke: (_) {
                  requestFocusSafely(_composerFocusNode);
                  return null;
                },
              ),
              SendComposerIntent: CallbackAction<Intent>(
                onInvoke: (_) {
                  _submitComposer();
                  return null;
                },
              ),
              OpenReviewIntent: CallbackAction<Intent>(
                onInvoke: (_) {
                  if (state.hasDraft) {
                    _openReview(autoOpened: false);
                  }
                  return null;
                },
              ),
              PublishDraftIntent: CallbackAction<Intent>(
                onInvoke: (_) {
                  if (state.hasDraft) {
                    _openReview(autoOpened: false);
                  }
                  return null;
                },
              ),
              DiscardDraftIntent: CallbackAction<Intent>(
                onInvoke: (_) {
                  if (!state.hasDraft) {
                    return null;
                  }
                  _confirmDiscardDraft().then((confirm) async {
                    if (!confirm) {
                      return;
                    }
                    try {
                      await state.discardDraft();
                    } on EngineError catch (err) {
                      await _handleEngineError(err);
                    }
                  });
                  return null;
                },
              ),
            },
            child: FocusTraversalGroup(
              policy: OrderedTraversalPolicy(),
              child: Scaffold(
                key: AppKeys.workbenchScreen,
                appBar: KeenBenchAppBar(
                  title: workbench?.name ?? 'Workbench',
                  showBack: true,
                  useCenteredContent: false,
                  actions: [
                    if (availableModels.isNotEmpty)
                      Semantics(
                        key: AppKeys.workbenchModelSelectorSemantics,
                        label:
                            'Current model: ${_modelDisplayNameFor(selectedModelId ?? '', availableModels)}',
                        hint: 'Dropdown',
                        child: Focus(
                          focusNode: _modelSelectorFocusNode,
                          child: Container(
                            constraints: const BoxConstraints(maxWidth: 220),
                            padding: const EdgeInsets.symmetric(horizontal: 8),
                            decoration: BoxDecoration(
                              color: KeenBenchTheme.colorSurfaceSubtle,
                              borderRadius: BorderRadius.circular(6),
                              border: Border.all(
                                color: KeenBenchTheme.colorBorderSubtle,
                              ),
                            ),
                            child: DropdownButtonHideUnderline(
                              child: DropdownButton<String>(
                                value: selectedModelId,
                                isExpanded: true,
                                items: availableModels
                                    .map(
                                      (model) => DropdownMenuItem<String>(
                                        value: model.id,
                                        child: Text(
                                          '${model.displayName} (${providerMap[model.providerId]?.displayName ?? model.providerId})',
                                          overflow: TextOverflow.ellipsis,
                                        ),
                                      ),
                                    )
                                    .toList(),
                                onChanged: state.isConversationBusy
                                    ? null
                                    : (value) {
                                        if (value != null) {
                                          _clearErrorSummary();
                                          state.setActiveModel(value);
                                        }
                                      },
                              ),
                            ),
                          ),
                        ),
                      )
                    else
                      Text(
                        'No models configured',
                        style: Theme.of(context).textTheme.bodySmall?.copyWith(
                          color: KeenBenchTheme.colorTextSecondary,
                        ),
                      ),
                    const SizedBox(width: 16),
                    if (state.clutter != null)
                      ClutterBar(
                        key: AppKeys.workbenchClutterBar,
                        score: state.clutter!.score,
                        level: state.clutter!.level,
                      ),
                    const SizedBox(width: 16),
                    Tooltip(
                      message: 'Checkpoints',
                      child: IconButton(
                        key: AppKeys.workbenchCheckpointsButton,
                        onPressed: () => Navigator.of(context).push(
                          MaterialPageRoute(
                            builder: (_) => CheckpointsScreen(
                              workbenchId: state.workbenchId,
                            ),
                          ),
                        ),
                        icon: const Icon(
                          Icons.history,
                          size: 20,
                          color: KeenBenchTheme.colorTextSecondary,
                        ),
                      ),
                    ),
                  ],
                ),
                body: Column(
                  crossAxisAlignment: CrossAxisAlignment.stretch,
                  children: [
                    A11ySkipLinks(
                      links: [
                        A11ySkipLink(
                          key: AppKeys.workbenchSkipToMainLink,
                          label: 'Skip to main content',
                          targetFocusNode: _mainContentFocusNode,
                        ),
                        A11ySkipLink(
                          key: AppKeys.workbenchSkipToComposerLink,
                          label: 'Skip to composer',
                          targetFocusNode: _composerFocusNode,
                        ),
                      ],
                    ),
                    if (_errorSummary != null)
                      Container(
                        key: AppKeys.workbenchErrorSummary,
                        margin: const EdgeInsets.fromLTRB(24, 0, 24, 8),
                        padding: const EdgeInsets.all(12),
                        decoration: BoxDecoration(
                          color: KeenBenchTheme.colorErrorBackground,
                          borderRadius: BorderRadius.circular(8),
                          border: Border.all(
                            color: KeenBenchTheme.colorErrorText,
                          ),
                        ),
                        child: Semantics(
                          liveRegion: true,
                          focused: true,
                          label: 'Error summary',
                          child: Focus(
                            focusNode: _errorSummaryFocusNode,
                            child: Row(
                              children: [
                                Expanded(
                                  child: Text(
                                    _errorSummary!,
                                    style: Theme.of(context).textTheme.bodySmall
                                        ?.copyWith(
                                          color: KeenBenchTheme.colorErrorText,
                                        ),
                                  ),
                                ),
                                TextButton(
                                  onPressed: () {
                                    _clearErrorSummary();
                                    requestFocusSafely(_composerFocusNode);
                                  },
                                  child: const Text('Dismiss'),
                                ),
                              ],
                            ),
                          ),
                        ),
                      ),
                    Expanded(
                      child: Padding(
                        padding: const EdgeInsets.symmetric(horizontal: 24),
                        child: Row(
                          children: [
                            Container(
                              width: 320,
                              decoration: const BoxDecoration(
                                border: Border(
                                  right: BorderSide(
                                    color: KeenBenchTheme.colorBorderDefault,
                                  ),
                                ),
                                color: KeenBenchTheme.colorBackgroundSecondary,
                              ),
                              child: Column(
                                crossAxisAlignment: CrossAxisAlignment.start,
                                children: [
                                  Padding(
                                    padding: const EdgeInsets.all(16),
                                    child: Column(
                                      crossAxisAlignment:
                                          CrossAxisAlignment.start,
                                      children: [
                                        Wrap(
                                          crossAxisAlignment:
                                              WrapCrossAlignment.center,
                                          spacing: 8,
                                          runSpacing: 6,
                                          children: [
                                            Text(
                                              'Workbench Files',
                                              style: Theme.of(
                                                context,
                                              ).textTheme.headlineSmall,
                                            ),
                                            if (scope != null)
                                              Container(
                                                key:
                                                    AppKeys.workbenchScopeBadge,
                                                padding:
                                                    const EdgeInsets.symmetric(
                                                      horizontal: 8,
                                                      vertical: 2,
                                                    ),
                                                decoration: BoxDecoration(
                                                  color: KeenBenchTheme
                                                      .colorInfoBackground,
                                                  borderRadius:
                                                      BorderRadius.circular(
                                                        999,
                                                      ),
                                                  border: Border.all(
                                                    color: KeenBenchTheme
                                                        .colorInfoBorder,
                                                  ),
                                                ),
                                                child: Text(
                                                  'Scoped',
                                                  style: Theme.of(context)
                                                      .textTheme
                                                      .labelSmall
                                                      ?.copyWith(
                                                        color: KeenBenchTheme
                                                            .colorInfoText,
                                                        fontWeight:
                                                            FontWeight.w600,
                                                        letterSpacing: 0.4,
                                                      ),
                                                ),
                                              ),
                                          ],
                                        ),
                                        const SizedBox(height: 8),
                                        Text(
                                          'Files are copied into the Workbench. Originals stay untouched.',
                                          style: Theme.of(context)
                                              .textTheme
                                              .bodySmall
                                              ?.copyWith(
                                                color: KeenBenchTheme
                                                    .colorTextSecondary,
                                              ),
                                        ),
                                        if (scopeLimitsText.isNotEmpty) ...[
                                          const SizedBox(height: 8),
                                          Text(
                                            scopeLimitsText,
                                            key: AppKeys.workbenchScopeLimits,
                                            style: Theme.of(context)
                                                .textTheme
                                                .bodySmall
                                                ?.copyWith(
                                                  color: KeenBenchTheme
                                                      .colorTextSecondary,
                                                ),
                                          ),
                                        ],
                                        const SizedBox(height: 12),
                                        Row(
                                          children: [
                                            Expanded(
                                              child: Tooltip(
                                                message: state.hasDraft
                                                    ? 'Publish or discard the Draft to add files.'
                                                    : 'Add files to the Workbench.',
                                                child: SizedBox(
                                                  width: double.infinity,
                                                  child: ElevatedButton.icon(
                                                    key: AppKeys
                                                        .workbenchAddFilesButton,
                                                    onPressed: state.hasDraft
                                                        ? null
                                                        : () async {
                                                            final files =
                                                                await openFiles();
                                                            final paths = files
                                                                .map(
                                                                  (file) =>
                                                                      file.path,
                                                                )
                                                                .toList();
                                                            if (paths
                                                                .isNotEmpty) {
                                                              try {
                                                                await state
                                                                    .addFiles(
                                                                      paths,
                                                                    );
                                                              } catch (err) {
                                                                if (!context
                                                                    .mounted) {
                                                                  return;
                                                                }
                                                                _showMessage(
                                                                  err.toString(),
                                                                  isError: true,
                                                                );
                                                              }
                                                            }
                                                          },
                                                    icon: const Icon(Icons.add),
                                                    label: const Text(
                                                      'Add files',
                                                    ),
                                                  ),
                                                ),
                                              ),
                                            ),
                                            const SizedBox(width: 8),
                                            Expanded(
                                              child: Tooltip(
                                                message: state.hasDraft
                                                    ? 'View context. Add/edit/delete are blocked while a Draft exists.'
                                                    : 'Add or edit workbench context.',
                                                child: SizedBox(
                                                  width: double.infinity,
                                                  child: OutlinedButton.icon(
                                                    key: AppKeys
                                                        .workbenchAddContextButton,
                                                    onPressed:
                                                        _openContextOverview,
                                                    icon: const Icon(
                                                      Icons.auto_awesome,
                                                    ),
                                                    label: const Text(
                                                      'Add Context',
                                                    ),
                                                  ),
                                                ),
                                              ),
                                            ),
                                          ],
                                        ),
                                        if (state.clutter?.contextWarning ==
                                            true) ...[
                                          const SizedBox(height: 8),
                                          Text(
                                            'Context is using a large share of the prompt window. Consider shortening context items.',
                                            key:
                                                AppKeys.workbenchContextWarning,
                                            style: Theme.of(context)
                                                .textTheme
                                                .bodySmall
                                                ?.copyWith(
                                                  color: KeenBenchTheme
                                                      .colorWarningText,
                                                ),
                                          ),
                                        ],
                                      ],
                                    ),
                                  ),
                                  Expanded(
                                    child: Focus(
                                      focusNode: _fileListFocusNode,
                                      child: Semantics(
                                        label: 'Workbench file list',
                                        child: ListView.builder(
                                          key: AppKeys.workbenchFileList,
                                          padding: const EdgeInsets.symmetric(
                                            horizontal: 16,
                                          ),
                                          itemCount: state.files.length,
                                          itemBuilder: (context, index) {
                                            final file = state.files[index];
                                            return Padding(
                                              padding: const EdgeInsets.only(
                                                bottom: 8,
                                              ),
                                              child: _WorkbenchFileRow(
                                                file: file,
                                                canExtract: !state.hasDraft,
                                                onExtract: () =>
                                                    _extractFile(file),
                                                canRemove: !state.hasDraft,
                                                onRemove: () =>
                                                    _confirmRemoveFile(file),
                                              ),
                                            );
                                          },
                                        ),
                                      ),
                                    ),
                                  ),
                                  Padding(
                                    padding: const EdgeInsets.fromLTRB(
                                      8,
                                      0,
                                      8,
                                      8,
                                    ),
                                    child: Align(
                                      alignment: Alignment.bottomLeft,
                                      child: Tooltip(
                                        message: 'Settings',
                                        child: IconButton(
                                          key: AppKeys.workbenchSettingsButton,
                                          onPressed: () =>
                                              Navigator.of(context).push(
                                                MaterialPageRoute(
                                                  builder: (_) =>
                                                      const SettingsScreen(),
                                                ),
                                              ),
                                          icon: const Icon(
                                            Icons.settings_outlined,
                                          ),
                                          iconSize: 40,
                                          constraints:
                                              const BoxConstraints.tightFor(
                                                width: 72,
                                                height: 72,
                                              ),
                                          color:
                                              KeenBenchTheme.colorTextSecondary,
                                          hoverColor: KeenBenchTheme
                                              .colorBackgroundHover,
                                        ),
                                      ),
                                    ),
                                  ),
                                ],
                              ),
                            ),
                            Expanded(
                              child: Focus(
                                key: AppKeys.workbenchMainContentRegion,
                                focusNode: _mainContentFocusNode,
                                child: Column(
                                  children: [
                                    if (state.hasDraft)
                                      Container(
                                        key: AppKeys.workbenchDraftBanner,
                                        width: double.infinity,
                                        padding: const EdgeInsets.symmetric(
                                          horizontal: 16,
                                          vertical: 10,
                                        ),
                                        decoration: BoxDecoration(
                                          color:
                                              KeenBenchTheme.colorSurfaceMuted,
                                          border: const Border(
                                            bottom: BorderSide(
                                              color: KeenBenchTheme
                                                  .colorBorderDefault,
                                            ),
                                          ),
                                        ),
                                        child: Row(
                                          children: [
                                            const _StatusChip(
                                              label: 'Draft',
                                              backgroundColor: KeenBenchTheme
                                                  .colorDraftIndicator,
                                            ),
                                            const SizedBox(width: 12),
                                            Expanded(
                                              child: Column(
                                                crossAxisAlignment:
                                                    CrossAxisAlignment.start,
                                                children: [
                                                  Text(
                                                    'Draft in progress. Review changes before publishing.',
                                                    style: Theme.of(
                                                      context,
                                                    ).textTheme.bodyMedium,
                                                  ),
                                                  if (draftMetadataText !=
                                                      null) ...[
                                                    const SizedBox(height: 2),
                                                    Text(
                                                      draftMetadataText,
                                                      key: AppKeys
                                                          .workbenchDraftMetadata,
                                                      style: Theme.of(context)
                                                          .textTheme
                                                          .bodySmall
                                                          ?.copyWith(
                                                            color: KeenBenchTheme
                                                                .colorTextSecondary,
                                                          ),
                                                    ),
                                                  ],
                                                ],
                                              ),
                                            ),
                                          ],
                                        ),
                                      ),
                                    Expanded(
                                      child: Padding(
                                        padding: const EdgeInsets.all(24),
                                        child: Column(
                                          crossAxisAlignment:
                                              CrossAxisAlignment.stretch,
                                          children: [
                                            Expanded(
                                              child: Container(
                                                decoration: BoxDecoration(
                                                  color: KeenBenchTheme
                                                      .colorBackgroundElevated,
                                                  borderRadius:
                                                      BorderRadius.circular(8),
                                                  border: Border.all(
                                                    color: KeenBenchTheme
                                                        .colorBorderSubtle,
                                                  ),
                                                ),
                                                child: ListView.builder(
                                                  key: AppKeys
                                                      .workbenchMessageList,
                                                  controller:
                                                      _messageScrollController,
                                                  padding: const EdgeInsets.all(
                                                    16,
                                                  ),
                                                  itemCount:
                                                      state.messages.length,
                                                  itemBuilder: (context, index) {
                                                    final message =
                                                        state.messages[index];
                                                    final checkpointId =
                                                        message.checkpointId ??
                                                        '';
                                                    final isRestoring =
                                                        checkpointId
                                                            .isNotEmpty &&
                                                        _restoringCheckpointId ==
                                                            checkpointId;
                                                    if (message
                                                        .isPublishCheckpointEvent) {
                                                      final canRestore =
                                                          !state.hasDraft &&
                                                          !isRestoring &&
                                                          checkpointId
                                                              .isNotEmpty &&
                                                          _restoringCheckpointId ==
                                                              null;
                                                      final disabledReason =
                                                          state.hasDraft
                                                          ? 'Publish or discard Draft to restore.'
                                                          : checkpointId.isEmpty
                                                          ? 'Checkpoint metadata unavailable.'
                                                          : _restoringCheckpointId !=
                                                                null
                                                          ? 'Restore in progress.'
                                                          : 'Restore this checkpoint.';
                                                      return _CheckpointEventCard(
                                                        message: message,
                                                        canRestore: canRestore,
                                                        isRestoring:
                                                            isRestoring,
                                                        disabledReason:
                                                            disabledReason,
                                                        onRestore: () =>
                                                            _restorePublishCheckpoint(
                                                              message,
                                                            ),
                                                      );
                                                    }
                                                    if (message.isSystemEvent) {
                                                      return _SystemEventItem(
                                                        message: message,
                                                      );
                                                    }
                                                    final isUser =
                                                        message.role == 'user';
                                                    final hasMessageId = message
                                                        .id
                                                        .trim()
                                                        .isNotEmpty;
                                                    return _ChatMessageBubble(
                                                      message: message,
                                                      isUser: isUser,
                                                      canRewind:
                                                          canRunConversationAction &&
                                                          hasMessageId,
                                                      canRegenerate:
                                                          canRunConversationAction &&
                                                          hasMessageId,
                                                      onRewind: () =>
                                                          _rewindToMessage(
                                                            message,
                                                          ),
                                                      onRegenerate: () =>
                                                          _regenerateFromMessage(
                                                            message,
                                                          ),
                                                    );
                                                  },
                                                ),
                                              ),
                                            ),
                                            const SizedBox(height: 12),
                                            if ((state.rateLimitWarning ?? '')
                                                .isNotEmpty)
                                              Semantics(
                                                liveRegion: true,
                                                label: state.rateLimitWarning!,
                                                child: Container(
                                                  key: AppKeys
                                                      .workbenchRateLimitWarning,
                                                  margin: const EdgeInsets.only(
                                                    bottom: 8,
                                                  ),
                                                  padding:
                                                      const EdgeInsets.symmetric(
                                                        horizontal: 10,
                                                        vertical: 8,
                                                      ),
                                                  decoration: BoxDecoration(
                                                    color: KeenBenchTheme
                                                        .colorWarningBackground,
                                                    borderRadius:
                                                        BorderRadius.circular(
                                                          6,
                                                        ),
                                                    border: Border.all(
                                                      color: KeenBenchTheme
                                                          .colorBorderDefault,
                                                    ),
                                                  ),
                                                  child: Row(
                                                    children: [
                                                      const Icon(
                                                        Icons
                                                            .warning_amber_rounded,
                                                        size: 16,
                                                        color: KeenBenchTheme
                                                            .colorWarningText,
                                                      ),
                                                      const SizedBox(width: 8),
                                                      Expanded(
                                                        child: Text(
                                                          state
                                                              .rateLimitWarning!,
                                                          maxLines: 2,
                                                          overflow: TextOverflow
                                                              .ellipsis,
                                                          style: Theme.of(context)
                                                              .textTheme
                                                              .bodySmall
                                                              ?.copyWith(
                                                                color: KeenBenchTheme
                                                                    .colorWarningText,
                                                              ),
                                                        ),
                                                      ),
                                                    ],
                                                  ),
                                                ),
                                              ),
                                            if (_phaseStatusLabel(state) !=
                                                null)
                                              Semantics(
                                                liveRegion: true,
                                                label: _phaseStatusLabel(state),
                                                child: Padding(
                                                  key: AppKeys
                                                      .workbenchPhaseStatus,
                                                  padding:
                                                      const EdgeInsets.only(
                                                        bottom: 8,
                                                      ),
                                                  child: Row(
                                                    children: [
                                                      const SizedBox(
                                                        width: 14,
                                                        height: 14,
                                                        child:
                                                            CircularProgressIndicator(
                                                              strokeWidth: 2,
                                                            ),
                                                      ),
                                                      const SizedBox(width: 8),
                                                      Expanded(
                                                        child: Text(
                                                          _phaseStatusLabel(
                                                            state,
                                                          )!,
                                                          maxLines: 2,
                                                          overflow: TextOverflow
                                                              .ellipsis,
                                                          style: Theme.of(context)
                                                              .textTheme
                                                              .bodySmall
                                                              ?.copyWith(
                                                                color: KeenBenchTheme
                                                                    .colorTextSecondary,
                                                              ),
                                                        ),
                                                      ),
                                                    ],
                                                  ),
                                                ),
                                              ),
                                            if (state.isToolExecuting)
                                              Semantics(
                                                liveRegion: true,
                                                label: _toolStatusLabel(
                                                  state.currentToolName ?? '',
                                                ),
                                                child: Padding(
                                                  key: AppKeys
                                                      .workbenchToolStatus,
                                                  padding:
                                                      const EdgeInsets.only(
                                                        bottom: 8,
                                                      ),
                                                  child: Row(
                                                    children: [
                                                      const SizedBox(
                                                        width: 14,
                                                        height: 14,
                                                        child:
                                                            CircularProgressIndicator(
                                                              strokeWidth: 2,
                                                            ),
                                                      ),
                                                      const SizedBox(width: 8),
                                                      Expanded(
                                                        child: Text(
                                                          _toolStatusLabel(
                                                            state.currentToolName ??
                                                                '',
                                                          ),
                                                          maxLines: 1,
                                                          overflow: TextOverflow
                                                              .ellipsis,
                                                          style: Theme.of(context)
                                                              .textTheme
                                                              .bodySmall
                                                              ?.copyWith(
                                                                color: KeenBenchTheme
                                                                    .colorTextSecondary,
                                                              ),
                                                        ),
                                                      ),
                                                    ],
                                                  ),
                                                ),
                                              ),
                                            if (!state.hasDraft)
                                              Container(
                                                padding: const EdgeInsets.all(
                                                  12,
                                                ),
                                                decoration: BoxDecoration(
                                                  color: KeenBenchTheme
                                                      .colorBackgroundElevated,
                                                  borderRadius:
                                                      BorderRadius.circular(8),
                                                  border: Border.all(
                                                    color: KeenBenchTheme
                                                        .colorBorderSubtle,
                                                  ),
                                                ),
                                                child: Row(
                                                  children: [
                                                    Expanded(
                                                      child: Focus(
                                                        onKeyEvent: (node, event) {
                                                          if (event
                                                                  is KeyDownEvent &&
                                                              event.logicalKey ==
                                                                  LogicalKeyboardKey
                                                                      .enter) {
                                                            if (HardwareKeyboard
                                                                .instance
                                                                .isShiftPressed) {
                                                              return KeyEventResult
                                                                  .ignored;
                                                            }
                                                            _submitComposer();
                                                            return KeyEventResult
                                                                .handled;
                                                          }
                                                          return KeyEventResult
                                                              .ignored;
                                                        },
                                                        child: TextField(
                                                          key: AppKeys
                                                              .workbenchComposerField,
                                                          focusNode:
                                                              _composerFocusNode,
                                                          controller:
                                                              _composerController,
                                                          minLines: 1,
                                                          maxLines: 3,
                                                          decoration: InputDecoration.collapsed(
                                                            hintText:
                                                                state.chatMode ==
                                                                    ChatMode
                                                                        .agent
                                                                ? 'Describe a task...'
                                                                : 'Ask a question...',
                                                          ),
                                                        ),
                                                      ),
                                                    ),
                                                    const SizedBox(width: 12),
                                                    _ChatModeToggle(
                                                      mode: state.chatMode,
                                                      onChanged:
                                                          state
                                                              .isConversationBusy
                                                          ? null
                                                          : state.setChatMode,
                                                    ),
                                                    const SizedBox(width: 8),
                                                    ElevatedButton(
                                                      key: AppKeys
                                                          .workbenchSendButton,
                                                      onPressed:
                                                          state.isApplyingDraft
                                                          ? null
                                                          : state
                                                                .isConversationBusy
                                                          ? _cancelActiveRun
                                                          : _submitComposer,
                                                      child:
                                                          state
                                                              .isConversationBusy
                                                          ? const Text('Cancel')
                                                          : const Text('Send'),
                                                    ),
                                                  ],
                                                ),
                                              )
                                            else
                                              Container(
                                                padding: const EdgeInsets.all(
                                                  12,
                                                ),
                                                decoration: BoxDecoration(
                                                  color: KeenBenchTheme
                                                      .colorBackgroundElevated,
                                                  borderRadius:
                                                      BorderRadius.circular(8),
                                                  border: Border.all(
                                                    color: KeenBenchTheme
                                                        .colorBorderSubtle,
                                                  ),
                                                ),
                                                child: Row(
                                                  children: [
                                                    Expanded(
                                                      child: Text(
                                                        'Draft in progress. Review opens automatically. Publish or discard to continue.',
                                                        style: Theme.of(
                                                          context,
                                                        ).textTheme.bodyMedium,
                                                      ),
                                                    ),
                                                    TextButton(
                                                      key: AppKeys
                                                          .workbenchReviewButton,
                                                      onPressed: () =>
                                                          _openReview(
                                                            autoOpened: false,
                                                          ),
                                                      child: const Text(
                                                        'Open review',
                                                      ),
                                                    ),
                                                    const SizedBox(width: 8),
                                                    TextButton(
                                                      key: AppKeys
                                                          .workbenchDiscardButton,
                                                      onPressed: () async {
                                                        final confirm =
                                                            await _confirmDiscardDraft();
                                                        if (!confirm) {
                                                          return;
                                                        }
                                                        try {
                                                          await state
                                                              .discardDraft();
                                                        } on EngineError catch (
                                                          err
                                                        ) {
                                                          await _handleEngineError(
                                                            err,
                                                            onRetry: () => state
                                                                .discardDraft(),
                                                          );
                                                        }
                                                      },
                                                      style: TextButton.styleFrom(
                                                        foregroundColor:
                                                            KeenBenchTheme
                                                                .colorErrorText,
                                                      ),
                                                      child: const Text(
                                                        'Discard',
                                                      ),
                                                    ),
                                                  ],
                                                ),
                                              ),
                                            const SizedBox(height: 12),
                                            if (state.isApplyingDraft)
                                              Row(
                                                children: const [
                                                  SizedBox(
                                                    width: 16,
                                                    height: 16,
                                                    child:
                                                        CircularProgressIndicator(
                                                          strokeWidth: 2,
                                                        ),
                                                  ),
                                                  SizedBox(width: 8),
                                                  Text(
                                                    'Applying draft changes...',
                                                  ),
                                                ],
                                              ),
                                          ],
                                        ),
                                      ),
                                    ),
                                  ],
                                ),
                              ),
                            ),
                          ],
                        ),
                      ),
                    ),
                  ],
                ),
              ),
            ),
          ),
        );
      },
    );
  }
}

class _WorkbenchFileRow extends StatelessWidget {
  const _WorkbenchFileRow({
    required this.file,
    required this.canExtract,
    required this.onExtract,
    required this.canRemove,
    required this.onRemove,
  });

  final WorkbenchFile file;
  final bool canExtract;
  final VoidCallback onExtract;
  final bool canRemove;
  final VoidCallback onRemove;

  String _semanticLabel(String extension, bool readOnly) {
    final parts = <String>[file.path, '${extension.toUpperCase()} file'];
    if (readOnly) {
      parts.add('read only');
    }
    if (file.isOpaque) {
      parts.add('opaque');
    }
    parts.add('${file.size} bytes');
    return parts.join(', ');
  }

  @override
  Widget build(BuildContext context) {
    final extension = _extensionLabel(file.path);
    final icon = _fileIcon(extension);
    final readOnly =
        file.fileKind == 'pdf' ||
        file.fileKind == 'image' ||
        file.fileKind == 'odt';
    return Semantics(
      container: true,
      label: _semanticLabel(extension, readOnly),
      child: Container(
        key: AppKeys.workbenchFileRow(file.path),
        padding: const EdgeInsets.all(10),
        decoration: BoxDecoration(
          color: KeenBenchTheme.colorSurfaceSubtle,
          borderRadius: BorderRadius.circular(6),
          border: Border.all(color: KeenBenchTheme.colorBorderSubtle),
        ),
        child: Row(
          children: [
            Container(
              width: 28,
              height: 28,
              decoration: BoxDecoration(
                color: KeenBenchTheme.colorSurfaceMuted,
                borderRadius: BorderRadius.circular(6),
              ),
              child: Icon(
                icon,
                size: 16,
                color: KeenBenchTheme.colorTextSecondary,
              ),
            ),
            const SizedBox(width: 8),
            Expanded(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Tooltip(
                    message: file.path,
                    waitDuration: const Duration(milliseconds: 250),
                    child: Text(
                      file.path,
                      style: Theme.of(context).textTheme.bodyMedium,
                      overflow: TextOverflow.ellipsis,
                      maxLines: 1,
                    ),
                  ),
                  const SizedBox(height: 4),
                  Wrap(
                    spacing: 6,
                    runSpacing: 4,
                    children: [
                      Container(
                        padding: const EdgeInsets.symmetric(
                          horizontal: 8,
                          vertical: 2,
                        ),
                        decoration: BoxDecoration(
                          color: KeenBenchTheme.colorBackgroundSelected,
                          borderRadius: BorderRadius.circular(999),
                        ),
                        child: Text(
                          extension.toUpperCase(),
                          style: Theme.of(context).textTheme.labelSmall
                              ?.copyWith(
                                color: KeenBenchTheme.colorTextSecondary,
                                fontWeight: FontWeight.w600,
                                letterSpacing: 0.6,
                              ),
                        ),
                      ),
                      if (readOnly)
                        const _TagChip(
                          label: 'Read-only',
                          backgroundColor: KeenBenchTheme.colorInfoBackground,
                          textColor: KeenBenchTheme.colorInfoText,
                        ),
                      if (file.isOpaque)
                        const _TagChip(
                          label: 'Opaque',
                          backgroundColor: KeenBenchTheme.colorSurfaceMuted,
                          textColor: KeenBenchTheme.colorTextSecondary,
                        ),
                    ],
                  ),
                ],
              ),
            ),
            const SizedBox(width: 6),
            Tooltip(
              message: canExtract
                  ? 'Extract this file'
                  : 'Publish or discard the Draft to extract files.',
              child: IconButton(
                key: AppKeys.workbenchFileExtractButton(file.path),
                onPressed: canExtract ? onExtract : null,
                icon: const Icon(Icons.download_outlined),
                iconSize: 16,
                padding: EdgeInsets.zero,
                constraints: const BoxConstraints.tightFor(
                  width: 28,
                  height: 28,
                ),
                color: KeenBenchTheme.colorTextSecondary,
                disabledColor: KeenBenchTheme.colorTextTertiary,
                hoverColor: KeenBenchTheme.colorBackgroundHover,
                splashRadius: 18,
              ),
            ),
            const SizedBox(width: 2),
            Tooltip(
              message: canRemove
                  ? 'Remove file'
                  : 'Publish or discard the Draft to remove files.',
              child: IconButton(
                key: AppKeys.workbenchFileRemoveButton(file.path),
                onPressed: canRemove ? onRemove : null,
                icon: const Icon(Icons.delete_outline),
                iconSize: 16,
                padding: EdgeInsets.zero,
                constraints: const BoxConstraints.tightFor(
                  width: 28,
                  height: 28,
                ),
                color: KeenBenchTheme.colorTextSecondary,
                disabledColor: KeenBenchTheme.colorTextTertiary,
                hoverColor: KeenBenchTheme.colorBackgroundHover,
                splashRadius: 18,
              ),
            ),
          ],
        ),
      ),
    );
  }
}

class _ChatMessageBubble extends StatefulWidget {
  const _ChatMessageBubble({
    required this.message,
    required this.isUser,
    required this.canRewind,
    required this.canRegenerate,
    required this.onRewind,
    required this.onRegenerate,
  });

  final ChatMessage message;
  final bool isUser;
  final bool canRewind;
  final bool canRegenerate;
  final VoidCallback onRewind;
  final VoidCallback onRegenerate;

  @override
  State<_ChatMessageBubble> createState() => _ChatMessageBubbleState();
}

class _ChatMessageBubbleState extends State<_ChatMessageBubble> {
  bool _hovered = false;

  String _messageToken() {
    if (widget.message.id.isNotEmpty) {
      return widget.message.id;
    }
    return widget.message.text.hashCode.toString();
  }

  void _copyToClipboard(BuildContext context) {
    if (widget.message.text.isEmpty) {
      return;
    }
    Clipboard.setData(ClipboardData(text: widget.message.text));
    A11yAnnouncer.instance.announce(context, 'Copied to clipboard.');
    ScaffoldMessenger.of(
      context,
    ).showSnackBar(const SnackBar(content: Text('Copied to clipboard')));
  }

  MarkdownStyleSheet _markdownStyle(BuildContext context) {
    final theme = Theme.of(context);
    return MarkdownStyleSheet.fromTheme(theme).copyWith(
      p: theme.textTheme.bodyMedium,
      h1: theme.textTheme.headlineLarge,
      h2: theme.textTheme.headlineMedium,
      h3: theme.textTheme.headlineSmall,
      code: KeenBenchTheme.mono.copyWith(
        backgroundColor: KeenBenchTheme.colorSurfaceMuted,
      ),
      codeblockPadding: const EdgeInsets.all(12),
      codeblockDecoration: BoxDecoration(
        color: KeenBenchTheme.colorSurfaceSubtle,
        borderRadius: BorderRadius.circular(6),
        border: Border.all(color: KeenBenchTheme.colorBorderSubtle),
      ),
      blockquoteDecoration: BoxDecoration(
        color: KeenBenchTheme.colorSurfaceMuted,
        border: const Border(
          left: BorderSide(color: KeenBenchTheme.colorBorderStrong, width: 3),
        ),
      ),
    );
  }

  @override
  Widget build(BuildContext context) {
    final footerMetadata = <String>[];
    final timestampLabel = _formatEventTimestamp(widget.message.createdAt);
    if (timestampLabel.isNotEmpty) {
      footerMetadata.add(timestampLabel);
    }
    final elapsedLabel = _formatElapsedLabel(widget.message.jobElapsedMs);
    if (elapsedLabel != null) {
      footerMetadata.add(elapsedLabel);
    }
    final footerText = footerMetadata.isEmpty
        ? null
        : footerMetadata.join(' · ');
    final bubbleColor = widget.isUser
        ? KeenBenchTheme.colorInfoBackground
        : KeenBenchTheme.colorSurfaceSubtle;
    final borderColor = widget.isUser
        ? KeenBenchTheme.colorInfoBorder
        : KeenBenchTheme.colorBorderSubtle;
    return Align(
      alignment: widget.isUser ? Alignment.centerRight : Alignment.centerLeft,
      child: MouseRegion(
        onEnter: (_) => setState(() => _hovered = true),
        onExit: (_) => setState(() => _hovered = false),
        child: Container(
          margin: const EdgeInsets.symmetric(vertical: 6),
          constraints: const BoxConstraints(maxWidth: 520),
          decoration: BoxDecoration(
            color: bubbleColor,
            borderRadius: BorderRadius.circular(8),
            border: Border.all(color: borderColor),
          ),
          child: Stack(
            children: [
              Padding(
                padding: const EdgeInsets.fromLTRB(12, 12, 108, 30),
                child: SelectionArea(
                  child: widget.isUser
                      ? Text(
                          widget.message.text,
                          style: Theme.of(context).textTheme.bodyMedium,
                        )
                      : MarkdownBody(
                          data: widget.message.text,
                          selectable: false,
                          styleSheet: _markdownStyle(context),
                          onTapLink: (text, href, title) {},
                        ),
                ),
              ),
              if (footerText != null)
                Positioned(
                  left: 8,
                  bottom: 6,
                  child: Text(
                    footerText,
                    style: Theme.of(context).textTheme.labelSmall?.copyWith(
                      color: KeenBenchTheme.colorTextSecondary,
                    ),
                  ),
                ),
              Positioned(
                right: 6,
                bottom: 6,
                child: IgnorePointer(
                  ignoring: !_hovered,
                  child: AnimatedOpacity(
                    duration:
                        MediaQuery.maybeOf(context)?.disableAnimations == true
                        ? Duration.zero
                        : const Duration(milliseconds: 120),
                    opacity: _hovered ? 1 : 0,
                    child: Row(
                      mainAxisSize: MainAxisSize.min,
                      children: [
                        IconButton(
                          key: AppKeys.workbenchMessageRewindButton(
                            _messageToken(),
                          ),
                          onPressed: widget.canRewind ? widget.onRewind : null,
                          icon: const Icon(Icons.undo),
                          iconSize: 16,
                          padding: EdgeInsets.zero,
                          constraints: const BoxConstraints.tightFor(
                            width: 28,
                            height: 28,
                          ),
                          color: KeenBenchTheme.colorTextSecondary,
                          disabledColor: KeenBenchTheme.colorTextTertiary,
                          hoverColor: KeenBenchTheme.colorBackgroundHover,
                          splashRadius: 18,
                          tooltip: 'Rewind',
                        ),
                        const SizedBox(width: 2),
                        IconButton(
                          key: AppKeys.workbenchMessageRegenerateButton(
                            _messageToken(),
                          ),
                          onPressed: widget.canRegenerate
                              ? widget.onRegenerate
                              : null,
                          icon: const Icon(Icons.refresh),
                          iconSize: 16,
                          padding: EdgeInsets.zero,
                          constraints: const BoxConstraints.tightFor(
                            width: 28,
                            height: 28,
                          ),
                          color: KeenBenchTheme.colorTextSecondary,
                          disabledColor: KeenBenchTheme.colorTextTertiary,
                          hoverColor: KeenBenchTheme.colorBackgroundHover,
                          splashRadius: 18,
                          tooltip: 'Regenerate',
                        ),
                        const SizedBox(width: 2),
                        IconButton(
                          key: AppKeys.workbenchMessageCopyButton(
                            _messageToken(),
                          ),
                          onPressed: () => _copyToClipboard(context),
                          icon: const Icon(Icons.content_copy),
                          iconSize: 16,
                          padding: EdgeInsets.zero,
                          constraints: const BoxConstraints.tightFor(
                            width: 28,
                            height: 28,
                          ),
                          color: KeenBenchTheme.colorTextSecondary,
                          hoverColor: KeenBenchTheme.colorBackgroundHover,
                          splashRadius: 18,
                          tooltip: 'Copy',
                        ),
                      ],
                    ),
                  ),
                ),
              ),
            ],
          ),
        ),
      ),
    );
  }
}

class _SystemEventItem extends StatelessWidget {
  const _SystemEventItem({required this.message});

  final ChatMessage message;

  @override
  Widget build(BuildContext context) {
    final eventText = message.text.trim().isNotEmpty
        ? message.text.trim()
        : message.isRestoreCheckpointEvent
        ? 'Checkpoint restored.'
        : 'System event';
    return Padding(
      padding: const EdgeInsets.symmetric(vertical: 8),
      child: Row(
        children: [
          const Expanded(
            child: Divider(color: KeenBenchTheme.colorBorderSubtle),
          ),
          const SizedBox(width: 10),
          Flexible(
            child: Text(
              eventText,
              textAlign: TextAlign.center,
              style: Theme.of(context).textTheme.labelMedium?.copyWith(
                color: KeenBenchTheme.colorTextSecondary,
              ),
            ),
          ),
          const SizedBox(width: 10),
          const Expanded(
            child: Divider(color: KeenBenchTheme.colorBorderSubtle),
          ),
        ],
      ),
    );
  }
}

class _CheckpointEventCard extends StatelessWidget {
  const _CheckpointEventCard({
    required this.message,
    required this.canRestore,
    required this.isRestoring,
    required this.disabledReason,
    required this.onRestore,
  });

  final ChatMessage message;
  final bool canRestore;
  final bool isRestoring;
  final String disabledReason;
  final VoidCallback onRestore;

  String _eventToken() {
    final checkpointId = message.checkpointId?.trim() ?? '';
    if (checkpointId.isNotEmpty) {
      return checkpointId;
    }
    final fallback = '${message.id}|${message.createdAt}|${message.text}';
    return fallback.hashCode.toString();
  }

  @override
  Widget build(BuildContext context) {
    final checkpointId = message.checkpointId?.trim() ?? '';
    final timestamp = message.checkpointCreatedAt?.trim() ?? '';
    final description = message.checkpointDescription?.trim() ?? '';
    final messageText = message.text.trim().isNotEmpty
        ? message.text.trim()
        : 'Publish checkpoint created.';
    final eventToken = _eventToken();
    return Container(
      key: AppKeys.workbenchCheckpointEventCard(eventToken),
      margin: const EdgeInsets.symmetric(vertical: 8),
      padding: const EdgeInsets.all(12),
      decoration: BoxDecoration(
        color: KeenBenchTheme.colorBackgroundSecondary,
        borderRadius: BorderRadius.circular(8),
        border: Border.all(color: KeenBenchTheme.colorBorderSubtle),
      ),
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Container(
            width: 30,
            height: 30,
            decoration: BoxDecoration(
              color: KeenBenchTheme.colorSurfaceMuted,
              borderRadius: BorderRadius.circular(6),
            ),
            child: const Icon(
              Icons.bookmark_border,
              size: 18,
              color: KeenBenchTheme.colorTextSecondary,
            ),
          ),
          const SizedBox(width: 10),
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text(
                  messageText,
                  style: Theme.of(context).textTheme.bodyMedium,
                ),
                const SizedBox(height: 4),
                Wrap(
                  spacing: 8,
                  runSpacing: 4,
                  children: [
                    if (checkpointId.isNotEmpty)
                      Text(
                        'ID: $checkpointId',
                        style: Theme.of(context).textTheme.labelSmall?.copyWith(
                          color: KeenBenchTheme.colorTextSecondary,
                        ),
                      ),
                    if (timestamp.isNotEmpty)
                      Text(
                        _formatEventTimestamp(timestamp),
                        style: Theme.of(context).textTheme.labelSmall?.copyWith(
                          color: KeenBenchTheme.colorTextSecondary,
                        ),
                      ),
                    if (description.isNotEmpty)
                      Text(
                        description,
                        style: Theme.of(context).textTheme.labelSmall?.copyWith(
                          color: KeenBenchTheme.colorTextSecondary,
                        ),
                      ),
                  ],
                ),
              ],
            ),
          ),
          const SizedBox(width: 10),
          Tooltip(
            message: disabledReason,
            child: TextButton(
              key: AppKeys.workbenchCheckpointRestoreButton(eventToken),
              onPressed: canRestore ? onRestore : null,
              child: isRestoring
                  ? const SizedBox(
                      width: 14,
                      height: 14,
                      child: CircularProgressIndicator(strokeWidth: 2),
                    )
                  : const Text('Restore'),
            ),
          ),
        ],
      ),
    );
  }
}

class _StatusChip extends StatelessWidget {
  const _StatusChip({required this.label, required this.backgroundColor});

  final String label;
  final Color backgroundColor;

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 4),
      decoration: BoxDecoration(
        color: backgroundColor,
        borderRadius: BorderRadius.circular(999),
      ),
      child: Text(
        label.toUpperCase(),
        style: Theme.of(context).textTheme.labelSmall?.copyWith(
          color: KeenBenchTheme.colorTextPrimary,
          fontWeight: FontWeight.w600,
          letterSpacing: 0.6,
        ),
      ),
    );
  }
}

class _TagChip extends StatelessWidget {
  const _TagChip({
    required this.label,
    required this.backgroundColor,
    required this.textColor,
  });

  final String label;
  final Color backgroundColor;
  final Color textColor;

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 2),
      decoration: BoxDecoration(
        color: backgroundColor,
        borderRadius: BorderRadius.circular(999),
      ),
      child: Text(
        label.toUpperCase(),
        style: Theme.of(context).textTheme.labelSmall?.copyWith(
          color: textColor,
          fontWeight: FontWeight.w600,
          letterSpacing: 0.6,
        ),
      ),
    );
  }
}

class _ChatModeToggle extends StatelessWidget {
  const _ChatModeToggle({required this.mode, required this.onChanged});

  final ChatMode mode;
  final void Function(ChatMode)? onChanged;

  @override
  Widget build(BuildContext context) {
    return ToggleButtons(
      key: AppKeys.workbenchChatModeToggle,
      isSelected: [mode == ChatMode.ask, mode == ChatMode.agent],
      onPressed: onChanged == null
          ? null
          : (index) => onChanged!(index == 0 ? ChatMode.ask : ChatMode.agent),
      borderRadius: BorderRadius.circular(6),
      constraints: const BoxConstraints(minWidth: 52, minHeight: 32),
      textStyle: Theme.of(context).textTheme.labelSmall,
      children: const [Text('Ask'), Text('Agent')],
    );
  }
}

String _formatEventTimestamp(String value) {
  final trimmed = value.trim();
  if (trimmed.isEmpty) {
    return '';
  }
  final parsed = DateTime.tryParse(trimmed);
  if (parsed == null) {
    return trimmed;
  }
  final local = parsed.toLocal();
  final twoDigits = (int n) => n.toString().padLeft(2, '0');
  final date =
      '${local.year}-${twoDigits(local.month)}-${twoDigits(local.day)}';
  final time =
      '${twoDigits(local.hour)}:${twoDigits(local.minute)}:${twoDigits(local.second)}';
  return '$date $time';
}

String? _formatElapsedLabel(int? elapsedMs) {
  if (elapsedMs == null || elapsedMs < 0) {
    return null;
  }
  final totalSeconds = elapsedMs ~/ 1000;
  if (totalSeconds < 60) {
    return 'Elapsed: ${totalSeconds}s';
  }
  final hours = totalSeconds ~/ 3600;
  final minutes = (totalSeconds % 3600) ~/ 60;
  final seconds = totalSeconds % 60;
  if (hours > 0) {
    return 'Elapsed: ${hours}h ${minutes}m ${seconds}s';
  }
  return 'Elapsed: ${minutes}m ${seconds}s';
}

String? _phaseStatusLabel(WorkbenchState state) {
  switch (state.currentPhase) {
    case 'research':
      return 'Analyzing files...';
    case 'plan':
      return 'Planning approach...';
    case 'implement':
      final current = state.implementCurrentItem;
      final total = state.implementTotalItems;
      final label = state.implementItemLabel?.trim() ?? '';
      if (current != null && total != null && total > 0) {
        if (label.isNotEmpty) {
          return 'Working on step $current of $total: $label';
        }
        return 'Working on step $current of $total...';
      }
      return 'Implementing changes...';
    case 'summary':
      return 'Summarizing results...';
    default:
      return null;
  }
}

String _toolStatusLabel(String toolName) {
  switch (toolName) {
    case 'read_file':
      return 'Reading file...';
    case 'write_file':
      return 'Writing file...';
    case 'list_files':
      return 'Listing files...';
    case 'read_file_chunk':
      return 'Reading file chunk...';
    case 'get_file_map':
      return 'Mapping file structure...';
    case 'xlsx_get_styles':
    case 'docx_get_styles':
    case 'pptx_get_styles':
      return 'Inspecting styles...';
    case 'xlsx_copy_assets':
    case 'docx_copy_assets':
    case 'pptx_copy_assets':
      return 'Copying styles/assets...';
    default:
      if (toolName.isEmpty) {
        return 'Running tool...';
      }
      return 'Running $toolName...';
  }
}

String _buildScopeLimitsText(WorkbenchScope? scope, int currentFileCount) {
  if (scope == null) {
    return '';
  }
  final parts = <String>[];
  if (scope.limits.maxFiles > 0) {
    parts.add('Files: $currentFileCount/${scope.limits.maxFiles}');
  }
  if (scope.limits.maxFileBytes > 0) {
    parts.add('Max file size: ${_formatBytes(scope.limits.maxFileBytes)}');
  }
  if (scope.supportedTypes.isNotEmpty) {
    final maxTypes = scope.supportedTypes.length > 4
        ? '${scope.supportedTypes.take(4).join(', ')}, ...'
        : scope.supportedTypes.join(', ');
    parts.add('Types: $maxTypes');
  }
  return parts.join(' · ');
}

String? _buildDraftMetadataText(WorkbenchState state) {
  if (!state.hasDraft) {
    return null;
  }
  final parts = <String>[];
  final sourceText = _formatDraftSource(state.draftSource);
  if (sourceText != null) {
    parts.add('Source: $sourceText');
  }
  final createdAt = state.draftCreatedAt?.trim() ?? '';
  if (createdAt.isNotEmpty) {
    parts.add('Created: ${_formatEventTimestamp(createdAt)}');
  }
  if (parts.isEmpty) {
    return null;
  }
  return parts.join(' · ');
}

String? _formatDraftSource(DraftSource? source) {
  if (source == null) {
    return null;
  }
  final kind = source.kind.trim();
  if (kind.isEmpty) {
    return null;
  }
  switch (kind.toLowerCase()) {
    case 'workshop':
      return 'Workshop';
    case 'system':
      return 'System';
    default:
      final normalized = kind[0].toUpperCase() + kind.substring(1);
      return normalized;
  }
}

String _formatBytes(int bytes) {
  if (bytes >= 1024 * 1024) {
    final value = bytes / (1024 * 1024);
    final text = value >= 10
        ? value.toStringAsFixed(0)
        : value.toStringAsFixed(1);
    return '$text MB';
  }
  if (bytes >= 1024) {
    final value = bytes / 1024;
    final text = value >= 10
        ? value.toStringAsFixed(0)
        : value.toStringAsFixed(1);
    return '$text KB';
  }
  return '$bytes B';
}

String _extensionLabel(String path) {
  final parts = path.split('.');
  if (parts.length < 2) {
    return 'file';
  }
  return parts.last.toLowerCase();
}

IconData _fileIcon(String ext) {
  switch (ext) {
    case 'csv':
      return Icons.table_chart_outlined;
    case 'md':
      return Icons.text_snippet_outlined;
    case 'txt':
    default:
      return Icons.description_outlined;
  }
}
