package workbench

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	maxFiles   = 10
	maxSize    = 25 * 1024 * 1024
	schema     = 2
	metaFolder = "meta"
)

const (
	ForkModeCloneFilesOnly             = "clone_files_only"
	ForkModeCloneFilesAndContextNoChat = "clone_files_and_context_no_chat"
	ForkModeCloneAll                   = "clone_all"
)

const (
	FileKindText   = "text"
	FileKindDocx   = "docx"
	FileKindOdt    = "odt"
	FileKindXlsx   = "xlsx"
	FileKindPptx   = "pptx"
	FileKindPdf    = "pdf"
	FileKindImage  = "image"
	FileKindBinary = "binary"
)

var textWriteExtensions = map[string]bool{
	".txt":  true,
	".csv":  true,
	".md":   true,
	".json": true,
	".xml":  true,
	".yaml": true,
	".yml":  true,
	".html": true,
	".js":   true,
	".ts":   true,
	".py":   true,
	".java": true,
	".go":   true,
	".rb":   true,
	".rs":   true,
	".c":    true,
	".cpp":  true,
	".h":    true,
	".css":  true,
	".sql":  true,
}

var officeExtensions = map[string]string{
	".docx": FileKindDocx,
	".odt":  FileKindOdt,
	".xlsx": FileKindXlsx,
	".pptx": FileKindPptx,
}

var imageExtensions = map[string]bool{
	".png":  true,
	".jpg":  true,
	".jpeg": true,
	".gif":  true,
	".webp": true,
	".svg":  true,
}

var mimeFallbacks = map[string]string{
	".md":   "text/markdown",
	".csv":  "text/csv",
	".yaml": "text/yaml",
	".yml":  "text/yaml",
	".json": "application/json",
	".xml":  "application/xml",
	".docx": "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
	".odt":  "application/vnd.oasis.opendocument.text",
	".xlsx": "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
	".pptx": "application/vnd.openxmlformats-officedocument.presentationml.presentation",
	".pdf":  "application/pdf",
	".svg":  "image/svg+xml",
}

var (
	ErrSandboxViolation   = errors.New("sandbox violation")
	ErrInvalidPath        = errors.New("invalid path")
	ErrInvalidDestination = errors.New("invalid destination")
	ErrDeletionDetected   = errors.New("deletion detected")
)

// Workbench describes a workbench on disk.
type Workbench struct {
	ID                  string `json:"id"`
	Name                string `json:"name"`
	CreatedAt           string `json:"created_at"`
	UpdatedAt           string `json:"updated_at"`
	DefaultModelID      string `json:"default_model_id,omitempty"`
	ParentWorkbenchID   string `json:"parent_workbench_id,omitempty"`
	ForkMode            string `json:"fork_mode,omitempty"`
	ForkedFromMessageID string `json:"forked_from_message_id,omitempty"`
	ForkedAt            string `json:"forked_at,omitempty"`
}

type FileEntry struct {
	Path       string `json:"path"`
	Size       int64  `json:"size"`
	ModifiedAt string `json:"modified_at"`
	AddedAt    string `json:"added_at"`
	FileKind   string `json:"file_kind"`
	MimeType   string `json:"mime_type"`
	IsOpaque   bool   `json:"is_opaque"`
}

type FileManifest struct {
	SchemaVersion int         `json:"schema_version"`
	Files         []FileEntry `json:"files"`
}

type AddResult struct {
	SourcePath string `json:"source_path"`
	FileName   string `json:"file_name"`
	Status     string `json:"status"`
	Reason     string `json:"reason,omitempty"`
}

type RemoveResult struct {
	Path   string `json:"path"`
	Status string `json:"status"`
	Reason string `json:"reason,omitempty"`
}

type ExtractResult struct {
	Path      string `json:"path"`
	Status    string `json:"status"`
	Reason    string `json:"reason,omitempty"`
	FinalPath string `json:"final_path,omitempty"`
}

type DraftState struct {
	DraftID    string `json:"draft_id"`
	CreatedAt  string `json:"created_at"`
	SourceKind string `json:"source_kind,omitempty"`
	SourceRef  string `json:"source_ref,omitempty"`
}

type Change struct {
	Path       string `json:"path"`
	ChangeType string `json:"change_type"`
}

type Consent struct {
	SchemaVersion int             `json:"schema_version"`
	Workshop      WorkshopConsent `json:"workshop"`
}

type WorkshopConsent struct {
	ProviderID  string `json:"provider_id"`
	ModelID     string `json:"model_id,omitempty"`
	ScopeHash   string `json:"scope_hash"`
	ConsentedAt string `json:"consented_at"`
	Persisted   bool   `json:"persisted"`
}

type Manager struct {
	baseDir string
}

func NewManager(baseDir string) *Manager {
	return &Manager{baseDir: baseDir}
}

func (m *Manager) Init() error {
	if err := os.MkdirAll(m.baseDir, 0o755); err != nil {
		return err
	}
	if err := m.cleanupTransientDirs(); err != nil {
		return err
	}
	if err := m.cleanupRestoreArtifacts(); err != nil {
		return err
	}
	return m.cleanupReviewArtifacts()
}

