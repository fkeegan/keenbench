package workbench

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type CheckpointMetadata struct {
	CheckpointID string          `json:"checkpoint_id"`
	CreatedAt    string          `json:"created_at"`
	Reason       string          `json:"reason"`
	Description  string          `json:"description,omitempty"`
	Stats        CheckpointStats `json:"stats,omitempty"`
}

type CheckpointStats struct {
	Files      int   `json:"files"`
	TotalBytes int64 `json:"total_bytes"`
}

type restoreMarker struct {
	CheckpointID   string `json:"checkpoint_id"`
	PreRestoreID   string `json:"pre_restore_checkpoint_id"`
	CreatedAt      string `json:"created_at"`
	PublishedPrev  string `json:"published_prev,omitempty"`
	MetaRestoreTmp string `json:"meta_restore_tmp,omitempty"`
}

func (m *Manager) checkpointsRoot(id string) (string, error) {
	root, err := m.workbenchRoot(id)
	if err != nil {
		return "", err
	}
	return filepath.Join(root, metaFolder, "checkpoints"), nil
}

func (m *Manager) CheckpointsList(id string) ([]CheckpointMetadata, error) {
	root, err := m.checkpointsRoot(id)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return []CheckpointMetadata{}, nil
		}
		return nil, err
	}
	var results []CheckpointMetadata
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		path := filepath.Join(root, entry.Name())
		var meta CheckpointMetadata
		if err := readJSON(path, &meta); err != nil {
			continue
		}
		results = append(results, meta)
	}
	sort.Slice(results, func(i, j int) bool {
		return results[i].CreatedAt > results[j].CreatedAt
	})
	return results, nil
}

func (m *Manager) CheckpointGet(id, checkpointID string) (*CheckpointMetadata, error) {
	root, err := m.checkpointsRoot(id)
	if err != nil {
		return nil, err
	}
	path := filepath.Join(root, fmt.Sprintf("%s.json", checkpointID))
	var meta CheckpointMetadata
	if err := readJSON(path, &meta); err != nil {
		return nil, err
	}
	return &meta, nil
}

func (m *Manager) CheckpointCreate(id, reason, description string) (string, error) {
	root, err := m.workbenchRoot(id)
	if err != nil {
		return "", err
	}
	checkpointsRoot, err := m.checkpointsRoot(id)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(checkpointsRoot, 0o755); err != nil {
		return "", err
	}
	checkpointID := newID()
	checkpointDir := filepath.Join(checkpointsRoot, checkpointID)
	publishedSnapshot := filepath.Join(checkpointDir, "published_snapshot")
	metaSnapshot := filepath.Join(checkpointDir, "meta_snapshot")
	if err := os.MkdirAll(publishedSnapshot, 0o755); err != nil {
		return "", err
	}
	if err := os.MkdirAll(metaSnapshot, 0o755); err != nil {
		return "", err
	}
	if err := snapshotDir(filepath.Join(root, "published"), publishedSnapshot); err != nil {
		return "", err
	}
	if err := snapshotMeta(filepath.Join(root, metaFolder), metaSnapshot); err != nil {
		return "", err
	}
	stats := dirStats(publishedSnapshot)
	meta := CheckpointMetadata{
		CheckpointID: checkpointID,
		CreatedAt:    time.Now().UTC().Format(time.RFC3339),
		Reason:       reason,
		Description:  description,
		Stats:        stats,
	}
	if err := writeJSON(filepath.Join(checkpointsRoot, fmt.Sprintf("%s.json", checkpointID)), meta); err != nil {
		return "", err
	}
	m.pruneCheckpoints(id)
	return checkpointID, nil
}

