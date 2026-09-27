package files

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// DirEntry is one browseable entry returned by ListDirectory.
type DirEntry struct {
	Name string
	Path string
	Dir  bool
}

// ListDirectory returns the entries of path for the restore file picker:
// directories first (so they are easy to enter), then dump files (.zip/.dump),
// each group sorted by name. Hidden entries are skipped.
func ListDirectory(path string) ([]DirEntry, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}

	dirs := make([]DirEntry, 0, len(entries))
	files := make([]DirEntry, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		full := filepath.Join(path, name)
		if entry.IsDir() {
			dirs = append(dirs, DirEntry{Name: name, Path: full, Dir: true})
			continue
		}
		switch strings.ToLower(filepath.Ext(name)) {
		case ".zip", ".dump":
			files = append(files, DirEntry{Name: name, Path: full})
		}
	}
	sort.Slice(dirs, func(i, j int) bool { return dirs[i].Name < dirs[j].Name })
	sort.Slice(files, func(i, j int) bool { return files[i].Name < files[j].Name })

	entriesOut := make([]DirEntry, 0, len(dirs)+len(files))
	entriesOut = append(entriesOut, dirs...)
	entriesOut = append(entriesOut, files...)
	return entriesOut, nil
}
