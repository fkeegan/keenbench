import 'dart:async';

import 'package:flutter/foundation.dart';

import '../engine/engine_client.dart';
import '../logging.dart';
import '../models/models.dart';

enum ChatMode { ask, agent }

class WorkbenchState extends ChangeNotifier {
  WorkbenchState({required this.engine, required this.workbenchId});

  final EngineApi engine;
  final String workbenchId;

  Workbench? workbench;
  List<WorkbenchFile> files = [];
  List<ChatMessage> messages = [];
  List<ModelInfo> models = [];
  List<ProviderStatus> providers = [];
  WorkbenchScope? scope;
  bool hasDraft = false;
  String? draftId;
  String? draftCreatedAt;
  DraftSource? draftSource;
  String? activeModelId;
  String? defaultModelId;
  ClutterState? clutter;
  List<ContextItemSummary> contextItems = [];
  ContextItem? selectedContextItem;
  List<ContextArtifactFile> selectedContextFiles = [];
  String? contextError;
  bool isContextLoading = false;
  bool isSending = false;
  bool isApplyingDraft = false;
  bool isConversationActionInFlight = false;

  String? currentToolName;
  String? currentToolCallId;
  bool get isToolExecuting => currentToolName != null;

  String? currentPhase;
  int? implementCurrentItem;
  int? implementTotalItems;
  String? implementItemLabel;
  String? rateLimitWarning;

  ChatMode chatMode = ChatMode.agent;

  bool get isConversationBusy => isSending || isConversationActionInFlight;

  StreamSubscription<EngineNotification>? _subscription;

  Future<void> load() async {
    AppLog.info('workbench.load', {'workbench_id': workbenchId});
    await _ensureSubscribed();
    final workbenchResponse = await engine.call('WorkbenchOpen', {
      'workbench_id': workbenchId,
    });
    workbench = Workbench.fromJson(
      workbenchResponse['workbench'] as Map<String, dynamic>,
    );
    defaultModelId = workbench?.defaultModelId;

    final workshopState = await engine.call('WorkshopGetState', {
      'workbench_id': workbenchId,
    });
    activeModelId = workshopState['active_model_id'] as String?;
    defaultModelId =
        workshopState['default_model_id'] as String? ?? defaultModelId;

    final providersResponse = await engine.call('ProvidersGetStatus');
    final providerList =
        (providersResponse['providers'] as List<dynamic>? ?? [])
            .cast<Map<String, dynamic>>();
    providers = providerList.map(ProviderStatus.fromJson).toList();

    final modelsResponse = await engine.call('ModelsListSupported');
    final modelList = (modelsResponse['models'] as List<dynamic>? ?? [])
        .cast<Map<String, dynamic>>();
    models = modelList.map(ModelInfo.fromJson).toList();
    await _reconcileActiveModel();

    final filesResponse = await engine.call('WorkbenchFilesList', {
      'workbench_id': workbenchId,
    });
    final fileList = (filesResponse['files'] as List<dynamic>? ?? [])
        .cast<Map<String, dynamic>>();
    files = fileList.map(WorkbenchFile.fromJson).toList();

    await refreshScope(notify: false);
    await loadContextItems(notify: false);

    final convoResponse = await engine.call('WorkshopGetConversation', {
      'workbench_id': workbenchId,
    });
    messages = _conversationMessagesFromResponse(convoResponse);

    final stateResponse = await engine.call('DraftGetState', {
      'workbench_id': workbenchId,
    });
    _applyDraftMetadata(DraftMetadata.fromJson(_asMap(stateResponse)));

    final clutterResponse = await engine.call('WorkbenchGetClutter', {
      'workbench_id': workbenchId,
    });
    clutter = ClutterState.fromJson(clutterResponse as Map<String, dynamic>);

    AppLog.debug('workbench.loaded', {
      'workbench_id': workbenchId,
      'files': files.length,
      'messages': messages.length,
      'has_draft': hasDraft,
      'scope_loaded': scope != null,
    });
    notifyListeners();
  }

