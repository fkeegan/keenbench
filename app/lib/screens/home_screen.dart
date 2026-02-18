import 'package:flutter/material.dart';
import 'package:provider/provider.dart';

import '../app_keys.dart';
import '../engine/engine_client.dart';
import '../logging.dart';
import '../models/models.dart';
import '../theme.dart';
import '../widgets/centered_content.dart';
import '../widgets/dialog_keyboard_shortcuts.dart';
import '../widgets/keenbench_app_bar.dart';
import 'settings_screen.dart';
import 'workbench_screen.dart';

class HomeScreen extends StatefulWidget {
  const HomeScreen({super.key});

  @override
  State<HomeScreen> createState() => _HomeScreenState();
}

class _HomeScreenState extends State<HomeScreen> {
  List<Workbench> _workbenches = [];
  bool _loading = true;

  @override
  void initState() {
    super.initState();
    _load();
  }

  Future<void> _load() async {
    final engine = context.read<EngineApi>();
    final response = await engine.call('WorkbenchList');
    final items = (response['workbenches'] as List<dynamic>? ?? [])
        .cast<Map<String, dynamic>>();
    setState(() {
      _workbenches = items.map(Workbench.fromJson).toList();
      _loading = false;
    });
    AppLog.debug('home.workbench_list_loaded', {'count': _workbenches.length});
  }

  Future<void> _createWorkbench() async {
    final controller = TextEditingController();
    final name = await showDialog<String>(
      context: context,
      barrierColor: KeenBenchTheme.colorSurfaceOverlay,
      builder: (dialogContext) {
        void cancel() => Navigator.of(dialogContext).pop();

        void submit() =>
            Navigator.of(dialogContext).pop(controller.text.trim());

        return DialogKeyboardShortcuts(
          onCancel: cancel,
          onSubmit: submit,
          child: AlertDialog(
            key: AppKeys.newWorkbenchDialog,
            title: const Text('New Workbench'),
            content: TextField(
              key: AppKeys.newWorkbenchNameField,
              controller: controller,
              textInputAction: TextInputAction.done,
              onSubmitted: (_) => submit(),
              decoration: const InputDecoration(labelText: 'Workbench name'),
            ),
            actions: [
              OutlinedButton(
                key: AppKeys.newWorkbenchCancelButton,
                onPressed: cancel,
                child: const Text('Cancel'),
              ),
              ElevatedButton(
                key: AppKeys.newWorkbenchCreateButton,
                onPressed: submit,
                child: const Text('Create'),
              ),
            ],
          ),
        );
      },
    );
    if (name == null) {
      return;
    }
    final engine = context.read<EngineApi>();
    AppLog.info('home.create_workbench', {'name': name});
    final response = await engine.call('WorkbenchCreate', {'name': name});
    final workbenchId = response['workbench_id'] as String;
    if (!mounted) {
      return;
    }
    await Navigator.of(context).push(
      MaterialPageRoute(
        builder: (_) => WorkbenchScreen(workbenchId: workbenchId),
      ),
    );
    _load();
  }

