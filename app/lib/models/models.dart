class Workbench {
  Workbench({
    required this.id,
    required this.name,
    required this.createdAt,
    required this.updatedAt,
    required this.defaultModelId,
  });

  final String id;
  final String name;
  final String createdAt;
  final String updatedAt;
  final String defaultModelId;

  factory Workbench.fromJson(Map<String, dynamic> json) {
    return Workbench(
      id: json['id'] as String,
      name: json['name'] as String? ?? 'Untitled Workbench',
      createdAt: json['created_at'] as String? ?? '',
      updatedAt: json['updated_at'] as String? ?? '',
      defaultModelId: json['default_model_id'] as String? ?? '',
    );
  }
}

class WorkbenchScope {
  WorkbenchScope({
    required this.limits,
    required this.supportedTypes,
    required this.sandboxRoot,
  });

  final WorkbenchScopeLimits limits;
  final List<String> supportedTypes;
  final String sandboxRoot;

  factory WorkbenchScope.fromJson(Map<String, dynamic> json) {
    final limitsJson = _asStringKeyedMap(json['limits']);
    final supportedTypes = <String>{};
    final rawSupportedTypes = json['supported_types'];
    if (rawSupportedTypes is List) {
      for (final value in rawSupportedTypes) {
        final text = value.toString().trim();
        if (text.isNotEmpty) {
          supportedTypes.add(text);
        }
      }
    } else {
      final supportedMap = _asStringKeyedMap(rawSupportedTypes);
      for (final key in const ['editable_extensions', 'readable_kinds']) {
        final list = supportedMap[key];
        if (list is List) {
          for (final value in list) {
            final text = value.toString().trim();
            if (text.isNotEmpty) {
              supportedTypes.add(text);
            }
          }
        }
      }
    }
    return WorkbenchScope(
      limits: WorkbenchScopeLimits.fromJson(
        limitsJson.isNotEmpty ? limitsJson : json,
      ),
      supportedTypes: supportedTypes.toList(),
      sandboxRoot: _normalizedString(json['sandbox_root']) ?? '',
    );
  }
}

class WorkbenchScopeLimits {
  WorkbenchScopeLimits({required this.maxFiles, required this.maxFileBytes});

  final int maxFiles;
  final int maxFileBytes;

  factory WorkbenchScopeLimits.fromJson(Map<String, dynamic> json) {
    return WorkbenchScopeLimits(
      maxFiles: _intValue(json['max_files'], fallback: json['max_file_count']),
      maxFileBytes: _intValue(
        json['max_file_bytes'],
        fallback: json['max_file_size_bytes'],
      ),
    );
  }
}

class DraftMetadata {
  DraftMetadata({
    required this.hasDraft,
    required this.draftId,
    required this.createdAt,
    required this.source,
  });

  final bool hasDraft;
  final String? draftId;
  final String createdAt;
  final DraftSource? source;

  factory DraftMetadata.fromJson(Map<String, dynamic> json) {
    final hasDraft = json['has_draft'] == true;
    final sourceJson = _asStringKeyedMap(json['source']);
    DraftSource? source;
    if (sourceJson.isNotEmpty) {
      source = DraftSource.fromJson(sourceJson);
    } else {
      final sourceKind = _normalizedString(json['source_kind']) ?? '';
      final sourceRef =
          _normalizedString(json['source_ref']) ??
          _normalizedString(json['source_job_id']);
      if (sourceKind.isNotEmpty || sourceRef != null) {
        source = DraftSource(kind: sourceKind, jobId: sourceRef);
      }
    }
    return DraftMetadata(
      hasDraft: hasDraft,
      draftId: hasDraft ? _normalizedString(json['draft_id']) : null,
      createdAt: hasDraft ? (_normalizedString(json['created_at']) ?? '') : '',
      source: hasDraft ? source : null,
    );
  }
}

class DraftSource {
  DraftSource({required this.kind, required this.jobId});

  final String kind;
  final String? jobId;

  factory DraftSource.fromJson(Map<String, dynamic> json) {
    return DraftSource(
      kind: _normalizedString(json['kind']) ?? '',
      jobId:
          _normalizedString(json['job_id']) ??
          _normalizedString(json['ref']) ??
          _normalizedString(json['source_ref']),
    );
  }
}

