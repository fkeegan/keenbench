import 'dart:convert';
import 'dart:math' as math;
import 'dart:typed_data';

import 'package:flutter/material.dart';
import 'package:flutter_markdown_plus/flutter_markdown_plus.dart';
import 'package:provider/provider.dart';

import '../app_keys.dart';
import '../engine/engine_client.dart';
import '../models/models.dart';
import '../theme.dart';
import '../widgets/keenbench_app_bar.dart';

class ReviewScreen extends StatefulWidget {
  const ReviewScreen({super.key, required this.workbenchId});

  final String workbenchId;

  @override
  State<ReviewScreen> createState() => _ReviewScreenState();
}

class _ReviewScreenState extends State<ReviewScreen> {
  static const double _defaultPagePreviewScale = 1.0;
  static const double _docxPagePreviewScale = 1.5;
  static const Size _defaultPptxCanvasSize = Size(960, 540);
  static const double _emuPerPoint = 12700.0;
  static const String _pptxFallbackFontFamily = 'Inter';
  static const Map<String, String> _bundledFontFamilies = {
    'inter': 'Inter',
    'jetbrains mono': 'JetBrains Mono',
  };
  static const Map<String, String> _osKnownFontFamilies = {
    'arial': 'Arial',
    'calibri': 'Calibri',
    'cambria': 'Cambria',
    'consolas': 'Consolas',
    'courier new': 'Courier New',
    'georgia': 'Georgia',
    'helvetica': 'Helvetica',
    'menlo': 'Menlo',
    'noto sans': 'Noto Sans',
    'noto serif': 'Noto Serif',
    'roboto': 'Roboto',
    'segoe ui': 'Segoe UI',
    'tahoma': 'Tahoma',
    'times new roman': 'Times New Roman',
    'trebuchet ms': 'Trebuchet MS',
    'verdana': 'Verdana',
  };

  List<ChangeItem> _changes = [];
  String? _draftSummary;
  List<DiffHunk> _hunks = [];
  bool _loading = true;
  bool _diffLoading = false;
  bool _diffTooLarge = false;
  bool _baselineMissing = false;
  String? _diffReferenceWarning;

  ChangeItem? _selected;

  int _pageIndex = 0;
  int _pageCount = 0;
  String? _draftPreview;
  String? _publishedPreview;
  bool _previewLoading = false;
  String? _previewError;
  int _structuredIndex = 0;
  int _structuredCount = 0;
  Map<String, dynamic>? _structuredDraftContent;
  Map<String, dynamic>? _structuredBaselineContent;
  bool _structuredFallbackActive = false;
  bool _structuredLoading = false;
  bool _structuredBaselineMissing = false;
  String _structuredReferenceSource = 'none';
  String? _structuredReferenceWarning;
  String? _structuredError;

  List<String> _sheets = [];
  String? _sheet;
  int _rowStart = 0;
  int _colStart = 0;
  int _rowCount = 20;
  int _colCount = 10;
  List<List<dynamic>> _draftGrid = [];
  List<List<dynamic>> _publishedGrid = [];
  bool _gridLoading = false;

  Map<String, dynamic>? _imageDraft;
  Map<String, dynamic>? _imagePublished;
  bool _hasPublishedImage = false;

  @override
  void initState() {
    super.initState();
    _loadChanges();
  }

  Future<void> _loadChanges() async {
    final engine = context.read<EngineApi>();
    try {
      final response = await engine.call('ReviewGetChangeSet', {
        'workbench_id': widget.workbenchId,
      });
      final changeList = (response['changes'] as List<dynamic>? ?? [])
          .cast<Map<String, dynamic>>();
      final draftSummary = response['draft_summary']?.toString();
      setState(() {
        _changes = changeList.map(ChangeItem.fromJson).toList();
        _draftSummary = draftSummary;
        _loading = false;
        if (_changes.isNotEmpty) {
          _selected = _changes.first;
        }
      });
      if (_selected != null) {
        await _selectChange(_selected!);
      }
    } catch (err) {
      setState(() {
        _loading = false;
      });
      if (mounted) {
        ScaffoldMessenger.of(
          context,
        ).showSnackBar(SnackBar(content: Text(err.toString())));
      }
    }
  }

  Future<void> _selectChange(ChangeItem change) async {
    setState(() {
      _selected = change;
      _hunks = [];
      _diffTooLarge = false;
      _baselineMissing = false;
      _diffReferenceWarning = null;
      _pageIndex = 0;
      _pageCount = 0;
      _draftPreview = null;
      _publishedPreview = null;
      _previewError = null;
      _structuredIndex = 0;
      _structuredCount = 0;
      _structuredDraftContent = null;
      _structuredBaselineContent = null;
      _structuredFallbackActive = false;
      _structuredLoading = false;
      _structuredBaselineMissing = false;
      _structuredReferenceSource = 'none';
      _structuredReferenceWarning = null;
      _structuredError = null;
      _sheets = [];
      _sheet = null;
      _rowStart = 0;
      _colStart = 0;
      _draftGrid = [];
      _publishedGrid = [];
      _imageDraft = null;
      _imagePublished = null;
      _hasPublishedImage = change.changeType != 'added';
      final hint = change.focusHint;
      if (change.fileKind == 'xlsx' && hint != null) {
        final sheet = hint['sheet'] as String?;
        final rowStart = (hint['row_start'] as num?)?.toInt();
        final colStart = (hint['col_start'] as num?)?.toInt();
        if (sheet != null && sheet.isNotEmpty) {
          _sheet = sheet;
        }
        if (rowStart != null && rowStart >= 0) {
          _rowStart = rowStart;
        }
        if (colStart != null && colStart >= 0) {
          _colStart = colStart;
        }
      }
      if (change.fileKind == 'docx' && hint != null) {
        final sectionIndex = (hint['section_index'] as num?)?.toInt();
        if (sectionIndex != null && sectionIndex >= 0) {
          _structuredIndex = sectionIndex;
        }
      }
      if (change.fileKind == 'pptx' && hint != null) {
        final slideIndex = (hint['slide_index'] as num?)?.toInt();
        if (slideIndex != null && slideIndex >= 0) {
          _pageIndex = slideIndex;
          _structuredIndex = slideIndex;
        }
      }
    });

    if (_shouldLoadDiff(change)) {
      await _loadDiff(change.path);
    }
    await _loadPreview(change);
  }

  bool _shouldLoadDiff(ChangeItem change) {
    if (change.fileKind == 'pptx') {
      return false;
    }
    if (change.changeType == 'added') {
      return change.fileKind == 'text';
    }
    if (change.previewKind == 'image' || change.isOpaque) {
      return false;
    }
    return change.fileKind == 'text' ||
        change.fileKind == 'docx' ||
        change.fileKind == 'odt' ||
        change.fileKind == 'xlsx' ||
        change.fileKind == 'pdf';
  }

  Future<void> _loadDiff(String path) async {
    final engine = context.read<EngineApi>();
    setState(() {
      _diffLoading = true;
    });
    final response = await engine.call('ReviewGetTextDiff', {
      'workbench_id': widget.workbenchId,
      'path': path,
    });
    final hunkList = (response['hunks'] as List<dynamic>? ?? [])
        .cast<Map<String, dynamic>>();
    setState(() {
      _hunks = hunkList.map(DiffHunk.fromJson).toList();
      _diffTooLarge = response['too_large'] == true;
      _baselineMissing = response['baseline_missing'] == true;
      _diffReferenceWarning = response['reference_warning']?.toString();
      _diffLoading = false;
    });
  }