  Future<void> _deleteWorkbench(Workbench wb) async {
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
            key: AppKeys.homeDeleteWorkbenchDialog,
            title: const Text('Delete Workbench'),
            content: Text(
              'Delete "${wb.name}"? This removes the Workbench and its files. Originals remain untouched.',
            ),
            actions: [
              OutlinedButton(
                key: AppKeys.homeDeleteWorkbenchCancel,
                onPressed: cancel,
                child: const Text('Cancel'),
              ),
              ElevatedButton(
                key: AppKeys.homeDeleteWorkbenchConfirm,
                onPressed: submit,
                style: ElevatedButton.styleFrom(
                  backgroundColor: KeenBenchTheme.colorErrorText,
                ),
                child: const Text('Delete'),
              ),
            ],
          ),
        );
      },
    );
    if (confirm != true) {
      return;
    }
    final engine = context.read<EngineApi>();
    try {
      AppLog.info('home.delete_workbench', {'workbench_id': wb.id});
      await engine.call('WorkbenchDelete', {'workbench_id': wb.id});
      await _load();
    } on EngineError catch (err) {
      if (!mounted) return;
      ScaffoldMessenger.of(
        context,
      ).showSnackBar(SnackBar(content: Text(err.message)));
    }
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      key: AppKeys.homeScreen,
      appBar: KeenBenchAppBar(
        title: 'KeenBench',
        actions: [
          TextButton(
            key: AppKeys.homeSettingsButton,
            onPressed: () => Navigator.of(
              context,
            ).push(MaterialPageRoute(builder: (_) => const SettingsScreen())),
            child: const Text('Settings'),
          ),
        ],
      ),
      body: _loading
          ? const Center(child: CircularProgressIndicator())
          : CenteredContent(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Row(
                    children: [
                      Expanded(
                        child: Text(
                          'Workbenches',
                          style: Theme.of(context).textTheme.headlineMedium,
                        ),
                      ),
                      ElevatedButton(
                        key: AppKeys.homeNewWorkbenchButton,
                        onPressed: _createWorkbench,
                        child: const Text('New Workbench'),
                      ),
                    ],
                  ),
                  const SizedBox(height: 16),
                  if (_workbenches.isEmpty)
                    Expanded(
                      child: Center(
                        child: Text(
                          key: AppKeys.homeEmptyState,
                          'Create a Workbench to begin.',
                          style: Theme.of(context).textTheme.bodySmall
                              ?.copyWith(
                                color: KeenBenchTheme.colorTextSecondary,
                              ),
                        ),
                      ),
                    )
                  else
                    Expanded(
                      child: GridView.builder(
                        key: AppKeys.homeWorkbenchGrid,
                        gridDelegate:
                            const SliverGridDelegateWithFixedCrossAxisCount(
                              crossAxisCount: 3,
                              mainAxisSpacing: 16,
                              crossAxisSpacing: 16,
                              childAspectRatio: 1.6,
                            ),
                        itemCount: _workbenches.length,
                        itemBuilder: (context, index) {
                          final wb = _workbenches[index];
                          return InkWell(
                            key: AppKeys.workbenchTile(wb.id),
                            onTap: () => Navigator.of(context).push(
                              MaterialPageRoute(
                                builder: (_) =>
                                    WorkbenchScreen(workbenchId: wb.id),
                              ),
                            ),
                            child: Container(
                              padding: const EdgeInsets.all(16),
                              decoration: BoxDecoration(
                                color: KeenBenchTheme.colorBackgroundElevated,
                                borderRadius: BorderRadius.circular(8),
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
                                    crossAxisAlignment:
                                        CrossAxisAlignment.start,
                                    children: [
                                      Expanded(
                                        child: Text(
                                          wb.name,
                                          style: Theme.of(
                                            context,
                                          ).textTheme.headlineSmall,
                                        ),
                                      ),
                                      PopupMenuButton<String>(
                                        key: AppKeys.workbenchTileMenu(wb.id),
                                        padding: EdgeInsets.zero,
                                        tooltip: 'Workbench actions',
                                        icon: const Icon(
                                          Icons.more_horiz,
                                          size: 18,
                                          color:
                                              KeenBenchTheme.colorTextSecondary,
                                        ),
                                        onSelected: (value) {
                                          if (value == 'delete') {
                                            _deleteWorkbench(wb);
                                          }
                                        },
                                        itemBuilder: (context) => [
                                          PopupMenuItem<String>(
                                            key: AppKeys.workbenchTileDelete(
                                              wb.id,
                                            ),
                                            value: 'delete',
                                            child: Text(
                                              'Delete Workbench',
                                              style: Theme.of(context)
                                                  .textTheme
                                                  .bodyMedium
                                                  ?.copyWith(
                                                    color: KeenBenchTheme
                                                        .colorErrorText,
                                                  ),
                                            ),
                                          ),
                                        ],
                                      ),
                                    ],
                                  ),
                                  const SizedBox(height: 8),
                                  Text(
                                    'Updated ${wb.updatedAt.isEmpty ? 'just now' : wb.updatedAt}',
                                    style: Theme.of(context).textTheme.bodySmall
                                        ?.copyWith(
                                          color:
                                              KeenBenchTheme.colorTextSecondary,
                                        ),
                                  ),
                                ],
                              ),
                            ),
                          );
                        },
                      ),
                    ),
                ],
              ),
            ),
    );
  }
}
