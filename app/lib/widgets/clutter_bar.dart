import 'package:flutter/material.dart';

import '../theme.dart';

class ClutterBar extends StatelessWidget {
  const ClutterBar({super.key, required this.score, required this.level});

  final double score;
  final String level;

  Color _fillColor() {
    switch (level.toLowerCase()) {
      case 'heavy':
        return KeenBenchTheme.colorErrorText;
      case 'moderate':
        return KeenBenchTheme.colorWarningText;
      default:
        return KeenBenchTheme.colorSuccessText;
    }
  }

  @override
  Widget build(BuildContext context) {
    final fillColor = _fillColor();
    return SizedBox(
      width: 180,
      child: Column(
        mainAxisSize: MainAxisSize.min,
        crossAxisAlignment: CrossAxisAlignment.end,
        children: [
          Text(
            level,
            style: Theme.of(context).textTheme.bodySmall?.copyWith(
              color: KeenBenchTheme.colorTextSecondary,
            ),
          ),
          const SizedBox(height: 4),
          Container(
            height: 6,
            decoration: BoxDecoration(
              color: KeenBenchTheme.colorSurfaceMuted,
              borderRadius: BorderRadius.circular(6),
              border: Border.all(color: KeenBenchTheme.colorBorderSubtle),
            ),
            child: Align(
              alignment: Alignment.centerLeft,
              child: FractionallySizedBox(
                widthFactor: score.clamp(0.0, 1.0),
                child: Container(
                  decoration: BoxDecoration(
                    color: fillColor,
                    borderRadius: BorderRadius.circular(6),
                  ),
                ),
              ),
            ),
          ),
        ],
      ),
    );
  }
}
