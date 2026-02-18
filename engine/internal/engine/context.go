package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"
	"unicode"

	"keenbench/engine/internal/errinfo"
	"keenbench/engine/internal/llm"
	"keenbench/engine/internal/workbench"
)

const (
	contextCategoryCompany       = "company-context"
	contextCategoryDepartment    = "department-context"
	contextCategorySituation     = "situation"
	contextCategoryDocumentStyle = "document-style"

	contextInputModeText = "text"
	contextInputModeFile = "file"

	contextSourceFileName = "source.json"
	contextSourceDirName  = "source_file"
)

var contextCategories = []string{
	contextCategoryCompany,
	contextCategoryDepartment,
	contextCategorySituation,
	contextCategoryDocumentStyle,
}

type contextSourceMetadata struct {
	Mode            string `json:"mode"`
	Text            string `json:"text,omitempty"`
	OriginalFile    string `json:"original_filename,omitempty"`
	Note            string `json:"note,omitempty"`
	CreatedAt       string `json:"created_at"`
	LastProcessedAt string `json:"last_processed_at"`
	LastDirectEdit  string `json:"last_direct_edit_at,omitempty"`
	ModelID         string `json:"model_id,omitempty"`
	HasDirectEdits  bool   `json:"has_direct_edits"`
}

type contextArtifactFile struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

type contextListItem struct {
	Category        string `json:"category"`
	Status          string `json:"status"`
	Summary         string `json:"summary,omitempty"`
	HasDirectEdits  bool   `json:"has_direct_edits"`
	CreatedAt       string `json:"created_at,omitempty"`
	LastProcessedAt string `json:"last_processed_at,omitempty"`
	LastDirectEdit  string `json:"last_direct_edit_at,omitempty"`
}

type contextItem struct {
	Category        string                 `json:"category"`
	Status          string                 `json:"status"`
	Summary         string                 `json:"summary,omitempty"`
	HasDirectEdits  bool                   `json:"has_direct_edits"`
	CreatedAt       string                 `json:"created_at,omitempty"`
	LastProcessedAt string                 `json:"last_processed_at,omitempty"`
	LastDirectEdit  string                 `json:"last_direct_edit_at,omitempty"`
	Source          *contextSourceMetadata `json:"source,omitempty"`
	Files           []contextArtifactFile  `json:"files,omitempty"`
}

type contextModelOutput struct {
	Summary string                `json:"summary,omitempty"`
	Files   []contextArtifactFile `json:"files"`
}

func (e *Engine) ContextList(ctx context.Context, params json.RawMessage) (any, *errinfo.ErrorInfo) {
	var req struct {
		WorkbenchID string `json:"workbench_id"`
	}
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, errinfo.ValidationFailed(errinfo.PhaseWorkbench, "invalid params")
	}
	if errInfo := e.ensureWorkbenchExists(req.WorkbenchID); errInfo != nil {
		return nil, errInfo
	}
	items := make([]contextListItem, 0, len(contextCategories))
	for _, category := range contextCategories {
		item, err := e.readContextItem(req.WorkbenchID, category, false)
		if err != nil {
			e.logger.Warn("context.list_read_failed", "workbench_id", req.WorkbenchID, "category", category, "error", err.Error())
			items = append(items, contextListItem{Category: category, Status: "empty"})
			continue
		}
		if item == nil {
			items = append(items, contextListItem{Category: category, Status: "empty"})
			continue
		}
		items = append(items, contextListItem{
			Category:        item.Category,
			Status:          item.Status,
			Summary:         item.Summary,
			HasDirectEdits:  item.HasDirectEdits,
			CreatedAt:       item.CreatedAt,
			LastProcessedAt: item.LastProcessedAt,
			LastDirectEdit:  item.LastDirectEdit,
		})
	}
	return map[string]any{"items": items}, nil
}

func (e *Engine) ContextGet(ctx context.Context, params json.RawMessage) (any, *errinfo.ErrorInfo) {
	var req struct {
		WorkbenchID string `json:"workbench_id"`
		Category    string `json:"category"`
	}
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, errinfo.ValidationFailed(errinfo.PhaseWorkbench, "invalid params")
	}
	if errInfo := e.ensureWorkbenchExists(req.WorkbenchID); errInfo != nil {
		return nil, errInfo
	}
	if errInfo := validateContextCategory(req.Category); errInfo != nil {
		return nil, errInfo
	}
	item, err := e.readContextItem(req.WorkbenchID, req.Category, true)
	if err != nil {
		return nil, errinfo.FileReadFailed(errinfo.PhaseWorkbench, err.Error())
	}
	if item == nil {
		return map[string]any{
			"item": contextItem{Category: req.Category, Status: "empty"},
		}, nil
	}
	return map[string]any{"item": item}, nil
}

func (e *Engine) ContextGetArtifact(ctx context.Context, params json.RawMessage) (any, *errinfo.ErrorInfo) {
	var req struct {
		WorkbenchID string `json:"workbench_id"`
		Category    string `json:"category"`
	}
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, errinfo.ValidationFailed(errinfo.PhaseWorkbench, "invalid params")
	}
	if errInfo := e.ensureWorkbenchExists(req.WorkbenchID); errInfo != nil {
		return nil, errInfo
	}
	if errInfo := validateContextCategory(req.Category); errInfo != nil {
		return nil, errInfo
	}
	item, err := e.readContextItem(req.WorkbenchID, req.Category, true)
	if err != nil {
		return nil, errinfo.FileReadFailed(errinfo.PhaseWorkbench, err.Error())
	}
	if item == nil {
		return map[string]any{
			"files":            []contextArtifactFile{},
			"has_direct_edits": false,
		}, nil
	}
	return map[string]any{
		"files":            item.Files,
		"has_direct_edits": item.HasDirectEdits,
	}, nil
}

func (e *Engine) ContextUpdateDirect(ctx context.Context, params json.RawMessage) (any, *errinfo.ErrorInfo) {
	var req struct {
		WorkbenchID string                `json:"workbench_id"`
		Category    string                `json:"category"`
		Files       []contextArtifactFile `json:"files"`
	}
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, errinfo.ValidationFailed(errinfo.PhaseWorkbench, "invalid params")
	}
	if errInfo := e.ensureWorkbenchExists(req.WorkbenchID); errInfo != nil {
		return nil, errInfo
	}
	if errInfo := validateContextCategory(req.Category); errInfo != nil {
		return nil, errInfo
	}
	if errInfo := e.ensureContextMutable(req.WorkbenchID); errInfo != nil {
		return nil, errInfo
	}
	if len(req.Files) == 0 {
		return nil, errinfo.ValidationFailed(errinfo.PhaseWorkbench, "files are required")
	}

	source, err := e.readContextSource(req.WorkbenchID, req.Category)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, errinfo.ValidationFailed(errinfo.PhaseWorkbench, "context item not found")
		}
		return nil, errinfo.FileReadFailed(errinfo.PhaseWorkbench, err.Error())
	}
	normalizedFiles, errInfo := normalizeContextArtifactFiles(req.Files)
	if errInfo != nil {
		return nil, errInfo
	}
	now := time.Now().UTC().Format(time.RFC3339)
	source.HasDirectEdits = true
	source.LastDirectEdit = now
	if source.LastProcessedAt == "" {
		source.LastProcessedAt = now
	}

	if err := e.writeContextCategory(req.WorkbenchID, req.Category, source, normalizedFiles, "", true); err != nil {
		return nil, errinfo.FileWriteFailed(errinfo.PhaseWorkbench, err.Error())
	}
	e.emitContextChanged(req.WorkbenchID, req.Category, "updated")
	e.emitClutterChanged(req.WorkbenchID)
	item, err := e.readContextItem(req.WorkbenchID, req.Category, true)
	if err != nil {
		return nil, errinfo.FileReadFailed(errinfo.PhaseWorkbench, err.Error())
	}
	return map[string]any{"item": item}, nil
}

func (e *Engine) ContextDelete(ctx context.Context, params json.RawMessage) (any, *errinfo.ErrorInfo) {
	var req struct {
		WorkbenchID string `json:"workbench_id"`
		Category    string `json:"category"`
	}
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, errinfo.ValidationFailed(errinfo.PhaseWorkbench, "invalid params")
	}
	if errInfo := e.ensureWorkbenchExists(req.WorkbenchID); errInfo != nil {
		return nil, errInfo
	}
	if errInfo := validateContextCategory(req.Category); errInfo != nil {
		return nil, errInfo
	}
	if errInfo := e.ensureContextMutable(req.WorkbenchID); errInfo != nil {
		return nil, errInfo
	}
	if err := os.RemoveAll(e.contextCategoryPath(req.WorkbenchID, req.Category)); err != nil {
		return nil, errinfo.FileWriteFailed(errinfo.PhaseWorkbench, err.Error())
	}
	e.emitContextChanged(req.WorkbenchID, req.Category, "deleted")
	e.emitClutterChanged(req.WorkbenchID)
	return map[string]any{}, nil
}

