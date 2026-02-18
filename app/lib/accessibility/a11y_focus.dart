import 'package:flutter/widgets.dart';

FocusNode? captureFocusedNode() {
  final node = FocusManager.instance.primaryFocus;
  if (node == null || !node.canRequestFocus) {
    return null;
  }
  return node;
}

void requestFocusSafely(FocusNode node) {
  if (!node.canRequestFocus) {
    return;
  }
  WidgetsBinding.instance.addPostFrameCallback((_) {
    if (node.canRequestFocus) {
      node.requestFocus();
    }
  });
}

void restoreFocus({
  FocusNode? previousFocus,
  required FocusNode fallbackFocus,
}) {
  if (previousFocus != null && previousFocus.canRequestFocus) {
    requestFocusSafely(previousFocus);
    return;
  }
  requestFocusSafely(fallbackFocus);
}