  Future<void> refreshScope({bool notify = true}) async {
    try {
      final scopeResponse = await engine.call('WorkbenchGetScope', {
        'workbench_id': workbenchId,
      });
      final scopeJson = _asMap(scopeResponse);
      if (scopeJson.isNotEmpty) {
        scope = WorkbenchScope.fromJson(scopeJson);
      }
    } on EngineError catch (err) {
      if (_isMethodUnavailable(err)) {
        AppLog.debug('workbench.scope_not_supported', {
          'workbench_id': workbenchId,
          'message': err.message,
        });
      } else {
        AppLog.warn('workbench.scope_refresh_failed', {
          'workbench_id': workbenchId,
          'error_code': err.errorCode,
          'message': err.message,
        });
      }
    }
    if (notify) {
      notifyListeners();
    }
  }

  Future<void> refreshFiles() async {
    final filesResponse = await engine.call('WorkbenchFilesList', {
      'workbench_id': workbenchId,
    });
    final fileList = (filesResponse['files'] as List<dynamic>? ?? [])
        .cast<Map<String, dynamic>>();
    files = fileList.map(WorkbenchFile.fromJson).toList();
    AppLog.debug('workbench.files_refreshed', {
      'workbench_id': workbenchId,
      'count': files.length,
    });
    await refreshClutter();
    notifyListeners();
  }

  Future<void> refreshConversation() async {
    final convoResponse = await engine.call('WorkshopGetConversation', {
      'workbench_id': workbenchId,
    });
    messages = _conversationMessagesFromResponse(convoResponse);
    AppLog.debug('workbench.conversation_refreshed', {
      'workbench_id': workbenchId,
      'count': messages.length,
    });
    notifyListeners();
  }

  List<ChatMessage> _conversationMessagesFromResponse(dynamic response) {
    final convoList = (response['messages'] as List<dynamic>? ?? [])
        .cast<Map<String, dynamic>>();
    return convoList
        .map(ChatMessage.fromJson)
        .where((message) => message.shouldRenderInConversation)
        .toList();
  }

  Future<void> refreshClutter() async {
    final response = await engine.call('WorkbenchGetClutter', {
      'workbench_id': workbenchId,
    });
    clutter = ClutterState.fromJson(response as Map<String, dynamic>);
  }

  Future<void> _reconcileActiveModel() async {
    final providerMap = <String, ProviderStatus>{
      for (final provider in providers) provider.id: provider,
    };
    final availableModelMap = <String, ModelInfo>{};
    for (final model in models) {
      final provider = providerMap[model.providerId];
      if (provider == null) {
        continue;
      }
      if (provider.enabled && provider.configured) {
        availableModelMap.putIfAbsent(model.id, () => model);
      }
    }
    final availableModels = availableModelMap.values.toList();
    if (availableModels.isEmpty) {
      return;
    }
    final currentActive = activeModelId?.trim() ?? '';
    final activeStillAvailable = availableModels.any(
      (model) => model.id == currentActive,
    );
    if (activeStillAvailable) {
      return;
    }
    final fallbackModelId = availableModels.first.id;
    try {
      await engine.call('WorkshopSetActiveModel', {
        'workbench_id': workbenchId,
        'model_id': fallbackModelId,
      });
      activeModelId = fallbackModelId;
      defaultModelId = fallbackModelId;
      AppLog.info('workbench.active_model_reconciled', {
        'workbench_id': workbenchId,
        'model_id': fallbackModelId,
      });
    } on EngineError catch (err) {
      AppLog.warn('workbench.active_model_reconcile_failed', {
        'workbench_id': workbenchId,
        'error_code': err.errorCode,
        'message': err.message,
      });
    }
  }

