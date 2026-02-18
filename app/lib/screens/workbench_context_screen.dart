import 'package:file_selector/file_selector.dart';
import 'package:flutter/material.dart';
import 'package:provider/provider.dart';

import '../app_keys.dart';
import '../models/models.dart';
import '../state/workbench_state.dart';
import '../theme.dart';
import '../widgets/keenbench_app_bar.dart';

class WorkbenchContextScreen extends StatefulWidget {
  const WorkbenchContextScreen({super.key});

  @override
  State<WorkbenchContextScreen> createState() => _WorkbenchContextScreenState();
}

class _WorkbenchContextScreenState extends State<WorkbenchContextScreen> {
  static const _categories = <String>[
    'company-context',
    'department-context',
    'situation',
    'document-style',
  ];

  static const _titles = <String, String>{
    'company-context': 'Company-Wide Information',
    'department-context': 'Department-Specific Information',
    'situation': 'Situation',
    'document-style': 'Document Style & Formatting',
  };

  static const _descriptions = <String, String>{
    'company-context':
        'Company mission, products, audience, and positioning context.',
    'department-context':
        'Department goals, KPIs, workflows, tools, and terminology.',
    'situation':
        'Current project, audience, constraints, deadlines, and priorities.',
    'document-style':
        'Style and formatting rules for created or edited deliverables.',
  };

  @override
  Widget build(BuildContext context) {
    return Consumer<WorkbenchState>(
      builder: (context, state, _) {
        final byCategory = <String, ContextItemSummary>{
          for (final item in state.contextItems) item.category: item,
        };
        return Scaffold(
          key: AppKeys.contextOverviewScreen,
          appBar: const KeenBenchAppBar(
            title: 'Workbench Context',
            showBack: true,
            useCenteredContent: false,
          ),
          body: Padding(
            padding: const EdgeInsets.all(24),
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text(
                  'Attach persistent context to improve consistency across Workshop calls.',
                  style: Theme.of(context).textTheme.bodyMedium,
                ),
                const SizedBox(height: 8),
                Text(
                  'One item per category. Context edits are blocked while a Draft exists.',
                  style: Theme.of(context).textTheme.bodySmall?.copyWith(
                    color: KeenBenchTheme.colorTextSecondary,
                  ),
                ),
                if (state.contextError != null &&
                    state.contextError!.isNotEmpty)
                  Padding(
                    padding: const EdgeInsets.only(top: 12),
                    child: Text(
                      state.contextError!,
                      style: Theme.of(context).textTheme.bodySmall?.copyWith(
                        color: KeenBenchTheme.colorErrorText,
                      ),
                    ),
                  ),
                const SizedBox(height: 20),
                if (state.isContextLoading)
                  const LinearProgressIndicator(
                    key: AppKeys.contextProcessingIndicator,
                  ),
                if (state.isContextLoading) const SizedBox(height: 12),
                Expanded(
                  child: GridView.count(
                    crossAxisCount: 2,
                    mainAxisSpacing: 12,
                    crossAxisSpacing: 12,
                    childAspectRatio: 1.6,
                    children: _categories.map((category) {
                      final item = byCategory[category];
                      return _ContextCategoryCard(
                        category: category,
                        title: _titles[category] ?? category,
                        description: _descriptions[category] ?? '',
                        item: item,
                        draftBlocked: state.hasDraft,
                        onAddOrEdit: () => _openProcessDialog(
                          context,
                          category,
                          hasDirectEdits: item?.hasDirectEdits ?? false,
                        ),
                        onInspect: item == null || !item.isActive
                            ? null
                            : () => _openInspectDialog(context, category),
                        onDelete: item == null || !item.isActive
                            ? null
                            : () => _deleteCategory(context, category),
                      );
                    }).toList(),
                  ),
                ),
              ],
            ),
          ),
        );
      },
    );
  }

  Future<void> _deleteCategory(BuildContext context, String category) async {
    final state = context.read<WorkbenchState>();
    try {
      await state.deleteContextItem(category);
    } catch (err) {
      if (!context.mounted) return;
      ScaffoldMessenger.of(
        context,
      ).showSnackBar(SnackBar(content: Text(err.toString())));
    }
  }

  Future<void> _openProcessDialog(
    BuildContext context,
    String category, {
    required bool hasDirectEdits,
  }) async {
    final state = context.read<WorkbenchState>();
    ContextItem? existing;
    try {
      existing = await state.getContextItem(category);
    } catch (_) {
      existing = null;
    }
    if (!context.mounted) {
      return;
    }

    await showDialog<void>(
      context: context,
      barrierDismissible: false,
      builder: (dialogContext) => _ContextProcessDialog(
        category: category,
        title:
            '${existing?.status == 'active' ? 'Reprocess' : 'Add'} ${_titles[category] ?? category}',
        existing: existing,
        state: state,
        hasDirectEdits: hasDirectEdits,
      ),
    );
  }

  Future<void> _openInspectDialog(BuildContext context, String category) async {
    final state = context.read<WorkbenchState>();
    ContextItem item;
    try {
      item = await state.getContextItem(category);
    } catch (err) {
      if (!context.mounted) return;
      ScaffoldMessenger.of(
        context,
      ).showSnackBar(SnackBar(content: Text(err.toString())));
      return;
    }
    if (!context.mounted) {
      return;
    }

    await showDialog<void>(
      context: context,
      barrierDismissible: false,
      builder: (dialogContext) => _ContextInspectDialog(
        category: category,
        title: _titles[category] ?? category,
        item: item,
        state: state,
        onReprocess: () => _openProcessDialog(
          context,
          category,
          hasDirectEdits: item.hasDirectEdits,
        ),
      ),
    );
  }
}

