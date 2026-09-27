package docker

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
)

// VersionOption describes an Odoo version offered by the workspace.
// Dockerfile reports whether `docker/Dockerfile.<version>` exists (the file run
// needs to build an image) and Image whether `lidoo-odoo:<version>` is already
// built.
type VersionOption struct {
	Version    string
	Dockerfile bool
	Image      bool
}

// knownVersions are always offered, even without a local Dockerfile, so the
// selector matches the versions operators expect.
var knownVersions = []string{"17", "18", "19"}

// VersionOptions lists the Odoo versions the workspace can target, newest
// first: the known versions plus every `docker/Dockerfile.<version>` found. A
// version without a Dockerfile is listed with Dockerfile=false so the UI can
// warn that it cannot be built yet.
func VersionOptions(ctx context.Context) ([]VersionOption, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	withDockerfile := make(map[string]bool)
	entries, err := os.ReadDir("docker")
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("read docker directory: %w", err)
	}
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
		withDockerfile[version] = true
	}

	known := make(map[string]bool)
	for _, version := range knownVersions {
		if odooVersion.MatchString(version) {
			known[version] = true
		}
	}
	for version := range withDockerfile {
		known[version] = true
	}

	versions := make([]string, 0, len(known))
	for version := range known {
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
			Version:    version,
			Dockerfile: withDockerfile[version],
			Image:      dockerCommandAvailableWithContext(ctx, "image", "inspect", "lidoo-odoo:"+version),
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