func (m *Manager) cleanupTransientDirs() error {
	entries, err := os.ReadDir(m.baseDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		root := filepath.Join(m.baseDir, entry.Name())
		_ = os.RemoveAll(filepath.Join(root, "draft.prev"))
		children, err := os.ReadDir(root)
		if err != nil {
			continue
		}
		for _, child := range children {
			if !child.IsDir() {
				continue
			}
			name := child.Name()
			if strings.HasPrefix(name, "draft.") && strings.HasSuffix(name, ".staging") {
				_ = os.RemoveAll(filepath.Join(root, name))
			}
		}
	}
	return nil
}

func (m *Manager) cleanupReviewArtifacts() error {
	entries, err := os.ReadDir(m.baseDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		root := filepath.Join(m.baseDir, entry.Name())
		draftPath := filepath.Join(root, metaFolder, "draft.json")
		hasDraft := false
		draftID := ""
		if state, err := readDraftStateFile(draftPath); err == nil && state != nil && state.DraftID != "" {
			hasDraft = true
			draftID = state.DraftID
		}
		reviewRoot := filepath.Join(root, metaFolder, "review")
		if !hasDraft {
			_ = os.RemoveAll(reviewRoot)
			continue
		}
		reviewEntries, err := os.ReadDir(reviewRoot)
		if err != nil {
			continue
		}
		for _, reviewEntry := range reviewEntries {
			if !reviewEntry.IsDir() {
				continue
			}
			if reviewEntry.Name() != draftID {
				_ = os.RemoveAll(filepath.Join(reviewRoot, reviewEntry.Name()))
			}
		}
	}
	return nil
}

func (m *Manager) Create(name, defaultModelID string) (*Workbench, error) {
	if name == "" {
		name = "Untitled Workbench"
	}
	if defaultModelID == "" {
		defaultModelID = "openai:gpt-5.2"
	}
	id := newID()
	root := filepath.Join(m.baseDir, id)
	if err := os.MkdirAll(filepath.Join(root, "published"), 0o755); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Join(root, metaFolder), 0o755); err != nil {
		return nil, err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	wb := &Workbench{ID: id, Name: name, CreatedAt: now, UpdatedAt: now, DefaultModelID: defaultModelID}
	if err := writeJSON(filepath.Join(root, metaFolder, "workbench.json"), wb); err != nil {
		return nil, err
	}
	manifest := FileManifest{SchemaVersion: schema, Files: []FileEntry{}}
	if err := writeJSON(filepath.Join(root, metaFolder, "files.json"), manifest); err != nil {
		return nil, err
	}
	return wb, nil
}

func (m *Manager) Fork(sourceID, mode, name, fromMessageID string) (*Workbench, error) {
	if err := validateForkMode(mode); err != nil {
		return nil, err
	}
	sourceRoot, err := m.workbenchRoot(sourceID)
	if err != nil {
		return nil, err
	}
	sourceWB, err := m.Open(sourceID)
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(filepath.Join(sourceRoot, "draft")); err == nil {
		return nil, errors.New("draft exists")
	}
	if strings.TrimSpace(name) == "" {
		name = sourceWB.Name
	}
	targetWB, err := m.Create(name, sourceWB.DefaultModelID)
	if err != nil {
		return nil, err
	}
	targetRoot, err := m.workbenchRoot(targetWB.ID)
	if err != nil {
		_ = os.RemoveAll(filepath.Join(m.baseDir, targetWB.ID))
		return nil, err
	}
	if err := m.copyForkData(sourceRoot, targetRoot); err != nil {
		_ = os.RemoveAll(targetRoot)
		return nil, err
	}
	if err := m.cleanForkMetadata(targetRoot, mode); err != nil {
		_ = os.RemoveAll(targetRoot)
		return nil, err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	targetWB.ParentWorkbenchID = sourceWB.ID
	targetWB.ForkMode = mode
	targetWB.ForkedAt = now
	targetWB.UpdatedAt = now
	if trimmed := strings.TrimSpace(fromMessageID); trimmed != "" {
		targetWB.ForkedFromMessageID = trimmed
	}
	if err := writeJSON(filepath.Join(targetRoot, metaFolder, "workbench.json"), targetWB); err != nil {
		_ = os.RemoveAll(targetRoot)
		return nil, err
	}
	return targetWB, nil
}

func validateForkMode(mode string) error {
	switch mode {
	case ForkModeCloneFilesOnly, ForkModeCloneFilesAndContextNoChat, ForkModeCloneAll:
		return nil
	default:
		return errors.New("invalid fork mode")
	}
}

func (m *Manager) copyForkData(sourceRoot, targetRoot string) error {
	sourcePublished := filepath.Join(sourceRoot, "published")
	targetPublished := filepath.Join(targetRoot, "published")
	_ = os.RemoveAll(targetPublished)
	if err := copyDir(sourcePublished, targetPublished); err != nil {
		return err
	}
	sourceMeta := filepath.Join(sourceRoot, metaFolder)
	targetMeta := filepath.Join(targetRoot, metaFolder)
	_ = os.RemoveAll(targetMeta)
	return copyDir(sourceMeta, targetMeta)
}

func (m *Manager) cleanForkMetadata(targetRoot, mode string) error {
	metaRoot := filepath.Join(targetRoot, metaFolder)
	_ = os.RemoveAll(filepath.Join(targetRoot, "draft"))
	_ = os.Remove(filepath.Join(metaRoot, "draft.json"))
	_ = os.RemoveAll(filepath.Join(metaRoot, "review"))
	// Consent is scoped per workbench and must be granted independently.
	_ = os.Remove(filepath.Join(metaRoot, "egress_consent.json"))

	if mode == ForkModeCloneFilesAndContextNoChat || mode == ForkModeCloneFilesOnly {
		for _, rel := range []string{
			"conversation.jsonl",
			"workshop_state.json",
			"workshop",
			"checkpoints",
			"jobs",
			"workbench_events.jsonl",
			"egress_events.jsonl",
		} {
			_ = os.RemoveAll(filepath.Join(metaRoot, rel))
		}
		if mode == ForkModeCloneFilesOnly {
			_ = os.RemoveAll(filepath.Join(metaRoot, "context"))
		}
	}
	return nil
}

func (m *Manager) List() ([]Workbench, error) {
	entries, err := os.ReadDir(m.baseDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []Workbench{}, nil
		}
		return nil, err
	}
	var workbenches []Workbench
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		wb, err := m.Open(entry.Name())
		if err != nil {
			continue
		}
		workbenches = append(workbenches, *wb)
	}
	sort.Slice(workbenches, func(i, j int) bool {
		return workbenches[i].UpdatedAt > workbenches[j].UpdatedAt
	})
	return workbenches, nil
}

