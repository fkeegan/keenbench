import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter/gestures.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:provider/provider.dart';

import 'package:keenbench/app_keys.dart';
import 'package:keenbench/engine/engine_client.dart';
import 'package:keenbench/screens/review_screen.dart';
import 'package:keenbench/screens/workbench_screen.dart';
import 'package:keenbench/theme.dart';

class _FakeWorkbenchEngine implements EngineApi {
  _FakeWorkbenchEngine({
    required this.hasDraft,
    required this.draftId,
    required this.messages,
    required this.reviewChanges,
    this.draftSummary,
    this.files = const [],
    this.contextItems = const [],
    this.failingMethods = const <String>{},
    this.failPublishedMethods = const <String>{},
    this.includePptxPositioned = true,
    this.xlsxDraftAvailableSheets,
    this.clutterContextWarning = false,
  });

  final _notifications = StreamController<EngineNotification>.broadcast();
  static const String _tinyPngBase64 =
      'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO3ZkXQAAAAASUVORK5CYII=';
  final List<String> calls = [];
  final Map<String, Map<String, dynamic>?> lastParams = {};
  final Map<String, List<Map<String, dynamic>?>> paramsHistory = {};
  final List<Map<String, dynamic>> messages;
  final List<Map<String, dynamic>> reviewChanges;
  final List<Map<String, dynamic>> files;
  final List<Map<String, dynamic>> contextItems;
  final Set<String> failingMethods;
  final Set<String> failPublishedMethods;
  final bool includePptxPositioned;
  final List<String>? xlsxDraftAvailableSheets;
  final bool clutterContextWarning;
  bool hasDraft;
  String draftId;
  String? draftSummary;

  @override
  Stream<EngineNotification> get notifications => _notifications.stream;

  int callCount(String method) => calls.where((m) => m == method).length;