func (e *Engine) ContextProcess(ctx context.Context, params json.RawMessage) (any, *errinfo.ErrorInfo) {
	var req struct {
		WorkbenchID string `json:"workbench_id"`
		Category    string `json:"category"`
		Input       struct {
			Mode       string `json:"mode"`
			Text       string `json:"text"`
			SourcePath string `json:"source_path"`
			Note       string `json:"note"`
		} `json:"input"`
	}
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, errinfo.ValidationFailed(errinfo.PhaseWorkbench, "invalid params")
	}
	if errInfo := e.ensureWorkbenchExists(req.WorkbenchID); errInfo != nil {
		return nil, errInfo
	}
	if errInfo := validateContextCategory(req.Category); errInfo != nil {
		return nil, errInfo
	}
	if errInfo := e.ensureContextMutable(req.WorkbenchID); errInfo != nil {
		return nil, errInfo
	}

	modelID, errInfo := e.resolveActiveModel(req.WorkbenchID)
	if errInfo != nil {
		return nil, errInfo
	}
	model, ok := getModel(modelID)
	if !ok {
		return nil, errinfo.ValidationFailed(errinfo.PhaseWorkbench, "unsupported model")
	}
	if errInfo := e.ensureProviderReadyFor(ctx, model.ProviderID); errInfo != nil {
		return nil, errInfo
	}
	apiKey, errInfo := e.providerKey(ctx, model.ProviderID)
	if errInfo != nil {
		return nil, errInfo
	}
	client, errInfo := e.clientForProvider(model.ProviderID)
	if errInfo != nil {
		return nil, errInfo
	}

	rawText := ""
	sourcePath := ""
	sourceFilename := ""
	hadExistingContext := false
	if info, statErr := os.Stat(e.contextCategoryPath(req.WorkbenchID, req.Category)); statErr == nil && info.IsDir() {
		hadExistingContext = true
	}
	note := strings.TrimSpace(req.Input.Note)
	mode := strings.TrimSpace(req.Input.Mode)
	source, _ := e.readContextSource(req.WorkbenchID, req.Category)
	now := time.Now().UTC().Format(time.RFC3339)
	if source == nil {
		source = &contextSourceMetadata{CreatedAt: now}
	}
	if source.CreatedAt == "" {
		source.CreatedAt = now
	}
	if mode == "" {
		mode = contextInputModeText
	}
	if mode != contextInputModeText && mode != contextInputModeFile {
		return nil, errinfo.ValidationFailed(errinfo.PhaseWorkbench, "invalid input mode")
	}
	if mode == contextInputModeText {
		rawText = strings.TrimSpace(req.Input.Text)
		if rawText == "" {
			return nil, errinfo.ValidationFailed(errinfo.PhaseWorkbench, "text is required")
		}
		source.Text = rawText
		source.OriginalFile = ""
		source.Note = ""
	} else {
		extracted, filename, errInfo := e.extractContextInputText(ctx, req.WorkbenchID, req.Input.SourcePath)
		if errInfo != nil {
			return nil, errInfo
		}
		rawText = extracted
		sourcePath = req.Input.SourcePath
		sourceFilename = filename
		source.Text = ""
		source.OriginalFile = filename
		source.Note = note
	}

	processedFiles, summary, errInfo := e.processContextWithModel(ctx, client, apiKey, model, req.Category, rawText, source.Note, source.OriginalFile)
	if errInfo != nil {
		return nil, errInfo
	}

	source.Mode = mode
	source.LastProcessedAt = now
	source.LastDirectEdit = ""
	source.HasDirectEdits = false
	source.ModelID = modelID
	if mode == contextInputModeFile && sourceFilename == "" {
		sourceFilename = filepath.Base(sourcePath)
		source.OriginalFile = sourceFilename
	}
	if err := e.writeContextCategory(req.WorkbenchID, req.Category, source, processedFiles, sourcePath, false); err != nil {
		return nil, errinfo.FileWriteFailed(errinfo.PhaseWorkbench, err.Error())
	}

	changeAction := "added"
	if hadExistingContext {
		changeAction = "updated"
	}
	e.emitContextChanged(req.WorkbenchID, req.Category, changeAction)
	e.emitClutterChanged(req.WorkbenchID)
	item, err := e.readContextItem(req.WorkbenchID, req.Category, true)
	if err != nil {
		return nil, errinfo.FileReadFailed(errinfo.PhaseWorkbench, err.Error())
	}
	if item != nil && strings.TrimSpace(item.Summary) == "" {
		item.Summary = summary
	}
	return map[string]any{"item": item}, nil
}

func (e *Engine) processContextWithModel(
	ctx context.Context,
	client LLMClient,
	apiKey string,
	model ModelInfo,
	category string,
	rawInput string,
	note string,
	originalFilename string,
) ([]contextArtifactFile, string, *errinfo.ErrorInfo) {
	systemPrompt := contextProcessingSystemPrompt(category)
	userPrompt := contextProcessingUserPrompt(rawInput, note, originalFilename)

	messages := []llm.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userPrompt},
	}
	output, err := e.callContextModel(ctx, client, apiKey, model.ModelID, messages)
	if err != nil {
		return nil, "", mapLLMError(errinfo.PhaseWorkbench, model.ProviderID, err)
	}
	files, summary, validateErr := e.parseAndValidateContextModelOutput(category, output)
	if validateErr == nil {
		return files, summary, nil
	}

	repairPrompt := fmt.Sprintf(
		"The previous response was invalid for category %s. Errors:\n%s\n\nReturn corrected JSON only.",
		category,
		validateErr.Detail,
	)
	repairMessages := []llm.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userPrompt},
		{Role: "assistant", Content: output},
		{Role: "user", Content: repairPrompt},
	}
	repairOutput, err := e.callContextModel(ctx, client, apiKey, model.ModelID, repairMessages)
	if err != nil {
		return nil, "", mapLLMError(errinfo.PhaseWorkbench, model.ProviderID, err)
	}
	files, summary, validateErr = e.parseAndValidateContextModelOutput(category, repairOutput)
	if validateErr != nil {
		return nil, "", validateErr
	}
	return files, summary, nil
}

func (e *Engine) callContextModel(ctx context.Context, client LLMClient, apiKey, modelID string, messages []llm.Message) (string, error) {
	callCtx, cancel := context.WithTimeout(ctx, 600*time.Second)
	defer cancel()
	return client.Chat(callCtx, apiKey, providerModelName(modelID), messages)
}

func (e *Engine) parseAndValidateContextModelOutput(category, output string) ([]contextArtifactFile, string, *errinfo.ErrorInfo) {
	parsed, err := parseContextModelOutput(output)
	if err != nil {
		return nil, "", errinfo.ValidationFailed(errinfo.PhaseWorkbench, err.Error())
	}
	files, errInfo := normalizeContextArtifactFiles(parsed.Files)
	if errInfo != nil {
		return nil, "", errInfo
	}
	if err := validateContextArtifacts(category, files); err != nil {
		return nil, "", errinfo.ValidationFailed(errinfo.PhaseWorkbench, err.Error())
	}
	return files, strings.TrimSpace(parsed.Summary), nil
}

func parseContextModelOutput(output string) (*contextModelOutput, error) {
	candidates := contextModelOutputCandidates(output)
	if len(candidates) == 0 {
		return nil, errors.New("invalid processing output json")
	}

	parsedPayload := false
	for _, candidate := range candidates {
		parsed, ok := decodeContextModelOutputCandidate(candidate, 0)
		if !ok {
			continue
		}
		parsedPayload = true
		if len(parsed.Files) == 0 {
			continue
		}
		return parsed, nil
	}

	if parsedPayload {
		return nil, errors.New("processing output missing files")
	}
	return nil, errors.New("invalid processing output json")
}

func contextModelOutputCandidates(output string) []string {
	trimmed := strings.TrimSpace(output)
	if trimmed == "" {
		return nil
	}

	candidates := make([]string, 0, 4)
	seen := make(map[string]struct{}, 4)
	addCandidate := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		if _, exists := seen[value]; exists {
			return
		}
		seen[value] = struct{}{}
		candidates = append(candidates, value)
	}

	addCandidate(trimmed)
	for _, block := range extractJSONFenceBlocks(trimmed) {
		addCandidate(block)
	}
	if embedded := extractEmbeddedJSONValue(trimmed); embedded != "" {
		addCandidate(embedded)
	}

	// Keep a broad fallback for chatty responses where JSON appears between prose.
	if !json.Valid([]byte(trimmed)) {
		start := strings.Index(trimmed, "{")
		end := strings.LastIndex(trimmed, "}")
		if start >= 0 && end > start {
			addCandidate(trimmed[start : end+1])
		}
	}

	return candidates
}

func decodeContextModelOutputCandidate(candidate string, depth int) (*contextModelOutput, bool) {
	if depth > 4 {
		return nil, false
	}
	trimmed := strings.TrimSpace(candidate)
	if trimmed == "" || !json.Valid([]byte(trimmed)) {
		return nil, false
	}
	var raw json.RawMessage
	if err := json.Unmarshal([]byte(trimmed), &raw); err != nil {
		return nil, false
	}
	return decodeContextModelOutputRaw(raw, depth)
}