func (m *Manager) Open(id string) (*Workbench, error) {
	root, err := m.workbenchRoot(id)
	if err != nil {
		return nil, err
	}
	var wb Workbench
	if err := readJSON(filepath.Join(root, metaFolder, "workbench.json"), &wb); err != nil {
		return nil, err
	}
	return &wb, nil
}

func (m *Manager) Delete(id string) error {
	root, err := m.workbenchRoot(id)
	if err != nil {
		return err
	}
	if _, err := os.Stat(root); err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(root, "draft")); err == nil {
		return errors.New("draft exists")
	}
	return os.RemoveAll(root)
}

func (m *Manager) FilesList(id string) ([]FileEntry, error) {
	root, err := m.workbenchRoot(id)
	if err != nil {
		return nil, err
	}
	manifest, err := m.readManifest(root)
	if err != nil {
		return nil, err
	}
	return manifest.Files, nil
}

// DraftFilesList returns file entries by scanning the draft directory on disk.
// Unlike FilesList (which reads from the manifest and only knows published
// files), this sees files created by the agent during the workshop loop.
// Falls back to FilesList if no draft exists.
func (m *Manager) DraftFilesList(id string) ([]FileEntry, error) {
	root, err := m.workbenchRoot(id)
	if err != nil {
		return nil, err
	}
	draftDir := filepath.Join(root, "draft")
	if _, err := os.Stat(draftDir); err != nil {
		return m.FilesList(id)
	}
	manifest, err := buildManifestFromDir(draftDir)
	if err != nil {
		return m.FilesList(id)
	}
	return manifest.Files, nil
}

func (m *Manager) FilesRemove(id string, paths []string) ([]RemoveResult, error) {
	root, err := m.workbenchRoot(id)
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(filepath.Join(root, "draft")); err == nil {
		return nil, errors.New("draft exists")
	}
	manifest, err := m.readManifest(root)
	if err != nil {
		return nil, err
	}
	results := make([]RemoveResult, 0, len(paths))
	if len(paths) == 0 {
		return results, nil
	}
	fileIndex := make(map[string]FileEntry)
	for _, entry := range manifest.Files {
		fileIndex[strings.ToLower(entry.Path)] = entry
	}
	seen := make(map[string]bool)
	removeManifest := make(map[string]bool)
	cleanupTabular := make(map[string]string)
	for _, path := range paths {
		result := RemoveResult{Path: path}
		if err := validateFlatFilePath(path); err != nil {
			result.Status = "failed"
			result.Reason = "invalid_path"
			results = append(results, result)
			continue
		}
		key := strings.ToLower(path)
		if seen[key] {
			result.Status = "skipped"
			result.Reason = "duplicate_request"
			results = append(results, result)
			continue
		}
		seen[key] = true
		if entry, ok := fileIndex[key]; ok {
			target := filepath.Join(root, "published", entry.Path)
			if err := os.Remove(target); err != nil {
				if os.IsNotExist(err) {
					result.Status = "removed"
					result.Reason = "missing_on_disk"
					removeManifest[key] = true
					cleanupTabular[key] = entry.Path
				} else {
					result.Status = "failed"
					result.Reason = "delete_failed"
				}
			} else {
				result.Status = "removed"
				removeManifest[key] = true
				cleanupTabular[key] = entry.Path
			}
			results = append(results, result)
			continue
		}
		target := filepath.Join(root, "published", path)
		if err := os.Remove(target); err != nil {
			if os.IsNotExist(err) {
				result.Status = "skipped"
				result.Reason = "not_found"
			} else {
				result.Status = "failed"
				result.Reason = "delete_failed"
			}
		} else {
			result.Status = "removed"
			result.Reason = "untracked"
			cleanupTabular[key] = path
		}
		results = append(results, result)
	}
	if len(removeManifest) > 0 {
		updated := make([]FileEntry, 0, len(manifest.Files))
		for _, entry := range manifest.Files {
			if removeManifest[strings.ToLower(entry.Path)] {
				continue
			}
			updated = append(updated, entry)
		}
		manifest.Files = updated
		if err := m.writeManifest(root, manifest); err != nil {
			return nil, err
		}
	}
	if len(cleanupTabular) > 0 {
		tabularPaths := make([]string, 0, len(cleanupTabular))
		for _, path := range cleanupTabular {
			tabularPaths = append(tabularPaths, path)
		}
		m.cleanupTabularArtifacts(root, tabularPaths)
	}
	return results, nil
}

