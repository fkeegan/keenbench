import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter/semantics.dart';

class A11yAnnouncer {
  A11yAnnouncer._();

  static final A11yAnnouncer instance = A11yAnnouncer._();

  String _lastMessage = '';
  DateTime _lastAnnouncedAt = DateTime.fromMillisecondsSinceEpoch(0);
  Duration dedupeWindow = const Duration(milliseconds: 900);

  void announce(
    BuildContext context,
    String message, {
    bool force = false,
    TextDirection? textDirection,
  }) {
    final normalized = message.trim();
    if (normalized.isEmpty) {
      return;
    }

    final now = DateTime.now();
    final withinDedupe =
        normalized == _lastMessage &&
        now.difference(_lastAnnouncedAt) < dedupeWindow;
    if (!force && withinDedupe) {
      return;
    }

    _lastMessage = normalized;
    _lastAnnouncedAt = now;

    final direction =
        textDirection ?? Directionality.maybeOf(context) ?? TextDirection.ltr;
    unawaited(
      SemanticsService.sendAnnouncement(
        View.of(context),
        normalized,
        direction,
      ),
    );
  }
}