func decodeContextModelOutputRaw(raw json.RawMessage, depth int) (*contextModelOutput, bool) {
	if depth > 4 {
		return nil, false
	}
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" {
		return nil, false
	}

	switch trimmed[0] {
	case '{':
		var object map[string]json.RawMessage
		if err := json.Unmarshal([]byte(trimmed), &object); err != nil {
			return nil, false
		}
		if _, hasFiles := object["files"]; hasFiles {
			var parsed contextModelOutput
			if err := json.Unmarshal([]byte(trimmed), &parsed); err != nil {
				return nil, false
			}
			return &parsed, true
		}
		if _, hasSummary := object["summary"]; hasSummary {
			var parsed contextModelOutput
			if err := json.Unmarshal([]byte(trimmed), &parsed); err != nil {
				return nil, false
			}
			return &parsed, true
		}
		for _, key := range []string{"result", "output", "data", "response", "content", "json"} {
			inner, ok := object[key]
			if !ok {
				continue
			}
			if parsed, ok := decodeContextModelOutputRaw(inner, depth+1); ok {
				return parsed, true
			}
		}
	case '[':
		var entries []json.RawMessage
		if err := json.Unmarshal([]byte(trimmed), &entries); err != nil {
			return nil, false
		}
		for _, entry := range entries {
			if parsed, ok := decodeContextModelOutputRaw(entry, depth+1); ok {
				return parsed, true
			}
		}
	case '"':
		var inner string
		if err := json.Unmarshal([]byte(trimmed), &inner); err != nil {
			return nil, false
		}
		return decodeContextModelOutputCandidate(inner, depth+1)
	}

	return nil, false
}

func extractJSONFenceBlocks(output string) []string {
	blocks := make([]string, 0, 2)
	for len(output) > 0 {
		start := strings.Index(output, "```")
		if start < 0 {
			break
		}
		remaining := output[start+3:]
		newline := strings.Index(remaining, "\n")
		if newline < 0 {
			break
		}
		lang := strings.TrimSpace(remaining[:newline])
		contentAndTail := remaining[newline+1:]
		end := strings.Index(contentAndTail, "```")
		if end < 0 {
			break
		}
		if lang == "" || strings.EqualFold(lang, "json") {
			block := strings.TrimSpace(contentAndTail[:end])
			if block != "" {
				blocks = append(blocks, block)
			}
		}
		output = contentAndTail[end+3:]
	}
	return blocks
}

func extractEmbeddedJSONValue(output string) string {
	for index := 0; index < len(output); index++ {
		if output[index] != '{' && output[index] != '[' {
			continue
		}
		end := findBalancedJSONValueEnd(output[index:])
		if end <= 0 {
			continue
		}
		candidate := strings.TrimSpace(output[index : index+end])
		if candidate != "" {
			return candidate
		}
	}
	return ""
}

func findBalancedJSONValueEnd(input string) int {
	if len(input) == 0 {
		return -1
	}
	if input[0] != '{' && input[0] != '[' {
		return -1
	}

	stack := []byte{input[0]}
	inString := false
	escaped := false
	for index := 1; index < len(input); index++ {
		ch := input[index]
		if inString {
			if escaped {
				escaped = false
				continue
			}
			switch ch {
			case '\\':
				escaped = true
			case '"':
				inString = false
			}
			continue
		}

		switch ch {
		case '"':
			inString = true
		case '{', '[':
			stack = append(stack, ch)
		case '}':
			if len(stack) == 0 || stack[len(stack)-1] != '{' {
				return -1
			}
			stack = stack[:len(stack)-1]
		case ']':
			if len(stack) == 0 || stack[len(stack)-1] != '[' {
				return -1
			}
			stack = stack[:len(stack)-1]
		}

		if len(stack) == 0 {
			return index + 1
		}
	}
	return -1
}

func contextProcessingSystemPrompt(category string) string {
	common := `You are a context processor for KeenBench.
Return a single JSON object only (no markdown code fences).
JSON schema:
{"summary":"short summary","files":[{"path":"relative/path","content":"file content"}]}
Rules:
- paths must be relative and inside the artifact root.
- never include source.json or source_file paths.
- include complete file contents in each content value.
- for skill categories, SKILL.md must have YAML frontmatter with required fields name and description.`

	switch category {
	case contextCategoryCompany:
		return common + `
Category: company-context.
Required files:
- SKILL.md
- references/summary.md
SKILL requirements:
- frontmatter name must be exactly "company-context"
- description describes what the skill does and when to use it
Tone: suggestive (consider, keep in mind, be aware of).`
	case contextCategoryDepartment:
		return common + `
Category: department-context.
Required files:
- SKILL.md
- references/summary.md
SKILL requirements:
- frontmatter name must be exactly "department-context"
- description describes what the skill does and when to use it
Tone: imperative (must, always, ensure alignment).`
	case contextCategoryDocumentStyle:
		return common + `
Category: document-style.
Required files:
- SKILL.md
- references/style-rules.md
SKILL requirements:
- frontmatter name must be exactly "document-style"
- description should scope use to creating/editing files
Tone: prescriptive with explicit rules and examples.`
	default:
		return common + `
Category: situation.
Required files:
- context.md
No SKILL.md for this category.
Format context.md as concise structured bullets/sections.`
	}
}

func contextProcessingUserPrompt(rawInput, note, originalFilename string) string {
	var builder strings.Builder
	builder.WriteString("Source material:\n")
	if originalFilename != "" {
		builder.WriteString("Source file: ")
		builder.WriteString(originalFilename)
		builder.WriteString("\n")
	}
	if strings.TrimSpace(note) != "" {
		builder.WriteString("Guidance note: ")
		builder.WriteString(strings.TrimSpace(note))
		builder.WriteString("\n")
	}
	builder.WriteString("\nRaw content:\n")
	builder.WriteString(rawInput)
	return builder.String()
}

func validateContextArtifacts(category string, files []contextArtifactFile) error {
	filesByPath := make(map[string]string, len(files))
	for _, file := range files {
		if strings.TrimSpace(file.Content) == "" {
			return fmt.Errorf("file %s is empty", file.Path)
		}
		filesByPath[file.Path] = file.Content
	}

	switch category {
	case contextCategoryCompany, contextCategoryDepartment:
		if _, ok := filesByPath["SKILL.md"]; !ok {
			return errors.New("SKILL.md is required")
		}
		if _, ok := filesByPath["references/summary.md"]; !ok {
			return errors.New("references/summary.md is required")
		}
	case contextCategoryDocumentStyle:
		if _, ok := filesByPath["SKILL.md"]; !ok {
			return errors.New("SKILL.md is required")
		}
		if _, ok := filesByPath["references/style-rules.md"]; !ok {
			return errors.New("references/style-rules.md is required")
		}
	case contextCategorySituation:
		if _, ok := filesByPath["context.md"]; !ok {
			return errors.New("context.md is required")
		}
	}

	if category == contextCategorySituation {
		if strings.TrimSpace(filesByPath["context.md"]) == "" {
			return errors.New("context.md is empty")
		}
		return nil
	}

	skillContent, ok := filesByPath["SKILL.md"]
	if !ok {
		return errors.New("SKILL.md is required")
	}
	name, description, body, links, err := parseSkillFrontmatter(skillContent)
	if err != nil {
		return err
	}
	if err := validateSkillName(name, category); err != nil {
		return err
	}
	desc := strings.TrimSpace(description)
	if desc == "" {
		return errors.New("skill description is required")
	}
	if len(desc) > 1024 {
		return errors.New("skill description exceeds 1024 characters")
	}
	if strings.TrimSpace(body) == "" {
		return errors.New("skill body is empty")
	}
	for _, link := range links {
		if strings.HasPrefix(link, "http://") || strings.HasPrefix(link, "https://") || strings.HasPrefix(link, "#") || strings.HasPrefix(link, "mailto:") {
			continue
		}
		clean := path.Clean(link)
		if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") || strings.HasPrefix(clean, "/") {
			return fmt.Errorf("invalid referenced path in SKILL.md: %s", link)
		}
		if _, ok := filesByPath[clean]; !ok {
			return fmt.Errorf("referenced file missing: %s", clean)
		}
	}
	return nil
}

func validateSkillName(name, expected string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("skill name is required")
	}
	runes := []rune(name)
	if len(runes) < 1 || len(runes) > 64 {
		return errors.New("skill name must be 1-64 characters")
	}
	if runes[0] == '-' || runes[len(runes)-1] == '-' {
		return errors.New("skill name cannot start or end with hyphen")
	}
	for i, r := range runes {
		if r == '-' {
			if i > 0 && runes[i-1] == '-' {
				return errors.New("skill name cannot contain consecutive hyphens")
			}
			continue
		}
		if unicode.IsDigit(r) {
			continue
		}
		if unicode.IsLetter(r) && unicode.IsLower(r) {
			continue
		}
		return errors.New("skill name must be lowercase alphanumeric with hyphens")
	}
	if name != expected {
		return fmt.Errorf("skill name must match category (%s)", expected)
	}
	return nil
}

var markdownLinkPattern = regexp.MustCompile(`\[[^\]]+\]\(([^)]+)\)`)