class _ContextProcessDialog extends StatefulWidget {
  const _ContextProcessDialog({
    required this.category,
    required this.title,
    required this.existing,
    required this.state,
    required this.hasDirectEdits,
  });

  final String category;
  final String title;
  final ContextItem? existing;
  final WorkbenchState state;
  final bool hasDirectEdits;

  @override
  State<_ContextProcessDialog> createState() => _ContextProcessDialogState();
}

class _ContextProcessDialogState extends State<_ContextProcessDialog> {
  late final TextEditingController _textController;
  late final TextEditingController _noteController;
  late String _mode;
  String _sourcePath = '';
  bool _processing = false;

  @override
  void initState() {
    super.initState();
    _textController = TextEditingController(
      text: widget.existing?.source?.mode == 'text'
          ? widget.existing?.source?.text ?? ''
          : '',
    );
    _noteController = TextEditingController(
      text: widget.existing?.source?.mode == 'file'
          ? widget.existing?.source?.note ?? ''
          : '',
    );
    _mode = widget.existing?.source?.mode == 'file' ? 'file' : 'text';
  }

  @override
  void dispose() {
    _textController.dispose();
    _noteController.dispose();
    super.dispose();
  }

  Future<void> _runProcessing() async {
    if (_processing) {
      return;
    }
    if (widget.state.hasDraft) {
      ScaffoldMessenger.of(context).showSnackBar(
        const SnackBar(
          content: Text('Publish or discard your Draft to modify context.'),
        ),
      );
      return;
    }

    if (widget.hasDirectEdits) {
      final proceed = await showDialog<bool>(
        context: context,
        builder: (confirmContext) => AlertDialog(
          title: const Text('Overwrite manual edits?'),
          content: const Text(
            'This context item was manually edited. Reprocessing will overwrite your manual changes.',
          ),
          actions: [
            OutlinedButton(
              key: AppKeys.contextManualOverwriteCancel,
              onPressed: () => Navigator.of(confirmContext).pop(false),
              child: const Text('Cancel'),
            ),
            ElevatedButton(
              key: AppKeys.contextManualOverwriteConfirm,
              onPressed: () => Navigator.of(confirmContext).pop(true),
              child: const Text('Proceed'),
            ),
          ],
        ),
      );
      if (proceed != true || !mounted) {
        return;
      }
    }

    setState(() {
      _processing = true;
    });
    final success = await _processWithRetry(widget.state);
    if (!mounted) {
      return;
    }
    setState(() {
      _processing = false;
    });
    if (success) {
      Navigator.of(context).pop();
    }
  }