func (m *Manager) CheckpointRestore(id, checkpointID string, preRestoreID string) error {
	root, err := m.workbenchRoot(id)
	if err != nil {
		return err
	}
	checkpointsRoot, err := m.checkpointsRoot(id)
	if err != nil {
		return err
	}
	checkpointDir := filepath.Join(checkpointsRoot, checkpointID)
	publishedSnapshot := filepath.Join(checkpointDir, "published_snapshot")
	metaSnapshot := filepath.Join(checkpointDir, "meta_snapshot")
	if _, err := os.Stat(publishedSnapshot); err != nil {
		return err
	}
	if _, err := os.Stat(metaSnapshot); err != nil {
		return err
	}
	publishedTmp := filepath.Join(root, "published.restore_tmp")
	metaTmp := filepath.Join(root, metaFolder, "restore_tmp")
	_ = os.RemoveAll(publishedTmp)
	_ = os.RemoveAll(metaTmp)
	if err := snapshotDir(publishedSnapshot, publishedTmp); err != nil {
		return err
	}
	if err := snapshotDir(metaSnapshot, metaTmp); err != nil {
		return err
	}
	marker := restoreMarker{
		CheckpointID:   checkpointID,
		PreRestoreID:   preRestoreID,
		CreatedAt:      time.Now().UTC().Format(time.RFC3339),
		PublishedPrev:  filepath.Join(root, "published.prev"),
		MetaRestoreTmp: metaTmp,
	}
	if err := writeJSON(filepath.Join(root, metaFolder, "restore.json"), marker); err != nil {
		return err
	}
	publishedPath := filepath.Join(root, "published")
	publishedPrev := filepath.Join(root, "published.prev")
	_ = os.RemoveAll(publishedPrev)
	if err := os.Rename(publishedPath, publishedPrev); err != nil {
		return err
	}
	if err := os.Rename(publishedTmp, publishedPath); err != nil {
		_ = os.Rename(publishedPrev, publishedPath)
		return err
	}
	// swap meta
	if err := replaceMeta(filepath.Join(root, metaFolder), metaTmp); err != nil {
		return err
	}
	_ = os.RemoveAll(publishedPrev)
	_ = os.RemoveAll(metaTmp)
	_ = os.Remove(filepath.Join(root, metaFolder, "restore.json"))
	return nil
}

func (m *Manager) CheckpointRestorePublished(id, checkpointID string) error {
	root, err := m.workbenchRoot(id)
	if err != nil {
		return err
	}
	checkpointsRoot, err := m.checkpointsRoot(id)
	if err != nil {
		return err
	}
	checkpointDir := filepath.Join(checkpointsRoot, checkpointID)
	publishedSnapshot := filepath.Join(checkpointDir, "published_snapshot")
	if _, err := os.Stat(publishedSnapshot); err != nil {
		return err
	}
	publishedTmp := filepath.Join(root, "published.restore_tmp")
	_ = os.RemoveAll(publishedTmp)
	if err := snapshotDir(publishedSnapshot, publishedTmp); err != nil {
		return err
	}
	publishedPath := filepath.Join(root, "published")
	publishedPrev := filepath.Join(root, "published.prev")
	_ = os.RemoveAll(publishedPrev)
	if err := os.Rename(publishedPath, publishedPrev); err != nil {
		return err
	}
	if err := os.Rename(publishedTmp, publishedPath); err != nil {
		_ = os.Rename(publishedPrev, publishedPath)
		return err
	}
	_ = os.RemoveAll(publishedPrev)
	return nil
}

func (m *Manager) pruneCheckpoints(id string) {
	checkpoints, err := m.CheckpointsList(id)
	if err != nil {
		return
	}
	const maxAuto = 200
	const maxManual = 50
	var auto []CheckpointMetadata
	var manual []CheckpointMetadata
	var publish string
	var preRestore string
	for _, cp := range checkpoints {
		switch cp.Reason {
		case "publish":
			publish = cp.CheckpointID
		case "pre_restore":
			preRestore = cp.CheckpointID
		case "manual":
			manual = append(manual, cp)
		default:
			auto = append(auto, cp)
		}
	}
	remove := func(list []CheckpointMetadata, keep int) {
		if len(list) <= keep {
			return
		}
		for i := keep; i < len(list); i++ {
			cpID := list[i].CheckpointID
			if cpID == publish || cpID == preRestore {
				continue
			}
			_ = m.deleteCheckpoint(id, cpID)
		}
	}
	remove(auto, maxAuto)
	remove(manual, maxManual)
}