func parseSkillFrontmatter(content string) (string, string, string, []string, error) {
	trimmed := strings.TrimLeft(content, "\ufeff")
	if !strings.HasPrefix(trimmed, "---\n") && !strings.HasPrefix(trimmed, "---\r\n") {
		return "", "", "", nil, errors.New("SKILL.md must start with YAML frontmatter")
	}
	lines := strings.Split(strings.ReplaceAll(trimmed, "\r\n", "\n"), "\n")
	if len(lines) < 3 || strings.TrimSpace(lines[0]) != "---" {
		return "", "", "", nil, errors.New("SKILL.md frontmatter is invalid")
	}
	end := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			end = i
			break
		}
	}
	if end <= 0 {
		return "", "", "", nil, errors.New("SKILL.md frontmatter terminator missing")
	}
	frontmatter := lines[1:end]
	body := strings.Join(lines[end+1:], "\n")

	name := ""
	description := ""
	for i := 0; i < len(frontmatter); i++ {
		line := strings.TrimSpace(frontmatter[i])
		if strings.HasPrefix(line, "name:") {
			name = strings.Trim(strings.TrimSpace(strings.TrimPrefix(line, "name:")), `"'`)
		}
		if strings.HasPrefix(line, "description:") {
			rest := strings.TrimSpace(strings.TrimPrefix(line, "description:"))
			if rest == "" || rest == ">" || rest == "|" {
				var descLines []string
				for j := i + 1; j < len(frontmatter); j++ {
					next := frontmatter[j]
					if strings.TrimSpace(next) == "" {
						descLines = append(descLines, "")
						continue
					}
					if !strings.HasPrefix(next, " ") && !strings.HasPrefix(next, "\t") {
						break
					}
					descLines = append(descLines, strings.TrimSpace(next))
					i = j
				}
				description = strings.TrimSpace(strings.Join(descLines, " "))
			} else {
				description = strings.Trim(rest, `"'`)
			}
		}
	}

	matches := markdownLinkPattern.FindAllStringSubmatch(body, -1)
	links := make([]string, 0, len(matches))
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		links = append(links, strings.TrimSpace(match[1]))
	}
	return name, description, body, links, nil
}

func normalizeContextArtifactFiles(files []contextArtifactFile) ([]contextArtifactFile, *errinfo.ErrorInfo) {
	normalized := make([]contextArtifactFile, 0, len(files))
	seen := make(map[string]bool, len(files))
	for _, file := range files {
		p, err := normalizeContextArtifactPath(file.Path)
		if err != nil {
			return nil, errinfo.ValidationFailed(errinfo.PhaseWorkbench, err.Error())
		}
		if seen[p] {
			return nil, errinfo.ValidationFailed(errinfo.PhaseWorkbench, "duplicate artifact path: "+p)
		}
		seen[p] = true
		normalized = append(normalized, contextArtifactFile{
			Path:    p,
			Content: file.Content,
		})
	}
	sort.Slice(normalized, func(i, j int) bool {
		return normalized[i].Path < normalized[j].Path
	})
	return normalized, nil
}

func normalizeContextArtifactPath(p string) (string, error) {
	trimmed := strings.TrimSpace(strings.ReplaceAll(p, "\\", "/"))
	if trimmed == "" {
		return "", errors.New("artifact path is required")
	}
	if strings.HasPrefix(trimmed, "/") {
		return "", errors.New("artifact path must be relative")
	}
	clean := path.Clean(trimmed)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", errors.New("invalid artifact path")
	}
	if clean == contextSourceFileName || strings.HasPrefix(clean, contextSourceDirName+"/") {
		return "", errors.New("artifact path is reserved")
	}
	return clean, nil
}

func (e *Engine) extractContextInputText(ctx context.Context, workbenchID, sourcePath string) (string, string, *errinfo.ErrorInfo) {
	sourcePath = strings.TrimSpace(sourcePath)
	if sourcePath == "" {
		return "", "", errinfo.ValidationFailed(errinfo.PhaseWorkbench, "source_path is required")
	}
	info, err := os.Lstat(sourcePath)
	if err != nil {
		return "", "", errinfo.FileReadFailed(errinfo.PhaseWorkbench, err.Error())
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return "", "", errinfo.ValidationFailed(errinfo.PhaseWorkbench, "symlink source files are not allowed")
	}
	if info.IsDir() {
		return "", "", errinfo.ValidationFailed(errinfo.PhaseWorkbench, "source_path must be a file")
	}
	if info.Size() > workbenchMaxFileBytes {
		return "", "", errinfo.ValidationFailed(errinfo.PhaseWorkbench, "file exceeds 25MB limit")
	}
	filename := filepath.Base(sourcePath)
	kind, opaque := workbench.FileKindForPath(filename)
	if opaque || kind == workbench.FileKindBinary {
		return "", "", errinfo.ValidationFailed(errinfo.PhaseWorkbench, "this file type is not supported for context input")
	}
	if kind == workbench.FileKindImage {
		return "", "", errinfo.ValidationFailed(errinfo.PhaseWorkbench, "image files cannot be used as context input")
	}
	if kind == workbench.FileKindText {
		data, err := os.ReadFile(sourcePath)
		if err != nil {
			return "", "", errinfo.FileReadFailed(errinfo.PhaseWorkbench, err.Error())
		}
		return string(data), filename, nil
	}

	if e.toolWorker == nil {
		return "", "", errinfo.ToolWorkerUnavailable(errinfo.PhaseWorkbench, "tool worker unavailable")
	}
	stagedDir, err := os.MkdirTemp(filepath.Join(e.workbenchesRoot(), workbenchID), "contexttmp-")
	if err != nil {
		return "", "", errinfo.FileWriteFailed(errinfo.PhaseWorkbench, err.Error())
	}
	defer os.RemoveAll(stagedDir)
	stagedRoot := filepath.Base(stagedDir)
	stagedName := fmt.Sprintf("%d_%s", time.Now().UnixNano(), sanitizeContextFilename(filename))
	stagedPath := filepath.Join(stagedDir, stagedName)
	if err := copyContextFile(sourcePath, stagedPath); err != nil {
		return "", "", errinfo.FileWriteFailed(errinfo.PhaseWorkbench, err.Error())
	}

	text, err := e.extractText(ctx, workbenchID, stagedRoot, kind, stagedName)
	if err != nil {
		if mapped := mapToolWorkerError(errinfo.PhaseWorkbench, err); mapped != nil {
			return "", "", mapped
		}
		return "", "", errinfo.FileReadFailed(errinfo.PhaseWorkbench, err.Error())
	}
	return text, filename, nil
}

func sanitizeContextFilename(name string) string {
	name = filepath.Base(strings.TrimSpace(name))
	if name == "" {
		return "source.txt"
	}
	name = strings.ReplaceAll(name, " ", "_")
	name = strings.ReplaceAll(name, "/", "_")
	name = strings.ReplaceAll(name, "\\", "_")
	return name
}

func (e *Engine) ensureContextMutable(workbenchID string) *errinfo.ErrorInfo {
	draft, err := e.workbenches.DraftState(workbenchID)
	if err != nil {
		return errinfo.FileReadFailed(errinfo.PhaseWorkbench, err.Error())
	}
	if draft != nil {
		return errinfo.ValidationFailed(errinfo.PhaseWorkbench, "draft exists; review or discard before continuing")
	}
	return nil
}

func validateContextCategory(category string) *errinfo.ErrorInfo {
	for _, value := range contextCategories {
		if category == value {
			return nil
		}
	}
	return errinfo.ValidationFailed(errinfo.PhaseWorkbench, "invalid context category")
}

func (e *Engine) ensureWorkbenchExists(workbenchID string) *errinfo.ErrorInfo {
	if strings.TrimSpace(workbenchID) == "" {
		return errinfo.ValidationFailed(errinfo.PhaseWorkbench, "invalid workbench id")
	}
	if _, err := e.workbenches.Open(workbenchID); err != nil {
		if errors.Is(err, workbench.ErrInvalidPath) || strings.Contains(err.Error(), "invalid workbench id") {
			return errinfo.ValidationFailed(errinfo.PhaseWorkbench, err.Error())
		}
		if os.IsNotExist(err) {
			return errinfo.ValidationFailed(errinfo.PhaseWorkbench, "workbench not found")
		}
		return errinfo.FileReadFailed(errinfo.PhaseWorkbench, err.Error())
	}
	return nil
}

func (e *Engine) contextRootPath(workbenchID string) string {
	return filepath.Join(e.workbenchesRoot(), workbenchID, "meta", "context")
}

func (e *Engine) contextCategoryPath(workbenchID, category string) string {
	return filepath.Join(e.contextRootPath(workbenchID), category)
}

func (e *Engine) contextSourcePath(workbenchID, category string) string {
	return filepath.Join(e.contextCategoryPath(workbenchID, category), contextSourceFileName)
}

func (e *Engine) contextSourceDirPath(workbenchID, category string) string {
	return filepath.Join(e.contextCategoryPath(workbenchID, category), contextSourceDirName)
}

func (e *Engine) readContextSource(workbenchID, category string) (*contextSourceMetadata, error) {
	var source contextSourceMetadata
	if err := readJSON(e.contextSourcePath(workbenchID, category), &source); err != nil {
		return nil, err
	}
	return &source, nil
}