  Future<void> loadContextItems({bool notify = true}) async {
    isContextLoading = true;
    contextError = null;
    if (notify) {
      notifyListeners();
    }
    try {
      final response = await engine.call('ContextList', {
        'workbench_id': workbenchId,
      });
      final items = (response['items'] as List<dynamic>? ?? [])
          .cast<Map<String, dynamic>>();
      contextItems = items.map(ContextItemSummary.fromJson).toList();
    } catch (err) {
      contextError = err.toString();
    } finally {
      isContextLoading = false;
      if (notify) {
        notifyListeners();
      }
    }
  }

  Future<ContextItem> getContextItem(String category) async {
    final response = await engine.call('ContextGet', {
      'workbench_id': workbenchId,
      'category': category,
    });
    final item = ContextItem.fromJson(
      response['item'] as Map<String, dynamic>? ?? const {},
    );
    selectedContextItem = item;
    selectedContextFiles = item.files;
    notifyListeners();
    return item;
  }

  Future<ContextItem> processContextItem({
    required String category,
    required String mode,
    String text = '',
    String sourcePath = '',
    String note = '',
  }) async {
    final response = await engine.call('ContextProcess', {
      'workbench_id': workbenchId,
      'category': category,
      'input': {
        'mode': mode,
        'text': text,
        'source_path': sourcePath,
        'note': note,
      },
    });
    final item = ContextItem.fromJson(
      response['item'] as Map<String, dynamic>? ?? const {},
    );
    selectedContextItem = item;
    selectedContextFiles = item.files;
    await loadContextItems(notify: false);
    await refreshClutter();
    notifyListeners();
    return item;
  }

  Future<ContextItem> updateContextDirect(
    String category,
    List<ContextArtifactFile> files,
  ) async {
    final response = await engine.call('ContextUpdateDirect', {
      'workbench_id': workbenchId,
      'category': category,
      'files': files.map((file) => file.toJson()).toList(),
    });
    final item = ContextItem.fromJson(
      response['item'] as Map<String, dynamic>? ?? const {},
    );
    selectedContextItem = item;
    selectedContextFiles = item.files;
    await loadContextItems(notify: false);
    await refreshClutter();
    notifyListeners();
    return item;
  }

  Future<void> deleteContextItem(String category) async {
    await engine.call('ContextDelete', {
      'workbench_id': workbenchId,
      'category': category,
    });
    if (selectedContextItem?.category == category) {
      selectedContextItem = null;
      selectedContextFiles = [];
    }
    await loadContextItems(notify: false);
    await refreshClutter();
    notifyListeners();
  }

  Future<List<ContextArtifactFile>> getContextArtifact(String category) async {
    final response = await engine.call('ContextGetArtifact', {
      'workbench_id': workbenchId,
      'category': category,
    });
    final files = (response['files'] as List<dynamic>? ?? [])
        .cast<Map<String, dynamic>>()
        .map(ContextArtifactFile.fromJson)
        .toList();
    selectedContextFiles = files;
    notifyListeners();
    return files;
  }

  Future<void> refreshDraftState() async {
    final stateResponse = await engine.call('DraftGetState', {
      'workbench_id': workbenchId,
    });
    _applyDraftMetadata(DraftMetadata.fromJson(_asMap(stateResponse)));
    AppLog.debug('workbench.draft_state_refreshed', {
      'workbench_id': workbenchId,
      'has_draft': hasDraft,
      'draft_id': draftId,
      'created_at': draftCreatedAt,
      'source_kind': draftSource?.kind,
    });
    notifyListeners();
  }

  Future<void> refreshAfterCheckpointRestore() async {
    await refreshFiles();
    await refreshConversation();
    await refreshDraftState();
    await loadContextItems(notify: false);
    await refreshClutter();
    notifyListeners();
  }

  void setChatMode(ChatMode mode) {
    chatMode = mode;
    notifyListeners();
  }