  Future<void> _loadPreview(ChangeItem change) async {
    if (change.previewKind == 'none') {
      return;
    }
    if (change.previewKind == 'image') {
      await _loadImagePreview(change.path);
      return;
    }
    if (change.fileKind == 'xlsx') {
      await _loadXlsxPreview(change);
      return;
    }
    if (change.fileKind == 'pptx') {
      final loadedStructured = await _loadStructuredContentDiff(change);
      if (!loadedStructured) {
        await _loadSlidePreview(change);
      }
      return;
    }
    if (change.fileKind == 'pdf' ||
        change.fileKind == 'docx' ||
        change.fileKind == 'odt') {
      await _loadPagePreview(change);
    }
  }

  Future<void> _loadPagePreview(ChangeItem change) async {
    final engine = context.read<EngineApi>();
    final previewScale = _previewScaleForPage(change);
    setState(() {
      _previewLoading = true;
      _previewError = null;
    });
    try {
      final method = change.fileKind == 'pdf'
          ? 'ReviewGetPdfPreviewPage'
          : change.fileKind == 'docx'
          ? 'ReviewGetDocxPreviewPage'
          : 'ReviewGetOdtPreviewPage';
      final draftResp =
          await engine.call(method, {
                'workbench_id': widget.workbenchId,
                'path': change.path,
                'version': 'draft',
                'page_index': _pageIndex,
                'scale': previewScale,
              })
              as Map<String, dynamic>;
      Map<String, dynamic>? pubResp;
      if (change.changeType != 'added') {
        pubResp =
            await engine.call(method, {
                  'workbench_id': widget.workbenchId,
                  'path': change.path,
                  'version': 'published',
                  'page_index': _pageIndex,
                  'scale': previewScale,
                })
                as Map<String, dynamic>;
      }
      if (!mounted) {
        return;
      }
      setState(() {
        _draftPreview = draftResp['bytes_base64'] as String?;
        _publishedPreview = pubResp?['bytes_base64'] as String?;
        _pageCount =
            (draftResp['page_count'] as num?)?.toInt() ??
            (pubResp?['page_count'] as num?)?.toInt() ??
            0;
        _previewLoading = false;
      });
    } catch (err) {
      if (!mounted) {
        return;
      }
      if (change.fileKind == 'docx') {
        final loadedFallback = await _loadStructuredContentDiff(change);
        if (!mounted) {
          return;
        }
        if (loadedFallback) {
          return;
        }
      }
      setState(() {
        _draftPreview = null;
        _publishedPreview = null;
        _pageCount = 0;
        _previewLoading = false;
        _previewError = _formatPreviewError(err);
      });
    }
  }

  Future<void> _loadSlidePreview(ChangeItem change) async {
    final engine = context.read<EngineApi>();
    setState(() {
      _previewLoading = true;
      _previewError = null;
    });
    try {
      final draftResp =
          await engine.call('ReviewGetPptxPreviewSlide', {
                'workbench_id': widget.workbenchId,
                'path': change.path,
                'version': 'draft',
                'slide_index': _pageIndex,
                'scale': 1.0,
              })
              as Map<String, dynamic>;
      Map<String, dynamic>? pubResp;
      if (change.changeType != 'added') {
        try {
          pubResp =
              await engine.call('ReviewGetPptxPreviewSlide', {
                    'workbench_id': widget.workbenchId,
                    'path': change.path,
                    'version': 'published',
                    'slide_index': _pageIndex,
                    'scale': 1.0,
                  })
                  as Map<String, dynamic>;
        } catch (err) {
          if (!_isMissingPublishedTargetError(err)) {
            rethrow;
          }
        }
      }
      if (!mounted) {
        return;
      }
      setState(() {
        _draftPreview = draftResp['bytes_base64'] as String?;
        _publishedPreview = pubResp?['bytes_base64'] as String?;
        _pageCount =
            (draftResp['slide_count'] as num?)?.toInt() ??
            (pubResp?['slide_count'] as num?)?.toInt() ??
            0;
        _previewLoading = false;
      });
    } catch (err) {
      if (!mounted) {
        return;
      }
      final loadedFallback = await _loadStructuredContentDiff(change);
      if (!mounted) {
        return;
      }
      if (loadedFallback) {
        return;
      }
      setState(() {
        _draftPreview = null;
        _publishedPreview = null;
        _pageCount = 0;
        _previewLoading = false;
        _previewError = _formatPreviewError(err);
      });
    }
  }

  Future<bool> _loadStructuredContentDiff(ChangeItem change) async {
    if (change.fileKind != 'docx' && change.fileKind != 'pptx') {
      return false;
    }
    final engine = context.read<EngineApi>();
    setState(() {
      _structuredFallbackActive = true;
      _structuredLoading = true;
      _structuredError = null;
      _previewLoading = false;
    });
    try {
      final method = change.fileKind == 'docx'
          ? 'ReviewGetDocxContentDiff'
          : 'ReviewGetPptxContentDiff';
      final indexKey = change.fileKind == 'docx'
          ? 'section_index'
          : 'slide_index';
      final countKey = change.fileKind == 'docx'
          ? 'section_count'
          : 'slide_count';
      final response =
          await engine.call(method, {
                'workbench_id': widget.workbenchId,
                'path': change.path,
                indexKey: _structuredIndex,
              })
              as Map<String, dynamic>;
      final diff = StructuredContentDiff.fromJson(response, countKey: countKey);
      if (!mounted) {
        return false;
      }
      setState(() {
        _structuredDraftContent = diff.draft;
        _structuredBaselineContent = diff.baseline;
        _structuredCount = diff.itemCount;
        _structuredBaselineMissing = diff.baselineMissing;
        _structuredReferenceSource = diff.referenceSource;
        _structuredReferenceWarning = diff.referenceWarning;
        _structuredLoading = false;
        _previewError = null;
      });
      return true;
    } catch (err) {
      if (!mounted) {
        return false;
      }
      setState(() {
        _structuredFallbackActive = false;
        _structuredLoading = false;
        _structuredError = _formatPreviewError(err);
      });
      return false;
    }
  }

  Future<void> _loadXlsxPreview(ChangeItem change) async {
    setState(() {
      _gridLoading = true;
      _previewError = null;
    });
    try {
      var requestedSheet = (_sheet ?? '').trim();
      Map<String, dynamic> draftResp;
      try {
        draftResp = await _requestXlsxPreviewGrid(
          change: change,
          version: 'draft',
          sheet: requestedSheet,
        );
      } catch (err) {
        if (!_isMissingXlsxSheetError(err) || requestedSheet.isEmpty) {
          rethrow;
        }
        requestedSheet = '';
        draftResp = await _requestXlsxPreviewGrid(
          change: change,
          version: 'draft',
          sheet: requestedSheet,
        );
      }
      final sheets = (draftResp['sheets'] as List<dynamic>? ?? [])
          .map((e) => e.toString())
          .toList();
      if (requestedSheet.isEmpty || !sheets.contains(requestedSheet)) {
        requestedSheet = sheets.isNotEmpty ? sheets.first : '';
      }
      Map<String, dynamic>? pubResp;
      if (change.changeType != 'added') {
        try {
          pubResp = await _requestXlsxPreviewGrid(
            change: change,
            version: 'published',
            sheet: requestedSheet,
          );
        } catch (err) {
          if (!_isMissingPublishedTargetError(err)) {
            rethrow;
          }
        }
      }
      if (!mounted) {
        return;
      }
      setState(() {
        _sheets = sheets;
        _sheet = requestedSheet.isEmpty ? null : requestedSheet;
        _draftGrid = (draftResp['cells'] as List<dynamic>? ?? [])
            .map((row) => row as List<dynamic>)
            .toList();
        _publishedGrid = (pubResp?['cells'] as List<dynamic>? ?? [])
            .map((row) => row as List<dynamic>)
            .toList();
        _gridLoading = false;
      });
    } catch (err) {
      if (!mounted) {
        return;
      }
      setState(() {
        _sheets = [];
        _draftGrid = [];
        _publishedGrid = [];
        _gridLoading = false;
        _previewError = _formatPreviewError(err);
      });
    }
  }