func (e *Engine) readContextItem(workbenchID, category string, includeFiles bool) (*contextItem, error) {
	categoryDir := e.contextCategoryPath(workbenchID, category)
	if _, err := os.Stat(categoryDir); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	source, err := e.readContextSource(workbenchID, category)
	if err != nil {
		return nil, err
	}
	item := &contextItem{
		Category:        category,
		Status:          "active",
		HasDirectEdits:  source.HasDirectEdits,
		CreatedAt:       source.CreatedAt,
		LastProcessedAt: source.LastProcessedAt,
		LastDirectEdit:  source.LastDirectEdit,
		Source:          source,
	}
	files, err := e.contextArtifactFiles(workbenchID, category)
	if err != nil {
		return nil, err
	}
	item.Summary = contextSummaryFromArtifacts(category, files)
	if includeFiles {
		item.Files = files
	}
	return item, nil
}

func (e *Engine) contextArtifactFiles(workbenchID, category string) ([]contextArtifactFile, error) {
	root := e.contextCategoryPath(workbenchID, category)
	entries := []contextArtifactFile{}
	err := filepath.WalkDir(root, func(current string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(root, current)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if rel == "." {
			return nil
		}
		if rel == contextSourceFileName {
			return nil
		}
		if rel == contextSourceDirName || strings.HasPrefix(rel, contextSourceDirName+"/") {
			if d.IsDir() {
				return nil
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}
		data, err := os.ReadFile(current)
		if err != nil {
			return err
		}
		entries = append(entries, contextArtifactFile{Path: rel, Content: string(data)})
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Path < entries[j].Path
	})
	return entries, nil
}

func contextSummaryFromArtifacts(category string, files []contextArtifactFile) string {
	if category == contextCategorySituation {
		for _, file := range files {
			if file.Path == "context.md" {
				return summarizeContextText(file.Content, 100)
			}
		}
		return ""
	}
	for _, file := range files {
		if file.Path != "SKILL.md" {
			continue
		}
		_, description, body, _, err := parseSkillFrontmatter(file.Content)
		if err == nil && strings.TrimSpace(description) != "" {
			return strings.TrimSpace(description)
		}
		return summarizeContextText(body, 100)
	}
	return ""
}

func summarizeContextText(text string, limit int) string {
	normalized := strings.Join(strings.Fields(strings.TrimSpace(text)), " ")
	if normalized == "" {
		return ""
	}
	runes := []rune(normalized)
	if len(runes) <= limit || limit <= 0 {
		return normalized
	}
	return string(runes[:limit])
}

func (e *Engine) writeContextCategory(
	workbenchID string,
	category string,
	source *contextSourceMetadata,
	files []contextArtifactFile,
	sourceFilePath string,
	copyExistingSourceFile bool,
) error {
	contextRoot := e.contextRootPath(workbenchID)
	if err := os.MkdirAll(contextRoot, 0o755); err != nil {
		return err
	}
	tmpDir, err := os.MkdirTemp(contextRoot, "."+category+".tmp-")
	if err != nil {
		return err
	}
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.RemoveAll(tmpDir)
		}
	}()

	for _, file := range files {
		rel := filepath.FromSlash(file.Path)
		fullPath := filepath.Join(tmpDir, rel)
		if err := ensurePathWithinRoot(tmpDir, fullPath); err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(fullPath, []byte(file.Content), 0o600); err != nil {
			return err
		}
	}
	if err := writeJSON(filepath.Join(tmpDir, contextSourceFileName), source); err != nil {
		return err
	}

	if strings.TrimSpace(sourceFilePath) != "" {
		sourceDestDir := filepath.Join(tmpDir, contextSourceDirName)
		if err := os.MkdirAll(sourceDestDir, 0o755); err != nil {
			return err
		}
		filename := sanitizeContextFilename(filepath.Base(sourceFilePath))
		if err := copyContextFile(sourceFilePath, filepath.Join(sourceDestDir, filename)); err != nil {
			return err
		}
	} else if copyExistingSourceFile {
		existingSourceDir := e.contextSourceDirPath(workbenchID, category)
		if info, err := os.Stat(existingSourceDir); err == nil && info.IsDir() {
			if err := copyContextDir(existingSourceDir, filepath.Join(tmpDir, contextSourceDirName)); err != nil {
				return err
			}
		}
	}

	destination := e.contextCategoryPath(workbenchID, category)
	backupDir := ""
	if info, statErr := os.Stat(destination); statErr == nil && info.IsDir() {
		backupDir = filepath.Join(contextRoot, fmt.Sprintf(".%s.bak-%d", category, time.Now().UnixNano()))
		if err := os.Rename(destination, backupDir); err != nil {
			return err
		}
	} else if statErr != nil && !os.IsNotExist(statErr) {
		return statErr
	}

	if err := os.Rename(tmpDir, destination); err != nil {
		if backupDir != "" {
			_ = os.Rename(backupDir, destination)
		}
		return err
	}
	cleanup = false
	if backupDir != "" {
		_ = os.RemoveAll(backupDir)
	}
	return nil
}

func ensurePathWithinRoot(root, fullPath string) error {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	fullAbs, err := filepath.Abs(fullPath)
	if err != nil {
		return err
	}
	if fullAbs == rootAbs {
		return errors.New("invalid artifact path")
	}
	if !strings.HasPrefix(fullAbs, rootAbs+string(filepath.Separator)) {
		return errors.New("artifact path escapes context directory")
	}
	return nil
}

func copyContextFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	return nil
}

func copyContextDir(src, dst string) error {
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())
		if entry.IsDir() {
			if err := copyContextDir(srcPath, dstPath); err != nil {
				return err
			}
			continue
		}
		if err := copyContextFile(srcPath, dstPath); err != nil {
			return err
		}
	}
	return nil
}

func (e *Engine) emitContextChanged(workbenchID, category, action string) {
	if e.notify == nil {
		return
	}
	e.notify("ContextChanged", map[string]any{
		"workbench_id": workbenchID,
		"category":     category,
		"action":       action,
	})
}

type contextSkillInjection struct {
	Name  string
	Files []contextArtifactFile
}

type markdownLevel2Section struct {
	Heading string
	Content string
	Raw     string
}

var formatStyleKeywords = map[string][]string{
	"xlsx": {"spreadsheet", "excel", "workbook", "worksheet"},
	"docx": {"docx", "word document", "word doc", "ms word"},
	"pptx": {"pptx", "powerpoint", "presentation", "slide deck", "slides"},
}

var mergeStopWords = map[string]struct{}{
	"about": {}, "after": {}, "also": {}, "always": {}, "an": {}, "and": {}, "are": {}, "because": {},
	"before": {}, "between": {}, "but": {}, "for": {}, "from": {}, "have": {}, "into": {}, "must": {},
	"only": {}, "should": {}, "that": {}, "the": {}, "their": {}, "there": {}, "these": {}, "this": {},
	"use": {}, "when": {}, "with": {}, "your": {},
}

func (e *Engine) buildWorkbenchContextInjection(workbenchID string) (string, float64) {
	situation := e.loadSituationContext(workbenchID)
	skills := make([]contextSkillInjection, 0, 6)
	for _, category := range []string{contextCategoryCompany, contextCategoryDepartment} {
		if skill, ok := e.loadContextSkillInjection(workbenchID, category); ok {
			skills = append(skills, skill)
		}
	}
	documentStyleSkill, hasDocumentStyle := e.loadContextSkillInjection(workbenchID, contextCategoryDocumentStyle)
	formatSkillInjections := e.buildFormatSkillInjections(workbenchID, hasDocumentStyle, documentStyleSkill.Files)
	if hasDocumentStyle && len(formatSkillInjections) == 0 {
		skills = append(skills, documentStyleSkill)
	}
	skills = append(skills, formatSkillInjections...)

	var builder strings.Builder
	if situation != "" {
		builder.WriteString("<workbench-situation>\n")
		builder.WriteString(situation)
		builder.WriteString("\n</workbench-situation>\n")
	}
	for _, skill := range skills {
		writeWorkbenchSkillInjection(&builder, skill)
	}
	content := strings.TrimSpace(builder.String())
	if content == "" {
		return "", 0
	}
	return content, float64(len(content)) / 4.0
}

func (e *Engine) loadSituationContext(workbenchID string) string {
	files, err := e.contextArtifactFiles(workbenchID, contextCategorySituation)
	if err != nil {
		if !os.IsNotExist(err) {
			e.logger.Warn("context.load_situation_failed", "workbench_id", workbenchID, "error", err.Error())
		}
		return ""
	}
	for _, file := range files {
		if file.Path == "context.md" {
			return strings.TrimSpace(file.Content)
		}
	}
	return ""
}

func (e *Engine) loadContextSkillInjection(workbenchID, category string) (contextSkillInjection, bool) {
	files, err := e.contextArtifactFiles(workbenchID, category)
	if err != nil {
		if !os.IsNotExist(err) {
			e.logger.Warn("context.load_skill_failed", "workbench_id", workbenchID, "category", category, "error", err.Error())
		}
		return contextSkillInjection{}, false
	}
	for _, file := range files {
		if file.Path == "SKILL.md" && strings.TrimSpace(file.Content) != "" {
			return contextSkillInjection{Name: category, Files: files}, true
		}
	}
	return contextSkillInjection{}, false
}