  Future<void> sendMessage(String text) async {
    if (hasDraft) {
      AppLog.warn('workbench.send_blocked_draft', {
        'workbench_id': workbenchId,
      });
      return;
    }
    isSending = true;
    rateLimitWarning = null;
    notifyListeners();
    try {
      AppLog.info('workbench.send_message', {
        'workbench_id': workbenchId,
        'length': text.length,
      });
      AppLog.debug('workbench.send_message_payload', {
        'workbench_id': workbenchId,
        'text': text,
      });
      final response = await engine.call('WorkshopSendUserMessage', {
        'workbench_id': workbenchId,
        'text': text,
      });
      final messageId = response['message_id'] as String? ?? '';
      messages.add(ChatMessage(id: messageId, role: 'user', text: text));
      notifyListeners();

      if (chatMode == ChatMode.agent) {
        // Agentic flow: model has tools to explore and modify files
        final agentResponse = await engine.call('WorkshopRunAgent', {
          'workbench_id': workbenchId,
          'message_id': messageId,
        });
        await refreshConversation();
        if (agentResponse['has_draft'] == true) {
          hasDraft = true;
          await refreshDraftState();
        }
      } else {
        // Ask flow: simple streaming chat, no tools
        await engine.call('WorkshopStreamAssistantReply', {
          'workbench_id': workbenchId,
          'message_id': messageId,
        });
      }
    } finally {
      isSending = false;
      currentToolName = null;
      currentToolCallId = null;
      currentPhase = null;
      implementCurrentItem = null;
      implementTotalItems = null;
      implementItemLabel = null;
      rateLimitWarning = null;
      notifyListeners();
    }
  }

  Future<void> cancelRun() async {
    await engine.call('WorkshopCancelRun', {'workbench_id': workbenchId});
  }

  Future<void> setActiveModel(String modelId) async {
    await engine.call('WorkshopSetActiveModel', {
      'workbench_id': workbenchId,
      'model_id': modelId,
    });
    activeModelId = modelId;
    defaultModelId = modelId;
    await refreshClutter();
    notifyListeners();
  }

  Future<void> publishDraft() async {
    AppLog.info('workbench.publish_draft', {'workbench_id': workbenchId});
    await engine.call('DraftPublish', {'workbench_id': workbenchId});
    _clearDraftMetadata();
    await refreshFiles();
    notifyListeners();
  }

  Future<void> discardDraft() async {
    AppLog.info('workbench.discard_draft', {'workbench_id': workbenchId});
    await engine.call('DraftDiscard', {'workbench_id': workbenchId});
    _clearDraftMetadata();
    notifyListeners();
  }

  Future<void> undoToMessage(String messageId) async {
    final targetMessageId = messageId.trim();
    if (targetMessageId.isEmpty) {
      return;
    }
    isConversationActionInFlight = true;
    notifyListeners();
    try {
      AppLog.info('workbench.undo_to_message', {
        'workbench_id': workbenchId,
        'message_id': targetMessageId,
      });
      await engine.call('WorkshopUndoToMessage', {
        'workbench_id': workbenchId,
        'message_id': targetMessageId,
      });
      await refreshConversation();
      await refreshDraftState();
      await refreshFiles();
    } finally {
      isConversationActionInFlight = false;
      notifyListeners();
    }
  }

  Future<void> regenerate({String? messageId}) async {
    final targetMessageId = messageId?.trim();
    isConversationActionInFlight = true;
    notifyListeners();
    try {
      AppLog.info('workbench.regenerate', {
        'workbench_id': workbenchId,
        'message_id': targetMessageId,
      });
      await engine.call('WorkshopRegenerate', {
        'workbench_id': workbenchId,
        if (targetMessageId != null && targetMessageId.isNotEmpty)
          'message_id': targetMessageId,
      });
      await refreshConversation();
      await refreshDraftState();
      await refreshFiles();
    } finally {
      isConversationActionInFlight = false;
      notifyListeners();
    }
  }

