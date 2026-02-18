import 'package:flutter/material.dart';
import 'package:flutter/services.dart';

class _DialogSubmitIntent extends Intent {
  const _DialogSubmitIntent();
}

class _DialogCancelIntent extends Intent {
  const _DialogCancelIntent();
}

class DialogKeyboardShortcuts extends StatelessWidget {
  const DialogKeyboardShortcuts({
    super.key,
    required this.child,
    required this.onCancel,
    this.onSubmit,
    this.submitOnEnter = true,
  });

  final Widget child;
  final VoidCallback onCancel;
  final VoidCallback? onSubmit;
  final bool submitOnEnter;

  @override
  Widget build(BuildContext context) {
    final shortcuts = <ShortcutActivator, Intent>{
      const SingleActivator(LogicalKeyboardKey.escape):
          const _DialogCancelIntent(),
      if (submitOnEnter && onSubmit != null)
        const SingleActivator(LogicalKeyboardKey.enter):
            const _DialogSubmitIntent(),
      if (submitOnEnter && onSubmit != null)
        const SingleActivator(LogicalKeyboardKey.numpadEnter):
            const _DialogSubmitIntent(),
    };

    return Shortcuts(
      shortcuts: shortcuts,
      child: Actions(
        actions: <Type, Action<Intent>>{
          _DialogSubmitIntent: CallbackAction<Intent>(
            onInvoke: (_) {
              onSubmit?.call();
              return null;
            },
          ),
          _DialogCancelIntent: CallbackAction<Intent>(
            onInvoke: (_) {
              onCancel();
              return null;
            },
          ),
        },
        child: Focus(autofocus: true, child: child),
      ),
    );
  }
}