func writeWorkbenchSkillInjection(builder *strings.Builder, skill contextSkillInjection) {
	builder.WriteString(fmt.Sprintf("<workbench-skill name=\"%s\">\n", skill.Name))
	for _, file := range skill.Files {
		if file.Path != "SKILL.md" {
			continue
		}
		builder.WriteString("## SKILL.md\n")
		builder.WriteString(file.Content)
		builder.WriteString("\n")
	}
	for _, file := range skill.Files {
		if file.Path == "SKILL.md" {
			continue
		}
		builder.WriteString("\n## ")
		builder.WriteString(file.Path)
		builder.WriteString("\n")
		builder.WriteString(file.Content)
		builder.WriteString("\n")
	}
	builder.WriteString("</workbench-skill>\n")
}

func (e *Engine) buildFormatSkillInjections(workbenchID string, hasDocumentStyle bool, documentStyleFiles []contextArtifactFile) []contextSkillInjection {
	relevant := e.detectRelevantStyleFormats(workbenchID)
	injections := make([]contextSkillInjection, 0, 3)
	for _, format := range []string{"xlsx", "docx", "pptx"} {
		if !relevant[format] {
			continue
		}
		genericSkill, err := loadBundledFormatStyleSkill(format)
		if err != nil {
			e.logger.Warn(
				"context.style_skill_load_failed",
				"error_code",
				errinfo.CodeStyleSkillLoadFailed,
				"workbench_id",
				workbenchID,
				"format",
				format,
				"error",
				err.Error(),
			)
			e.appendStyleGuidanceNotice(
				workbenchID,
				errinfo.CodeStyleSkillLoadFailed,
				format,
				fmt.Sprintf("Style guidance could not be loaded for %s. Default formatting knowledge is being used.", strings.ToUpper(format)),
			)
			continue
		}
		if hasDocumentStyle {
			merged, mergeErr := mergeDocumentStyleWithFormatSkill(format, genericSkill, documentStyleFiles)
			if mergeErr != nil {
				e.logger.Warn(
					"context.style_merge_failed",
					"error_code",
					errinfo.CodeStyleMergeFailed,
					"workbench_id",
					workbenchID,
					"format",
					format,
					"error",
					mergeErr.Error(),
				)
				e.appendStyleGuidanceNotice(
					workbenchID,
					errinfo.CodeStyleMergeFailed,
					format,
					fmt.Sprintf("Your Document Style could not be merged with %s style guidance. Default formatting guidance is being used.", strings.ToUpper(format)),
				)
				injections = append(injections, genericSkill)
				continue
			}
			injections = append(injections, merged)
			continue
		}
		injections = append(injections, genericSkill)
	}
	return injections
}

func (e *Engine) appendStyleGuidanceNotice(workbenchID, code, format, message string) {
	code = strings.TrimSpace(code)
	format = strings.TrimSpace(strings.ToLower(format))
	message = strings.TrimSpace(message)
	if code == "" || format == "" || message == "" {
		return
	}

	if items, err := e.readConversation(workbenchID); err == nil {
		for idx := len(items) - 1; idx >= 0 && idx >= len(items)-20; idx-- {
			item := items[idx]
			if item.Type != "system_event" || item.EventKind != "style_notice" {
				continue
			}
			if item.Metadata == nil {
				continue
			}
			if strings.TrimSpace(stringValue(item.Metadata["error_code"])) != code {
				continue
			}
			if strings.TrimSpace(strings.ToLower(stringValue(item.Metadata["format"]))) != format {
				continue
			}
			if strings.TrimSpace(item.Text) == message {
				return
			}
		}
	}

	entry := conversationMessage{
		Type:      "system_event",
		MessageID: fmt.Sprintf("s-style-%d", time.Now().UnixNano()),
		Role:      "system",
		Text:      message,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
		EventKind: "style_notice",
		Metadata: map[string]any{
			"error_code": code,
			"format":     format,
		},
	}
	if err := e.appendConversation(workbenchID, entry); err != nil {
		e.logger.Warn("context.style_notice_append_failed", "workbench_id", workbenchID, "format", format, "error", err.Error())
	}
}

func (e *Engine) detectRelevantStyleFormats(workbenchID string) map[string]bool {
	relevant := make(map[string]bool, 3)
	for format, include := range e.detectFormatsInManifest(workbenchID) {
		if include {
			relevant[format] = true
		}
	}
	for format, include := range e.detectFormatsFromConversationIntent(workbenchID) {
		if include {
			relevant[format] = true
		}
	}
	return relevant
}

func (e *Engine) detectFormatsInManifest(workbenchID string) map[string]bool {
	formats := map[string]bool{
		"xlsx": false,
		"docx": false,
		"pptx": false,
	}
	files, err := e.workbenches.FilesList(workbenchID)
	if err != nil {
		e.logger.Warn("context.format_manifest_scan_failed", "workbench_id", workbenchID, "error", err.Error())
		return formats
	}
	for _, file := range files {
		switch strings.ToLower(strings.TrimPrefix(filepath.Ext(file.Path), ".")) {
		case "xlsx":
			formats["xlsx"] = true
		case "docx":
			formats["docx"] = true
		case "pptx":
			formats["pptx"] = true
		}
	}
	return formats
}

func (e *Engine) detectFormatsFromConversationIntent(workbenchID string) map[string]bool {
	formats := map[string]bool{
		"xlsx": false,
		"docx": false,
		"pptx": false,
	}
	items, err := e.readConversation(workbenchID)
	if err != nil {
		e.logger.Warn("context.format_intent_scan_failed", "workbench_id", workbenchID, "error", err.Error())
		return formats
	}
	start := 0
	if len(items) > 40 {
		start = len(items) - 40
	}
	for _, item := range items[start:] {
		if item.Role != "user" && item.Role != "assistant" {
			continue
		}
		markFormatsFromText(item.Text, formats)
	}
	return formats
}

func markFormatsFromText(text string, formats map[string]bool) {
	lower := strings.ToLower(text)
	switch {
	case strings.Contains(lower, ".xlsx"):
		formats["xlsx"] = true
	case containsAny(lower, formatStyleKeywords["xlsx"]):
		formats["xlsx"] = true
	}
	switch {
	case strings.Contains(lower, ".docx"):
		formats["docx"] = true
	case containsAny(lower, formatStyleKeywords["docx"]):
		formats["docx"] = true
	}
	switch {
	case strings.Contains(lower, ".pptx"):
		formats["pptx"] = true
	case containsAny(lower, formatStyleKeywords["pptx"]):
		formats["pptx"] = true
	}
}

func containsAny(value string, needles []string) bool {
	for _, needle := range needles {
		if strings.Contains(value, needle) {
			return true
		}
	}
	return false
}

func loadBundledFormatStyleSkill(format string) (contextSkillInjection, error) {
	baseDir, err := bundledSkillsBaseDir()
	if err != nil {
		return contextSkillInjection{}, err
	}
	skillName := format + "-style-skill"
	root := filepath.Join(baseDir, skillName)
	files, err := readContextArtifactsFromDir(root)
	if err != nil {
		return contextSkillInjection{}, err
	}
	if err := validateBundledSkillArtifacts(files, skillName); err != nil {
		return contextSkillInjection{}, err
	}
	return contextSkillInjection{Name: skillName, Files: files}, nil
}

func bundledSkillsBaseDir() (string, error) {
	if configured := strings.TrimSpace(os.Getenv("KEENBENCH_BUNDLED_SKILLS_DIR")); configured != "" {
		clean := filepath.Clean(configured)
		info, err := os.Stat(clean)
		if err != nil || !info.IsDir() {
			return "", fmt.Errorf("bundled style skills directory not found: %s", clean)
		}
		return clean, nil
	}
	candidates := make([]string, 0, 5)
	if _, sourcePath, _, ok := runtime.Caller(0); ok {
		candidates = append(candidates, filepath.Clean(filepath.Join(filepath.Dir(sourcePath), "..", "..", "skills", "bundled")))
	}
	if cwd, err := os.Getwd(); err == nil {
		candidates = append(candidates, filepath.Join(cwd, "engine", "skills", "bundled"))
		candidates = append(candidates, filepath.Join(cwd, "skills", "bundled"))
	}
	if executable, err := os.Executable(); err == nil {
		execDir := filepath.Dir(executable)
		candidates = append(candidates, filepath.Join(execDir, "skills", "bundled"))
		candidates = append(candidates, filepath.Join(execDir, "..", "skills", "bundled"))
	}
	seen := make(map[string]struct{}, len(candidates))
	for _, candidate := range candidates {
		clean := filepath.Clean(strings.TrimSpace(candidate))
		if clean == "" {
			continue
		}
		if _, exists := seen[clean]; exists {
			continue
		}
		seen[clean] = struct{}{}
		info, err := os.Stat(clean)
		if err == nil && info.IsDir() {
			return clean, nil
		}
	}
	return "", errors.New("bundled style skills directory not found")
}