func (m *Manager) FilesExtract(id, destinationDir string, paths []string) ([]ExtractResult, error) {
	root, err := m.workbenchRoot(id)
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(filepath.Join(root, "draft")); err == nil {
		return nil, errors.New("draft exists")
	}
	destinationDir = strings.TrimSpace(destinationDir)
	if destinationDir == "" {
		return nil, ErrInvalidDestination
	}
	destInfo, err := os.Stat(destinationDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, errors.New("destination not found")
		}
		return nil, err
	}
	if !destInfo.IsDir() {
		return nil, ErrInvalidDestination
	}

	manifest, err := m.readManifest(root)
	if err != nil {
		return nil, err
	}

	requested := paths
	if len(requested) == 0 {
		requested = make([]string, 0, len(manifest.Files))
		for _, entry := range manifest.Files {
			requested = append(requested, entry.Path)
		}
	}

	results := make([]ExtractResult, 0, len(requested))
	manifestIndex := make(map[string]FileEntry, len(manifest.Files))
	for _, entry := range manifest.Files {
		manifestIndex[strings.ToLower(entry.Path)] = entry
	}
	seen := make(map[string]bool, len(requested))

	for _, path := range requested {
		result := ExtractResult{Path: path}
		if err := validateFlatFilePath(path); err != nil {
			result.Status = "failed"
			result.Reason = "invalid_path"
			results = append(results, result)
			continue
		}
		key := strings.ToLower(path)
		if seen[key] {
			result.Status = "skipped"
			result.Reason = "duplicate_request"
			results = append(results, result)
			continue
		}
		seen[key] = true

		sourcePath := path
		if entry, ok := manifestIndex[key]; ok {
			sourcePath = entry.Path
		}
		src := filepath.Join(root, "published", sourcePath)
		if _, err := os.Stat(src); err != nil {
			if os.IsNotExist(err) {
				result.Status = "skipped"
				result.Reason = "not_found"
			} else {
				result.Status = "failed"
				result.Reason = "copy_failed"
			}
			results = append(results, result)
			continue
		}

		dest, finalPath, err := uniqueExtractDestination(destinationDir, sourcePath)
		if err != nil {
			result.Status = "failed"
			result.Reason = "copy_failed"
			results = append(results, result)
			continue
		}
		if err := copyFile(src, dest); err != nil {
			result.Status = "failed"
			result.Reason = "copy_failed"
		} else {
			result.Status = "extracted"
			result.FinalPath = finalPath
		}
		results = append(results, result)
	}
	return results, nil
}

func (m *Manager) FilesAdd(id string, sourcePaths []string) ([]AddResult, error) {
	root, err := m.workbenchRoot(id)
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(filepath.Join(root, "draft")); err == nil {
		return nil, errors.New("draft exists")
	}
	manifest, err := m.readManifest(root)
	if err != nil {
		return nil, err
	}
	existing := make(map[string]bool)
	for _, file := range manifest.Files {
		existing[strings.ToLower(file.Path)] = true
	}
	batch := make(map[string]bool)
	var results []AddResult
	var toAdd []struct {
		source string
		name   string
		info   os.FileInfo
	}
	for _, src := range sourcePaths {
		result := AddResult{SourcePath: src}
		info, err := os.Lstat(src)
		if err != nil {
			result.Status = "skipped"
			result.Reason = "unreadable"
			results = append(results, result)
			continue
		}
		if info.Mode()&os.ModeSymlink != 0 {
			result.Status = "skipped"
			result.Reason = "symlink"
			results = append(results, result)
			continue
		}
		if info.IsDir() {
			result.Status = "skipped"
			result.Reason = "directory"
			results = append(results, result)
			continue
		}
		if info.Size() > maxSize {
			result.Status = "skipped"
			result.Reason = "size_limit"
			results = append(results, result)
			continue
		}
		name := filepath.Base(src)
		result.FileName = name
		key := strings.ToLower(name)
		if existing[key] || batch[key] {
			result.Status = "skipped"
			result.Reason = "duplicate"
			results = append(results, result)
			continue
		}
		batch[key] = true
		result.Status = "added"
		results = append(results, result)
		toAdd = append(toAdd, struct {
			source string
			name   string
			info   os.FileInfo
		}{source: src, name: name, info: info})
	}
	if len(manifest.Files)+len(toAdd) > maxFiles {
		return nil, fmt.Errorf("file limit exceeded")
	}
	for _, item := range toAdd {
		dest := filepath.Join(root, "published", item.name)
		if err := copyFile(item.source, dest); err != nil {
			return nil, err
		}
		fileKind, mimeType, isOpaque := classifyPath(item.name)
		manifest.Files = append(manifest.Files, FileEntry{
			Path:       item.name,
			Size:       item.info.Size(),
			ModifiedAt: item.info.ModTime().UTC().Format(time.RFC3339),
			AddedAt:    time.Now().UTC().Format(time.RFC3339),
			FileKind:   fileKind,
			MimeType:   mimeType,
			IsOpaque:   isOpaque,
		})
	}
	if err := m.writeManifest(root, manifest); err != nil {
		return nil, err
	}
	return results, nil
}

