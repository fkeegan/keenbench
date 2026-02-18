import 'package:flutter/material.dart';

class CenteredContent extends StatelessWidget {
  const CenteredContent({
    super.key,
    required this.child,
    this.padding = const EdgeInsets.all(24),
    this.alignment = Alignment.topCenter,
  });

  final Widget child;
  final EdgeInsets padding;
  final Alignment alignment;

  double _maxWidthFor(double width) {
    if (width >= 3840) return 1200;
    if (width >= 2560) return 1080;
    return 960;
  }

  @override
  Widget build(BuildContext context) {
    final width = MediaQuery.of(context).size.width;
    final maxWidth = _maxWidthFor(width);
    return Align(
      alignment: alignment,
      child: ConstrainedBox(
        constraints: BoxConstraints(maxWidth: maxWidth),
        child: Padding(padding: padding, child: child),
      ),
    );
  }
}
