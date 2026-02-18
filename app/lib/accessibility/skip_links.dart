import 'package:flutter/material.dart';

import 'a11y_focus.dart';

class A11ySkipLink {
  const A11ySkipLink({
    required this.label,
    required this.targetFocusNode,
    this.key,
  });

  final String label;
  final FocusNode targetFocusNode;
  final Key? key;
}

class A11ySkipLinks extends StatelessWidget {
  const A11ySkipLinks({
    super.key,
    required this.links,
    this.padding = const EdgeInsets.symmetric(horizontal: 24, vertical: 8),
  });

  final List<A11ySkipLink> links;
  final EdgeInsetsGeometry padding;

  @override
  Widget build(BuildContext context) {
    if (links.isEmpty) {
      return const SizedBox.shrink();
    }

    return Padding(
      padding: padding,
      child: Wrap(
        spacing: 8,
        runSpacing: 8,
        children: links.map((link) => _A11ySkipLinkButton(link: link)).toList(),
      ),
    );
  }
}

class _A11ySkipLinkButton extends StatefulWidget {
  const _A11ySkipLinkButton({required this.link});

  final A11ySkipLink link;

  @override
  State<_A11ySkipLinkButton> createState() => _A11ySkipLinkButtonState();
}

class _A11ySkipLinkButtonState extends State<_A11ySkipLinkButton> {
  static const double _collapsedFactor = 0.02;
  bool _isFocused = false;

  @override
  Widget build(BuildContext context) {
    return Focus(
      onFocusChange: (focused) {
        if (_isFocused != focused) {
          setState(() {
            _isFocused = focused;
          });
        }
      },
      child: ClipRect(
        child: AnimatedAlign(
          duration: const Duration(milliseconds: 120),
          curve: Curves.easeOut,
          alignment: Alignment.topLeft,
          widthFactor: _isFocused ? 1 : _collapsedFactor,
          heightFactor: _isFocused ? 1 : _collapsedFactor,
          child: DecoratedBox(
            decoration: BoxDecoration(
              color: _isFocused
                  ? Theme.of(context).colorScheme.primary
                  : Colors.transparent,
              borderRadius: BorderRadius.circular(6),
            ),
            child: TextButton(
              key: widget.link.key,
              onPressed: () => requestFocusSafely(widget.link.targetFocusNode),
              child: Text(
                widget.link.label,
                style: Theme.of(context).textTheme.bodySmall?.copyWith(
                  color: _isFocused
                      ? Theme.of(context).colorScheme.onPrimary
                      : Colors.transparent,
                  fontWeight: FontWeight.w600,
                ),
              ),
            ),
          ),
        ),
      ),
    );
  }
}