func (m *Manager) ComputeScopeHash(id string) (string, error) {
	root, err := m.workbenchRoot(id)
	if err != nil {
		return "", err
	}
	manifest, err := m.readManifest(root)
	if err != nil {
		return "", err
	}
	sort.Slice(manifest.Files, func(i, j int) bool {
		return manifest.Files[i].Path < manifest.Files[j].Path
	})
	hasher := sha256.New()
	for _, file := range manifest.Files {
		_, _ = hasher.Write([]byte(file.Path))
		_, _ = hasher.Write([]byte("\n"))
		_, _ = hasher.Write([]byte(fmt.Sprintf("%d", file.Size)))
		_, _ = hasher.Write([]byte("\n"))
		_, _ = hasher.Write([]byte(file.ModifiedAt))
		_, _ = hasher.Write([]byte("\n"))
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}

func (m *Manager) ReadConsent(id string) (*Consent, error) {
	root, err := m.workbenchRoot(id)
	if err != nil {
		return nil, err
	}
	path := filepath.Join(root, metaFolder, "egress_consent.json")
	var consent Consent
	if err := readJSON(path, &consent); err != nil {
		if os.IsNotExist(err) {
			return &Consent{SchemaVersion: schema}, nil
		}
		return nil, err
	}
	return &consent, nil
}

func (m *Manager) WriteConsent(id string, consent *Consent) error {
	root, err := m.workbenchRoot(id)
	if err != nil {
		return err
	}
	consent.SchemaVersion = schema
	return writeJSON(filepath.Join(root, metaFolder, "egress_consent.json"), consent)
}

func (m *Manager) DraftState(id string) (*DraftState, error) {
	root, err := m.workbenchRoot(id)
	if err != nil {
		return nil, err
	}
	path := filepath.Join(root, metaFolder, "draft.json")
	state, err := readDraftStateFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	return state, nil
}

func (m *Manager) CreateDraft(id string) (*DraftState, error) {
	return m.CreateDraftWithSource(id, "", "")
}

func (m *Manager) CreateDraftWithSource(id, sourceKind, sourceRef string) (*DraftState, error) {
	root, err := m.workbenchRoot(id)
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(filepath.Join(root, "draft")); err == nil {
		state, _ := m.DraftState(id)
		return state, nil
	}
	if err := copyDir(filepath.Join(root, "published"), filepath.Join(root, "draft")); err != nil {
		return nil, err
	}
	state := &DraftState{DraftID: newID(), CreatedAt: time.Now().UTC().Format(time.RFC3339)}
	if trimmed := strings.TrimSpace(sourceKind); trimmed != "" {
		state.SourceKind = trimmed
	}
	if trimmed := strings.TrimSpace(sourceRef); trimmed != "" {
		state.SourceRef = trimmed
	}
	if err := writeJSON(filepath.Join(root, metaFolder, "draft.json"), state); err != nil {
		return nil, err
	}
	return state, nil
}

func (m *Manager) ApplyDraftWrite(id, relPath, content string) error {
	return m.ApplyWriteToArea(id, "draft", relPath, content)
}

func (m *Manager) ApplyWriteToArea(id, area, relPath, content string) error {
	root, err := m.workbenchRoot(id)
	if err != nil {
		return err
	}
	if err := validateAreaName(area); err != nil {
		return err
	}
	if relPath == "" {
		return ErrInvalidPath
	}
	clean := filepath.Clean(relPath)
	if strings.HasPrefix(clean, "..") || filepath.IsAbs(clean) || strings.Contains(clean, "\\") || strings.Contains(clean, string(filepath.Separator)+"..") {
		return ErrInvalidPath
	}
	if filepath.Dir(clean) != "." {
		return ErrInvalidPath
	}
	ext := strings.ToLower(filepath.Ext(clean))
	if !textWriteExtensions[ext] {
		return errors.New("unsupported extension")
	}
	areaPath := filepath.Join(root, area, clean)
	if !strings.HasPrefix(areaPath, filepath.Join(root, area)+string(filepath.Separator)) {
		return ErrSandboxViolation
	}
	if err := os.MkdirAll(filepath.Dir(areaPath), 0o755); err != nil {
		return err
	}
	if err := atomicWrite(areaPath, []byte(content)); err != nil {
		return err
	}
	if area == "draft" && ext == ".csv" {
		m.cleanupTabularArtifacts(root, []string{clean})
	}
	return nil
}

func (m *Manager) CreateDraftStaging(id, stagingName string) error {
	root, err := m.workbenchRoot(id)
	if err != nil {
		return err
	}
	if err := validateAreaName(stagingName); err != nil {
		return err
	}
	draftPath := filepath.Join(root, "draft")
	if _, err := os.Stat(draftPath); err != nil {
		return err
	}
	stagingPath := filepath.Join(root, stagingName)
	_ = os.RemoveAll(stagingPath)
	return copyDir(draftPath, stagingPath)
}

func (m *Manager) CommitDraftStaging(id, stagingName string) error {
	root, err := m.workbenchRoot(id)
	if err != nil {
		return err
	}
	if err := validateAreaName(stagingName); err != nil {
		return err
	}
	draftPath := filepath.Join(root, "draft")
	stagingPath := filepath.Join(root, stagingName)
	if _, err := os.Stat(stagingPath); err != nil {
		return err
	}
	prevPath := filepath.Join(root, "draft.prev")
	_ = os.RemoveAll(prevPath)
	if err := os.Rename(draftPath, prevPath); err != nil {
		return err
	}
	if err := os.Rename(stagingPath, draftPath); err != nil {
		_ = os.Rename(prevPath, draftPath)
		return err
	}
	_ = os.RemoveAll(prevPath)
	return nil
}

func (m *Manager) RemoveDraftStaging(id, stagingName string) error {
	root, err := m.workbenchRoot(id)
	if err != nil {
		return err
	}
	if err := validateAreaName(stagingName); err != nil {
		return err
	}
	return os.RemoveAll(filepath.Join(root, stagingName))
}

func (m *Manager) PublishDraft(id string) (time.Time, error) {
	root, err := m.workbenchRoot(id)
	if err != nil {
		return time.Time{}, err
	}
	draftState, _ := m.DraftState(id)
	draftPath := filepath.Join(root, "draft")
	if _, err := os.Stat(draftPath); err != nil {
		return time.Time{}, err
	}
	publishedPath := filepath.Join(root, "published")
	prevPath := filepath.Join(root, "published.prev")
	_ = os.RemoveAll(prevPath)
	if err := os.Rename(publishedPath, prevPath); err != nil {
		return time.Time{}, err
	}
	if err := os.Rename(draftPath, publishedPath); err != nil {
		_ = os.Rename(prevPath, publishedPath)
		return time.Time{}, err
	}
	_ = os.RemoveAll(prevPath)
	_ = os.Remove(filepath.Join(root, metaFolder, "draft.json"))
	if draftState != nil && draftState.DraftID != "" {
		_ = os.RemoveAll(filepath.Join(root, metaFolder, "review", draftState.DraftID))
	}
	// Remove agent scratch files (underscore-prefixed) so they don't accumulate on disk.
	deleteUnderscoreFiles(publishedPath)
	manifest, err := buildManifestFromDir(publishedPath)
	if err == nil {
		_ = m.writeManifest(root, manifest)
	}
	now := time.Now().UTC()
	_ = m.touchUpdated(root, now)
	return now, nil
}

func (m *Manager) DiscardDraft(id string) error {
	root, err := m.workbenchRoot(id)
	if err != nil {
		return err
	}
	draftState, _ := m.DraftState(id)
	_ = os.RemoveAll(filepath.Join(root, "draft"))
	_ = os.Remove(filepath.Join(root, metaFolder, "draft.json"))
	if draftState != nil && draftState.DraftID != "" {
		_ = os.RemoveAll(filepath.Join(root, metaFolder, "review", draftState.DraftID))
	}
	return nil
}

func (m *Manager) ChangeSet(id string) ([]Change, error) {
	root, err := m.workbenchRoot(id)
	if err != nil {
		return nil, err
	}
	published := filepath.Join(root, "published")
	draft := filepath.Join(root, "draft")
	publishedFiles, err := listFiles(published)
	if err != nil {
		return nil, err
	}
	draftFiles, err := listFiles(draft)
	if err != nil {
		return nil, err
	}
	changes := []Change{}
	for path, draftHash := range draftFiles {
		pubHash, ok := publishedFiles[path]
		if !ok {
			changes = append(changes, Change{Path: path, ChangeType: "added"})
			continue
		}
		if pubHash != draftHash {
			changes = append(changes, Change{Path: path, ChangeType: "modified"})
		}
	}
	for path := range publishedFiles {
		if _, ok := draftFiles[path]; !ok {
			return nil, ErrDeletionDetected
		}
	}
	return changes, nil
}

func (m *Manager) ReadFile(id, area, relPath string) (string, error) {
	data, err := m.ReadFileBytes(id, area, relPath)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (m *Manager) ReadFileBytes(id, area, relPath string) ([]byte, error) {
	root, err := m.workbenchRoot(id)
	if err != nil {
		return nil, err
	}
	clean := filepath.Clean(relPath)
	if strings.HasPrefix(clean, "..") || filepath.IsAbs(clean) {
		return nil, ErrInvalidPath
	}
	if filepath.Dir(clean) != "." {
		return nil, ErrInvalidPath
	}
	base := filepath.Join(root, area)
	full := filepath.Join(base, clean)
	if !strings.HasPrefix(full, base+string(filepath.Separator)) {
		return nil, ErrSandboxViolation
	}
	data, err := os.ReadFile(full)
	if err != nil {
		return nil, err
	}
	return data, nil
}

func (m *Manager) StatFile(id, area, relPath string) (os.FileInfo, error) {
	root, err := m.workbenchRoot(id)
	if err != nil {
		return nil, err
	}
	clean := filepath.Clean(relPath)
	if strings.HasPrefix(clean, "..") || filepath.IsAbs(clean) {
		return nil, ErrInvalidPath
	}
	if filepath.Dir(clean) != "." {
		return nil, ErrInvalidPath
	}
	base := filepath.Join(root, area)
	full := filepath.Join(base, clean)
	if !strings.HasPrefix(full, base+string(filepath.Separator)) {
		return nil, ErrSandboxViolation
	}
	info, err := os.Stat(full)
	if err != nil {
		return nil, err
	}
	return info, nil
}

func (m *Manager) workbenchRoot(id string) (string, error) {
	if id == "" || strings.ContainsAny(id, string(filepath.Separator)+"\\") {
		return "", errors.New("invalid workbench id")
	}
	root := filepath.Join(m.baseDir, id)
	return root, nil
}

func validateAreaName(name string) error {
	if name == "" {
		return ErrInvalidPath
	}
	if strings.ContainsAny(name, string(filepath.Separator)+"\\") {
		return ErrInvalidPath
	}
	if strings.Contains(name, "..") {
		return ErrInvalidPath
	}
	return nil
}

func validateFlatFilePath(path string) error {
	if path == "" || filepath.IsAbs(path) {
		return ErrInvalidPath
	}
	if filepath.Base(path) != path {
		return ErrInvalidPath
	}
	if strings.Contains(path, "\\") {
		return ErrInvalidPath
	}
	return nil
}

func (m *Manager) readManifest(root string) (*FileManifest, error) {
	var manifest FileManifest
	path := filepath.Join(root, metaFolder, "files.json")
	if err := readJSON(path, &manifest); err != nil {
		if os.IsNotExist(err) {
			return &FileManifest{SchemaVersion: schema, Files: []FileEntry{}}, nil
		}
		return nil, err
	}
	migrated := migrateManifest(&manifest)
	if migrated {
		if err := m.writeManifest(root, &manifest); err != nil {
			return nil, err
		}
	}
	return &manifest, nil
}

func (m *Manager) writeManifest(root string, manifest *FileManifest) error {
	manifest.SchemaVersion = schema
	return writeJSON(filepath.Join(root, metaFolder, "files.json"), manifest)
}

func (m *Manager) touchUpdated(root string, at time.Time) error {
	path := filepath.Join(root, metaFolder, "workbench.json")
	var wb Workbench
	if err := readJSON(path, &wb); err != nil {
		return err
	}
	wb.UpdatedAt = at.Format(time.RFC3339)
	return writeJSON(path, &wb)
}

func readJSON(path string, dest interface{}) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, dest)
}