  Future<void> restoreCheckpoint(String checkpointId) async {
    if (checkpointId.isEmpty) {
      return;
    }
    await engine.call('CheckpointRestore', {
      'workbench_id': workbenchId,
      'checkpoint_id': checkpointId,
    });
    await refreshAfterCheckpointRestore();
  }

  Future<void> addFiles(List<String> paths) async {
    if (paths.isEmpty) {
      return;
    }
    AppLog.info('workbench.add_files', {
      'workbench_id': workbenchId,
      'count': paths.length,
    });
    AppLog.debug('workbench.add_files_payload', {
      'workbench_id': workbenchId,
      'paths': paths,
    });
    await engine.call('WorkbenchFilesAdd', {
      'workbench_id': workbenchId,
      'source_paths': paths,
    });
    await refreshFiles();
  }

  Future<void> removeFiles(List<String> paths) async {
    if (paths.isEmpty) {
      return;
    }
    AppLog.info('workbench.remove_files', {
      'workbench_id': workbenchId,
      'count': paths.length,
    });
    AppLog.debug('workbench.remove_files_payload', {
      'workbench_id': workbenchId,
      'paths': paths,
    });
    await engine.call('WorkbenchFilesRemove', {
      'workbench_id': workbenchId,
      'workbench_paths': paths,
    });
    await refreshFiles();
  }

  Future<List<WorkbenchExtractResult>> extractFiles(
    String destinationDir, {
    List<String>? paths,
  }) async {
    final response = await engine.call('WorkbenchFilesExtract', {
      'workbench_id': workbenchId,
      'destination_dir': destinationDir,
      if (paths != null && paths.isNotEmpty) 'workbench_paths': paths,
    });
    final items = (response['extract_results'] as List<dynamic>? ?? [])
        .cast<Map<String, dynamic>>();
    return items.map(WorkbenchExtractResult.fromJson).toList();
  }

  Future<void> _ensureSubscribed() async {
    _subscription ??= engine.notifications.listen((notification) {
      final params = notification.params;
      if (params['workbench_id'] != workbenchId) {
        return;
      }
      switch (notification.method) {
        case 'WorkshopAssistantStreamDelta':
          final messageId = params['message_id'] as String? ?? '';
          final delta = params['token_delta'] as String? ?? '';
          if (messageId.isEmpty || delta.isEmpty) {
            return;
          }
          var existing = messages.where((m) => m.id == messageId).toList();
          if (existing.isEmpty) {
            final msg = ChatMessage(
              id: messageId,
              role: 'assistant',
              text: delta,
            );
            messages.add(msg);
          } else {
            existing.first.text += delta;
          }
          notifyListeners();
          break;
        case 'WorkbenchDraftStateChanged':
        case 'DraftStateChanged':
          _applyDraftNotification(params);
          notifyListeners();
          break;
        case 'WorkbenchFilesChanged':
          unawaited(refreshFiles());
          break;
        case 'WorkbenchClutterChanged':
          clutter = ClutterState.fromJson(params);
          notifyListeners();
          break;
        case 'ContextChanged':
          unawaited(_refreshContextFromNotification());
          break;
        case 'WorkshopModelChanged':
          activeModelId = params['model_id'] as String?;
          notifyListeners();
          break;
        case 'WorkshopToolExecuting':
          currentToolName = params['tool_name'] as String? ?? '';
          currentToolCallId = params['tool_call_id'] as String?;
          notifyListeners();
          break;
        case 'WorkshopToolComplete':
          currentToolName = null;
          currentToolCallId = null;
          notifyListeners();
          break;
        case 'WorkshopPhaseStarted':
          currentPhase = (params['phase'] as String?)?.trim();
          if (currentPhase != 'implement') {
            implementCurrentItem = null;
            implementTotalItems = null;
            implementItemLabel = null;
          }
          notifyListeners();
          break;
        case 'WorkshopImplementProgress':
          currentPhase = 'implement';
          implementCurrentItem = (params['current_item'] as num?)?.toInt();
          implementTotalItems = (params['total_items'] as num?)?.toInt();
          implementItemLabel = (params['item_label'] as String?)?.trim();
          notifyListeners();
          break;
        case 'WorkshopPhaseCompleted':
          currentPhase = null;
          implementCurrentItem = null;
          implementTotalItems = null;
          implementItemLabel = null;
          notifyListeners();
          break;
        case 'WorkshopRateLimitWarning':
          final retryAttempt = (params['retry_attempt'] as num?)?.toInt();
          final retryMax = (params['retry_max'] as num?)?.toInt();
          final waitMs = (params['wait_ms'] as num?)?.toInt();
          final providerId = (params['provider_id'] as String?)?.trim();
          final providerLabel = providerId == null || providerId.isEmpty
              ? 'provider'
              : providerId;
          final waitSeconds = waitMs == null ? 0 : (waitMs / 1000).ceil();
          if (retryAttempt != null && retryMax != null && retryMax > 0) {
            rateLimitWarning =
                'Rate limit hit on $providerLabel. Retrying ($retryAttempt/$retryMax) in ${waitSeconds}s. You can cancel.';
          } else {
            rateLimitWarning =
                'Rate limit hit on $providerLabel. Retrying shortly. You can cancel.';
          }
          notifyListeners();
          break;
        case 'WorkshopRunCancelRequested':
          rateLimitWarning = 'Cancel requested. Stopping run...';
          notifyListeners();
          break;
        default:
          break;
      }
    });
  }

