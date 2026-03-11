import 'package:flutter/material.dart';

import '../models/models.dart';
import '../theme.dart';
import 'dialog_keyboard_shortcuts.dart';

Future<String?> showModelPickerDialog(
  BuildContext context, {
  required List<ModelInfo> models,
  required String title,
  String? selectedModelId,
  int searchThreshold = 12,
}) {
  return showDialog<String>(
    context: context,
    barrierColor: KeenBenchTheme.colorSurfaceOverlay,
    builder: (dialogContext) {
      return _ModelPickerDialog(
        title: title,
        models: models,
        selectedModelId: selectedModelId,
        searchThreshold: searchThreshold,
      );
    },
  );
}

String modelPickerLabel(ModelInfo model) {
  final suffixes = <String>[];
  if (model.isFree) {
    suffixes.add('Free');
  }
  if (model.isAnalysisOnly) {
    suffixes.add('Analysis only');
  }
  if (suffixes.isEmpty) {
    return model.displayName;
  }
  return '${model.displayName} (${suffixes.join(' · ')})';
}

class _ModelPickerDialog extends StatefulWidget {
  const _ModelPickerDialog({
    required this.title,
    required this.models,
    required this.selectedModelId,
    required this.searchThreshold,
  });

  final String title;
  final List<ModelInfo> models;
  final String? selectedModelId;
  final int searchThreshold;

  @override
  State<_ModelPickerDialog> createState() => _ModelPickerDialogState();
}

class _ModelPickerDialogState extends State<_ModelPickerDialog> {
  late final TextEditingController _searchController;
  String _query = '';

  bool get _showSearch => widget.models.length > widget.searchThreshold;

  @override
  void initState() {
    super.initState();
    _searchController = TextEditingController();
  }

  @override
  void dispose() {
    _searchController.dispose();
    super.dispose();
  }

  List<ModelInfo> get _filteredModels {
    final query = _query.trim().toLowerCase();
    if (query.isEmpty) {
      return widget.models;
    }
    return widget.models.where((model) {
      return model.displayName.toLowerCase().contains(query) ||
          model.id.toLowerCase().contains(query);
    }).toList();
  }

  @override
  Widget build(BuildContext context) {
    final filteredModels = _filteredModels;
    return DialogKeyboardShortcuts(
      onCancel: () => Navigator.of(context).pop(),
      child: AlertDialog(
        title: Text(widget.title),
        content: SizedBox(
          width: 640,
          height: 560,
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              if (_showSearch) ...[
                TextField(
                  controller: _searchController,
                  autofocus: true,
                  onChanged: (value) {
                    setState(() {
                      _query = value;
                    });
                  },
                  decoration: const InputDecoration(
                    labelText: 'Search models',
                    hintText: 'Filter by name or model ID',
                    prefixIcon: Icon(Icons.search),
                  ),
                ),
                const SizedBox(height: 12),
              ],
              Expanded(
                child: DecoratedBox(
                  decoration: BoxDecoration(
                    border: Border.all(
                      color: KeenBenchTheme.colorBorderDefault,
                    ),
                    borderRadius: BorderRadius.circular(12),
                  ),
                  child: filteredModels.isEmpty
                      ? const Center(
                          child: Text('No models match the current search.'),
                        )
                      : ListView.separated(
                          itemCount: filteredModels.length,
                          separatorBuilder: (_, _) => const Divider(height: 1),
                          itemBuilder: (context, index) {
                            final model = filteredModels[index];
                            final isSelected =
                                model.id == widget.selectedModelId;
                            return ListTile(
                              selected: isSelected,
                              title: Text(model.displayName),
                              subtitle: Padding(
                                padding: const EdgeInsets.only(top: 6),
                                child: Wrap(
                                  spacing: 8,
                                  runSpacing: 8,
                                  crossAxisAlignment: WrapCrossAlignment.center,
                                  children: [
                                    Text(
                                      model.id,
                                      style: Theme.of(context)
                                          .textTheme
                                          .bodySmall
                                          ?.copyWith(
                                            color: KeenBenchTheme
                                                .colorTextSecondary,
                                          ),
                                    ),
                                    if (model.isFree)
                                      _ModelBadge(
                                        label: 'Free',
                                        foregroundColor:
                                            KeenBenchTheme.colorSuccessText,
                                      ),
                                    if (model.isAnalysisOnly)
                                      _ModelBadge(
                                        label: 'Analysis only',
                                        foregroundColor:
                                            KeenBenchTheme.colorWarningText,
                                      ),
                                  ],
                                ),
                              ),
                              trailing: isSelected
                                  ? const Icon(Icons.check, size: 18)
                                  : null,
                              onTap: () {
                                Navigator.of(context).pop(model.id);
                              },
                            );
                          },
                        ),
                ),
              ),
            ],
          ),
        ),
        actions: [
          OutlinedButton(
            onPressed: () => Navigator.of(context).pop(),
            child: const Text('Cancel'),
          ),
        ],
      ),
    );
  }
}

class _ModelBadge extends StatelessWidget {
  const _ModelBadge({required this.label, required this.foregroundColor});

  final String label;
  final Color foregroundColor;

  @override
  Widget build(BuildContext context) {
    return DecoratedBox(
      decoration: BoxDecoration(
        color: foregroundColor.withValues(alpha: 0.1),
        borderRadius: BorderRadius.circular(999),
      ),
      child: Padding(
        padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 3),
        child: Text(
          label,
          style: Theme.of(context).textTheme.labelSmall?.copyWith(
            color: foregroundColor,
            fontWeight: FontWeight.w600,
          ),
        ),
      ),
    );
  }
}