  Future<bool> _processWithRetry(WorkbenchState state) async {
    while (true) {
      try {
        await state.processContextItem(
          category: widget.category,
          mode: _mode,
          text: _textController.text,
          sourcePath: _sourcePath,
          note: _noteController.text,
        );
        return true;
      } catch (err) {
        if (!mounted) {
          return false;
        }
        final retry = await showDialog<bool>(
          context: context,
          builder: (retryContext) => AlertDialog(
            title: const Text('Processing failed'),
            content: Text(err.toString()),
            actions: [
              OutlinedButton(
                onPressed: () => Navigator.of(retryContext).pop(false),
                child: const Text('Cancel'),
              ),
              ElevatedButton(
                onPressed: () => Navigator.of(retryContext).pop(true),
                child: const Text('Retry'),
              ),
            ],
          ),
        );
        if (retry != true) {
          return false;
        }
      }
    }
  }

  @override
  Widget build(BuildContext context) {
    return AlertDialog(
      title: Text(widget.title),
      content: SizedBox(
        width: 640,
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            Row(
              children: [
                Expanded(
                  child: RadioListTile<String>(
                    key: AppKeys.contextModeTextRadio,
                    value: 'text',
                    groupValue: _mode,
                    onChanged: _processing
                        ? null
                        : (value) {
                            if (value == null) return;
                            setState(() {
                              _mode = value;
                            });
                          },
                    title: const Text('Write text'),
                    dense: true,
                  ),
                ),
                Expanded(
                  child: RadioListTile<String>(
                    key: AppKeys.contextModeFileRadio,
                    value: 'file',
                    groupValue: _mode,
                    onChanged: _processing
                        ? null
                        : (value) {
                            if (value == null) return;
                            setState(() {
                              _mode = value;
                            });
                          },
                    title: const Text('Upload file'),
                    dense: true,
                  ),
                ),
              ],
            ),
            if (_mode == 'text')
              TextField(
                key: AppKeys.contextTextField,
                controller: _textController,
                enabled: !_processing,
                minLines: 8,
                maxLines: 12,
                decoration: const InputDecoration(
                  hintText: 'Paste context text...',
                ),
              ),
            if (_mode == 'file') ...[
              Container(
                key: AppKeys.contextFilePathField,
                width: double.infinity,
                padding: const EdgeInsets.symmetric(
                  horizontal: 12,
                  vertical: 10,
                ),
                decoration: BoxDecoration(
                  color: KeenBenchTheme.colorSurfaceSubtle,
                  border: Border.all(color: KeenBenchTheme.colorBorderDefault),
                  borderRadius: BorderRadius.circular(6),
                ),
                child: Text(
                  _sourcePath.isEmpty ? 'No file selected' : _sourcePath,
                  style: Theme.of(context).textTheme.bodySmall,
                ),
              ),
              const SizedBox(height: 8),
              Align(
                alignment: Alignment.centerLeft,
                child: OutlinedButton.icon(
                  onPressed: _processing
                      ? null
                      : () async {
                          final file = await openFile();
                          if (file == null || !mounted) {
                            return;
                          }
                          setState(() {
                            _sourcePath = file.path;
                          });
                        },
                  icon: const Icon(Icons.upload_file),
                  label: const Text('Choose file'),
                ),
              ),
              const SizedBox(height: 8),
              TextField(
                key: AppKeys.contextNoteField,
                controller: _noteController,
                enabled: !_processing,
                minLines: 2,
                maxLines: 4,
                decoration: const InputDecoration(
                  hintText: 'Optional note to guide processing...',
                ),
              ),
            ],
            if (_processing) ...[
              const SizedBox(height: 12),
              const LinearProgressIndicator(
                key: AppKeys.contextProcessingIndicator,
              ),
            ],
          ],
        ),
      ),
      actions: [
        OutlinedButton(
          key: AppKeys.contextCancelButton,
          onPressed: _processing ? null : () => Navigator.of(context).pop(),
          child: const Text('Cancel'),
        ),
        ElevatedButton(
          key: AppKeys.contextProcessButton,
          onPressed: _processing ? null : _runProcessing,
          child: Text(
            widget.existing?.status == 'active' ? 'Reprocess' : 'Process',
          ),
        ),
      ],
    );
  }
}

