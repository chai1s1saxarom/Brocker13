package broker

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type FileStore struct {
	baseDir string
}

func NewFileStore(baseDir string) (*FileStore, error) {
	if err := os.MkdirAll(baseDir, 0o755); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}
	return &FileStore{baseDir: baseDir}, nil
}

func (s *FileStore) LoadStreams() ([]*stream, error) {
	files, err := filepath.Glob(filepath.Join(s.baseDir, "*.json"))
	if err != nil {
		return nil, fmt.Errorf("list stream files: %w", err)
	}

	result := make([]*stream, 0, len(files))
	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			return nil, fmt.Errorf("read stream file %s: %w", f, err)
		}
		var st stream
		if err := json.Unmarshal(raw, &st); err != nil {
			return nil, fmt.Errorf("unmarshal stream file %s: %w", f, err)
		}
		st.rebuildIndexes()
		result = append(result, &st)
	}
	return result, nil
}

func (s *FileStore) SaveStream(st *stream) error {
	raw, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal stream: %w", err)
	}
	path := filepath.Join(s.baseDir, st.Name+".json")
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		return fmt.Errorf("write stream file: %w", err)
	}
	return nil
}