  @override
  void dispose() {
    _subscription?.cancel();
    super.dispose();
  }

  void _applyDraftMetadata(DraftMetadata metadata) {
    hasDraft = metadata.hasDraft;
    draftId = metadata.draftId;
    draftCreatedAt = metadata.createdAt.isEmpty ? null : metadata.createdAt;
    draftSource = metadata.source;
    if (!hasDraft) {
      _clearDraftMetadata();
    }
  }

  void _applyDraftNotification(Map<String, dynamic> params) {
    hasDraft = params['has_draft'] == true;
    draftId = _normalizedString(params['draft_id']);
    if (!hasDraft) {
      _clearDraftMetadata();
      return;
    }
    final createdAt = _normalizedString(params['created_at']);
    if (createdAt != null) {
      draftCreatedAt = createdAt;
    }
    final sourceJson = _asMap(params['source']);
    if (sourceJson.isNotEmpty) {
      draftSource = DraftSource.fromJson(sourceJson);
      return;
    }
    final sourceKind = _normalizedString(params['source_kind']) ?? '';
    final sourceRef =
        _normalizedString(params['source_ref']) ??
        _normalizedString(params['source_job_id']);
    if (sourceKind.isNotEmpty || sourceRef != null) {
      draftSource = DraftSource(kind: sourceKind, jobId: sourceRef);
    }
  }

  void _clearDraftMetadata() {
    hasDraft = false;
    draftId = null;
    draftCreatedAt = null;
    draftSource = null;
  }

  bool _isMethodUnavailable(EngineError err) {
    final message = err.message.toLowerCase();
    return message.contains('method not found') ||
        err.errorCode == 'METHOD_NOT_FOUND';
  }

  Map<String, dynamic> _asMap(dynamic value) {
    if (value is Map<String, dynamic>) {
      return value;
    }
    if (value is Map) {
      final mapped = <String, dynamic>{};
      for (final entry in value.entries) {
        mapped[entry.key.toString()] = entry.value;
      }
      return mapped;
    }
    return const {};
  }

  String? _normalizedString(dynamic value) {
    if (value == null) {
      return null;
    }
    final text = value.toString().trim();
    return text.isEmpty ? null : text;
  }

  Future<void> _refreshContextFromNotification() async {
    await loadContextItems(notify: false);
    notifyListeners();
  }
}