class ProviderStatus {
  ProviderStatus({
    required this.id,
    required this.displayName,
    required this.enabled,
    required this.configured,
    required this.models,
    this.rpiReasoning,
    this.authMode = 'api_key',
    this.oauthConnected,
    this.oauthAccountLabel,
    this.oauthExpiresAt,
    this.oauthExpired,
    this.tokenConnected,
    this.tokenAccountLabel,
  });

  final String id;
  final String displayName;
  final bool enabled;
  final bool configured;
  final List<String> models;
  final ProviderRpiReasoning? rpiReasoning;
  final String authMode;
  final bool? oauthConnected;
  final String? oauthAccountLabel;
  final String? oauthExpiresAt;
  final bool? oauthExpired;
  final bool? tokenConnected;
  final String? tokenAccountLabel;

  bool get isOAuth => authMode == 'oauth';
  bool get isSetupToken => authMode == 'setup_token';

  factory ProviderStatus.fromJson(Map<String, dynamic> json) {
    final reasoningJson = _asStringKeyedMap(json['rpi_reasoning']);
    return ProviderStatus(
      id: json['provider_id'] as String? ?? '',
      displayName: json['display_name'] as String? ?? '',
      enabled: json['enabled'] == true,
      configured: json['configured'] == true,
      models: (json['models'] as List<dynamic>? ?? [])
          .map((e) => e.toString())
          .toList(),
      rpiReasoning: reasoningJson.isEmpty
          ? null
          : ProviderRpiReasoning.fromJson(reasoningJson),
      authMode: _normalizedString(json['auth_mode']) ?? 'api_key',
      oauthConnected: json['oauth_connected'] as bool?,
      oauthAccountLabel: _normalizedString(json['oauth_account_label']),
      oauthExpiresAt: _normalizedString(json['oauth_expires_at']),
      oauthExpired: json['oauth_expired'] as bool?,
      tokenConnected: json['token_connected'] as bool?,
      tokenAccountLabel: _normalizedString(json['token_account_label']),
    );
  }
}

class ProviderRpiReasoning {
  ProviderRpiReasoning({
    this.researchEffort,
    this.planEffort,
    this.implementEffort,
  });

  final String? researchEffort;
  final String? planEffort;
  final String? implementEffort;

  factory ProviderRpiReasoning.fromJson(Map<String, dynamic> json) {
    return ProviderRpiReasoning(
      researchEffort:
          _normalizedString(json['research_effort']) ??
          _normalizedString(json['researchEffort']),
      planEffort:
          _normalizedString(json['plan_effort']) ??
          _normalizedString(json['planEffort']),
      implementEffort:
          _normalizedString(json['implement_effort']) ??
          _normalizedString(json['implementEffort']),
    );
  }
}

class ModelInfo {
  ModelInfo({
    required this.id,
    required this.providerId,
    required this.displayName,
    required this.contextTokensEstimate,
    required this.supportsFileRead,
    required this.supportsFileWrite,
  });

  final String id;
  final String providerId;
  final String displayName;
  final int contextTokensEstimate;
  final bool supportsFileRead;
  final bool supportsFileWrite;

  factory ModelInfo.fromJson(Map<String, dynamic> json) {
    return ModelInfo(
      id: json['model_id'] as String? ?? '',
      providerId: json['provider_id'] as String? ?? '',
      displayName: json['display_name'] as String? ?? '',
      contextTokensEstimate:
          (json['context_tokens_estimate'] as num?)?.toInt() ?? 0,
      supportsFileRead: json['supports_file_read'] == true,
      supportsFileWrite: json['supports_file_write'] == true,
    );
  }
}

class ClutterState {
  ClutterState({
    required this.score,
    required this.level,
    required this.modelId,
    required this.contextItemsWeight,
    required this.contextShare,
    required this.contextWarning,
  });

  final double score;
  final String level;
  final String modelId;
  final double contextItemsWeight;
  final double contextShare;
  final bool contextWarning;

  factory ClutterState.fromJson(Map<String, dynamic> json) {
    return ClutterState(
      score: (json['score'] as num?)?.toDouble() ?? 0.0,
      level: json['level'] as String? ?? 'Light',
      modelId: json['model_id'] as String? ?? '',
      contextItemsWeight:
          (json['context_items_weight'] as num?)?.toDouble() ?? 0.0,
      contextShare: (json['context_share'] as num?)?.toDouble() ?? 0.0,
      contextWarning: json['context_warning'] == true,
    );
  }
}

class ContextItemSummary {
  ContextItemSummary({
    required this.category,
    required this.status,
    required this.summary,
    required this.hasDirectEdits,
    required this.createdAt,
    required this.lastProcessedAt,
    required this.lastDirectEditAt,
  });

