package appdirs

import (
	"os"
	"path/filepath"
)

const (
	appDirName = "keenbench"
)

func DataDir() (string, error) {
	if override := os.Getenv("KEENBENCH_DATA_DIR"); override != "" {
		return override, nil
	}
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, appDirName), nil
}

func WorkbenchesDir(dataDir string) string {
	return filepath.Join(dataDir, "workbenches")
}