func readContextArtifactsFromDir(root string) ([]contextArtifactFile, error) {
	info, err := os.Stat(root)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("not a directory: %s", root)
	}
	files := make([]contextArtifactFile, 0, 4)
	err = filepath.WalkDir(root, func(current string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		rel, relErr := filepath.Rel(root, current)
		if relErr != nil {
			return relErr
		}
		rel = filepath.ToSlash(rel)
		pathValue, pathErr := normalizeContextArtifactPath(rel)
		if pathErr != nil {
			return pathErr
		}
		data, readErr := os.ReadFile(current)
		if readErr != nil {
			return readErr
		}
		files = append(files, contextArtifactFile{
			Path:    pathValue,
			Content: string(data),
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(files, func(i, j int) bool {
		return files[i].Path < files[j].Path
	})
	return files, nil
}

func validateBundledSkillArtifacts(files []contextArtifactFile, expectedName string) error {
	if len(files) == 0 {
		return errors.New("skill artifact is empty")
	}
	filesByPath := make(map[string]string, len(files))
	for _, file := range files {
		filesByPath[file.Path] = file.Content
	}
	skillContent, ok := filesByPath["SKILL.md"]
	if !ok || strings.TrimSpace(skillContent) == "" {
		return errors.New("SKILL.md is required")
	}
	name, description, body, links, err := parseSkillFrontmatter(skillContent)
	if err != nil {
		return err
	}
	if strings.TrimSpace(name) == "" {
		return errors.New("skill name is required")
	}
	if expectedName != "" && name != expectedName {
		return fmt.Errorf("skill name mismatch: expected %s", expectedName)
	}
	if strings.TrimSpace(description) == "" {
		return errors.New("skill description is required")
	}
	if strings.TrimSpace(body) == "" {
		return errors.New("skill body is empty")
	}
	for _, link := range links {
		if strings.HasPrefix(link, "http://") || strings.HasPrefix(link, "https://") || strings.HasPrefix(link, "#") || strings.HasPrefix(link, "mailto:") {
			continue
		}
		clean := path.Clean(link)
		if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") || strings.HasPrefix(clean, "/") {
			return fmt.Errorf("invalid referenced path in SKILL.md: %s", link)
		}
		if _, ok := filesByPath[clean]; !ok {
			return fmt.Errorf("referenced file missing: %s", clean)
		}
	}
	return nil
}

func mergeDocumentStyleWithFormatSkill(format string, generic contextSkillInjection, documentStyleFiles []contextArtifactFile) (contextSkillInjection, error) {
	genericSkillContent, found := findContextArtifactFile(generic.Files, "SKILL.md")
	if !found {
		return contextSkillInjection{}, errors.New("generic format skill missing SKILL.md")
	}
	_, _, genericBody, _, err := parseSkillFrontmatter(genericSkillContent)
	if err != nil {
		return contextSkillInjection{}, fmt.Errorf("generic format skill invalid: %w", err)
	}
	userRules, userExamples, err := extractDocumentStyleGuidance(documentStyleFiles)
	if err != nil {
		return contextSkillInjection{}, err
	}
	prelude, sections := splitMarkdownLevel2Sections(genericBody)
	if len(sections) == 0 {
		return contextSkillInjection{}, errors.New("generic format skill missing markdown sections")
	}
	catalogIndex := -1
	principlesIndex := -1
	examplesIndex := -1
	for index, section := range sections {
		key := normalizeHeadingKey(section.Heading)
		if catalogIndex < 0 && strings.Contains(key, "toolcapabilitycatalog") {
			catalogIndex = index
		}
		if principlesIndex < 0 && strings.Contains(key, "formattingprinciples") {
			principlesIndex = index
		}
		if examplesIndex < 0 && (strings.Contains(key, "workedexamples") || key == "examples") {
			examplesIndex = index
		}
	}
	if catalogIndex < 0 {
		return contextSkillInjection{}, errors.New("generic format skill missing tool capability catalog section")
	}

	mergedPrinciples := ""
	if principlesIndex >= 0 {
		mergedPrinciples = mergeFormattingPrinciples(sections[principlesIndex].Content, userRules)
	} else {
		mergedPrinciples = strings.Join(userRules, "\n\n")
	}
	mergedExamples := ""
	if examplesIndex >= 0 {
		mergedExamples = mergeWorkedExamples(sections[examplesIndex].Content, userExamples)
	} else {
		mergedExamples = mergeWorkedExamples("", userExamples)
	}
	mergedBody := rebuildMergedSkillBody(prelude, sections, principlesIndex, examplesIndex, mergedPrinciples, mergedExamples)
	mergedSkillName := fmt.Sprintf("%s-style-custom", format)
	mergedDescription := fmt.Sprintf("Merged %s style guidance combining bundled format capabilities with workbench document style rules.", strings.ToUpper(format))
	mergedSkillContent, err := rewriteSkillFrontmatter(genericSkillContent, mergedSkillName, mergedDescription, mergedBody)
	if err != nil {
		return contextSkillInjection{}, err
	}
	mergedFiles := make([]contextArtifactFile, 0, len(generic.Files))
	for _, file := range generic.Files {
		if file.Path == "SKILL.md" {
			mergedFiles = append(mergedFiles, contextArtifactFile{
				Path:    "SKILL.md",
				Content: mergedSkillContent,
			})
			continue
		}
		mergedFiles = append(mergedFiles, file)
	}
	return contextSkillInjection{
		Name:  mergedSkillName,
		Files: mergedFiles,
	}, nil
}

func findContextArtifactFile(files []contextArtifactFile, path string) (string, bool) {
	for _, file := range files {
		if file.Path == path {
			return file.Content, true
		}
	}
	return "", false
}

func splitMarkdownLevel2Sections(body string) (string, []markdownLevel2Section) {
	normalized := strings.ReplaceAll(body, "\r\n", "\n")
	lines := strings.Split(normalized, "\n")
	indices := make([]int, 0, 8)
	for index, line := range lines {
		if strings.HasPrefix(line, "## ") {
			indices = append(indices, index)
		}
	}
	if len(indices) == 0 {
		return strings.TrimSpace(normalized), nil
	}
	prelude := strings.TrimSpace(strings.Join(lines[:indices[0]], "\n"))
	sections := make([]markdownLevel2Section, 0, len(indices))
	for index, start := range indices {
		end := len(lines)
		if index+1 < len(indices) {
			end = indices[index+1]
		}
		heading := strings.TrimSpace(strings.TrimPrefix(lines[start], "## "))
		content := strings.TrimSpace(strings.Join(lines[start+1:end], "\n"))
		raw := strings.TrimSpace(strings.Join(lines[start:end], "\n"))
		sections = append(sections, markdownLevel2Section{
			Heading: heading,
			Content: content,
			Raw:     raw,
		})
	}
	return prelude, sections
}

func normalizeHeadingKey(value string) string {
	lower := strings.ToLower(strings.TrimSpace(value))
	var builder strings.Builder
	builder.Grow(len(lower))
	for _, r := range lower {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			builder.WriteRune(r)
		}
	}
	return builder.String()
}

func extractDocumentStyleGuidance(files []contextArtifactFile) ([]string, []string, error) {
	if len(files) == 0 {
		return nil, nil, errors.New("document-style artifact is empty")
	}
	skillContent, found := findContextArtifactFile(files, "SKILL.md")
	if !found || strings.TrimSpace(skillContent) == "" {
		return nil, nil, errors.New("document-style SKILL.md is missing")
	}
	_, _, skillBody, _, err := parseSkillFrontmatter(skillContent)
	if err != nil {
		return nil, nil, fmt.Errorf("document-style SKILL.md invalid: %w", err)
	}
	_, sections := splitMarkdownLevel2Sections(skillBody)
	rules := make([]string, 0, 8)
	examples := make([]string, 4)
	explicitSectionsFound := false
	for _, section := range sections {
		content := strings.TrimSpace(section.Content)
		if content == "" {
			continue
		}
		key := normalizeHeadingKey(section.Heading)
		if strings.Contains(key, "workedexamples") || key == "examples" {
			examples = append(examples, content)
			explicitSectionsFound = true
			continue
		}
		if strings.Contains(key, "formattingrules") || strings.Contains(key, "stylerules") || strings.Contains(key, "formattingprinciples") || strings.Contains(key, "tonevoice") {
			rules = append(rules, content)
			explicitSectionsFound = true
		}
	}
	for _, file := range files {
		if file.Path == "SKILL.md" {
			continue
		}
		content := strings.TrimSpace(file.Content)
		if content == "" {
			continue
		}
		if strings.Contains(strings.ToLower(file.Path), "example") {
			examples = append(examples, content)
			continue
		}
		rules = append(rules, content)
	}
	if !explicitSectionsFound {
		raw := renderContextArtifactAsRawGuidance(files)
		if raw != "" {
			rules = []string{raw}
		}
	}
	rules = normalizeGuidanceBlocks(rules)
	examples = normalizeGuidanceBlocks(examples)
	if len(rules) == 0 {
		raw := renderContextArtifactAsRawGuidance(files)
		if raw == "" {
			return nil, nil, errors.New("document-style guidance is empty")
		}
		rules = []string{raw}
	}
	return rules, examples, nil
}

func renderContextArtifactAsRawGuidance(files []contextArtifactFile) string {
	ordered := make([]contextArtifactFile, len(files))
	copy(ordered, files)
	sort.Slice(ordered, func(i, j int) bool {
		return ordered[i].Path < ordered[j].Path
	})
	var builder strings.Builder
	for _, file := range ordered {
		content := strings.TrimSpace(file.Content)
		if content == "" {
			continue
		}
		builder.WriteString("### ")
		builder.WriteString(file.Path)
		builder.WriteString("\n")
		builder.WriteString(content)
		builder.WriteString("\n\n")
	}
	return strings.TrimSpace(builder.String())
}

func normalizeGuidanceBlocks(values []string) []string {
	normalized := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		clean := strings.TrimSpace(value)
		if clean == "" {
			continue
		}
		if _, exists := seen[clean]; exists {
			continue
		}
		seen[clean] = struct{}{}
		normalized = append(normalized, clean)
	}
	return normalized
}

