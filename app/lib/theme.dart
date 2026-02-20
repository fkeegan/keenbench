import 'package:flutter/material.dart';

class KeenBenchTheme {
  static const colorBackgroundPrimary = Color(0xFFFDFCFB);
  static const colorBackgroundSecondary = Color(0xFFF9F7F5);
  static const colorBackgroundElevated = Color(0xFFFFFFFF);
  static const colorBackgroundHover = Color(0xFFF5F2EF);
  static const colorBackgroundSelected = Color(0xFFEDE8E4);

  static const colorSurfaceSubtle = Color(0xFFFAF8F6);
  static const colorSurfaceMuted = Color(0xFFF3F0ED);
  static const colorSurfaceOverlay = Color(0x661F1F1F);

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
  static const colorSuccessBorder = Color(0xFFA8D4A8);
  static const colorSuccessText = Color(0xFF2E7D32);
  static const colorWarningBackground = Color(0xFFFFF8E6);
  static const colorWarningBorder = Color(0xFFF5D88C);
  static const colorWarningText = Color(0xFFB8860B);
  static const colorErrorBackground = Color(0xFFFDF2F2);
  static const colorErrorBorder = Color(0xFFF5B0AC);
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
        contentPadding: const EdgeInsets.symmetric(
          horizontal: 12,
          vertical: 10,
        ),
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
          borderSide: const BorderSide(color: colorAccentPrimary, width: 1.5),
        ),
      ),
      elevatedButtonTheme: ElevatedButtonThemeData(
        style: ButtonStyle(
          backgroundColor: MaterialStateProperty.resolveWith((states) {
            if (states.contains(MaterialState.disabled)) {
              return colorAccentPrimary.withOpacity(0.5);
            }
            if (states.contains(MaterialState.pressed)) {
              return colorAccentPrimaryActive;
            }
            if (states.contains(MaterialState.hovered)) {
              return colorAccentPrimaryHover;
            }
            return colorAccentPrimary;
          }),
          foregroundColor: const MaterialStatePropertyAll(Colors.white),
          shadowColor: const MaterialStatePropertyAll(
            Color.fromRGBO(100, 90, 80, 0.1),
          ),
          elevation: MaterialStateProperty.resolveWith((states) {
            if (states.contains(MaterialState.pressed)) {
              return 0;
            }
            if (states.contains(MaterialState.hovered)) {
              return 2;
            }
            return 1;
          }),
          padding: const MaterialStatePropertyAll(
            EdgeInsets.symmetric(horizontal: 16, vertical: 8),
          ),
          shape: MaterialStatePropertyAll(
            RoundedRectangleBorder(borderRadius: BorderRadius.circular(6)),
          ),
          textStyle: const MaterialStatePropertyAll(
            TextStyle(fontWeight: FontWeight.w600),
          ),
        ),
      ),
      outlinedButtonTheme: OutlinedButtonThemeData(
        style: ButtonStyle(
          foregroundColor: const MaterialStatePropertyAll(colorAccentPrimary),
          side: const MaterialStatePropertyAll(
            BorderSide(color: colorBorderDefault),
          ),
          backgroundColor: MaterialStateProperty.resolveWith((states) {
            if (states.contains(MaterialState.hovered) ||
                states.contains(MaterialState.focused)) {
              return colorBackgroundHover;
            }
            return Colors.transparent;
          }),
          padding: const MaterialStatePropertyAll(
            EdgeInsets.symmetric(horizontal: 16, vertical: 8),
          ),
          shape: MaterialStatePropertyAll(
            RoundedRectangleBorder(borderRadius: BorderRadius.circular(6)),
          ),
        ),
      ),
      textButtonTheme: TextButtonThemeData(
        style: ButtonStyle(
          foregroundColor: MaterialStateProperty.resolveWith((states) {
            if (states.contains(MaterialState.hovered) ||
                states.contains(MaterialState.focused)) {
              return colorTextPrimary;
            }
            return colorTextSecondary;
          }),
          backgroundColor: MaterialStateProperty.resolveWith((states) {
            if (states.contains(MaterialState.hovered) ||
                states.contains(MaterialState.focused)) {
              return colorBackgroundHover;
            }
            return Colors.transparent;
          }),
          padding: const MaterialStatePropertyAll(
            EdgeInsets.symmetric(horizontal: 12, vertical: 8),
          ),
          textStyle: const MaterialStatePropertyAll(
            TextStyle(fontWeight: FontWeight.w600),
          ),
        ),
      ),
      dialogTheme: DialogThemeData(
        backgroundColor: colorBackgroundElevated,
        shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(8)),
        elevation: 0,
        shadowColor: const Color.fromRGBO(100, 90, 80, 0.12),
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