  final String category;
  final String status;
  final String summary;
  final bool hasDirectEdits;
  final String createdAt;
  final String lastProcessedAt;
  final String lastDirectEditAt;

  bool get isActive => status == 'active';

  factory ContextItemSummary.fromJson(Map<String, dynamic> json) {
    return ContextItemSummary(
      category: json['category'] as String? ?? '',
      status: json['status'] as String? ?? 'empty',
      summary: json['summary'] as String? ?? '',
      hasDirectEdits: json['has_direct_edits'] == true,
      createdAt: json['created_at'] as String? ?? '',
      lastProcessedAt: json['last_processed_at'] as String? ?? '',
      lastDirectEditAt: json['last_direct_edit_at'] as String? ?? '',
    );
  }
}

class ContextSource {
  ContextSource({
    required this.mode,
    required this.text,
    required this.originalFilename,
    required this.note,
    required this.createdAt,
    required this.lastProcessedAt,
    required this.lastDirectEditAt,
    required this.modelId,
    required this.hasDirectEdits,
  });

  final String mode;
  final String text;
  final String originalFilename;
  final String note;
  final String createdAt;
  final String lastProcessedAt;
  final String lastDirectEditAt;
  final String modelId;
  final bool hasDirectEdits;

  factory ContextSource.fromJson(Map<String, dynamic> json) {
    return ContextSource(
      mode: json['mode'] as String? ?? '',
      text: json['text'] as String? ?? '',
      originalFilename: json['original_filename'] as String? ?? '',
      note: json['note'] as String? ?? '',
      createdAt: json['created_at'] as String? ?? '',
      lastProcessedAt: json['last_processed_at'] as String? ?? '',
      lastDirectEditAt: json['last_direct_edit_at'] as String? ?? '',
      modelId: json['model_id'] as String? ?? '',
      hasDirectEdits: json['has_direct_edits'] == true,
    );
  }
}

class ContextArtifactFile {
  ContextArtifactFile({required this.path, required this.content});

  final String path;
  final String content;

  factory ContextArtifactFile.fromJson(Map<String, dynamic> json) {
    return ContextArtifactFile(
      path: json['path'] as String? ?? '',
      content: json['content'] as String? ?? '',
    );
  }

  Map<String, dynamic> toJson() {
    return {'path': path, 'content': content};
  }
}

class ContextItem {
  ContextItem({
    required this.category,
    required this.status,
    required this.summary,
    required this.hasDirectEdits,
    required this.createdAt,
    required this.lastProcessedAt,
    required this.lastDirectEditAt,
    required this.source,
    required this.files,
  });

  final String category;
  final String status;
  final String summary;
  final bool hasDirectEdits;
  final String createdAt;
  final String lastProcessedAt;
  final String lastDirectEditAt;
  final ContextSource? source;
  final List<ContextArtifactFile> files;

  factory ContextItem.fromJson(Map<String, dynamic> json) {
    final sourceJson = _asStringKeyedMap(json['source']);
    final filesJson = (json['files'] as List<dynamic>? ?? [])
        .cast<Map<String, dynamic>>();
    return ContextItem(
      category: json['category'] as String? ?? '',
      status: json['status'] as String? ?? 'empty',
      summary: json['summary'] as String? ?? '',
      hasDirectEdits: json['has_direct_edits'] == true,
      createdAt: json['created_at'] as String? ?? '',
      lastProcessedAt: json['last_processed_at'] as String? ?? '',
      lastDirectEditAt: json['last_direct_edit_at'] as String? ?? '',
      source: sourceJson.isEmpty ? null : ContextSource.fromJson(sourceJson),
      files: filesJson.map(ContextArtifactFile.fromJson).toList(),
    );
  }
}

class CheckpointMetadata {
  CheckpointMetadata({
    required this.id,
    required this.createdAt,
    required this.reason,
    required this.description,
    required this.files,
    required this.totalBytes,
  });

  final String id;
  final String createdAt;
  final String reason;
  final String description;
  final int files;
  final int totalBytes;

  factory CheckpointMetadata.fromJson(Map<String, dynamic> json) {
    final stats = json['stats'] as Map<String, dynamic>? ?? {};
    return CheckpointMetadata(
      id: json['checkpoint_id'] as String? ?? '',
      createdAt: json['created_at'] as String? ?? '',
      reason: json['reason'] as String? ?? '',
      description: json['description'] as String? ?? '',
      files: (stats['files'] as num?)?.toInt() ?? 0,
      totalBytes: (stats['total_bytes'] as num?)?.toInt() ?? 0,
    );
  }
}

