import 'package:flutter/material.dart';

import 'app_keys.dart';
import 'engine/engine_client.dart';
import 'models/models.dart';
import 'theme.dart';
import 'widgets/dialog_keyboard_shortcuts.dart';

const consentModeAsk = 'ask';
const consentModeAllowAll = 'allow_all';

class ConsentDecision {
  const ConsentDecision({required this.granted, required this.persist});

  final bool granted;
  final bool persist;
}

class EgressConsentStatus {
  const EgressConsentStatus({
    required this.consented,
    required this.scopeHash,
    required this.providerId,
    required this.modelId,
    required this.mode,
  });

  final bool consented;
  final String scopeHash;
  final String providerId;
  final String modelId;
  final String mode;

  bool get allowAllMode => mode == consentModeAllowAll;
}

Future<EgressConsentStatus> fetchEgressConsentStatus(
  EngineApi engine,
  String workbenchId,
) async {
  final response = await engine.call('EgressGetConsentStatus', {
    'workbench_id': workbenchId,
  });
  return EgressConsentStatus(
    consented: response['consented'] == true,
    scopeHash: response['scope_hash'] as String? ?? '',
    providerId: response['provider_id'] as String? ?? '',
    modelId: response['model_id'] as String? ?? '',
    mode: response['mode'] as String? ?? consentModeAsk,
  );
}

Future<bool> ensureEgressConsentForWorkbench({
  required BuildContext context,
  required EngineApi engine,
  required String workbenchId,
  required List<WorkbenchFile> files,
  required List<ModelInfo> models,
  required List<ProviderStatus> providers,
}) async {
  final status = await fetchEgressConsentStatus(engine, workbenchId);
  if (status.consented) {
    return true;
  }
  final decision = await showEgressConsentDialog(
    context,
    files: files,
    scopeHash: status.scopeHash,
    providerId: status.providerId,
    modelId: status.modelId,
    models: models,
    providers: providers,
  );
  if (decision == null || !decision.granted) {
    return false;
  }
  await engine.call('EgressGrantWorkshopConsent', {
    'workbench_id': workbenchId,
    'provider_id': status.providerId,
    'model_id': status.modelId,
    'scope_hash': status.scopeHash,
    'persist': decision.persist,
  });
  return true;
}

Future<ConsentDecision?> showEgressConsentDialog(
  BuildContext context, {
  required List<WorkbenchFile> files,
  required String scopeHash,
  required String providerId,
  required String modelId,
  required List<ModelInfo> models,
  required List<ProviderStatus> providers,
}) async {
  final providerName = _providerNameFor(providerId, providers);
  final modelName = _modelNameFor(modelId, providerId, models);
  var persist = true;
  return showDialog<ConsentDecision>(
    context: context,
    barrierColor: KeenBenchTheme.colorSurfaceOverlay,
    builder: (dialogContext) => StatefulBuilder(
      builder: (dialogContext, setState) {
        void cancel() => Navigator.of(
          dialogContext,
        ).pop(const ConsentDecision(granted: false, persist: false));

        void submit() => Navigator.of(
          dialogContext,
        ).pop(ConsentDecision(granted: true, persist: persist));

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
                            style: Theme.of(dialogContext).textTheme.bodySmall,
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

String _providerNameFor(String providerId, List<ProviderStatus> providers) {
  final normalizedId = providerId.trim();
  if (normalizedId.isEmpty) {
    return 'Provider';
  }
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

String _modelNameFor(
  String modelId,
  String providerId,
  List<ModelInfo> models,
) {
  final normalizedModelId = modelId.trim();
  if (normalizedModelId.isEmpty) {
    return providerId.trim().isEmpty ? 'Model' : providerId.trim();
  }
  for (final model in models) {
    if (model.id == normalizedModelId) {
      final displayName = model.displayName.trim();
      if (displayName.isNotEmpty) {
        return displayName;
      }
      break;
    }
  }
  return normalizedModelId;
}