func mergeFormattingPrinciples(genericContent string, userRules []string) string {
	genericRules := splitRuleBlocks(genericContent)
	if len(genericRules) == 0 && strings.TrimSpace(genericContent) != "" {
		genericRules = []string{strings.TrimSpace(genericContent)}
	}
	for _, userRule := range normalizeGuidanceBlocks(userRules) {
		conflictIndex := findConflictingRuleIndex(genericRules, userRule)
		if conflictIndex >= 0 {
			genericRules[conflictIndex] = userRule
			continue
		}
		genericRules = append(genericRules, userRule)
	}
	return strings.TrimSpace(strings.Join(genericRules, "\n\n"))
}

func splitRuleBlocks(content string) []string {
	normalized := strings.ReplaceAll(content, "\r\n", "\n")
	lines := strings.Split(normalized, "\n")
	blocks := make([]string, 0, len(lines))
	var paragraph []string
	flushParagraph := func() {
		text := strings.TrimSpace(strings.Join(paragraph, "\n"))
		if text != "" {
			blocks = append(blocks, text)
		}
		paragraph = paragraph[:0]
	}
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			flushParagraph()
			continue
		}
		if strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "* ") || isOrderedListItem(trimmed) {
			flushParagraph()
			blocks = append(blocks, trimmed)
			continue
		}
		paragraph = append(paragraph, strings.TrimRight(line, " \t"))
	}
	flushParagraph()
	if len(blocks) == 0 {
		text := strings.TrimSpace(normalized)
		if text != "" {
			blocks = append(blocks, text)
		}
	}
	return blocks
}

func isOrderedListItem(line string) bool {
	if len(line) < 3 {
		return false
	}
	index := 0
	for index < len(line) && line[index] >= '0' && line[index] <= '9' {
		index++
	}
	if index == 0 || index+1 >= len(line) {
		return false
	}
	return line[index] == '.' && line[index+1] == ' '
}

func findConflictingRuleIndex(genericRules []string, userRule string) int {
	userKeywords := extractRuleKeywords(userRule)
	if len(userKeywords) == 0 {
		return -1
	}
	for index, genericRule := range genericRules {
		genericKeywords := extractRuleKeywords(genericRule)
		if len(genericKeywords) == 0 {
			continue
		}
		overlap := 0
		for keyword := range userKeywords {
			if _, ok := genericKeywords[keyword]; ok {
				overlap++
			}
		}
		minKeywords := len(userKeywords)
		if len(genericKeywords) < minKeywords {
			minKeywords = len(genericKeywords)
		}
		if minKeywords == 0 {
			continue
		}
		if float64(overlap)/float64(minKeywords) >= 0.70 {
			return index
		}
	}
	return -1
}

func extractRuleKeywords(text string) map[string]struct{} {
	lower := strings.ToLower(text)
	var builder strings.Builder
	builder.Grow(len(lower))
	for _, r := range lower {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			builder.WriteRune(r)
			continue
		}
		builder.WriteByte(' ')
	}
	keywords := make(map[string]struct{}, 8)
	for _, token := range strings.Fields(builder.String()) {
		if len(token) < 4 {
			continue
		}
		if _, isStopWord := mergeStopWords[token]; isStopWord {
			continue
		}
		keywords[token] = struct{}{}
	}
	return keywords
}

func mergeWorkedExamples(genericContent string, userExamples []string) string {
	examples := normalizeGuidanceBlocks(userExamples)
	base := strings.TrimSpace(genericContent)
	if len(examples) == 0 {
		return base
	}
	var builder strings.Builder
	if base != "" {
		builder.WriteString(base)
		builder.WriteString("\n\n")
	}
	builder.WriteString("### User Custom Examples\n\n")
	for index, example := range examples {
		if index > 0 {
			builder.WriteString("\n\n")
		}
		builder.WriteString(example)
	}
	return strings.TrimSpace(builder.String())
}

func rebuildMergedSkillBody(
	prelude string,
	sections []markdownLevel2Section,
	principlesIndex int,
	examplesIndex int,
	mergedPrinciples string,
	mergedExamples string,
) string {
	var builder strings.Builder
	if strings.TrimSpace(prelude) != "" {
		builder.WriteString(strings.TrimSpace(prelude))
		builder.WriteString("\n\n")
	}
	for index, section := range sections {
		if index > 0 {
			builder.WriteString("\n\n")
		}
		switch index {
		case principlesIndex:
			builder.WriteString("## ")
			builder.WriteString(section.Heading)
			builder.WriteString("\n")
			if strings.TrimSpace(mergedPrinciples) != "" {
				builder.WriteString(strings.TrimSpace(mergedPrinciples))
			}
		case examplesIndex:
			builder.WriteString("## ")
			builder.WriteString(section.Heading)
			builder.WriteString("\n")
			if strings.TrimSpace(mergedExamples) != "" {
				builder.WriteString(strings.TrimSpace(mergedExamples))
			}
		default:
			builder.WriteString(strings.TrimSpace(section.Raw))
		}
	}
	if principlesIndex < 0 && strings.TrimSpace(mergedPrinciples) != "" {
		if builder.Len() > 0 {
			builder.WriteString("\n\n")
		}
		builder.WriteString("## Formatting Principles\n")
		builder.WriteString(strings.TrimSpace(mergedPrinciples))
	}
	if examplesIndex < 0 && strings.TrimSpace(mergedExamples) != "" {
		if builder.Len() > 0 {
			builder.WriteString("\n\n")
		}
		builder.WriteString("## Worked Examples\n")
		builder.WriteString(strings.TrimSpace(mergedExamples))
	}
	return strings.TrimSpace(builder.String())
}

func rewriteSkillFrontmatter(content, name, description, body string) (string, error) {
	trimmed := strings.TrimLeft(content, "\ufeff")
	lines := strings.Split(strings.ReplaceAll(trimmed, "\r\n", "\n"), "\n")
	if len(lines) < 3 || strings.TrimSpace(lines[0]) != "---" {
		return "", errors.New("SKILL.md must start with YAML frontmatter")
	}
	end := -1
	for index := 1; index < len(lines); index++ {
		if strings.TrimSpace(lines[index]) == "---" {
			end = index
			break
		}
	}
	if end <= 0 {
		return "", errors.New("SKILL.md frontmatter terminator missing")
	}
	frontmatter := lines[1:end]
	updated := make([]string, 0, len(frontmatter)+2)
	hasName := false
	hasDescription := false
	for i := 0; i < len(frontmatter); i++ {
		line := frontmatter[i]
		trimmedLine := strings.TrimSpace(line)
		if strings.HasPrefix(trimmedLine, "name:") {
			updated = append(updated, "name: "+name)
			hasName = true
			continue
		}
		if strings.HasPrefix(trimmedLine, "description:") {
			hasDescription = true
			updated = append(updated, fmt.Sprintf(`description: "%s"`, yamlEscapeDoubleQuoted(description)))
			value := strings.TrimSpace(strings.TrimPrefix(trimmedLine, "description:"))
			if value == "" || value == "|" || value == ">" {
				for i+1 < len(frontmatter) {
					next := frontmatter[i+1]
					if strings.TrimSpace(next) == "" {
						i++
						continue
					}
					if strings.HasPrefix(next, " ") || strings.HasPrefix(next, "\t") {
						i++
						continue
					}
					break
				}
			}
			continue
		}
		updated = append(updated, line)
	}
	if !hasName {
		updated = append([]string{"name: " + name}, updated...)
	}
	if !hasDescription {
		updated = append(updated, fmt.Sprintf(`description: "%s"`, yamlEscapeDoubleQuoted(description)))
	}
	var builder strings.Builder
	builder.WriteString("---\n")
	builder.WriteString(strings.Join(updated, "\n"))
	builder.WriteString("\n---\n\n")
	body = strings.TrimSpace(body)
	if body != "" {
		builder.WriteString(body)
		builder.WriteString("\n")
	}
	return builder.String(), nil
}

func yamlEscapeDoubleQuoted(value string) string {
	value = strings.ReplaceAll(value, "\\", "\\\\")
	return strings.ReplaceAll(value, `"`, `\"`)
}