class WorkbenchFile {
  WorkbenchFile({
    required this.path,
    required this.size,
    required this.modifiedAt,
    required this.addedAt,
    required this.fileKind,
    required this.mimeType,
    required this.isOpaque,
  });

  final String path;
  final int size;
  final String modifiedAt;
  final String addedAt;
  final String fileKind;
  final String mimeType;
  final bool isOpaque;

  factory WorkbenchFile.fromJson(Map<String, dynamic> json) {
    return WorkbenchFile(
      path: json['path'] as String,
      size: (json['size'] as num?)?.toInt() ?? 0,
      modifiedAt: json['modified_at'] as String? ?? '',
      addedAt: json['added_at'] as String? ?? '',
      fileKind: json['file_kind'] as String? ?? '',
      mimeType: json['mime_type'] as String? ?? '',
      isOpaque: json['is_opaque'] as bool? ?? false,
    );
  }
}

class WorkbenchExtractResult {
  WorkbenchExtractResult({
    required this.path,
    required this.status,
    required this.reason,
    required this.finalPath,
  });

  final String path;
  final String status;
  final String reason;
  final String finalPath;

  bool get isExtracted => status == 'extracted';
  bool get isSkipped => status == 'skipped';
  bool get isFailed => status == 'failed';

  factory WorkbenchExtractResult.fromJson(Map<String, dynamic> json) {
    return WorkbenchExtractResult(
      path: json['path'] as String? ?? '',
      status: json['status'] as String? ?? '',
      reason: json['reason'] as String? ?? '',
      finalPath: json['final_path'] as String? ?? '',
    );
  }
}

class ProposalWrite {
  ProposalWrite({required this.path, required this.content});

  final String path;
  final String content;

  factory ProposalWrite.fromJson(Map<String, dynamic> json) {
    return ProposalWrite(
      path: json['path'] as String? ?? '',
      content: json['content'] as String? ?? '',
    );
  }
}

class Proposal {
  Proposal({
    required this.proposalId,
    required this.summary,
    required this.noChanges,
    required this.writes,
    required this.ops,
    required this.warnings,
  });

  final String proposalId;
  final String summary;
  final bool noChanges;
  final List<ProposalWrite> writes;
  final List<ProposalOp> ops;
  final List<String> warnings;

  factory Proposal.fromJson(Map<String, dynamic> json) {
    final writesJson = (json['writes'] as List<dynamic>? ?? [])
        .cast<Map<String, dynamic>>();
    final opsJson = (json['ops'] as List<dynamic>? ?? [])
        .cast<Map<String, dynamic>>();
    return Proposal(
      proposalId: json['proposal_id'] as String? ?? '',
      summary: json['summary'] as String? ?? '',
      noChanges: json['no_changes'] as bool? ?? false,
      writes: writesJson.map(ProposalWrite.fromJson).toList(),
      ops: opsJson.map(ProposalOp.fromJson).toList(),
      warnings: (json['warnings'] as List<dynamic>? ?? [])
          .map((e) => e.toString())
          .toList(),
    );
  }
}

class ProposalOp {
  ProposalOp({required this.path, required this.kind, required this.summary});

  final String path;
  final String kind;
  final String summary;

  factory ProposalOp.fromJson(Map<String, dynamic> json) {
    return ProposalOp(
      path: json['path'] as String? ?? '',
      kind: json['kind'] as String? ?? '',
      summary: json['summary'] as String? ?? '',
    );
  }
}

class ChangeItem {
  ChangeItem({
    required this.path,
    required this.changeType,
    required this.fileKind,
    required this.previewKind,
    required this.mimeType,
    required this.isOpaque,
    required this.summary,
    required this.sizeBytes,
    required this.focusHint,
  });

  final String path;
  final String changeType;
  final String fileKind;
  final String previewKind;
  final String mimeType;
  final bool isOpaque;
  final String summary;
  final int sizeBytes;
  final Map<String, dynamic>? focusHint;