func readDraftStateFile(path string) (*DraftState, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var state DraftState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}
	if state.SourceKind == "" || state.SourceRef == "" {
		var legacy struct {
			SourceKind string `json:"source_kind"`
			SourceRef  string `json:"source_ref"`
			Source     struct {
				Kind  string `json:"kind"`
				Ref   string `json:"ref"`
				JobID string `json:"job_id"`
			} `json:"source"`
		}
		if err := json.Unmarshal(data, &legacy); err == nil {
			if state.SourceKind == "" {
				if trimmed := strings.TrimSpace(legacy.SourceKind); trimmed != "" {
					state.SourceKind = trimmed
				} else if trimmed := strings.TrimSpace(legacy.Source.Kind); trimmed != "" {
					state.SourceKind = trimmed
				}
			}
			if state.SourceRef == "" {
				switch {
				case strings.TrimSpace(legacy.SourceRef) != "":
					state.SourceRef = strings.TrimSpace(legacy.SourceRef)
				case strings.TrimSpace(legacy.Source.Ref) != "":
					state.SourceRef = strings.TrimSpace(legacy.Source.Ref)
				case strings.TrimSpace(legacy.Source.JobID) != "":
					state.SourceRef = strings.TrimSpace(legacy.Source.JobID)
				}
			}
		}
	}
	return &state, nil
}

