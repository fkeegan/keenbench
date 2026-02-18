package envfile

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

type Result struct {
	Path   string
	Loaded bool
	Keys   int
	Err    error
}

func Load() Result {
	if override := strings.TrimSpace(os.Getenv("KEENBENCH_ENV_PATH")); override != "" {
		return LoadPath(override)
	}
	cwd, err := os.Getwd()
	if err != nil {
		return Result{Err: err}
	}
	path := findUpwards(cwd, ".env")
	if path == "" {
		return Result{}
	}
	return LoadPath(path)
}

func LoadPath(path string) Result {
	res := Result{Path: path}
	file, err := os.Open(path)
	if err != nil {
		res.Err = err
		return res
	}
	defer file.Close()
	res.Loaded = true
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "export ") {
			line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
		}
		key, value, ok := splitLine(line)
		if !ok {
			continue
		}
		if _, exists := os.LookupEnv(key); exists {
			continue
		}
		if err := os.Setenv(key, value); err != nil {
			res.Err = err
			return res
		}
		res.Keys++
	}
	if err := scanner.Err(); err != nil {
		res.Err = err
	}
	return res
}

func splitLine(line string) (string, string, bool) {
	idx := strings.Index(line, "=")
	if idx <= 0 {
		return "", "", false
	}
	key := strings.TrimSpace(line[:idx])
	if key == "" {
		return "", "", false
	}
	value := strings.TrimSpace(line[idx+1:])
	value = stripQuotes(value)
	return key, value, true
}

func stripQuotes(value string) string {
	if len(value) < 2 {
		return value
	}
	first := value[0]
	last := value[len(value)-1]
	if (first == '"' && last == '"') || (first == '\'' && last == '\'') {
		return value[1 : len(value)-1]
	}
	return value
}

func findUpwards(start, filename string) string {
	dir := start
	for {
		candidate := filepath.Join(dir, filename)
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}
