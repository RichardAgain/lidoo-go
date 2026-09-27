package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// RestoreSources lists candidate dump files under backups/. It is a
// convenience for the chooser; restore still accepts any path the caller types.
// A missing backups/ directory is not an error.
func (s *Service) RestoreSources(ctx context.Context) ([]string, error) {
	if err := contextError(ctx); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir("backups")
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read backups directory: %w", err)
	}

	type candidate struct {
		path    string
		modTime int64
	}
	candidates := make([]candidate, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		switch strings.ToLower(filepath.Ext(name)) {
		case ".zip", ".dump":
		default:
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		candidates = append(candidates, candidate{
			path:    filepath.Join("backups", name),
			modTime: info.ModTime().UnixNano(),
		})
	}
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].modTime > candidates[j].modTime
	})

	sources := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		sources = append(sources, candidate.path)
	}
	if err := contextError(ctx); err != nil {
		return nil, err
	}
	return sources, nil
}