  Future<Map<String, dynamic>> _requestXlsxPreviewGrid({
    required ChangeItem change,
    required String version,
    required String sheet,
  }) async {
    final engine = context.read<EngineApi>();
    return await engine.call('ReviewGetXlsxPreviewGrid', {
          'workbench_id': widget.workbenchId,
          'path': change.path,
          'version': version,
          'sheet': sheet,
          'row_start': _rowStart,
          'row_count': _rowCount,
          'col_start': _colStart,
          'col_count': _colCount,
        })
        as Map<String, dynamic>;
  }

  Future<void> _loadImagePreview(String path) async {
    final engine = context.read<EngineApi>();
    setState(() {
      _previewLoading = true;
      _previewError = null;
    });
    try {
      final response =
          await engine.call('ReviewGetImagePreview', {
                'workbench_id': widget.workbenchId,
                'path': path,
              })
              as Map<String, dynamic>;
      if (!mounted) {
        return;
      }
      setState(() {
        _imageDraft = response['draft'] as Map<String, dynamic>?;
        _imagePublished = response['published'] as Map<String, dynamic>?;
        _hasPublishedImage = response['has_published'] == true;
        _previewLoading = false;
      });
    } catch (err) {
      if (!mounted) {
        return;
      }
      setState(() {
        _imageDraft = null;
        _imagePublished = null;
        _hasPublishedImage = false;
        _previewLoading = false;
        _previewError = _formatPreviewError(err);
      });
    }
  }