  @override
  Future<dynamic> call(String method, [Map<String, dynamic>? params]) async {
    calls.add(method);
    lastParams[method] = params;
    paramsHistory.putIfAbsent(method, () => []).add(params);
    if (failingMethods.contains(method)) {
      throw EngineError('preview unavailable', {
        'error_code': 'TOOL_WORKER_UNAVAILABLE',
      });
    }
    if (failPublishedMethods.contains(method) &&
        params?['version'] == 'published') {
      final message = method == 'ReviewGetXlsxPreviewGrid'
          ? 'unknown sheet'
          : method == 'ReviewGetPptxPreviewSlide'
          ? 'invalid slide_index'
          : 'target missing';
      throw EngineError(message, {'error_code': 'VALIDATION_FAILED'});
    }
    switch (method) {
      case 'WorkbenchOpen':
        return {
          'workbench': {
            'id': 'wb-1',
            'name': 'Workbench',
            'created_at': '2026-01-01T00:00:00Z',
            'updated_at': '2026-01-02T00:00:00Z',
            'default_model_id': 'openai/gpt-4o-mini',
          },
        };
      case 'WorkshopGetState':
        return {
          'active_model_id': 'openai/gpt-4o-mini',
          'default_model_id': 'openai/gpt-4o-mini',
          'has_draft': hasDraft,
          'pending_proposal_id': '',
        };
      case 'ProvidersGetStatus':
        return {
          'providers': [
            {
              'provider_id': 'openai',
              'display_name': 'OpenAI',
              'enabled': true,
              'configured': true,
              'models': ['openai/gpt-4o-mini'],
            },
          ],
        };
      case 'ModelsListSupported':
        return {
          'models': [
            {
              'model_id': 'openai/gpt-4o-mini',
              'provider_id': 'openai',
              'display_name': 'GPT-4o mini',
              'context_tokens_estimate': 128000,
              'supports_file_read': true,
              'supports_file_write': true,
            },
          ],
        };
      case 'WorkbenchFilesList':
        return {'files': files};
      case 'WorkbenchGetScope':
        return {
          'limits': {'max_files': 10, 'max_file_bytes': 26214400},
          'supported_types': ['txt', 'csv', 'md'],
          'sandbox_root': '/tmp/workbench',
        };
      case 'WorkshopGetConversation':
        return {'messages': messages};
      case 'DraftGetState':
        if (!hasDraft) {
          return {'has_draft': false};
        }
        return {
          'has_draft': true,
          'draft_id': draftId,
          'created_at': '2026-02-05T00:00:00Z',
        };
      case 'WorkbenchGetClutter':
        return {
          'score': 0.1,
          'level': 'Light',
          'model_id': 'openai/gpt-4o-mini',
          'context_items_weight': clutterContextWarning ? 90000.0 : 0.0,
          'context_share': clutterContextWarning ? 0.4 : 0.0,
          'context_warning': clutterContextWarning,
        };
      case 'ContextList':
        return {'items': contextItems};
      case 'ContextGet':
        return {
          'item': {
            'category': params?['category'] as String? ?? '',
            'status': 'empty',
            'files': const [],
          },
        };
      case 'ContextGetArtifact':
        return {'files': const [], 'has_direct_edits': false};
      case 'EgressGetConsentStatus':
        return {
          'consented': true,
          'scope_hash': 'scope-1',
          'provider_id': 'openai',
          'model_id': 'openai/gpt-4o-mini',
        };
      case 'WorkshopSendUserMessage':
        return {'message_id': 'u-1'};
      case 'WorkshopRunAgent':
        hasDraft = true;
        draftId = 'd-new';
        return {'has_draft': true};
      case 'ReviewGetChangeSet':
        final response = <String, dynamic>{
          'draft_id': draftId,
          'changes': reviewChanges,
        };
        if (draftSummary != null) {
          response['draft_summary'] = draftSummary;
        }
        return response;
      case 'ReviewGetTextDiff':
        return {'hunks': [], 'too_large': false, 'baseline_missing': false};
      case 'ReviewGetDocxContentDiff':
        final sectionIndex = (params?['section_index'] as num?)?.toInt() ?? 0;
        return {
          'baseline': {
            'heading': 'Section ${sectionIndex + 1}',
            'level': 1,
            'paragraphs': [
              {
                'index': 0,
                'text': 'Baseline paragraph for section ${sectionIndex + 1}',
              },
            ],
            'tables': [],
            'images': [],
          },
          'draft': {
            'heading': 'Section ${sectionIndex + 1}',
            'level': 1,
            'paragraphs': [
              {
                'index': 0,
                'text': 'Draft paragraph for section ${sectionIndex + 1}',
              },
            ],
            'tables': [],
            'images': [],
          },
          'section_count': 3,
          'baseline_missing': false,
        };
      case 'ReviewGetPptxContentDiff':
        final slideIndex = (params?['slide_index'] as num?)?.toInt() ?? 0;
        Map<String, dynamic> positionedPayload(String prefix) {
          final text = '$prefix block for slide ${slideIndex + 1}';
          return {
            'coordinate_space': 'slide_ratio',
            'slide_size': {'width': 9144000, 'height': 6858000, 'unit': 'emu'},
            'positioned_shapes': [
              {
                'index': 0,
                'z_index': 0,
                'name': 'Body',
                'shape_type': 'TEXT_BOX',
                'x': 914400,
                'y': 1371600,
                'w': 7315200,
                'h': 1714500,
                'bounds': {
                  'x': 0.1,
                  'y': 0.2,
                  'width': 0.8,
                  'height': 0.25,
                  'unit': 'slide_ratio',
                },
                'text_runs': [
                  {
                    'index': 0,
                    'text': text,
                    'font_name': 'Arial',
                    'size_pt': 18,
                  },
                ],
                'text_blocks': [
                  {'index': 0, 'text': text, 'runs': []},
                ],
              },
            ],
          };
        }
        return {
          'baseline': {
            'index': slideIndex,
            'title': 'Slide ${slideIndex + 1}',
            'layout': 'Title and Content',
            if (includePptxPositioned) ...{
              'render_mode': 'positioned',
              'positioned': positionedPayload('Baseline'),
            },
            'shapes': [
              {
                'index': 0,
                'name': 'Body',
                'shape_type': 'TEXT_BOX',
                'text_blocks': [
                  {
                    'index': 0,
                    'text': 'Baseline block for slide ${slideIndex + 1}',
                  },
                ],
              },
            ],
          },
          'draft': {
            'index': slideIndex,
            'title': 'Slide ${slideIndex + 1}',
            'layout': 'Title and Content',
            if (includePptxPositioned) ...{
              'render_mode': 'positioned',
              'positioned': positionedPayload('Draft'),
            },
            'shapes': [
              {
                'index': 0,
                'name': 'Body',
                'shape_type': 'TEXT_BOX',
                'text_blocks': [
                  {
                    'index': 0,
                    'text': 'Draft block for slide ${slideIndex + 1}',
                  },
                ],
              },
            ],
          },
          'slide_count': 4,
          'baseline_missing': false,
        };
      case 'ReviewGetDocxPreviewPage':
      case 'ReviewGetPdfPreviewPage':
      case 'ReviewGetOdtPreviewPage':
        return {
          'bytes_base64': _tinyPngBase64,
          'page_count': 1,
          'scaled_down': false,
        };
      case 'ReviewGetPptxPreviewSlide':
        return {
          'bytes_base64': _tinyPngBase64,
          'slide_count': 1,
          'scaled_down': false,
        };
      case 'ReviewGetXlsxPreviewGrid':
        final sheet = (params?['sheet'] as String?)?.trim();
        final availableSheets = xlsxDraftAvailableSheets;
        if (params?['version'] == 'draft' && availableSheets != null) {
          final requestedSheet = sheet ?? '';
          if (requestedSheet.isNotEmpty &&
              !availableSheets.contains(requestedSheet)) {
            throw EngineError('unknown sheet', {
              'error_code': 'VALIDATION_FAILED',
            });
          }
          final sheetName = requestedSheet.isNotEmpty
              ? requestedSheet
              : (availableSheets.isNotEmpty ? availableSheets.first : 'Sheet1');
          return {
            'sheets': availableSheets,
            'cells': [
              [
                {'value': '$sheetName:A1'},
                {'value': '$sheetName:B1'},
              ],
            ],
          };
        }
        final sheetName = (sheet != null && sheet.isNotEmpty)
            ? sheet
            : 'Sheet1';
        return {
          'sheets': [sheetName],
          'cells': [
            [
              {'value': '$sheetName:A1'},
              {'value': '$sheetName:B1'},
            ],
          ],
        };
      case 'ReviewGetImagePreview':
        return {
          'draft': {
            'bytes_base64': _tinyPngBase64,
            'metadata': {
              'format': 'png',
              'width': 1,
              'height': 1,
              'size_bytes': 68,
            },
          },
          'published': {
            'bytes_base64': _tinyPngBase64,
            'metadata': {
              'format': 'png',
              'width': 1,
              'height': 1,
              'size_bytes': 68,
            },
          },
          'has_published': true,
        };
      case 'DraftPublish':
        hasDraft = false;
        draftId = '';
        return {
          'published_at': '2026-02-05T12:00:00Z',
          'checkpoint_id': 'cp-1',
        };
      case 'DraftDiscard':
        hasDraft = false;
        draftId = '';
        return {};
      case 'CheckpointRestore':
        return {};
      case 'WorkshopUndoToMessage':
        final target = params?['message_id'] as String? ?? '';
        if (target.isNotEmpty) {
          final idx = messages.indexWhere((m) => m['message_id'] == target);
          if (idx >= 0) {
            messages.removeRange(idx + 1, messages.length);
          }
        }
        hasDraft = false;
        draftId = '';
        return {};
      case 'WorkshopRegenerate':
        final fromId = params?['message_id'] as String? ?? '';
        if (fromId.isNotEmpty) {
          final idx = messages.indexWhere((m) => m['message_id'] == fromId);
          if (idx >= 0 && idx + 1 < messages.length) {
            messages.removeRange(idx + 1, messages.length);
          }
        }
        messages.add({
          'message_id': 'regen-1',
          'role': 'assistant',
          'text': 'Regenerated reply',
          'created_at': '2026-02-05T12:00:00Z',
        });
        return {'message_id': 'regen-1'};
      case 'WorkbenchFilesExtract':
        return {
          'extract_results': [
            {'path': 'notes.txt', 'status': 'extracted'},
          ],
        };
      default:
        return {};
    }
  }
}