  factory ChangeItem.fromJson(Map<String, dynamic> json) {
    return ChangeItem(
      path: json['path'] as String? ?? '',
      changeType: json['change_type'] as String? ?? '',
      fileKind: json['file_kind'] as String? ?? '',
      previewKind: json['preview_kind'] as String? ?? '',
      mimeType: json['mime_type'] as String? ?? '',
      isOpaque: json['is_opaque'] as bool? ?? false,
      summary: json['summary'] as String? ?? '',
      sizeBytes: (json['size_bytes'] as num?)?.toInt() ?? 0,
      focusHint: json['focus_hint'] as Map<String, dynamic>?,
    );
  }
}

class DiffLine {
  DiffLine({
    required this.type,
    required this.text,
    this.oldLine,
    this.newLine,
  });

  final String type;
  final String text;
  final int? oldLine;
  final int? newLine;

  factory DiffLine.fromJson(Map<String, dynamic> json) {
    return DiffLine(
      type: json['type'] as String? ?? 'context',
      text: json['text'] as String? ?? '',
      oldLine: (json['old_line'] as num?)?.toInt(),
      newLine: (json['new_line'] as num?)?.toInt(),
    );
  }
}

class DiffHunk {
  DiffHunk({required this.lines});

  final List<DiffLine> lines;

  factory DiffHunk.fromJson(Map<String, dynamic> json) {
    final linesJson = (json['lines'] as List<dynamic>? ?? [])
        .cast<Map<String, dynamic>>();
    return DiffHunk(lines: linesJson.map(DiffLine.fromJson).toList());
  }
}

class StructuredContentDiff {
  StructuredContentDiff({
    required this.baseline,
    required this.draft,
    required this.itemCount,
    required this.baselineMissing,
    required this.referenceSource,
    this.referenceWarning,
  });

  final Map<String, dynamic>? baseline;
  final Map<String, dynamic>? draft;
  final int itemCount;
  final bool baselineMissing;
  final String referenceSource;
  final String? referenceWarning;

  factory StructuredContentDiff.fromJson(
    Map<String, dynamic> json, {
    required String countKey,
  }) {
    Map<String, dynamic>? asMap(dynamic value) {
      if (value == null) {
        return null;
      }
      if (value is Map<String, dynamic>) {
        return value;
      }
      if (value is Map) {
        return value.map((key, value) => MapEntry(key.toString(), value));
      }
      return null;
    }

    return StructuredContentDiff(
      baseline: asMap(json['baseline']),
      draft: asMap(json['draft']),
      itemCount: (json[countKey] as num?)?.toInt() ?? 0,
      baselineMissing: json['baseline_missing'] == true,
      referenceSource:
          json['reference_source']?.toString() ??
          (json['baseline_missing'] == true ? 'none' : 'draft_start_snapshot'),
      referenceWarning: json['reference_warning']?.toString(),
    );
  }
}

class PptxPositionedSlide {
  PptxPositionedSlide({
    required this.width,
    required this.height,
    required this.shapes,
  });

  final double width;
  final double height;
  final List<PptxPositionedShape> shapes;

  bool get hasSlideSize => width > 0 && height > 0;
  bool get hasRenderableShapes => shapes.any((shape) => shape.hasBounds);
  bool get isRenderable => hasSlideSize && hasRenderableShapes;

  Set<String> get requestedFontFamilies {
    final fonts = <String>{};
    for (final shape in shapes) {
      for (final run in shape.runs) {
        final family = run.fontFamily.trim();
        if (family.isNotEmpty) {
          fonts.add(family);
        }
      }
    }
    return fonts;
  }

