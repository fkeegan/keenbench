import 'package:flutter/material.dart';

import '../theme.dart';
import 'centered_content.dart';

class KeenBenchAppBar extends StatelessWidget implements PreferredSizeWidget {
  const KeenBenchAppBar({
    super.key,
    required this.title,
    this.actions = const [],
    this.showBack = false,
    this.useCenteredContent = true,
  });

  final String title;
  final List<Widget> actions;
  final bool showBack;
  final bool useCenteredContent;

  @override
  Size get preferredSize => const Size.fromHeight(kToolbarHeight);

  @override
  Widget build(BuildContext context) {
    return Material(
      color: KeenBenchTheme.colorBackgroundPrimary,
      child: Container(
        height: kToolbarHeight,
        decoration: const BoxDecoration(
          border: Border(
            bottom: BorderSide(color: KeenBenchTheme.colorBorderDefault),
          ),
        ),
        child: useCenteredContent
            ? CenteredContent(
                padding: const EdgeInsets.symmetric(horizontal: 24),
                alignment: Alignment.center,
                child: Row(
                  children: [
                    if (showBack)
                      BackButton(color: KeenBenchTheme.colorTextPrimary),
                    Expanded(
                      child: Text(
                        title,
                        style: Theme.of(context).textTheme.headlineMedium,
                        overflow: TextOverflow.ellipsis,
                      ),
                    ),
                    ...actions,
                  ],
                ),
              )
            : Padding(
                padding: const EdgeInsets.symmetric(horizontal: 24),
                child: Row(
                  children: [
                    if (showBack)
                      BackButton(color: KeenBenchTheme.colorTextPrimary),
                    Expanded(
                      child: Text(
                        title,
                        style: Theme.of(context).textTheme.headlineMedium,
                        overflow: TextOverflow.ellipsis,
                      ),
                    ),
                    ...actions,
                  ],
                ),
              ),
      ),
    );
  }
}
