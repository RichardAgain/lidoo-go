package docker

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
)

// VersionOption describes an Odoo version reachable from the current
// workspace. A version is selectable when a Dockerfile exists, and Image
// reports whether its local `lidoo-odoo:<version>` image has already been
// built.
type VersionOption struct {
	Version string
	Image   bool
}

// VersionOptions lists the Odoo versions the workspace can build, newest
// first. Versions are discovered from `docker/Dockerfile.<version>` files,
// which are the same files run uses to build a profile image.
func VersionOptions(ctx context.Context) ([]VersionOption, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	entries, err := os.ReadDir("docker")
	if err != nil {
		return nil, fmt.Errorf("read docker directory: %w", err)
	}
	found := make(map[string]bool)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasPrefix(name, "Dockerfile.") {
			continue
		}
		version := strings.TrimPrefix(name, "Dockerfile.")
		if !odooVersion.MatchString(version) {
			continue
		}
		found[version] = true
	}

	versions := make([]string, 0, len(found))
	for version := range found {
		versions = append(versions, version)
	}
	sort.Slice(versions, func(i, j int) bool {
		majorI, minorI := versionParts(versions[i])
		majorJ, minorJ := versionParts(versions[j])
		if majorI != majorJ {
			return majorI > majorJ
		}
		return minorI > minorJ
	})

	options := make([]VersionOption, 0, len(versions))
	for _, version := range versions {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		options = append(options, VersionOption{
			Version: version,
			Image:   dockerCommandAvailableWithContext(ctx, "image", "inspect", "lidoo-odoo:"+version),
		})
	}
	return options, nil
}

func versionParts(version string) (int, int) {
	parts := strings.SplitN(version, ".", 2)
	major, _ := strconv.Atoi(parts[0])
	minor := 0
	if len(parts) == 2 {
		minor, _ = strconv.Atoi(parts[1])
	}
	return major, minor
}
