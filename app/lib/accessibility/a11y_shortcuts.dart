import 'package:flutter/widgets.dart';
import 'package:flutter/services.dart';

class FocusComposerIntent extends Intent {
  const FocusComposerIntent();
}

class SendComposerIntent extends Intent {
  const SendComposerIntent();
}

class OpenReviewIntent extends Intent {
  const OpenReviewIntent();
}

class PublishDraftIntent extends Intent {
  const PublishDraftIntent();
}

class DiscardDraftIntent extends Intent {
  const DiscardDraftIntent();
}

Map<ShortcutActivator, Intent> workbenchShortcutMap() {
  return {
    const SingleActivator(LogicalKeyboardKey.keyL, control: true):
        const FocusComposerIntent(),
    const SingleActivator(LogicalKeyboardKey.keyL, meta: true):
        const FocusComposerIntent(),
    const SingleActivator(LogicalKeyboardKey.enter, control: true):
        const SendComposerIntent(),
    const SingleActivator(LogicalKeyboardKey.enter, meta: true):
        const SendComposerIntent(),
    const SingleActivator(LogicalKeyboardKey.keyR, control: true):
        const OpenReviewIntent(),
    const SingleActivator(LogicalKeyboardKey.keyR, meta: true):
        const OpenReviewIntent(),
    const SingleActivator(LogicalKeyboardKey.keyP, control: true, shift: true):
        const PublishDraftIntent(),
    const SingleActivator(LogicalKeyboardKey.keyP, meta: true, shift: true):
        const PublishDraftIntent(),
    const SingleActivator(LogicalKeyboardKey.keyD, control: true, shift: true):
        const DiscardDraftIntent(),
    const SingleActivator(LogicalKeyboardKey.keyD, meta: true, shift: true):
        const DiscardDraftIntent(),
  };
}

Map<ShortcutActivator, Intent> reviewShortcutMap() {
  return {
    const SingleActivator(LogicalKeyboardKey.keyP, control: true, shift: true):
        const PublishDraftIntent(),
    const SingleActivator(LogicalKeyboardKey.keyP, meta: true, shift: true):
        const PublishDraftIntent(),
    const SingleActivator(LogicalKeyboardKey.keyD, control: true, shift: true):
        const DiscardDraftIntent(),
    const SingleActivator(LogicalKeyboardKey.keyD, meta: true, shift: true):
        const DiscardDraftIntent(),
  };
}