  static PptxPositionedSlide? fromPayload(Map<String, dynamic>? payload) {
    final root = _asStringKeyedMap(payload);
    if (root.isEmpty) {
      return null;
    }
    final positionedRoot = _positionedRoot(root);
    if (positionedRoot == null || positionedRoot.isEmpty) {
      return null;
    }
    final size = _asStringKeyedMap(positionedRoot['slide_size']);
    final width =
        _doubleValue(
          size['width'],
          fallback:
              positionedRoot['slide_width'] ??
              positionedRoot['canvas_width'] ??
              positionedRoot['width'] ??
              positionedRoot['w'],
        ) ??
        0.0;
    final height =
        _doubleValue(
          size['height'],
          fallback:
              positionedRoot['slide_height'] ??
              positionedRoot['canvas_height'] ??
              positionedRoot['height'] ??
              positionedRoot['h'],
        ) ??
        0.0;
    final shapesRaw =
        positionedRoot['positioned_shapes'] ??
        positionedRoot['shapes'] ??
        positionedRoot['items'];
    final shapes =
        _asStringKeyedMapList(shapesRaw)
            .map(PptxPositionedShape.fromJson)
            .where((shape) => shape.hasBounds)
            .toList()
          ..sort((a, b) {
            final z = a.zIndex.compareTo(b.zIndex);
            if (z != 0) {
              return z;
            }
            return a.index.compareTo(b.index);
          });
    List<PptxPositionedShape> normalizedShapes = shapes;
    if (width > 0 &&
        height > 0 &&
        normalizedShapes.isNotEmpty &&
        normalizedShapes.every(
          (shape) =>
              shape.x >= 0 &&
              shape.y >= 0 &&
              shape.width > 0 &&
              shape.height > 0 &&
              shape.x <= 1.5 &&
              shape.y <= 1.5 &&
              shape.width <= 1.5 &&
              shape.height <= 1.5,
        )) {
      normalizedShapes = normalizedShapes
          .map((shape) => shape.scaledBySlide(width, height))
          .toList();
    }
    if (width <= 0 || height <= 0) {
      if (normalizedShapes.isEmpty) {
        return null;
      }
      var inferredWidth = 0.0;
      var inferredHeight = 0.0;
      for (final shape in normalizedShapes) {
        final right = shape.x + shape.width;
        final bottom = shape.y + shape.height;
        if (right > inferredWidth) {
          inferredWidth = right;
        }
        if (bottom > inferredHeight) {
          inferredHeight = bottom;
        }
      }
      return PptxPositionedSlide(
        width: inferredWidth,
        height: inferredHeight,
        shapes: normalizedShapes,
      );
    }
    return PptxPositionedSlide(
      width: width,
      height: height,
      shapes: normalizedShapes,
    );
  }

  static Map<String, dynamic>? _positionedRoot(Map<String, dynamic> payload) {
    final positioned = _asStringKeyedMap(
      payload['positioned'] ?? payload['positioned_slide'],
    );
    if (positioned.isNotEmpty) {
      return positioned;
    }
    final hasMarker =
        payload['render_mode']?.toString().toLowerCase() == 'positioned' ||
        payload.containsKey('positioned_shapes') ||
        payload.containsKey('slide_size') ||
        payload.containsKey('coordinate_space') ||
        payload.containsKey('canvas_width') ||
        payload.containsKey('canvas_height');
    if (!hasMarker) {
      return null;
    }
    return payload;
  }
}

class PptxPositionedShape {
  PptxPositionedShape({
    required this.index,
    required this.zIndex,
    required this.kind,
    required this.name,
    required this.x,
    required this.y,
    required this.width,
    required this.height,
    required this.runs,
    required this.plainText,
    this.fillColor,
  });

  final int index;
  final int zIndex;
  final String kind;
  final String name;
  final double x;
  final double y;
  final double width;
  final double height;
  final List<PptxTextRun> runs;
  final String plainText;
  final String? fillColor;

  bool get hasBounds => width > 0 && height > 0;

  PptxPositionedShape scaledBySlide(double slideWidth, double slideHeight) {
    return PptxPositionedShape(
      index: index,
      zIndex: zIndex,
      kind: kind,
      name: name,
      x: x * slideWidth,
      y: y * slideHeight,
      width: width * slideWidth,
      height: height * slideHeight,
      runs: runs,
      plainText: plainText,
      fillColor: fillColor,
    );
  }