func writeJSON(path string, payload interface{}) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

func newID() string {
	buf := make([]byte, 8)
	_, _ = rand.Read(buf)
	return hex.EncodeToString(buf)
}

func copyFile(src, dest string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Sync()
}

func uniqueExtractDestination(destinationDir, sourcePath string) (string, string, error) {
	candidate := sourcePath
	for i := 0; ; i++ {
		if i > 0 {
			stem, ext := splitExtractFileName(sourcePath)
			candidate = fmt.Sprintf("%s(%d)%s", stem, i, ext)
		}
		dest := filepath.Join(destinationDir, candidate)
		if _, err := os.Stat(dest); err == nil {
			continue
		} else if os.IsNotExist(err) {
			return dest, candidate, nil
		} else {
			return "", "", err
		}
	}
}

func splitExtractFileName(name string) (stem, ext string) {
	if strings.HasPrefix(name, ".") && strings.Count(name, ".") == 1 {
		return name, ""
	}
	ext = filepath.Ext(name)
	if ext == "" {
		return name, ""
	}
	stem = strings.TrimSuffix(name, ext)
	if stem == "" {
		return name, ""
	}
	return stem, ext
}

func copyDir(src, dest string) error {
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() {
			if err := copyDir(filepath.Join(src, entry.Name()), filepath.Join(dest, entry.Name())); err != nil {
				return err
			}
			continue
		}
		if err := copyFile(filepath.Join(src, entry.Name()), filepath.Join(dest, entry.Name())); err != nil {
			return err
		}
	}
	return nil
}