  Widget _buildDetailPane(BuildContext context) {
    final change = _selected!;
    final hasDiff = _shouldLoadDiff(change);
    final hasPreview = change.previewKind != 'none' || change.isOpaque;
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        _buildSummaryPanel(change),
        const SizedBox(height: 12),
        if (hasDiff && hasPreview) ...[
          Expanded(child: _buildDiffPanel(change)),
          const SizedBox(height: 12),
          SizedBox(height: 280, child: _buildPreviewPanel(change)),
        ] else if (hasDiff)
          Expanded(child: _buildDiffPanel(change))
        else if (hasPreview)
          Expanded(child: _buildPreviewPanel(change))
        else
          const Expanded(child: Center(child: Text('No preview available.'))),
      ],
    );
  }

  Widget _buildSummaryPanel(ChangeItem change) {
    final summary = _resolveSummary(change);
    final badges = <Widget>[];
    if (change.changeType == 'added') {
      badges.add(
        const _InlineBadge(
          label: 'New file',
          background: KeenBenchTheme.colorSuccessBackground,
          textColor: KeenBenchTheme.colorSuccessText,
        ),
      );
    }
    if (change.isOpaque) {
      badges.add(
        const _InlineBadge(
          label: 'Opaque',
          background: KeenBenchTheme.colorSurfaceMuted,
          textColor: KeenBenchTheme.colorTextSecondary,
        ),
      );
    } else if (change.fileKind == 'pdf' ||
        change.fileKind == 'image' ||
        change.fileKind == 'odt') {
      badges.add(
        const _InlineBadge(
          label: 'Read-only',
          background: KeenBenchTheme.colorInfoBackground,
          textColor: KeenBenchTheme.colorInfoText,
        ),
      );
    }
    return Container(
      padding: const EdgeInsets.all(12),
      decoration: BoxDecoration(
        color: KeenBenchTheme.colorBackgroundSecondary,
        borderRadius: BorderRadius.circular(8),
        border: Border.all(color: KeenBenchTheme.colorBorderSubtle),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            children: [
              Text('Summary', style: Theme.of(context).textTheme.bodyMedium),
              const SizedBox(width: 8),
              ...badges,
            ],
          ),
          const SizedBox(height: 6),
          MarkdownBody(
            data: summary,
            selectable: false,
            onTapLink: (_, __, ___) {},
            styleSheet: MarkdownStyleSheet.fromTheme(
              Theme.of(context),
            ).copyWith(p: Theme.of(context).textTheme.bodySmall),
          ),
        ],
      ),
    );
  }

  String _resolveSummary(ChangeItem change) {
    final perFile = change.summary.trim();
    if (perFile.isNotEmpty) {
      return perFile;
    }
    final draftSummary = _draftSummary?.trim() ?? '';
    if (draftSummary.isNotEmpty) {
      return draftSummary;
    }
    return 'Summary unavailable.';
  }

  Widget _buildDiffPanel(ChangeItem change) {
    if (_diffLoading) {
      return const Center(child: CircularProgressIndicator());
    }
    if (_baselineMissing) {
      final warning = _diffReferenceWarning?.trim();
      return Center(
        child: Text(
          warning == null || warning.isEmpty
              ? 'Reference unavailable for this file.'
              : warning,
        ),
      );
    }
    if (_diffTooLarge) {
      return const Center(child: Text('Diff too large to display.'));
    }
    if (_hunks.isEmpty) {
      return const Center(child: Text('No diff available for this file.'));
    }
    final diffList = ListView(
      key: AppKeys.reviewDiffList,
      children: _hunks.expand((hunk) => hunk.lines).map((line) {
        Color? background;
        Color? accent;
        if (line.type == 'added') {
          background = KeenBenchTheme.colorDiffAdded;
          accent = KeenBenchTheme.colorPublishedIndicator;
        } else if (line.type == 'removed') {
          background = KeenBenchTheme.colorDiffRemoved;
          accent = KeenBenchTheme.colorErrorText;
        }
        return Container(
          color: background,
          child: Row(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Container(width: 4, color: accent ?? Colors.transparent),
              Expanded(
                child: Padding(
                  padding: const EdgeInsets.symmetric(
                    vertical: 2,
                    horizontal: 8,
                  ),
                  child: Row(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      SizedBox(
                        width: 48,
                        child: Text(
                          line.oldLine?.toString() ?? '',
                          style: Theme.of(context).textTheme.labelSmall
                              ?.copyWith(
                                color: KeenBenchTheme.colorTextTertiary,
                              ),
                        ),
                      ),
                      SizedBox(
                        width: 48,
                        child: Text(
                          line.newLine?.toString() ?? '',
                          style: Theme.of(context).textTheme.labelSmall
                              ?.copyWith(
                                color: KeenBenchTheme.colorTextTertiary,
                              ),
                        ),
                      ),
                      Expanded(
                        child: Text(line.text, style: KeenBenchTheme.mono),
                      ),
                    ],
                  ),
                ),
              ),
            ],
          ),
        );
      }).toList(),
    );
    final warning = _diffReferenceWarning?.trim();
    if (warning == null || warning.isEmpty) {
      return diffList;
    }
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        Container(
          padding: const EdgeInsets.all(8),
          margin: const EdgeInsets.only(bottom: 8),
          decoration: BoxDecoration(
            color: KeenBenchTheme.colorInfoBackground,
            borderRadius: BorderRadius.circular(6),
            border: Border.all(color: KeenBenchTheme.colorBorderSubtle),
          ),
          child: Text(
            warning,
            style: Theme.of(context).textTheme.bodySmall?.copyWith(
              color: KeenBenchTheme.colorInfoText,
            ),
          ),
        ),
        Expanded(child: diffList),
      ],
    );
  }

  Widget _buildPreviewPanel(ChangeItem change) {
    if ((change.fileKind == 'docx' || change.fileKind == 'pptx') &&
        _structuredFallbackActive) {
      return _buildStructuredFallbackPreview(change);
    }
    if (_previewLoading || _gridLoading) {
      return const Center(child: CircularProgressIndicator());
    }
    if (_previewError != null && _previewError!.isNotEmpty) {
      return Center(
        child: Text(
          'Preview unavailable: $_previewError',
          textAlign: TextAlign.center,
        ),
      );
    }
    if (change.previewKind == 'image') {
      return _buildImagePreview();
    }
    if (change.fileKind == 'xlsx') {
      return _buildGridPreview(change);
    }
    if (change.fileKind == 'pdf' ||
        change.fileKind == 'docx' ||
        change.fileKind == 'odt' ||
        change.fileKind == 'pptx') {
      return _buildPagePreview(change);
    }
    if (change.isOpaque) {
      return _buildOpaquePreview(change);
    }
    return const Center(child: Text('No preview available.'));
  }

  Widget _buildStructuredFallbackPreview(ChangeItem change) {
    if (_structuredLoading) {
      return const Center(child: CircularProgressIndicator());
    }
    if (_structuredError != null && _structuredError!.isNotEmpty) {
      return Center(
        child: Text(
          'Structured diff unavailable: $_structuredError',
          textAlign: TextAlign.center,
        ),
      );
    }
    final hasPublished = change.changeType != 'added';
    final label = change.fileKind == 'docx' ? 'Sections' : 'Slides';
    var leftLabel = 'Reference';
    if (_structuredReferenceSource == 'published_current_fallback') {
      leftLabel = 'Reference (Published fallback)';
    } else if (_structuredReferenceSource == 'draft_start_snapshot') {
      leftLabel = 'Reference (Draft start)';
    }
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        _buildPreviewToolbar(
          label: label,
          count: _structuredCount,
          currentIndex: _structuredIndex,
          onPrev: _structuredIndex > 0
              ? () {
                  setState(() {
                    _structuredIndex -= 1;
                    if (change.fileKind == 'pptx') {
                      _pageIndex = _structuredIndex;
                    }
                  });
                  _loadStructuredContentDiff(change);
                }
              : null,
          onNext:
              _structuredCount > 0 && _structuredIndex < _structuredCount - 1
              ? () {
                  setState(() {
                    _structuredIndex += 1;
                    if (change.fileKind == 'pptx') {
                      _pageIndex = _structuredIndex;
                    }
                  });
                  _loadStructuredContentDiff(change);
                }
              : null,
        ),
        if ((_structuredReferenceWarning ?? '').trim().isNotEmpty)
          Padding(
            padding: const EdgeInsets.only(bottom: 8),
            child: Text(
              _structuredReferenceWarning!,
              style: Theme.of(context).textTheme.bodySmall?.copyWith(
                color: KeenBenchTheme.colorTextSecondary,
              ),
            ),
          ),
        if (_structuredBaselineMissing)
          Padding(
            padding: const EdgeInsets.only(bottom: 8),
            child: Text(
              'Reference unavailable for this selection.',
              style: Theme.of(context).textTheme.bodySmall?.copyWith(
                color: KeenBenchTheme.colorTextSecondary,
              ),
            ),
          ),
        Expanded(
          child: change.fileKind == 'pptx'
              ? _buildPptxPositionedOrFallbackComparison(
                  hasPublished: hasPublished,
                  leftLabel: leftLabel,
                )
              : _sideBySide(
                  leftLabel: leftLabel,
                  rightLabel: 'Draft',
                  leftChild: _buildStructuredSide(
                    change,
                    _structuredBaselineContent,
                  ),
                  rightChild: _buildStructuredSide(
                    change,
                    _structuredDraftContent,
                  ),
                  showLeft: hasPublished,
                ),
        ),
      ],
    );
  }

  Widget _buildStructuredSide(
    ChangeItem change,
    Map<String, dynamic>? payload,
  ) {
    if (payload == null) {
      return const Center(child: Text('No structured content.'));
    }
    if (change.fileKind == 'docx') {
      return _buildDocxStructured(payload);
    }
    if (change.fileKind == 'pptx') {
      return _buildPptxStructured(payload);
    }
    return const Center(child: Text('No structured content.'));
  }

  Widget _buildDocxStructured(Map<String, dynamic> section) {
    final heading = section['heading']?.toString().trim();
    final level = (section['level'] as num?)?.toInt();
    final paragraphs = _asMapList(section['paragraphs']);
    final tables = _asMapList(section['tables']);
    final images = _asMapList(section['images']);
    return ListView(
      padding: const EdgeInsets.all(8),
      children: [
        Text(
          heading == null || heading.isEmpty ? '(untitled)' : heading,
          style: Theme.of(context).textTheme.titleSmall,
        ),
        if (level != null) ...[
          const SizedBox(height: 2),
          Text(
            'Heading level $level',
            style: Theme.of(context).textTheme.bodySmall?.copyWith(
              color: KeenBenchTheme.colorTextSecondary,
            ),
          ),
        ],
        const SizedBox(height: 8),
        Text(
          'Paragraphs (${paragraphs.length})',
          style: Theme.of(context).textTheme.labelSmall,
        ),
        const SizedBox(height: 4),
        if (paragraphs.isEmpty)
          const Text('No paragraph content.')
        else
          ...paragraphs.map((paragraph) {
            final text = paragraph['text']?.toString() ?? '';
            return Padding(
              padding: const EdgeInsets.only(bottom: 6),
              child: Text(text.isEmpty ? '(empty paragraph)' : text),
            );
          }),
        const SizedBox(height: 8),
        Text(
          'Tables (${tables.length})',
          style: Theme.of(context).textTheme.labelSmall,
        ),
        const SizedBox(height: 4),
        if (tables.isEmpty)
          const Text('No tables.')
        else
          ...tables.map((table) {
            final rows = (table['row_count'] as num?)?.toInt() ?? 0;
            final cols = (table['col_count'] as num?)?.toInt() ?? 0;
            return Padding(
              padding: const EdgeInsets.only(bottom: 4),
              child: Text('Table ${table['index'] ?? 0}: ${rows} x ${cols}'),
            );
          }),
        const SizedBox(height: 8),
        Text(
          'Images (${images.length})',
          style: Theme.of(context).textTheme.labelSmall,
        ),
        const SizedBox(height: 4),
        if (images.isEmpty)
          const Text('No image references.')
        else
          ...images.map((image) {
            final target = image['target']?.toString() ?? '';
            final relId = image['rel_id']?.toString() ?? '';
            if (target.isNotEmpty) {
              return Padding(
                padding: const EdgeInsets.only(bottom: 4),
                child: Text(target),
              );
            }
            return Padding(
              padding: const EdgeInsets.only(bottom: 4),
              child: Text(relId.isEmpty ? 'image' : relId),
            );
          }),
      ],
    );
  }

  Widget _buildPptxStructured(Map<String, dynamic> slide) {
    final title = slide['title']?.toString().trim();
    final layout = slide['layout']?.toString().trim();
    final shapes = _asMapList(slide['shapes']);
    final notes = slide['notes']?.toString().trim() ?? '';
    return ListView(
      padding: const EdgeInsets.all(8),
      children: [
        Text(
          title == null || title.isEmpty ? '(untitled slide)' : title,
          style: Theme.of(context).textTheme.titleSmall,
        ),
        if (layout != null && layout.isNotEmpty) ...[
          const SizedBox(height: 2),
          Text(
            layout,
            style: Theme.of(context).textTheme.bodySmall?.copyWith(
              color: KeenBenchTheme.colorTextSecondary,
            ),
          ),
        ],
        const SizedBox(height: 8),
        Text(
          'Shapes (${shapes.length})',
          style: Theme.of(context).textTheme.labelSmall,
        ),
        const SizedBox(height: 4),
        if (shapes.isEmpty)
          const Text('No shapes.')
        else
          ...shapes.map((shape) {
            final name = shape['name']?.toString().trim();
            final type = shape['shape_type']?.toString().trim();
            final blocks = _asMapList(shape['text_blocks']);
            final textLines = blocks
                .map((block) => block['text']?.toString() ?? '')
                .where((line) => line.trim().isNotEmpty)
                .join('\n');
            return Container(
              margin: const EdgeInsets.only(bottom: 6),
              padding: const EdgeInsets.all(6),
              decoration: BoxDecoration(
                color: KeenBenchTheme.colorBackgroundSecondary,
                borderRadius: BorderRadius.circular(6),
                border: Border.all(color: KeenBenchTheme.colorBorderSubtle),
              ),
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text(
                    name == null || name.isEmpty ? '(unnamed shape)' : name,
                    style: Theme.of(context).textTheme.labelMedium,
                  ),
                  if (type != null && type.isNotEmpty)
                    Text(
                      type,
                      style: Theme.of(context).textTheme.bodySmall?.copyWith(
                        color: KeenBenchTheme.colorTextSecondary,
                      ),
                    ),
                  if (textLines.isNotEmpty) ...[
                    const SizedBox(height: 4),
                    Text(textLines),
                  ],
                ],
              ),
            );
          }),
        if (notes.isNotEmpty) ...[
          const SizedBox(height: 8),
          Text('Notes', style: Theme.of(context).textTheme.labelSmall),
          const SizedBox(height: 4),
          Text(notes),
        ],
      ],
    );
  }

  Widget _buildPptxPositionedOrFallbackComparison({
    required bool hasPublished,
    required String leftLabel,
  }) {
    final draftSlide = PptxPositionedSlide.fromPayload(_structuredDraftContent);
    final baselineSlide = PptxPositionedSlide.fromPayload(
      _structuredBaselineContent,
    );
    final draftRenderable = draftSlide?.isRenderable == true;
    final baselineRenderable = baselineSlide?.isRenderable == true;
    final positionedReady =
        draftRenderable &&
        (!hasPublished || _structuredBaselineMissing || baselineRenderable);
    if (!positionedReady) {
      return Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          Container(
            padding: const EdgeInsets.all(8),
            margin: const EdgeInsets.only(bottom: 8),
            decoration: BoxDecoration(
              color: KeenBenchTheme.colorBackgroundSecondary,
              borderRadius: BorderRadius.circular(6),
              border: Border.all(color: KeenBenchTheme.colorBorderSubtle),
            ),
            child: Text(
              'Positioned slide data is incomplete. Showing structured metadata fallback.',
              style: Theme.of(context).textTheme.bodySmall?.copyWith(
                color: KeenBenchTheme.colorTextSecondary,
              ),
            ),
          ),
          Expanded(
            child: _sideBySide(
              leftLabel: leftLabel,
              rightLabel: 'Draft',
              leftChild: _buildPptxStructuredFallbackSide(
                _structuredBaselineContent,
              ),
              rightChild: _buildPptxStructuredFallbackSide(
                _structuredDraftContent,
              ),
              showLeft: hasPublished,
            ),
          ),
        ],
      );
    }
    return LayoutBuilder(
      builder: (context, constraints) {
        final totalWidth = constraints.maxWidth;
        final paneWidth = hasPublished
            ? ((totalWidth - 8).isFinite
                  ? (totalWidth - 8) / 2
                  : totalWidth / 2)
            : totalWidth;
        final metrics = _computePptxRenderMetrics(
          draftSlide: draftSlide!,
          baselineSlide: baselineSlide,
          paneWidth: paneWidth,
        );
        return _sideBySide(
          leftLabel: leftLabel,
          rightLabel: 'Draft',
          leftChild: _structuredBaselineMissing
              ? const Center(
                  child: Text('Reference unavailable for this selection.'),
                )
              : _buildPptxPositionedSide(
                  slide: baselineSlide!,
                  metrics: metrics,
                ),
          rightChild: _buildPptxPositionedSide(
            slide: draftSlide,
            metrics: metrics,
          ),
          showLeft: hasPublished,
        );
      },
    );
  }

  _PptxRenderMetrics _computePptxRenderMetrics({
    required PptxPositionedSlide draftSlide,
    required PptxPositionedSlide? baselineSlide,
    required double paneWidth,
  }) {
    final coordinateScale = _resolvePptxCoordinateScale(
      draftSlide: draftSlide,
      baselineSlide: baselineSlide,
    );
    final draftWidth = draftSlide.width * coordinateScale;
    final draftHeight = draftSlide.height * coordinateScale;
    final baselineWidth = (baselineSlide?.width ?? 0) * coordinateScale;
    final baselineHeight = (baselineSlide?.height ?? 0) * coordinateScale;
    final canvasWidth = math.max(
      _defaultPptxCanvasSize.width,
      math.max(draftWidth, baselineWidth),
    );
    final canvasHeight = math.max(
      _defaultPptxCanvasSize.height,
      math.max(draftHeight, baselineHeight),
    );
    final usableWidth = math.max(160.0, paneWidth - 16);
    final scale = (usableWidth / canvasWidth).clamp(0.1, 1.0).toDouble();
    return _PptxRenderMetrics(
      coordinateScale: coordinateScale,
      canvasWidth: canvasWidth,
      canvasHeight: canvasHeight,
      viewScale: scale,
    );
  }

  double _resolvePptxCoordinateScale({
    required PptxPositionedSlide draftSlide,
    required PptxPositionedSlide? baselineSlide,
  }) {
    final candidates = <double>[
      draftSlide.width,
      draftSlide.height,
      for (final shape in draftSlide.shapes) ...[
        shape.x + shape.width,
        shape.y + shape.height,
      ],
    ];
    if (baselineSlide != null) {
      candidates.addAll([baselineSlide.width, baselineSlide.height]);
      for (final shape in baselineSlide.shapes) {
        candidates.addAll([shape.x + shape.width, shape.y + shape.height]);
      }
    }
    final maxExtent = candidates.fold<double>(
      0.0,
      (best, value) => value > best ? value : best,
    );
    if (maxExtent > 5000) {
      return 1 / _emuPerPoint;
    }
    return 1.0;
  }

  Widget _buildPptxPositionedSide({
    required PptxPositionedSlide slide,
    required _PptxRenderMetrics metrics,
  }) {
    final fontResolutions = _collectFontResolutions(slide);
    final missingFonts =
        fontResolutions
            .where((resolution) => resolution.missing)
            .map((resolution) => resolution.requestedFamily)
            .toSet()
            .toList()
          ..sort();
    final canvas = Container(
      decoration: BoxDecoration(
        color: Colors.white,
        borderRadius: BorderRadius.circular(6),
        border: Border.all(color: KeenBenchTheme.colorBorderSubtle),
      ),
      child: SingleChildScrollView(
        padding: const EdgeInsets.all(6),
        child: Align(
          alignment: Alignment.topCenter,
          child: SizedBox(
            width: metrics.canvasWidth * metrics.viewScale,
            height: metrics.canvasHeight * metrics.viewScale,
            child: Stack(
              children: slide.shapes.map((shape) {
                final left =
                    shape.x * metrics.coordinateScale * metrics.viewScale;
                final top =
                    shape.y * metrics.coordinateScale * metrics.viewScale;
                final width =
                    shape.width * metrics.coordinateScale * metrics.viewScale;
                final height =
                    shape.height * metrics.coordinateScale * metrics.viewScale;
                if (width <= 0 || height <= 0) {
                  return const SizedBox.shrink();
                }
                return Positioned(
                  left: left,
                  top: top,
                  width: width,
                  height: height,
                  child: Container(
                    padding: EdgeInsets.all(
                      math.max(2, 4 * metrics.viewScale).toDouble(),
                    ),
                    decoration: BoxDecoration(
                      color:
                          _parsePptxColor(shape.fillColor) ??
                          Colors.transparent,
                      borderRadius: BorderRadius.circular(2),
                      border: Border.all(
                        color: KeenBenchTheme.colorBorderSubtle,
                      ),
                    ),
                    child: _buildPptxShapeText(shape: shape, metrics: metrics),
                  ),
                );
              }).toList(),
            ),
          ),
        ),
      ),
    );
    final preface = <Widget>[
      Text(
        'Font resolution: bundled -> OS-known -> fallback',
        style: Theme.of(context).textTheme.labelSmall?.copyWith(
          color: KeenBenchTheme.colorTextSecondary,
        ),
      ),
      if (missingFonts.isNotEmpty) ...[
        const SizedBox(height: 4),
        Container(
          padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 6),
          decoration: BoxDecoration(
            color: KeenBenchTheme.colorInfoBackground,
            borderRadius: BorderRadius.circular(6),
            border: Border.all(color: KeenBenchTheme.colorBorderSubtle),
          ),
          child: Text(
            'Missing fonts (${missingFonts.length}): ${missingFonts.join(', ')}. Using $_pptxFallbackFontFamily.',
            style: Theme.of(context).textTheme.bodySmall?.copyWith(
              color: KeenBenchTheme.colorInfoText,
            ),
          ),
        ),
      ],
      const SizedBox(height: 6),
    ];
    return LayoutBuilder(
      builder: (context, constraints) {
        final compact =
            !constraints.maxHeight.isFinite || constraints.maxHeight < 40;
        if (compact) {
          return SingleChildScrollView(
            child: Column(
              mainAxisSize: MainAxisSize.min,
              crossAxisAlignment: CrossAxisAlignment.stretch,
              children: [...preface, canvas],
            ),
          );
        }
        return Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            ...preface,
            Expanded(child: canvas),
          ],
        );
      },
    );
  }

  Widget _buildPptxShapeText({
    required PptxPositionedShape shape,
    required _PptxRenderMetrics metrics,
  }) {
    if (shape.runs.isEmpty) {
      final text = shape.plainText.trim();
      if (text.isEmpty) {
        return const SizedBox.shrink();
      }
      return Text(
        text,
        overflow: TextOverflow.fade,
        style: Theme.of(context).textTheme.bodySmall?.copyWith(
          fontSize: _scaledPptxFontSize(null, metrics.viewScale),
          height: 1.2,
        ),
      );
    }
    return RichText(
      overflow: TextOverflow.fade,
      text: TextSpan(
        children: shape.runs.map((run) {
          final resolution = _resolveFont(run.fontFamily);
          return TextSpan(
            text: run.text,
            style: Theme.of(context).textTheme.bodySmall?.copyWith(
              fontFamily: resolution.resolvedFamily,
              fontSize: _scaledPptxFontSize(run.fontSizePt, metrics.viewScale),
              fontWeight: run.bold == true ? FontWeight.w600 : FontWeight.w400,
              fontStyle: run.italic == true
                  ? FontStyle.italic
                  : FontStyle.normal,
              decoration: run.underline == true
                  ? TextDecoration.underline
                  : TextDecoration.none,
              color:
                  _parsePptxColor(run.color) ?? KeenBenchTheme.colorTextPrimary,
              height: 1.2,
            ),
          );
        }).toList(),
      ),
    );
  }

  double _scaledPptxFontSize(double? fontSizePt, double scale) {
    final base = fontSizePt != null && fontSizePt > 0 ? fontSizePt : 12.0;
    return (base * scale).clamp(8.0, 40.0).toDouble();
  }

  List<FontResolution> _collectFontResolutions(PptxPositionedSlide slide) {
    final seen = <String>{};
    final resolutions = <FontResolution>[];
    final requested = slide.requestedFontFamilies.toList()..sort();
    for (final family in requested) {
      final normalized = _normalizeFontName(family);
      if (!seen.add(normalized)) {
        continue;
      }
      resolutions.add(_resolveFont(family));
    }
    return resolutions;
  }

  FontResolution _resolveFont(String requestedFamily) {
    final raw = requestedFamily.trim();
    if (raw.isEmpty) {
      return FontResolution(
        requestedFamily: raw,
        resolvedFamily: _pptxFallbackFontFamily,
        source: 'fallback',
        missing: false,
      );
    }
    final normalized = _normalizeFontName(raw);
    final bundled = _bundledFontFamilies[normalized];
    if (bundled != null) {
      return FontResolution(
        requestedFamily: raw,
        resolvedFamily: bundled,
        source: 'bundled',
        missing: false,
      );
    }
    final osKnown = _osKnownFontFamilies[normalized];
    if (osKnown != null) {
      return FontResolution(
        requestedFamily: raw,
        resolvedFamily: osKnown,
        source: 'os-known',
        missing: false,
      );
    }
    return FontResolution(
      requestedFamily: raw,
      resolvedFamily: _pptxFallbackFontFamily,
      source: 'fallback',
      missing: true,
    );
  }

  String _normalizeFontName(String value) {
    return value
        .trim()
        .toLowerCase()
        .replaceAll('"', '')
        .replaceAll("'", '')
        .replaceAll(RegExp(r'\s+'), ' ');
  }

  Color? _parsePptxColor(String? rawValue) {
    if (rawValue == null) {
      return null;
    }
    var hex = rawValue.trim();
    if (hex.isEmpty) {
      return null;
    }
    if (hex.startsWith('0x')) {
      hex = hex.substring(2);
    }
    if (hex.startsWith('#')) {
      hex = hex.substring(1);
    }
    if (hex.length == 6) {
      hex = 'FF$hex';
    }
    if (hex.length != 8) {
      return null;
    }
    final parsed = int.tryParse(hex, radix: 16);
    if (parsed == null) {
      return null;
    }
    return Color(parsed);
  }

  Widget _buildPptxStructuredFallbackSide(Map<String, dynamic>? payload) {
    if (payload == null) {
      return const Center(child: Text('No structured content.'));
    }
    return _buildPptxStructured(payload);
  }

  Widget _buildPagePreview(ChangeItem change) {
    final hasPublished = change.changeType != 'added';
    final draftBytes = _decodeBase64(_draftPreview);
    final pubBytes = _decodeBase64(_publishedPreview);
    final fitPageToWidth = _shouldFitPagePreviewToWidth(change);
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        _buildPreviewToolbar(
          label: change.fileKind == 'pptx' ? 'Slides' : 'Pages',
          count: _pageCount,
          currentIndex: _pageIndex,
          onPrev: _pageIndex > 0
              ? () {
                  setState(() {
                    _pageIndex -= 1;
                  });
                  _loadPreview(change);
                }
              : null,
          onNext: _pageCount > 0 && _pageIndex < _pageCount - 1
              ? () {
                  setState(() {
                    _pageIndex += 1;
                  });
                  _loadPreview(change);
                }
              : null,
        ),
        const SizedBox(height: 8),
        Expanded(
          child: _sideBySide(
            leftLabel: 'Published (current preview)',
            rightLabel: 'Draft',
            leftChild: pubBytes == null
                ? const Center(child: Text('No published preview.'))
                : _buildPageImage(pubBytes, fitToWidth: fitPageToWidth),
            rightChild: draftBytes == null
                ? const Center(child: Text('No draft preview.'))
                : _buildPageImage(draftBytes, fitToWidth: fitPageToWidth),
            showLeft: hasPublished,
          ),
        ),
      ],
    );
  }

  double _previewScaleForPage(ChangeItem change) {
    if (change.fileKind == 'docx') {
      return _docxPagePreviewScale;
    }
    return _defaultPagePreviewScale;
  }

  bool _shouldFitPagePreviewToWidth(ChangeItem change) {
    return change.fileKind == 'docx';
  }

  Widget _buildPageImage(Uint8List bytes, {required bool fitToWidth}) {
    if (!fitToWidth) {
      return Image.memory(bytes, fit: BoxFit.contain);
    }
    return LayoutBuilder(
      builder: (context, constraints) {
        final width = constraints.maxWidth;
        if (!width.isFinite || width <= 0) {
          return Image.memory(bytes, fit: BoxFit.contain);
        }
        return Align(
          alignment: Alignment.topCenter,
          child: SingleChildScrollView(
            child: Image.memory(
              bytes,
              width: width,
              fit: BoxFit.fitWidth,
              alignment: Alignment.topCenter,
            ),
          ),
        );
      },
    );
  }

  Widget _buildGridPreview(ChangeItem change) {
    final hasPublished = change.changeType != 'added';
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        Row(
          children: [
            if (_sheets.isNotEmpty && _sheet != null)
              DropdownButton<String>(
                value: _sheet!,
                items: _sheets
                    .map(
                      (sheet) =>
                          DropdownMenuItem(value: sheet, child: Text(sheet)),
                    )
                    .toList(),
                onChanged: (value) {
                  setState(() {
                    _sheet = value;
                  });
                  _loadXlsxPreview(change);
                },
              ),
            const Spacer(),
            IconButton(
              onPressed: _rowStart > 0
                  ? () {
                      setState(() {
                        _rowStart = (_rowStart - _rowCount)
                            .clamp(0, _rowStart)
                            .toInt();
                      });
                      _loadXlsxPreview(change);
                    }
                  : null,
              icon: const Icon(Icons.chevron_left),
            ),
            IconButton(
              onPressed: () {
                setState(() {
                  _rowStart += _rowCount;
                });
                _loadXlsxPreview(change);
              },
              icon: const Icon(Icons.chevron_right),
            ),
          ],
        ),
        const SizedBox(height: 8),
        Expanded(
          child: _sideBySide(
            leftLabel: 'Published (current preview)',
            rightLabel: 'Draft',
            leftChild: _buildGridTable(_publishedGrid),
            rightChild: _buildGridTable(_draftGrid),
            showLeft: hasPublished,
          ),
        ),
      ],
    );
  }

  Widget _buildImagePreview() {
    final draftBytes = _decodeBase64(_imageDraft?['bytes_base64'] as String?);
    final pubBytes = _decodeBase64(_imagePublished?['bytes_base64'] as String?);
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        Expanded(
          child: _sideBySide(
            leftLabel: 'Published (current preview)',
            rightLabel: 'Draft',
            leftChild: pubBytes == null
                ? const Center(child: Text('No published preview.'))
                : Image.memory(pubBytes, fit: BoxFit.contain),
            rightChild: draftBytes == null
                ? const Center(child: Text('No draft preview.'))
                : Image.memory(draftBytes, fit: BoxFit.contain),
            showLeft: _hasPublishedImage,
          ),
        ),
        const SizedBox(height: 8),
        _buildImageMetadata(),
      ],
    );
  }

  Widget _buildImageMetadata() {
    final meta = _imageDraft?['metadata'] as Map<String, dynamic>? ?? {};
    final format = meta['format']?.toString() ?? 'unknown';
    final width = meta['width'];
    final height = meta['height'];
    final sizeBytes = meta['size_bytes'];
    final dims = (width == null || height == null)
        ? 'scalable'
        : '${width}x$height';
    final sizeLabel = sizeBytes is num
        ? '${sizeBytes.toInt()} bytes'
        : 'size unknown';
    return Container(
      padding: const EdgeInsets.all(8),
      decoration: BoxDecoration(
        color: KeenBenchTheme.colorBackgroundSecondary,
        borderRadius: BorderRadius.circular(6),
        border: Border.all(color: KeenBenchTheme.colorBorderSubtle),
      ),
      child: Text('Format: $format • $dims • $sizeLabel'),
    );
  }

  Widget _buildOpaquePreview(ChangeItem change) {
    return Center(
      child: Text(
        'Opaque file (${change.mimeType}) • ${change.sizeBytes} bytes',
      ),
    );
  }

  Widget _buildPreviewToolbar({
    required String label,
    required int count,
    required int currentIndex,
    required VoidCallback? onPrev,
    required VoidCallback? onNext,
  }) {
    final displayCount = count == 0 ? '–' : '${currentIndex + 1} / $count';
    return Row(
      children: [
        Text(
          '$label $displayCount',
          style: Theme.of(context).textTheme.bodySmall,
        ),
        const Spacer(),
        IconButton(onPressed: onPrev, icon: const Icon(Icons.chevron_left)),
        IconButton(onPressed: onNext, icon: const Icon(Icons.chevron_right)),
      ],
    );
  }

  Widget _buildGridTable(List<List<dynamic>> rows) {
    if (rows.isEmpty) {
      return const Center(child: Text('No data.'));
    }
    return ListView.builder(
      itemCount: rows.length,
      itemBuilder: (context, rowIndex) {
        final row = rows[rowIndex];
        return Row(
          children: row.map((cell) {
            final cellMap = cell as Map<String, dynamic>? ?? {};
            final value = cellMap['value']?.toString() ?? '';
            final formula = cellMap['formula']?.toString();
            return Expanded(
              child: Container(
                padding: const EdgeInsets.all(4),
                decoration: BoxDecoration(
                  border: Border.all(color: KeenBenchTheme.colorBorderSubtle),
                ),
                child: Text(formula ?? value, overflow: TextOverflow.ellipsis),
              ),
            );
          }).toList(),
        );
      },
    );
  }

  Widget _sideBySide({
    required String leftLabel,
    required String rightLabel,
    required Widget leftChild,
    required Widget rightChild,
    required bool showLeft,
  }) {
    return Row(
      children: [
        if (showLeft)
          Expanded(
            child: Column(
              children: [
                Text(leftLabel, style: Theme.of(context).textTheme.labelSmall),
                const SizedBox(height: 4),
                Expanded(child: leftChild),
              ],
            ),
          ),
        Expanded(
          child: Column(
            children: [
              Text(rightLabel, style: Theme.of(context).textTheme.labelSmall),
              const SizedBox(height: 4),
              Expanded(child: rightChild),
            ],
          ),
        ),
      ],
    );
  }

  List<Map<String, dynamic>> _asMapList(dynamic value) {
    if (value is! List) {
      return const [];
    }
    return value
        .whereType<Map>()
        .map(
          (entry) => entry.map((key, value) => MapEntry(key.toString(), value)),
        )
        .toList();
  }

  Uint8List? _decodeBase64(String? value) {
    if (value == null || value.isEmpty) {
      return null;
    }
    try {
      return base64Decode(value);
    } catch (_) {
      return null;
    }
  }

  String _formatPreviewError(Object err) {
    if (err is EngineError) {
      final code = err.errorCode.trim();
      if (code.isNotEmpty) {
        return '${err.message} ($code)';
      }
      return err.message;
    }
    return err.toString();
  }

  bool _isMissingXlsxSheetError(Object err) {
    if (err is! EngineError) {
      return false;
    }
    final code = err.errorCode.toUpperCase();
    if (code != 'VALIDATION_FAILED' && code != 'FILE_READ_FAILED') {
      return false;
    }
    final message = err.message.toLowerCase();
    return message.contains('unknown sheet') ||
        message.contains('missing sheet');
  }

  bool _isMissingPublishedTargetError(Object err) {
    if (err is! EngineError) {
      return false;
    }
    final code = err.errorCode.toUpperCase();
    if (code != 'VALIDATION_FAILED' && code != 'FILE_READ_FAILED') {
      return false;
    }
    final message = err.message.toLowerCase();
    return message.contains('unknown sheet') ||
        message.contains('missing sheet') ||
        message.contains('invalid slide_index') ||
        message.contains('invalid slide index') ||
        message.contains('out of range');
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      key: AppKeys.reviewScreen,
      appBar: KeenBenchAppBar(
        title: 'Review Draft',
        showBack: true,
        useCenteredContent: false,
        actions: [
          ElevatedButton(
            key: AppKeys.reviewPublishButton,
            onPressed: _loading
                ? null
                : () async {
                    final engine = context.read<EngineApi>();
                    await engine.call('DraftPublish', {
                      'workbench_id': widget.workbenchId,
                    });
                    if (mounted) {
                      Navigator.of(context).pop(true);
                    }
                  },
            child: const Text('Publish'),
          ),
          const SizedBox(width: 8),
          TextButton(
            key: AppKeys.reviewDiscardButton,
            onPressed: _loading
                ? null
                : () async {
                    final engine = context.read<EngineApi>();
                    await engine.call('DraftDiscard', {
                      'workbench_id': widget.workbenchId,
                    });
                    if (mounted) {
                      Navigator.of(context).pop(false);
                    }
                  },
            style: TextButton.styleFrom(
              foregroundColor: KeenBenchTheme.colorErrorText,
            ),
            child: const Text('Discard'),
          ),
        ],
      ),
      body: _loading
          ? const Center(child: CircularProgressIndicator())
          : Padding(
              padding: const EdgeInsets.symmetric(horizontal: 24),
              child: Row(
                children: [
                  Container(
                    width: 260,
                    decoration: const BoxDecoration(
                      border: Border(
                        right: BorderSide(
                          color: KeenBenchTheme.colorBorderDefault,
                        ),
                      ),
                      color: KeenBenchTheme.colorBackgroundSecondary,
                    ),
                    child: _changes.isEmpty
                        ? const Center(child: Text('No changes'))
                        : ListView.separated(
                            key: AppKeys.reviewChangeList,
                            padding: const EdgeInsets.all(12),
                            itemBuilder: (context, index) {
                              final change = _changes[index];
                              final selected = change.path == _selected?.path;
                              return InkWell(
                                onTap: () => _selectChange(change),
                                child: Container(
                                  padding: const EdgeInsets.all(12),
                                  decoration: BoxDecoration(
                                    color: selected
                                        ? KeenBenchTheme.colorBackgroundSelected
                                        : Colors.transparent,
                                    borderRadius: BorderRadius.circular(6),
                                  ),
                                  child: Column(
                                    crossAxisAlignment:
                                        CrossAxisAlignment.start,
                                    children: [
                                      Text(
                                        change.path,
                                        style: Theme.of(
                                          context,
                                        ).textTheme.bodyMedium,
                                      ),
                                      const SizedBox(height: 4),
                                      Wrap(
                                        spacing: 6,
                                        runSpacing: 4,
                                        children: [
                                          _ChangeBadge(
                                            changeType: change.changeType,
                                          ),
                                          if (change.fileKind.isNotEmpty)
                                            _InlineBadge(
                                              label: change.fileKind
                                                  .toUpperCase(),
                                              background: KeenBenchTheme
                                                  .colorSurfaceMuted,
                                              textColor: KeenBenchTheme
                                                  .colorTextSecondary,
                                            ),
                                          if (change.fileKind == 'pdf' ||
                                              change.fileKind == 'image' ||
                                              change.fileKind == 'odt')
                                            const _InlineBadge(
                                              label: 'Read-only',
                                              background: KeenBenchTheme
                                                  .colorInfoBackground,
                                              textColor:
                                                  KeenBenchTheme.colorInfoText,
                                            ),
                                          if (change.isOpaque)
                                            const _InlineBadge(
                                              label: 'Opaque',
                                              background: KeenBenchTheme
                                                  .colorSurfaceMuted,
                                              textColor: KeenBenchTheme
                                                  .colorTextSecondary,
                                            ),
                                        ],
                                      ),
                                    ],
                                  ),
                                ),
                              );
                            },
                            separatorBuilder: (_, __) =>
                                const SizedBox(height: 8),
                            itemCount: _changes.length,
                          ),
                  ),
                  Expanded(
                    child: Container(
                      padding: const EdgeInsets.all(16),
                      color: KeenBenchTheme.colorBackgroundPrimary,
                      child: _selected == null
                          ? const Center(
                              child: Text('Select a file to view details.'),
                            )
                          : _buildDetailPane(context),
                    ),
                  ),
                ],
              ),
            ),
    );
  }
}