void main() {
  Widget appForTest(EngineApi engine, Widget home) {
    return Provider<EngineApi>.value(
      value: engine,
      child: MaterialApp(theme: KeenBenchTheme.theme(), home: home),
    );
  }

  Future<void> pumpUntilFound(
    WidgetTester tester,
    Finder finder, {
    Duration timeout = const Duration(seconds: 4),
  }) async {
    final end = DateTime.now().add(timeout);
    while (DateTime.now().isBefore(end)) {
      if (finder.evaluate().isNotEmpty) {
        return;
      }
      await tester.pump(const Duration(milliseconds: 50));
    }
    fail('Finder not found in time: $finder');
  }

  Future<void> useDesktopSurface(WidgetTester tester) async {
    final binding = tester.binding;
    await binding.setSurfaceSize(const Size(1600, 1000));
    addTearDown(() async {
      await binding.setSurfaceSize(null);
    });
  }

  Future<void> tapBackButton(WidgetTester tester) async {
    final backButton = find.byType(BackButton);
    if (backButton.evaluate().isEmpty) {
      fail('Back button not found.');
    }
    await tester.tap(backButton.last);
    await tester.pumpAndSettle();
  }

  Future<void> hover(WidgetTester tester, Finder finder) async {
    final gesture = await tester.createGesture(kind: PointerDeviceKind.mouse);
    addTearDown(gesture.removePointer);
    await gesture.addPointer();
    await gesture.moveTo(tester.getCenter(finder));
    await tester.pumpAndSettle();
  }

  testWidgets('auto-opens review when a draft is created after send', (
    tester,
  ) async {
    await useDesktopSurface(tester);
    final engine = _FakeWorkbenchEngine(
      hasDraft: false,
      draftId: '',
      messages: const [],
      reviewChanges: const [],
    );
    await tester.pumpWidget(
      appForTest(engine, const WorkbenchScreen(workbenchId: 'wb-1')),
    );
    await tester.pumpAndSettle();

    await tester.enterText(
      find.byKey(AppKeys.workbenchComposerField),
      'Please make a draft.',
    );
    await tester.tap(find.byKey(AppKeys.workbenchSendButton));
    await pumpUntilFound(tester, find.byKey(AppKeys.reviewScreen));

    await tapBackButton(tester);
    expect(find.byKey(AppKeys.workbenchScreen), findsOneWidget);
    expect(find.byKey(AppKeys.workbenchDiscardButton), findsOneWidget);
    await tester.pump(const Duration(milliseconds: 200));
    expect(find.byKey(AppKeys.reviewScreen), findsNothing);
  });

  testWidgets('auto-opens review when opening a workbench with draft', (
    tester,
  ) async {
    await useDesktopSurface(tester);
    final engine = _FakeWorkbenchEngine(
      hasDraft: true,
      draftId: 'd-existing',
      messages: const [],
      reviewChanges: const [],
    );
    await tester.pumpWidget(
      appForTest(engine, const WorkbenchScreen(workbenchId: 'wb-1')),
    );

    await pumpUntilFound(tester, find.byKey(AppKeys.reviewScreen));
  });

  testWidgets('workbench context action opens context overview', (
    tester,
  ) async {
    await useDesktopSurface(tester);
    final engine = _FakeWorkbenchEngine(
      hasDraft: false,
      draftId: '',
      messages: const [],
      reviewChanges: const [],
    );
    await tester.pumpWidget(
      appForTest(engine, const WorkbenchScreen(workbenchId: 'wb-1')),
    );
    await tester.pumpAndSettle();

    expect(find.byKey(AppKeys.workbenchAddContextButton), findsOneWidget);
    await tester.tap(find.byKey(AppKeys.workbenchAddContextButton));
    await tester.pumpAndSettle();

    expect(find.byKey(AppKeys.contextOverviewScreen), findsOneWidget);
    expect(engine.callCount('ContextList'), greaterThanOrEqualTo(2));
  });

  testWidgets('workbench context overview remains accessible during draft', (
    tester,
  ) async {
    await useDesktopSurface(tester);
    final engine = _FakeWorkbenchEngine(
      hasDraft: true,
      draftId: 'd-existing',
      messages: const [],
      reviewChanges: const [],
    );
    await tester.pumpWidget(
      appForTest(engine, const WorkbenchScreen(workbenchId: 'wb-1')),
    );
    await pumpUntilFound(tester, find.byKey(AppKeys.reviewScreen));

    await tapBackButton(tester);
    expect(find.byKey(AppKeys.workbenchScreen), findsOneWidget);
    expect(find.byKey(AppKeys.workbenchDraftBanner), findsOneWidget);

    await tester.tap(find.byKey(AppKeys.workbenchAddContextButton));
    await tester.pumpAndSettle();

    expect(find.byKey(AppKeys.contextOverviewScreen), findsOneWidget);
  });

  testWidgets('workbench shows high-context clutter warning message', (
    tester,
  ) async {
    await useDesktopSurface(tester);
    final engine = _FakeWorkbenchEngine(
      hasDraft: false,
      draftId: '',
      messages: const [],
      reviewChanges: const [],
      clutterContextWarning: true,
    );
    await tester.pumpWidget(
      appForTest(engine, const WorkbenchScreen(workbenchId: 'wb-1')),
    );
    await tester.pumpAndSettle();

    expect(find.byKey(AppKeys.workbenchContextWarning), findsOneWidget);
    expect(
      find.textContaining(
        'Context is using a large share of the prompt window.',
      ),
      findsOneWidget,
    );
  });

  testWidgets('review summary precedence uses per-file then draft summary', (
    tester,
  ) async {
    final engine = _FakeWorkbenchEngine(
      hasDraft: true,
      draftId: 'd-1',
      draftSummary: 'Fallback draft summary',
      messages: const [],
      reviewChanges: const [
        {
          'path': 'alpha.txt',
          'change_type': 'added',
          'file_kind': 'text',
          'preview_kind': 'none',
          'mime_type': 'text/plain',
          'is_opaque': false,
          'summary': 'Per-file summary',
        },
        {
          'path': 'beta.txt',
          'change_type': 'added',
          'file_kind': 'text',
          'preview_kind': 'none',
          'mime_type': 'text/plain',
          'is_opaque': false,
          'summary': '',
        },
      ],
    );

    await tester.pumpWidget(
      appForTest(engine, const ReviewScreen(workbenchId: 'wb-1')),
    );
    await tester.pumpAndSettle();

    expect(find.text('Per-file summary'), findsOneWidget);
    await tester.tap(find.text('beta.txt'));
    await tester.pumpAndSettle();
    expect(find.text('Fallback draft summary'), findsOneWidget);
  });

  testWidgets('review summary fallback shows unavailable when missing', (
    tester,
  ) async {
    final engine = _FakeWorkbenchEngine(
      hasDraft: true,
      draftId: 'd-2',
      draftSummary: '',
      messages: const [],
      reviewChanges: const [
        {
          'path': 'gamma.txt',
          'change_type': 'added',
          'file_kind': 'text',
          'preview_kind': 'none',
          'mime_type': 'text/plain',
          'is_opaque': false,
          'summary': '',
        },
      ],
    );

    await tester.pumpWidget(
      appForTest(engine, const ReviewScreen(workbenchId: 'wb-1')),
    );
    await tester.pumpAndSettle();

    expect(find.text('Summary unavailable.'), findsOneWidget);
  });

  testWidgets('xlsx focus hint drives initial preview request target', (
    tester,
  ) async {
    final engine = _FakeWorkbenchEngine(
      hasDraft: true,
      draftId: 'd-xlsx-focus',
      messages: const [],
      reviewChanges: const [
        {
          'path': 'quarterly_data.xlsx',
          'change_type': 'modified',
          'file_kind': 'xlsx',
          'preview_kind': 'grid',
          'mime_type':
              'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet',
          'is_opaque': false,
          'summary': '',
          'focus_hint': {'sheet': 'Annual', 'row_start': 8, 'col_start': 3},
        },
      ],
    );

    await tester.pumpWidget(
      appForTest(engine, const ReviewScreen(workbenchId: 'wb-1')),
    );
    await pumpUntilFound(tester, find.text('Annual:A1'));
    await tester.pumpAndSettle();

    final requests = engine.paramsHistory['ReviewGetXlsxPreviewGrid'] ?? [];
    expect(requests, isNotEmpty);
    expect(requests.first?['version'], 'draft');
    expect(requests.first?['sheet'], 'Annual');
    expect(requests.first?['row_start'], 8);
    expect(requests.first?['col_start'], 3);
  });

  testWidgets('xlsx stale focus hint retries draft preview with first sheet', (
    tester,
  ) async {
    final engine = _FakeWorkbenchEngine(
      hasDraft: true,
      draftId: 'd-xlsx-stale-focus',
      messages: const [],
      xlsxDraftAvailableSheets: const ['P1_Orders_Items'],
      reviewChanges: const [
        {
          'path': 'quarterly_data.xlsx',
          'change_type': 'modified',
          'file_kind': 'xlsx',
          'preview_kind': 'grid',
          'mime_type':
              'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet',
          'is_opaque': false,
          'summary': '',
          'focus_hint': {
            'sheet': 'P1_Consents',
            'row_start': 1,
            'col_start': 10,
          },
        },
      ],
    );

    await tester.pumpWidget(
      appForTest(engine, const ReviewScreen(workbenchId: 'wb-1')),
    );
    await pumpUntilFound(tester, find.text('P1_Orders_Items:A1'));
    await tester.pumpAndSettle();

    expect(find.textContaining('Preview unavailable:'), findsNothing);
    final requests = engine.paramsHistory['ReviewGetXlsxPreviewGrid'] ?? [];
    expect(requests.where((req) => req?['version'] == 'draft').length, 2);
    expect(requests.first?['sheet'], 'P1_Consents');
    expect(requests[1]?['sheet'], '');
  });

  testWidgets('pptx focus hint drives initial structured diff request target', (
    tester,
  ) async {
    final engine = _FakeWorkbenchEngine(
      hasDraft: true,
      draftId: 'd-pptx-focus',
      messages: const [],
      reviewChanges: const [
        {
          'path': 'slides.pptx',
          'change_type': 'modified',
          'file_kind': 'pptx',
          'preview_kind': 'page',
          'mime_type':
              'application/vnd.openxmlformats-officedocument.presentationml.presentation',
          'is_opaque': false,
          'summary': '',
          'focus_hint': {'slide_index': 3},
        },
      ],
    );

    await tester.pumpWidget(
      appForTest(engine, const ReviewScreen(workbenchId: 'wb-1')),
    );
    await tester.pumpAndSettle();

    final requests = engine.paramsHistory['ReviewGetPptxContentDiff'] ?? [];
    expect(requests, isNotEmpty);
    expect(requests.first?['slide_index'], 3);
    expect(engine.callCount('ReviewGetPptxPreviewSlide'), 0);
  });

  testWidgets('docx preview requests zoomed scale and fit-width rendering', (
    tester,
  ) async {
    final engine = _FakeWorkbenchEngine(
      hasDraft: true,
      draftId: 'd-docx-zoom',
      messages: const [],
      reviewChanges: const [
        {
          'path': 'report.docx',
          'change_type': 'modified',
          'file_kind': 'docx',
          'preview_kind': 'page',
          'mime_type':
              'application/vnd.openxmlformats-officedocument.wordprocessingml.document',
          'is_opaque': false,
          'summary': '',
        },
      ],
    );

    await tester.pumpWidget(
      appForTest(engine, const ReviewScreen(workbenchId: 'wb-1')),
    );
    await tester.pumpAndSettle();

    final requests = engine.paramsHistory['ReviewGetDocxPreviewPage'] ?? [];
    expect(requests, isNotEmpty);
    expect(requests.first?['version'], 'draft');
    expect(requests.first?['scale'], 1.5);

    final pageImages = tester.widgetList<Image>(find.byType(Image)).toList();
    expect(pageImages, isNotEmpty);
    expect(pageImages.every((image) => image.fit == BoxFit.fitWidth), isTrue);
    expect(find.byType(SingleChildScrollView), findsWidgets);
  });

  testWidgets(
    'pptx review prefers structured positioned diff and avoids preview error',
    (tester) async {
      final engine = _FakeWorkbenchEngine(
        hasDraft: true,
        draftId: 'd-pptx-soft',
        messages: const [],
        failPublishedMethods: const {'ReviewGetPptxPreviewSlide'},
        reviewChanges: const [
          {
            'path': 'slides.pptx',
            'change_type': 'modified',
            'file_kind': 'pptx',
            'preview_kind': 'page',
            'mime_type':
                'application/vnd.openxmlformats-officedocument.presentationml.presentation',
            'is_opaque': false,
            'summary': '',
            'focus_hint': {'slide_index': 2},
          },
        ],
      );

      await tester.pumpWidget(
        appForTest(engine, const ReviewScreen(workbenchId: 'wb-1')),
      );
      await tester.pumpAndSettle();

      expect(find.textContaining('Preview unavailable:'), findsNothing);
      expect(
        find.textContaining('Font resolution: bundled -> OS-known -> fallback'),
        findsWidgets,
      );
      expect(engine.callCount('ReviewGetPptxContentDiff'), 1);
      expect(engine.callCount('ReviewGetPptxPreviewSlide'), 0);
    },
  );

  testWidgets(
    'xlsx published target missing keeps draft grid and avoids global error',
    (tester) async {
      final engine = _FakeWorkbenchEngine(
        hasDraft: true,
        draftId: 'd-xlsx-soft',
        messages: const [],
        failPublishedMethods: const {'ReviewGetXlsxPreviewGrid'},
        reviewChanges: const [
          {
            'path': 'quarterly_data.xlsx',
            'change_type': 'modified',
            'file_kind': 'xlsx',
            'preview_kind': 'grid',
            'mime_type':
                'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet',
            'is_opaque': false,
            'summary': '',
            'focus_hint': {'sheet': 'Annual'},
          },
        ],
      );

      await tester.pumpWidget(
        appForTest(engine, const ReviewScreen(workbenchId: 'wb-1')),
      );
      await pumpUntilFound(tester, find.text('Annual:A1'));
      await tester.pumpAndSettle();

      expect(find.textContaining('Preview unavailable:'), findsNothing);
      expect(find.text('No data.'), findsOneWidget);
      expect(find.text('Annual:A1'), findsOneWidget);
    },
  );

  testWidgets('pptx positioned data missing falls back to structured metadata', (
    tester,
  ) async {
    final engine = _FakeWorkbenchEngine(
      hasDraft: true,
      draftId: 'd-pptx-fallback',
      messages: const [],
      includePptxPositioned: false,
      reviewChanges: const [
        {
          'path': 'slides.pptx',
          'change_type': 'modified',
          'file_kind': 'pptx',
          'preview_kind': 'page',
          'mime_type':
              'application/vnd.openxmlformats-officedocument.presentationml.presentation',
          'is_opaque': false,
          'summary': '',
          'focus_hint': {'slide_index': 1},
        },
      ],
    );

    await tester.pumpWidget(
      appForTest(engine, const ReviewScreen(workbenchId: 'wb-1')),
    );
    await tester.pumpAndSettle();

    expect(
      find.textContaining(
        'Positioned slide data is incomplete. Showing structured metadata fallback.',
      ),
      findsOneWidget,
    );
    expect(find.textContaining('Draft block for slide 2'), findsOneWidget);
    expect(engine.callCount('ReviewGetPptxContentDiff'), 1);
  });

  testWidgets(
    'docx preview failure falls back to structured diff and uses focus hint',
    (tester) async {
      final engine = _FakeWorkbenchEngine(
        hasDraft: true,
        draftId: 'd-3',
        messages: const [],
        failingMethods: const {'ReviewGetDocxPreviewPage'},
        reviewChanges: const [
          {
            'path': 'report.docx',
            'change_type': 'modified',
            'file_kind': 'docx',
            'preview_kind': 'page',
            'mime_type':
                'application/vnd.openxmlformats-officedocument.wordprocessingml.document',
            'is_opaque': false,
            'summary': '',
            'focus_hint': {'section_index': 1},
          },
        ],
      );

      await tester.pumpWidget(
        appForTest(engine, const ReviewScreen(workbenchId: 'wb-1')),
      );
      await pumpUntilFound(
        tester,
        find.textContaining('Draft paragraph for section 2'),
      );
      await tester.pumpAndSettle();

      expect(
        find.textContaining('Draft paragraph for section 2'),
        findsOneWidget,
      );
      expect(find.textContaining('Preview unavailable:'), findsNothing);
      expect(engine.callCount('ReviewGetDocxContentDiff'), 1);
      expect(
        engine.lastParams['ReviewGetDocxContentDiff']?['section_index'],
        1,
      );
      expect(find.byType(CircularProgressIndicator), findsNothing);
    },
  );

  testWidgets('pptx structured diff uses focus hint even if preview RPC fails', (
    tester,
  ) async {
    final engine = _FakeWorkbenchEngine(
      hasDraft: true,
      draftId: 'd-4',
      messages: const [],
      failingMethods: const {'ReviewGetPptxPreviewSlide'},
      reviewChanges: const [
        {
          'path': 'slides.pptx',
          'change_type': 'modified',
          'file_kind': 'pptx',
          'preview_kind': 'page',
          'mime_type':
              'application/vnd.openxmlformats-officedocument.presentationml.presentation',
          'is_opaque': false,
          'summary': '',
          'focus_hint': {'slide_index': 1},
        },
      ],
    );

    await tester.pumpWidget(
      appForTest(engine, const ReviewScreen(workbenchId: 'wb-1')),
    );
    await pumpUntilFound(
      tester,
      find.textContaining('Font resolution: bundled -> OS-known -> fallback'),
    );
    await tester.pumpAndSettle();

    expect(
      find.textContaining('Font resolution: bundled -> OS-known -> fallback'),
      findsWidgets,
    );
    expect(find.textContaining('Preview unavailable:'), findsNothing);
    expect(engine.callCount('ReviewGetPptxContentDiff'), 1);
    expect(engine.lastParams['ReviewGetPptxContentDiff']?['slide_index'], 1);
    expect(engine.callCount('ReviewGetPptxPreviewSlide'), 0);
    expect(find.byType(CircularProgressIndicator), findsNothing);
  });

  testWidgets(
    'image preview failure clears spinner and shows preview error message',
    (tester) async {
      final engine = _FakeWorkbenchEngine(
        hasDraft: true,
        draftId: 'd-5',
        messages: const [],
        failingMethods: const {'ReviewGetImagePreview'},
        reviewChanges: const [
          {
            'path': 'diagram.png',
            'change_type': 'modified',
            'file_kind': 'image',
            'preview_kind': 'image',
            'mime_type': 'image/png',
            'is_opaque': false,
            'summary': '',
          },
        ],
      );

      await tester.pumpWidget(
        appForTest(engine, const ReviewScreen(workbenchId: 'wb-1')),
      );
      await pumpUntilFound(tester, find.textContaining('Preview unavailable:'));
      await tester.pumpAndSettle();

      expect(find.textContaining('Preview unavailable:'), findsOneWidget);
      expect(find.textContaining('TOOL_WORKER_UNAVAILABLE'), findsOneWidget);
      expect(find.byType(CircularProgressIndicator), findsNothing);
    },
  );

  testWidgets('publish checkpoint card restores and refreshes state', (
    tester,
  ) async {
    await useDesktopSurface(tester);
    final engine = _FakeWorkbenchEngine(
      hasDraft: false,
      draftId: '',
      reviewChanges: const [],
      messages: [
        {
          'message_id': 'evt-1',
          'type': 'system_event',
          'role': 'system',
          'text': 'Publish checkpoint created.',
          'created_at': '2026-02-05T10:00:00Z',
          'event': {
            'kind': 'checkpoint_publish',
            'checkpoint_id': 'cp-1',
            'reason': 'publish',
          },
        },
      ],
    );
    await tester.pumpWidget(
      appForTest(engine, const WorkbenchScreen(workbenchId: 'wb-1')),
    );
    await tester.pumpAndSettle();

    final refreshFilesBefore = engine.callCount('WorkbenchFilesList');
    final refreshMessagesBefore = engine.callCount('WorkshopGetConversation');
    final refreshDraftBefore = engine.callCount('DraftGetState');
    final refreshClutterBefore = engine.callCount('WorkbenchGetClutter');

    final restoreButton = find.byKey(
      AppKeys.workbenchCheckpointRestoreButton('cp-1'),
    );
    expect(restoreButton, findsOneWidget);
    await tester.tap(restoreButton);
    await tester.pumpAndSettle();
    final confirmRestore = find.descendant(
      of: find.byType(AlertDialog),
      matching: find.widgetWithText(ElevatedButton, 'Restore'),
    );
    await tester.tap(confirmRestore);
    await tester.pumpAndSettle();

    expect(engine.callCount('CheckpointRestore'), 1);
    expect(
      engine.callCount('WorkbenchFilesList'),
      greaterThan(refreshFilesBefore),
    );
    expect(
      engine.callCount('WorkshopGetConversation'),
      greaterThan(refreshMessagesBefore),
    );
    expect(engine.callCount('DraftGetState'), greaterThan(refreshDraftBefore));
    expect(
      engine.callCount('WorkbenchGetClutter'),
      greaterThan(refreshClutterBefore),
    );
  });

  testWidgets('publish checkpoint restore is disabled while draft exists', (
    tester,
  ) async {
    await useDesktopSurface(tester);
    final engine = _FakeWorkbenchEngine(
      hasDraft: true,
      draftId: 'd-locked',
      reviewChanges: const [],
      messages: [
        {
          'message_id': 'evt-1',
          'type': 'system_event',
          'role': 'system',
          'text': 'Publish checkpoint created.',
          'created_at': '2026-02-05T10:00:00Z',
          'event': {
            'kind': 'checkpoint_publish',
            'checkpoint_id': 'cp-1',
            'reason': 'publish',
          },
        },
      ],
    );
    await tester.pumpWidget(
      appForTest(engine, const WorkbenchScreen(workbenchId: 'wb-1')),
    );
    await pumpUntilFound(tester, find.byKey(AppKeys.reviewScreen));
    await tapBackButton(tester);

    final restoreButton = find.byKey(
      AppKeys.workbenchCheckpointRestoreButton('cp-1'),
    );
    final buttonWidget = tester.widget<TextButton>(restoreButton);
    expect(buttonWidget.onPressed, isNull);
    expect(
      find.byWidgetPredicate(
        (widget) =>
            widget is Tooltip &&
            widget.message == 'Publish or discard Draft to restore.',
      ),
      findsOneWidget,
    );
  });

  testWidgets('message rewind action calls WorkshopUndoToMessage', (
    tester,
  ) async {
    await useDesktopSurface(tester);
    final engine = _FakeWorkbenchEngine(
      hasDraft: false,
      draftId: '',
      reviewChanges: const [],
      messages: [
        {
          'message_id': 'u-1',
          'role': 'user',
          'text': 'Please summarize this.',
          'created_at': '2026-02-05T10:00:00Z',
        },
        {
          'message_id': 'a-1',
          'role': 'assistant',
          'text': 'Here is a summary.',
          'created_at': '2026-02-05T10:00:05Z',
        },
      ],
    );
    await tester.pumpWidget(
      appForTest(engine, const WorkbenchScreen(workbenchId: 'wb-1')),
    );
    await tester.pumpAndSettle();

    await hover(tester, find.text('Please summarize this.'));
    await tester.tap(find.byKey(AppKeys.workbenchMessageRewindButton('u-1')));
    await tester.pumpAndSettle();

    await tester.tap(find.widgetWithText(ElevatedButton, 'Rewind'));
    await tester.pumpAndSettle();

    expect(engine.callCount('WorkshopUndoToMessage'), 1);
    expect(engine.lastParams['WorkshopUndoToMessage']?['message_id'], 'u-1');
  });

  testWidgets(
    'workbench hides tool payloads and generic system events from chat',
    (tester) async {
      await useDesktopSurface(tester);
      final engine = _FakeWorkbenchEngine(
        hasDraft: false,
        draftId: '',
        reviewChanges: const [],
        messages: const [
          {
            'message_id': 'u-1',
            'role': 'user',
            'text': 'Visible user message',
            'created_at': '2026-02-05T10:00:00Z',
          },
          {
            'message_id': 'a-1',
            'role': 'assistant',
            'text': 'Visible assistant message',
            'created_at': '2026-02-05T10:00:05Z',
          },
          {
            'message_id': 'a-tool',
            'role': 'assistant',
            'type': 'assistant_message',
            'text': '',
            'metadata': {
              'tool_calls': [
                {'id': 'call-1', 'name': 'read_file'},
              ],
            },
            'created_at': '2026-02-05T10:00:06Z',
          },
          {
            'message_id': 'tool-1',
            'role': 'tool',
            'type': 'tool_result',
            'text': '{"text":"internal tool output"}',
            'created_at': '2026-02-05T10:00:07Z',
          },
          {
            'message_id': 'sys-1',
            'role': 'system',
            'type': 'system_event',
            'text': 'Context compressed.',
            'event': {'kind': 'context_compressed'},
            'created_at': '2026-02-05T10:00:08Z',
          },
          {
            'message_id': 'evt-1',
            'role': 'system',
            'type': 'system_event',
            'text': 'Publish checkpoint created.',
            'event': {
              'kind': 'checkpoint_publish',
              'checkpoint_id': 'cp-visible',
              'reason': 'publish',
            },
            'created_at': '2026-02-05T10:00:09Z',
          },
        ],
      );
      await tester.pumpWidget(
        appForTest(engine, const WorkbenchScreen(workbenchId: 'wb-1')),
      );
      await tester.pumpAndSettle();

      expect(find.text('Visible user message'), findsOneWidget);
      expect(find.text('Visible assistant message'), findsOneWidget);
      expect(find.textContaining('internal tool output'), findsNothing);
      expect(find.text('Context compressed.'), findsNothing);
      expect(
        find.byKey(AppKeys.workbenchCheckpointEventCard('cp-visible')),
        findsOneWidget,
      );
      expect(
        find.byKey(AppKeys.workbenchMessageRewindButton('a-tool')),
        findsNothing,
      );
      expect(
        find.byKey(AppKeys.workbenchMessageRewindButton('tool-1')),
        findsNothing,
      );
      expect(
        find.byKey(AppKeys.workbenchMessageRewindButton('sys-1')),
        findsNothing,
      );
    },
  );

  testWidgets('message regenerate action calls WorkshopRegenerate', (
    tester,
  ) async {
    await useDesktopSurface(tester);
    final engine = _FakeWorkbenchEngine(
      hasDraft: false,
      draftId: '',
      reviewChanges: const [],
      messages: [
        {
          'message_id': 'u-1',
          'role': 'user',
          'text': 'Please summarize this.',
          'created_at': '2026-02-05T10:00:00Z',
        },
        {
          'message_id': 'a-1',
          'role': 'assistant',
          'text': 'Here is a summary.',
          'created_at': '2026-02-05T10:00:05Z',
        },
      ],
    );
    await tester.pumpWidget(
      appForTest(engine, const WorkbenchScreen(workbenchId: 'wb-1')),
    );
    await tester.pumpAndSettle();

    await hover(tester, find.text('Here is a summary.'));
    await tester.tap(
      find.byKey(AppKeys.workbenchMessageRegenerateButton('a-1')),
    );
    await tester.pumpAndSettle();

    expect(engine.callCount('WorkshopRegenerate'), 1);
    expect(engine.lastParams['WorkshopRegenerate']?['message_id'], 'a-1');
  });

  testWidgets(
    'workbench chrome uses icon actions and per-file extract control',
    (tester) async {
      await useDesktopSurface(tester);
      final engine = _FakeWorkbenchEngine(
        hasDraft: false,
        draftId: '',
        messages: const [],
        reviewChanges: const [],
        files: const [
          {
            'path': 'notes.txt',
            'size': 12,
            'file_kind': 'text',
            'mime_type': 'text/plain',
            'is_opaque': false,
          },
        ],
      );
      await tester.pumpWidget(
        appForTest(engine, const WorkbenchScreen(workbenchId: 'wb-1')),
      );
      await tester.pumpAndSettle();

      final checkpoints = find.byKey(AppKeys.workbenchCheckpointsButton);
      final settings = find.byKey(AppKeys.workbenchSettingsButton);
      final extract = find.byKey(
        AppKeys.workbenchFileExtractButton('notes.txt'),
      );

      expect(checkpoints, findsOneWidget);
      expect(settings, findsOneWidget);
      expect(extract, findsOneWidget);
      expect(tester.widget<IconButton>(checkpoints).onPressed, isNotNull);
      expect(tester.widget<IconButton>(settings).onPressed, isNotNull);
      expect(tester.widget<IconButton>(extract).onPressed, isNotNull);
      expect(tester.widget<IconButton>(settings).iconSize, 40);
    },
  );

  testWidgets('per-file extract control is disabled while draft exists', (
    tester,
  ) async {
    await useDesktopSurface(tester);
    final engine = _FakeWorkbenchEngine(
      hasDraft: true,
      draftId: 'd-locked',
      messages: const [],
      reviewChanges: const [],
      files: const [
        {
          'path': 'notes.txt',
          'size': 12,
          'file_kind': 'text',
          'mime_type': 'text/plain',
          'is_opaque': false,
        },
      ],
    );
    await tester.pumpWidget(
      appForTest(engine, const WorkbenchScreen(workbenchId: 'wb-1')),
    );
    await pumpUntilFound(tester, find.byKey(AppKeys.reviewScreen));
    await tapBackButton(tester);

    final extract = find.byKey(AppKeys.workbenchFileExtractButton('notes.txt'));
    expect(extract, findsOneWidget);
    expect(tester.widget<IconButton>(extract).onPressed, isNull);
  });
}