class _ContextInspectDialog extends StatefulWidget {
  const _ContextInspectDialog({
    required this.category,
    required this.title,
    required this.item,
    required this.state,
    required this.onReprocess,
  });

  final String category;
  final String title;
  final ContextItem item;
  final WorkbenchState state;
  final Future<void> Function() onReprocess;

  @override
  State<_ContextInspectDialog> createState() => _ContextInspectDialogState();
}

class _ContextInspectDialogState extends State<_ContextInspectDialog> {
  late final Map<String, TextEditingController> _controllers;
  bool _directEdit = false;

  @override
  void initState() {
    super.initState();
    _controllers = <String, TextEditingController>{
      for (final file in widget.item.files)
        file.path: TextEditingController(text: file.content),
    };
  }

  @override
  void dispose() {
    for (final controller in _controllers.values) {
      controller.dispose();
    }
    super.dispose();
  }

  Future<void> _saveDirectEdits() async {
    final files = _controllers.entries
        .map(
          (entry) =>
              ContextArtifactFile(path: entry.key, content: entry.value.text),
        )
        .toList();
    try {
      await widget.state.updateContextDirect(widget.category, files);
      if (!mounted) {
        return;
      }
      Navigator.of(context).pop();
    } catch (err) {
      if (!mounted) {
        return;
      }
      ScaffoldMessenger.of(
        context,
      ).showSnackBar(SnackBar(content: Text(err.toString())));
    }
  }

  @override
  Widget build(BuildContext context) {
    return AnimatedBuilder(
      animation: widget.state,
      builder: (context, _) {
        final hasDraft = widget.state.hasDraft;
        return AlertDialog(
          title: Text(widget.title),
          content: SizedBox(
            width: 760,
            child: Column(
              mainAxisSize: MainAxisSize.min,
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                if ((widget.item.summary).trim().isNotEmpty)
                  Text(
                    widget.item.summary,
                    style: Theme.of(context).textTheme.bodySmall?.copyWith(
                      color: KeenBenchTheme.colorTextSecondary,
                    ),
                  ),
                const SizedBox(height: 8),
                SwitchListTile(
                  key: AppKeys.contextDirectEditToggle,
                  value: _directEdit,
                  onChanged: hasDraft
                      ? null
                      : (value) {
                          setState(() {
                            _directEdit = value;
                          });
                        },
                  contentPadding: EdgeInsets.zero,
                  title: const Text('Direct Edit'),
                  subtitle: const Text(
                    'Direct edits save as-is and may bypass skill validation.',
                  ),
                ),
                const SizedBox(height: 8),
                Flexible(
                  child: ListView(
                    shrinkWrap: true,
                    children: widget.item.files.map((file) {
                      final controller = _controllers[file.path]!;
                      return Padding(
                        padding: const EdgeInsets.only(bottom: 12),
                        child: Column(
                          crossAxisAlignment: CrossAxisAlignment.start,
                          children: [
                            Text(
                              file.path,
                              style: Theme.of(context).textTheme.labelMedium
                                  ?.copyWith(fontFamily: 'JetBrainsMono'),
                            ),
                            const SizedBox(height: 4),
                            TextField(
                              key: AppKeys.contextArtifactField(file.path),
                              controller: controller,
                              minLines: 4,
                              maxLines: 10,
                              readOnly: !_directEdit,
                              decoration: const InputDecoration(),
                            ),
                          ],
                        ),
                      );
                    }).toList(),
                  ),
                ),
              ],
            ),
          ),
          actions: [
            OutlinedButton(
              onPressed: () => Navigator.of(context).pop(),
              child: const Text('Close'),
            ),
            OutlinedButton(
              key: AppKeys.contextReprocessButton,
              onPressed: hasDraft
                  ? null
                  : () async {
                      Navigator.of(context).pop();
                      await widget.onReprocess();
                    },
              child: const Text('Reprocess'),
            ),
            ElevatedButton(
              key: AppKeys.contextDirectSaveButton,
              onPressed: _directEdit && !hasDraft ? _saveDirectEdits : null,
              child: const Text('Save Direct Edit'),
            ),
          ],
        );
      },
    );
  }
}

