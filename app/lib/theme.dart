import 'package:flutter/material.dart';

class KeenBenchTheme {
  static const colorBackgroundPrimary = Color(0xFFFDFCFB);
  static const colorBackgroundSecondary = Color(0xFFF9F7F5);
  static const colorBackgroundElevated = Color(0xFFFFFFFF);
  static const colorBackgroundHover = Color(0xFFF5F2EF);
  static const colorBackgroundSelected = Color(0xFFEDE8E4);

  static const colorSurfaceSubtle = Color(0xFFFAF8F6);
  static const colorSurfaceMuted = Color(0xFFF3F0ED);
  static const colorSurfaceOverlay = Color(0xF2FDFCFB);

  static const colorTextPrimary = Color(0xFF1F1F1F);
  static const colorTextSecondary = Color(0xFF6B6560);
  static const colorTextTertiary = Color(0xFF9C9590);

  static const colorBorderSubtle = Color(0xFFEBE7E3);
  static const colorBorderDefault = Color(0xFFDDD8D3);
  static const colorBorderStrong = Color(0xFFC5C0BB);
  static const colorBorderFocus = Color(0xFF8B8580);

  static const colorAccentPrimary = Color(0xFF5B7FC2);
  static const colorAccentPrimaryHover = Color(0xFF4A6AAF);
  static const colorAccentPrimaryActive = Color(0xFF3D5A9A);
  static const colorAccentSecondary = Color(0xFF7B9FD4);

  static const colorSuccessBackground = Color(0xFFF0F7F0);
  static const colorSuccessText = Color(0xFF2E7D32);
  static const colorWarningBackground = Color(0xFFFFF8E6);
  static const colorWarningText = Color(0xFFB8860B);
  static const colorErrorBackground = Color(0xFFFDF2F2);
  static const colorErrorText = Color(0xFFC53030);
  static const colorInfoBackground = Color(0xFFF0F5FA);
  static const colorInfoBorder = Color(0xFFA3C4E8);
  static const colorInfoText = Color(0xFF1A5490);

  static const colorDraftIndicator = Color(0xFFE8B86D);
  static const colorPublishedIndicator = Color(0xFF6BAF8D);
  static const colorDiffAdded = Color(0xFFDCEDC8);
  static const colorDiffRemoved = Color(0xFFFFCDD2);

  static ThemeData theme() {
    final base = ThemeData.light();
    return base.copyWith(
      useMaterial3: false,
      scaffoldBackgroundColor: colorBackgroundPrimary,
      cardColor: colorBackgroundElevated,
      canvasColor: colorBackgroundSecondary,
      hoverColor: colorBackgroundHover,
      colorScheme: base.colorScheme.copyWith(
        primary: colorAccentPrimary,
        secondary: colorAccentSecondary,
        surface: colorBackgroundElevated,
        background: colorBackgroundPrimary,
        error: colorErrorText,
        onPrimary: colorBackgroundPrimary,
        onSurface: colorTextPrimary,
      ),
      textTheme: _textTheme(base.textTheme),
      appBarTheme: const AppBarTheme(
        backgroundColor: colorBackgroundPrimary,
        elevation: 0,
        foregroundColor: colorTextPrimary,
      ),
      dividerColor: colorBorderDefault,
      inputDecorationTheme: InputDecorationTheme(
        filled: true,
        fillColor: colorSurfaceSubtle,
        border: OutlineInputBorder(
          borderRadius: BorderRadius.circular(6),
          borderSide: const BorderSide(color: colorBorderDefault),
        ),
        enabledBorder: OutlineInputBorder(
          borderRadius: BorderRadius.circular(6),
          borderSide: const BorderSide(color: colorBorderDefault),
        ),
        focusedBorder: OutlineInputBorder(
          borderRadius: BorderRadius.circular(6),
          borderSide: const BorderSide(color: colorBorderFocus, width: 1.5),
        ),
      ),
      elevatedButtonTheme: ElevatedButtonThemeData(
        style: ElevatedButton.styleFrom(
          backgroundColor: colorAccentPrimary,
          foregroundColor: colorBackgroundPrimary,
          padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 8),
          shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(6)),
          textStyle: const TextStyle(fontWeight: FontWeight.w600),
        ),
      ),
      outlinedButtonTheme: OutlinedButtonThemeData(
        style: OutlinedButton.styleFrom(
          foregroundColor: colorAccentPrimary,
          side: const BorderSide(color: colorBorderDefault),
          padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 8),
          shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(6)),
        ),
      ),
      textButtonTheme: TextButtonThemeData(
        style: TextButton.styleFrom(
          foregroundColor: colorAccentPrimary,
          padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 8),
          textStyle: const TextStyle(fontWeight: FontWeight.w600),
        ),
      ),
    );
  }

  static TextTheme _textTheme(TextTheme base) {
    return base
        .copyWith(
          displayLarge: const TextStyle(
            fontFamily: 'Inter',
            fontSize: 32,
            fontWeight: FontWeight.w600,
            height: 1.2,
          ),
          headlineLarge: const TextStyle(
            fontFamily: 'Inter',
            fontSize: 24,
            fontWeight: FontWeight.w600,
            height: 1.3,
          ),
          headlineMedium: const TextStyle(
            fontFamily: 'Inter',
            fontSize: 20,
            fontWeight: FontWeight.w600,
            height: 1.35,
          ),
          headlineSmall: const TextStyle(
            fontFamily: 'Inter',
            fontSize: 16,
            fontWeight: FontWeight.w600,
            height: 1.4,
          ),
          bodyLarge: const TextStyle(
            fontFamily: 'Inter',
            fontSize: 14,
            fontWeight: FontWeight.w400,
            height: 1.5,
          ),
          bodyMedium: const TextStyle(
            fontFamily: 'Inter',
            fontSize: 14,
            fontWeight: FontWeight.w500,
            height: 1.5,
          ),
          bodySmall: const TextStyle(
            fontFamily: 'Inter',
            fontSize: 13,
            fontWeight: FontWeight.w400,
            height: 1.45,
          ),
          labelSmall: const TextStyle(
            fontFamily: 'Inter',
            fontSize: 12,
            fontWeight: FontWeight.w400,
            height: 1.4,
          ),
        )
        .apply(bodyColor: colorTextPrimary, displayColor: colorTextPrimary);
  }

  static const TextStyle mono = TextStyle(
    fontFamily: 'JetBrains Mono',
    fontSize: 13,
    height: 1.6,
  );
}