class _ChangeBadge extends StatelessWidget {
  const _ChangeBadge({required this.changeType});

  final String changeType;

  @override
  Widget build(BuildContext context) {
    final normalized = changeType.toLowerCase();
    final isAdded = normalized == 'added';
    final background = isAdded
        ? KeenBenchTheme.colorDiffAdded
        : KeenBenchTheme.colorInfoBackground;
    final textColor = isAdded
        ? KeenBenchTheme.colorSuccessText
        : KeenBenchTheme.colorInfoText;
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 2),
      decoration: BoxDecoration(
        color: background,
        borderRadius: BorderRadius.circular(999),
        border: Border.all(color: KeenBenchTheme.colorBorderSubtle),
      ),
      child: Text(
        normalized.toUpperCase(),
        style: Theme.of(context).textTheme.labelSmall?.copyWith(
          color: textColor,
          fontWeight: FontWeight.w600,
          letterSpacing: 0.6,
        ),
      ),
    );
  }
}

class _PptxRenderMetrics {
  const _PptxRenderMetrics({
    required this.coordinateScale,
    required this.canvasWidth,
    required this.canvasHeight,
    required this.viewScale,
  });

  final double coordinateScale;
  final double canvasWidth;
  final double canvasHeight;
  final double viewScale;
}

class _InlineBadge extends StatelessWidget {
  const _InlineBadge({
    required this.label,
    required this.background,
    required this.textColor,
  });

  final String label;
  final Color background;
  final Color textColor;

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 6, vertical: 2),
      decoration: BoxDecoration(
        color: background,
        borderRadius: BorderRadius.circular(999),
      ),
      child: Text(
        label,
        style: Theme.of(context).textTheme.labelSmall?.copyWith(
          color: textColor,
          fontWeight: FontWeight.w600,
          letterSpacing: 0.4,
        ),
      ),
    );
  }
}