func (m *Manager) deleteCheckpoint(workbenchID, checkpointID string) error {
	root, err := m.workbenchRoot(workbenchID)
	if err != nil {
		return err
	}
	checkpointsRoot := filepath.Join(root, metaFolder, "checkpoints")
	_ = os.RemoveAll(filepath.Join(checkpointsRoot, checkpointID))
	_ = os.Remove(filepath.Join(checkpointsRoot, fmt.Sprintf("%s.json", checkpointID)))
	return nil
}

func snapshotMeta(metaRoot, dest string) error {
	entries := []string{
		"workbench.json",
		"files.json",
		"conversation.jsonl",
		"workshop_state.json",
		"egress_consent.json",
		"workbench_events.jsonl",
		"egress_events.jsonl",
		"jobs",
	}
	for _, entry := range entries {
		src := filepath.Join(metaRoot, entry)
		if _, err := os.Stat(src); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return err
		}
		destPath := filepath.Join(dest, entry)
		info, err := os.Stat(src)
		if err != nil {
			return err
		}
		if info.IsDir() {
			if err := snapshotDir(src, destPath); err != nil {
				return err
			}
			continue
		}
		if err := copyOrLinkFile(src, destPath); err != nil {
			return err
		}
	}
	return nil
}

func snapshotDir(src, dest string) error {
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return err
	}
	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		destPath := filepath.Join(dest, entry.Name())
		if entry.IsDir() {
			if err := snapshotDir(srcPath, destPath); err != nil {
				return err
			}
			continue
		}
		if err := copyOrLinkFile(srcPath, destPath); err != nil {
			return err
		}
	}
	return nil
}

func copyOrLinkFile(src, dest string) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	if err := os.Link(src, dest); err == nil {
		return nil
	}
	return copyFile(src, dest)
}

func dirStats(root string) CheckpointStats {
	stats := CheckpointStats{}
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		stats.Files++
		stats.TotalBytes += info.Size()
		return nil
	})
	return stats
}

func replaceMeta(metaRoot, restoreTmp string) error {
	entries, err := os.ReadDir(restoreTmp)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		src := filepath.Join(restoreTmp, entry.Name())
		dest := filepath.Join(metaRoot, entry.Name())
		prev := dest + ".prev"
		_ = os.RemoveAll(prev)
		if _, err := os.Stat(dest); err == nil {
			if err := os.Rename(dest, prev); err != nil {
				return err
			}
		}
		if err := os.Rename(src, dest); err != nil {
			return err
		}
	}
	// cleanup prevs
	prevEntries, _ := os.ReadDir(metaRoot)
	for _, entry := range prevEntries {
		if strings.HasSuffix(entry.Name(), ".prev") {
			_ = os.RemoveAll(filepath.Join(metaRoot, entry.Name()))
		}
	}
	return nil
}

func (m *Manager) cleanupRestoreArtifacts() error {
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
		markerPath := filepath.Join(root, metaFolder, "restore.json")
		var marker restoreMarker
		if err := readJSON(markerPath, &marker); err != nil {
			continue
		}
		publishedPath := filepath.Join(root, "published")
		publishedPrev := filepath.Join(root, "published.prev")
		if _, err := os.Stat(publishedPath); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				if _, err := os.Stat(publishedPrev); err == nil {
					_ = os.Rename(publishedPrev, publishedPath)
				}
			}
		}
		_ = os.RemoveAll(filepath.Join(root, "published.restore_tmp"))
		_ = os.RemoveAll(filepath.Join(root, metaFolder, "restore_tmp"))
		_ = os.Remove(markerPath)
	}
	return nil
}