  factory PptxPositionedShape.fromJson(Map<String, dynamic> json) {
    final bounds = _asStringKeyedMap(json['bounds'] ?? json['frame']);
    final boundsUnit = bounds['unit']?.toString().trim().toLowerCase() ?? '';
    final absX = _doubleValue(json['x'], fallback: json['left']);
    final absY = _doubleValue(json['y'], fallback: json['top']);
    final absW = _doubleValue(json['w'], fallback: json['width']);
    final absH = _doubleValue(json['h'], fallback: json['height']);
    final preferAbsolute =
        boundsUnit == 'slide_ratio' &&
        absX != null &&
        absY != null &&
        absW != null &&
        absH != null;
    final index = _intValue(json['index']);
    final zIndex = _intValue(
      json['z_index'],
      fallback: json['z'] ?? json['order'] ?? index,
    );
    final x =
        (preferAbsolute
            ? absX
            : _doubleValue(bounds['x'], fallback: bounds['left'] ?? absX)) ??
        0.0;
    final y =
        (preferAbsolute
            ? absY
            : _doubleValue(bounds['y'], fallback: bounds['top'] ?? absY)) ??
        0.0;
    final width =
        (preferAbsolute
            ? absW
            : _doubleValue(bounds['width'], fallback: bounds['w'] ?? absW)) ??
        0.0;
    final height =
        (preferAbsolute
            ? absH
            : _doubleValue(bounds['height'], fallback: bounds['h'] ?? absH)) ??
        0.0;

    final runs = <PptxTextRun>[];
    for (final run in _asStringKeyedMapList(json['text_runs'])) {
      runs.add(PptxTextRun.fromJson(run));
    }
    for (final run in _asStringKeyedMapList(json['runs'])) {
      runs.add(PptxTextRun.fromJson(run));
    }
    for (final block in _asStringKeyedMapList(json['text_blocks'])) {
      final blockRuns = _asStringKeyedMapList(block['runs']);
      if (blockRuns.isEmpty) {
        final blockText = block['text']?.toString() ?? '';
        if (blockText.trim().isNotEmpty) {
          runs.add(
            PptxTextRun(
              text: blockText,
              fontFamily:
                  block['font_family']?.toString() ??
                  block['font_name']?.toString() ??
                  '',
              fontSizePt: _doubleValue(
                block['font_size_pt'],
                fallback: block['size_pt'] ?? block['font_size'],
              ),
              color:
                  block['font_color']?.toString() ?? block['color']?.toString(),
              bold: _boolValue(block['bold']),
              italic: _boolValue(block['italic']),
              underline: _boolValue(block['underline']),
            ),
          );
          continue;
        }
      }
      for (final run in blockRuns) {
        runs.add(PptxTextRun.fromJson(run));
      }
    }

    if (runs.isEmpty) {
      final text = json['text']?.toString() ?? '';
      if (text.trim().isNotEmpty) {
        runs.add(PptxTextRun(text: text));
      }
    }

    final plainText = runs.map((run) => run.text).join();
    return PptxPositionedShape(
      index: index,
      zIndex: zIndex,
      kind:
          json['shape_type']?.toString() ??
          json['kind']?.toString() ??
          json['type']?.toString() ??
          '',
      name: json['name']?.toString() ?? '',
      x: x,
      y: y,
      width: width,
      height: height,
      runs: runs,
      plainText: plainText,
      fillColor:
          json['fill_color']?.toString() ??
          json['background_color']?.toString() ??
          json['fill']?.toString(),
    );
  }
}

class PptxTextRun {
  PptxTextRun({
    required this.text,
    this.fontFamily = '',
    this.fontSizePt,
    this.color,
    this.bold,
    this.italic,
    this.underline,
  });

  final String text;
  final String fontFamily;
  final double? fontSizePt;
  final String? color;
  final bool? bold;
  final bool? italic;
  final bool? underline;

  factory PptxTextRun.fromJson(Map<String, dynamic> json) {
    return PptxTextRun(
      text: json['text']?.toString() ?? '',
      fontFamily:
          json['font_family']?.toString() ??
          json['font_name']?.toString() ??
          '',
      fontSizePt: _doubleValue(
        json['font_size_pt'],
        fallback: json['size_pt'] ?? json['font_size'],
      ),
      color: json['font_color']?.toString() ?? json['color']?.toString(),
      bold: _boolValue(json['bold']),
      italic: _boolValue(json['italic']),
      underline: _boolValue(json['underline']),
    );
  }
}

class FontResolution {
  FontResolution({
    required this.requestedFamily,
    required this.resolvedFamily,
    required this.source,
    required this.missing,
  });

  final String requestedFamily;
  final String resolvedFamily;
  final String source;
  final bool missing;
}

class ChatMessage {
  ChatMessage({
    required this.id,
    required this.role,
    required this.text,
    this.type = '',
    this.createdAt = '',
    this.event,
  });

  final String id;
  final String role;
  String text;
  final String type;
  final String createdAt;
  final Map<String, dynamic>? event;

  bool get isSystemEvent => type == 'system_event' || role == 'system';
  bool get hasRenderableText => text.trim().isNotEmpty;
  bool get isToolMessage => role == 'tool' || type == 'tool_result';
  bool get isUserOrAssistant => role == 'user' || role == 'assistant';

  bool get shouldRenderInConversation {
    if (isPublishCheckpointEvent || isRestoreCheckpointEvent) {
      return true;
    }
    if (isToolMessage || isSystemEvent) {
      return false;
    }
    if (!isUserOrAssistant) {
      return false;
    }
    return hasRenderableText;
  }

