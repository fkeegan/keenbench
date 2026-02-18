import 'package:flutter/material.dart';
import 'package:provider/provider.dart';

import '../app_keys.dart';
import '../engine/engine_client.dart';
import '../models/models.dart';
import '../theme.dart';
import '../widgets/keenbench_app_bar.dart';

class CheckpointsScreen extends StatefulWidget {
  const CheckpointsScreen({super.key, required this.workbenchId});

  final String workbenchId;

  @override
  State<CheckpointsScreen> createState() => _CheckpointsScreenState();
}

class _CheckpointsScreenState extends State<CheckpointsScreen> {
  bool _loading = true;
  bool _hasDraft = false;
  List<CheckpointMetadata> _checkpoints = [];

  @override
  void initState() {
    super.initState();
    _load();
  }

  Future<void> _load() async {
    final engine = context.read<EngineApi>();
    try {
      final response = await engine.call('CheckpointsList', {
        'workbench_id': widget.workbenchId,
      });
      final list = (response['checkpoints'] as List<dynamic>? ?? [])
          .cast<Map<String, dynamic>>();
      final draftState = await engine.call('DraftGetState', {
        'workbench_id': widget.workbenchId,
      });
      if (!mounted) {
        return;
      }
      setState(() {
        _checkpoints = list.map(CheckpointMetadata.fromJson).toList();
        _hasDraft = draftState['has_draft'] == true;
        _loading = false;
      });
    } catch (err) {
      if (!mounted) {
        return;
      }
      setState(() {
        _loading = false;
      });
      ScaffoldMessenger.of(
        context,
      ).showSnackBar(SnackBar(content: Text(err.toString())));
    }
  }

  Future<void> _createCheckpoint() async {
    final controller = TextEditingController();
    final description = await showDialog<String>(
      context: context,
      builder: (context) => AlertDialog(
        title: const Text('Create checkpoint'),
        content: TextField(
          controller: controller,
          decoration: const InputDecoration(
            labelText: 'Description (optional)',
          ),
        ),
        actions: [
          OutlinedButton(
            onPressed: () => Navigator.of(context).pop(null),
            child: const Text('Cancel'),
          ),
          ElevatedButton(
            onPressed: () => Navigator.of(context).pop(controller.text.trim()),
            child: const Text('Create'),
          ),
        ],
      ),
    );
    if (description == null) {
      return;
    }
    final engine = context.read<EngineApi>();
    await engine.call('CheckpointCreate', {
      'workbench_id': widget.workbenchId,
      'reason': 'manual',
      'description': description,
    });
    await _load();
  }

  Future<void> _restoreCheckpoint(CheckpointMetadata checkpoint) async {
    final confirm = await showDialog<bool>(
      context: context,
      builder: (context) => AlertDialog(
        title: const Text('Restore checkpoint'),
        content: Text(
          'Restoring will revert Published files and Workbench history to ${checkpoint.createdAt}.',
        ),
        actions: [
          OutlinedButton(
            onPressed: () => Navigator.of(context).pop(false),
            child: const Text('Cancel'),
          ),
          ElevatedButton(
            onPressed: () => Navigator.of(context).pop(true),
            child: const Text('Restore'),
          ),
        ],
      ),
    );
    if (confirm != true) {
      return;
    }
    final engine = context.read<EngineApi>();
    await engine.call('CheckpointRestore', {
      'workbench_id': widget.workbenchId,
      'checkpoint_id': checkpoint.id,
    });
    await _load();
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      key: AppKeys.checkpointsScreen,
      appBar: const KeenBenchAppBar(title: 'Checkpoints', showBack: true),
      body: _loading
          ? const Center(child: CircularProgressIndicator())
          : Padding(
              padding: const EdgeInsets.all(24),
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Row(
                    children: [
                      Expanded(
                        child: Text(
                          'Checkpoint timeline',
                          style: Theme.of(context).textTheme.headlineSmall,
                        ),
                      ),
                      Tooltip(
                        message: _hasDraft
                            ? 'Publish or discard Draft to create a checkpoint.'
                            : 'Create a new checkpoint.',
                        child: ElevatedButton(
                          key: AppKeys.checkpointsCreateButton,
                          onPressed: _hasDraft ? null : _createCheckpoint,
                          child: const Text('Create checkpoint'),
                        ),
                      ),
                    ],
                  ),
                  const SizedBox(height: 16),
                  Expanded(
                    child: _checkpoints.isEmpty
                        ? Center(
                            child: Text(
                              'No checkpoints yet.',
                              style: Theme.of(context).textTheme.bodySmall
                                  ?.copyWith(
                                    color: KeenBenchTheme.colorTextSecondary,
                                  ),
                            ),
                          )
                        : ListView.builder(
                            key: AppKeys.checkpointsList,
                            itemCount: _checkpoints.length,
                            itemBuilder: (context, index) {
                              final checkpoint = _checkpoints[index];
                              return Container(
                                margin: const EdgeInsets.only(bottom: 12),
                                padding: const EdgeInsets.all(12),
                                decoration: BoxDecoration(
                                  borderRadius: BorderRadius.circular(8),
                                  border: Border.all(
                                    color: KeenBenchTheme.colorBorderSubtle,
                                  ),
                                  color: KeenBenchTheme.colorBackgroundElevated,
                                ),
                                child: Row(
                                  children: [
                                    Expanded(
                                      child: Column(
                                        crossAxisAlignment:
                                            CrossAxisAlignment.start,
                                        children: [
                                          Text(
                                            checkpoint.description.isNotEmpty
                                                ? checkpoint.description
                                                : 'Checkpoint',
                                            style: Theme.of(
                                              context,
                                            ).textTheme.bodyMedium,
                                          ),
                                          const SizedBox(height: 4),
                                          Text(
                                            '${checkpoint.createdAt} • ${checkpoint.reason}',
                                            style: Theme.of(context)
                                                .textTheme
                                                .bodySmall
                                                ?.copyWith(
                                                  color: KeenBenchTheme
                                                      .colorTextSecondary,
                                                ),
                                          ),
                                        ],
                                      ),
                                    ),
                                    Tooltip(
                                      message: _hasDraft
                                          ? 'Publish or discard Draft to restore.'
                                          : 'Restore this checkpoint.',
                                      child: TextButton(
                                        onPressed: _hasDraft
                                            ? null
                                            : () => _restoreCheckpoint(
                                                checkpoint,
                                              ),
                                        child: const Text('Restore'),
                                      ),
                                    ),
                                  ],
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