class _ContextCategoryCard extends StatelessWidget {
  const _ContextCategoryCard({
    required this.category,
    required this.title,
    required this.description,
    required this.item,
    required this.draftBlocked,
    required this.onAddOrEdit,
    required this.onInspect,
    required this.onDelete,
  });

  final String category;
  final String title;
  final String description;
  final ContextItemSummary? item;
  final bool draftBlocked;
  final VoidCallback onAddOrEdit;
  final VoidCallback? onInspect;
  final VoidCallback? onDelete;

  @override
  Widget build(BuildContext context) {
    final isActive = item?.isActive == true;
    return Container(
      key: AppKeys.contextCategoryCard(category),
      decoration: BoxDecoration(
        color: KeenBenchTheme.colorBackgroundElevated,
        borderRadius: BorderRadius.circular(8),
        border: Border.all(color: KeenBenchTheme.colorBorderSubtle),
      ),
      padding: const EdgeInsets.all(12),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Expanded(
                child: Text(
                  title,
                  style: Theme.of(context).textTheme.titleSmall,
                ),
              ),
              if (item?.hasDirectEdits == true)
                Container(
                  padding: const EdgeInsets.symmetric(
                    horizontal: 6,
                    vertical: 2,
                  ),
                  decoration: BoxDecoration(
                    color: KeenBenchTheme.colorWarningBackground,
                    borderRadius: BorderRadius.circular(999),
                    border: Border.all(color: KeenBenchTheme.colorWarningText),
                  ),
                  child: Text(
                    'Manually edited',
                    style: Theme.of(context).textTheme.labelSmall?.copyWith(
                      color: KeenBenchTheme.colorWarningText,
                    ),
                  ),
                ),
            ],
          ),
          const SizedBox(height: 8),
          Expanded(
            child: Text(
              isActive && (item?.summary.trim().isNotEmpty ?? false)
                  ? item!.summary
                  : description,
              style: Theme.of(context).textTheme.bodySmall?.copyWith(
                color: KeenBenchTheme.colorTextSecondary,
              ),
            ),
          ),
          const SizedBox(height: 8),
          Row(
            children: [
              Expanded(
                child: Tooltip(
                  message: draftBlocked
                      ? 'Publish or discard your Draft to modify context.'
                      : '',
                  child: OutlinedButton(
                    key: isActive
                        ? AppKeys.contextCategoryEditButton(category)
                        : AppKeys.contextCategoryAddButton(category),
                    onPressed: draftBlocked ? null : onAddOrEdit,
                    child: Text(isActive ? 'Edit' : 'Add'),
                  ),
                ),
              ),
              const SizedBox(width: 8),
              Expanded(
                child: OutlinedButton(
                  key: AppKeys.contextCategoryInspectButton(category),
                  onPressed: onInspect,
                  child: const Text('Inspect'),
                ),
              ),
              const SizedBox(width: 8),
              Expanded(
                child: Tooltip(
                  message: draftBlocked && onDelete != null
                      ? 'Publish or discard your Draft to modify context.'
                      : '',
                  child: OutlinedButton(
                    key: AppKeys.contextCategoryDeleteButton(category),
                    onPressed: draftBlocked ? null : onDelete,
                    child: const Text('Delete'),
                  ),
                ),
              ),
            ],
          ),
        ],
      ),
    );
  }
}