  String? get eventKind =>
      _eventString('kind') ??
      _eventString('event_kind') ??
      _eventString('event_type');

  String? get checkpointId => _eventString('checkpoint_id');
  String? get checkpointReason => _eventString('reason');
  String? get checkpointDescription => _eventString('description');
  String? get checkpointCreatedAt => _eventString('created_at') ?? createdAt;
  int? get jobElapsedMs => _eventInt('job_elapsed_ms');

  bool get isPublishCheckpointEvent {
    final reason = checkpointReason?.toLowerCase();
    if (reason == 'publish') {
      return true;
    }
    final kind = eventKind?.toLowerCase();
    return kind == 'checkpoint_publish' ||
        kind == 'publish_checkpoint' ||
        kind == 'checkpoint.published';
  }

  bool get isRestoreCheckpointEvent {
    final reason = checkpointReason?.toLowerCase();
    if (reason == 'restore') {
      return true;
    }
    final kind = eventKind?.toLowerCase();
    return kind == 'checkpoint_restore' || kind == 'checkpoint.restored';
  }

  String? _eventString(String key) {
    final value = event?[key];
    if (value == null) {
      return null;
    }
    final text = value.toString().trim();
    return text.isEmpty ? null : text;
  }

  int? _eventInt(String key) {
    final value = event?[key];
    if (value == null) {
      return null;
    }
    if (value is int) {
      return value;
    }
    if (value is num) {
      return value.toInt();
    }
    final text = value.toString().trim();
    if (text.isEmpty) {
      return null;
    }
    return int.tryParse(text);
  }

  factory ChatMessage.fromJson(Map<String, dynamic> json) {
    final event = _parseEvent(json);
    final role = json['role'] as String? ?? '';
    return ChatMessage(
      id: json['message_id'] as String? ?? '',
      role: role,
      text: json['text'] as String? ?? '',
      type: json['type'] as String? ?? (role == 'system' ? 'system_event' : ''),
      createdAt: json['created_at'] as String? ?? '',
      event: event.isEmpty ? null : event,
    );
  }

  static Map<String, dynamic> _parseEvent(Map<String, dynamic> json) {
    final event = <String, dynamic>{};
    event.addAll(_asMap(json['event']));
    event.addAll(_asMap(json['metadata']));

    const topLevelKeys = [
      'checkpoint_id',
      'reason',
      'description',
      'kind',
      'event_kind',
      'event_type',
      'created_at',
    ];
    for (final key in topLevelKeys) {
      final value = json[key];
      if (value != null) {
        event.putIfAbsent(key, () => value);
      }
    }
    return event;
  }

  static Map<String, dynamic> _asMap(dynamic value) {
    if (value is Map<String, dynamic>) {
      return value;
    }
    if (value is Map) {
      final mapped = <String, dynamic>{};
      for (final entry in value.entries) {
        mapped[entry.key.toString()] = entry.value;
      }
      return mapped;
    }
    return const {};
  }
}

Map<String, dynamic> _asStringKeyedMap(dynamic value) {
  if (value is Map<String, dynamic>) {
    return value;
  }
  if (value is Map) {
    final mapped = <String, dynamic>{};
    for (final entry in value.entries) {
      mapped[entry.key.toString()] = entry.value;
    }
    return mapped;
  }
  return const {};
}

String? _normalizedString(dynamic value) {
  if (value == null) {
    return null;
  }
  final text = value.toString().trim();
  return text.isEmpty ? null : text;
}

int _intValue(dynamic value, {dynamic fallback}) {
  int? asInt(dynamic raw) {
    if (raw is num) {
      return raw.toInt();
    }
    if (raw is String) {
      return int.tryParse(raw.trim());
    }
    return null;
  }

  return asInt(value) ?? asInt(fallback) ?? 0;
}

double? _doubleValue(dynamic value, {dynamic fallback}) {
  double? asDouble(dynamic raw) {
    if (raw is num) {
      return raw.toDouble();
    }
    if (raw is String) {
      return double.tryParse(raw.trim());
    }
    return null;
  }

  return asDouble(value) ?? asDouble(fallback);
}

bool? _boolValue(dynamic value) {
  if (value is bool) {
    return value;
  }
  if (value is String) {
    final normalized = value.trim().toLowerCase();
    if (normalized == 'true') {
      return true;
    }
    if (normalized == 'false') {
      return false;
    }
  }
  return null;
}

List<Map<String, dynamic>> _asStringKeyedMapList(dynamic value) {
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