func atomicWrite(path string, data []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".tmp")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(name, path)
}

func listFiles(root string) (map[string]string, error) {
	results := make(map[string]string)
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		// Skip agent-internal scratch files — same convention as buildManifestFromDir.
		if strings.HasPrefix(entry.Name(), "_") {
			continue
		}
		path := entry.Name()
		data, err := os.ReadFile(filepath.Join(root, path))
		if err != nil {
			return nil, err
		}
		hash := sha256.Sum256(data)
		results[path] = hex.EncodeToString(hash[:])
	}
	return results, nil
}

// deleteUnderscoreFiles removes files whose names start with "_" from dir.
// These are agent-internal scratch files and should not persist after publish.
func deleteUnderscoreFiles(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasPrefix(entry.Name(), "_") {
			_ = os.Remove(filepath.Join(dir, entry.Name()))
		}
	}
}

func buildManifestFromDir(root string) (*FileManifest, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	manifest := &FileManifest{SchemaVersion: schema}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		fileKind, mimeType, isOpaque := classifyPath(entry.Name())
		manifest.Files = append(manifest.Files, FileEntry{
			Path:       entry.Name(),
			Size:       info.Size(),
			ModifiedAt: info.ModTime().UTC().Format(time.RFC3339),
			AddedAt:    time.Now().UTC().Format(time.RFC3339),
			FileKind:   fileKind,
			MimeType:   mimeType,
			IsOpaque:   isOpaque,
		})
	}
	return manifest, nil
}

func migrateManifest(manifest *FileManifest) bool {
	changed := false
	for i := range manifest.Files {
		entry := &manifest.Files[i]
		expectedKind, expectedMime, expectedOpaque := classifyPath(entry.Path)
		hadIncompleteMetadata := entry.FileKind == "" || entry.MimeType == ""

		normalizedKind := normalizeLegacyFileKind(entry.Path, entry.FileKind)
		kindChanged := normalizedKind != entry.FileKind
		if kindChanged {
			entry.FileKind = normalizedKind
			changed = true
		}
		if entry.FileKind == "" {
			entry.FileKind = expectedKind
			kindChanged = true
			changed = true
		}

		if hadIncompleteMetadata || kindChanged {
			if entry.MimeType != expectedMime {
				entry.MimeType = expectedMime
				changed = true
			}
			if entry.IsOpaque != expectedOpaque {
				entry.IsOpaque = expectedOpaque
				changed = true
			}
		}
	}
	if manifest.SchemaVersion < schema {
		manifest.SchemaVersion = schema
		changed = true
	}
	return changed
}

func normalizeLegacyFileKind(path, kind string) string {
	normalized := strings.ToLower(strings.TrimSpace(kind))
	switch normalized {
	case "":
		return ""
	case FileKindText, FileKindDocx, FileKindOdt, FileKindXlsx, FileKindPptx, FileKindPdf, FileKindImage, FileKindBinary:
		return normalized
	}

	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(path)), ".")
	alias := strings.TrimPrefix(normalized, ".")
	if ext != "" && alias == ext {
		inferredKind, _, _ := classifyPath(path)
		return inferredKind
	}
	return kind
}

func classifyPath(path string) (string, string, bool) {
	ext := strings.ToLower(filepath.Ext(path))
	if ext == "" {
		return FileKindBinary, "application/octet-stream", true
	}
	if textWriteExtensions[ext] {
		return FileKindText, mimeTypeForExtension(ext), false
	}
	if kind, ok := officeExtensions[ext]; ok {
		return kind, mimeTypeForExtension(ext), false
	}
	if ext == ".pdf" {
		return FileKindPdf, mimeTypeForExtension(ext), false
	}
	if imageExtensions[ext] {
		return FileKindImage, mimeTypeForExtension(ext), false
	}
	return FileKindBinary, mimeTypeForExtension(ext), true
}

func mimeTypeForExtension(ext string) string {
	if ext == "" {
		return "application/octet-stream"
	}
	mimeType := mime.TypeByExtension(ext)
	if mimeType == "" {
		mimeType = mimeFallbacks[ext]
	}
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}
	if idx := strings.Index(mimeType, ";"); idx >= 0 {
		mimeType = strings.TrimSpace(mimeType[:idx])
	}
	return mimeType
}

func tabularCacheKey(path string) string {
	normalized := strings.TrimSpace(strings.ToLower(path))
	if normalized == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(normalized))
	return hex.EncodeToString(sum[:])
}

func (m *Manager) cleanupTabularArtifacts(root string, paths []string) {
	tabularDir := filepath.Join(root, metaFolder, "tabular")
	for _, path := range paths {
		key := tabularCacheKey(path)
		if key == "" {
			continue
		}
		for _, suffix := range []string{".duckdb", ".duckdb.wal", ".meta.json", ".json", ".utf8.csv"} {
			_ = os.Remove(filepath.Join(tabularDir, key+suffix))
		}
	}
}

func IsTextWritePath(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return textWriteExtensions[ext]
}

func FileKindForPath(path string) (string, bool) {
	kind, _, opaque := classifyPath(path)
	return kind, opaque
}
