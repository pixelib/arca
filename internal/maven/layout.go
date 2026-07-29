package maven

import (
	"strings"

	"arca/internal/format"
)

type Layout struct{}

func (Layout) Coordinates(path string) (format.Coordinates, bool) {
	parsed := ParsePath(path)
	if parsed.Kind != KindArtifact {
		return format.Coordinates{}, false
	}
	return format.Coordinates{
		Namespace:   parsed.GroupID,
		Name:        parsed.ArtifactID,
		Version:     parsed.Version,
		BaseVersion: parsed.BaseVersion,
		IsSnapshot:  parsed.IsSnapshot,
	}, true
}

func (Layout) ComponentAt(prefix string) (format.Coordinates, bool) {
	segments := strings.FieldsFunc(prefix, func(character rune) bool { return character == '/' })
	if len(segments) < 2 {
		return format.Coordinates{}, false
	}
	return format.Coordinates{
		Namespace: strings.Join(segments[:len(segments)-1], "."),
		Name:      segments[len(segments)-1],
	}, true
}

func (Layout) ContentType(filename string) string { return ContentTypeFor(filename) }

// Mutable reports whether an upstream can change a path under a stable name.
// Metadata and non-timestamped snapshots can, released artifacts and timestamped
// snapshot builds cannot, so those are cached until evicted.
func (Layout) Mutable(path string) bool {
	segments := strings.Split(path, "/")
	base, checksum := SplitChecksum(segments[len(segments)-1])
	if base == MetadataFilename {
		return true
	}
	if checksum != "" {
		path = path[:len(path)-len(checksum)-1]
	}

	parsed := ParsePath(path)
	return parsed.Kind == KindArtifact && parsed.IsSnapshot && parsed.File.Timestamp == ""
}

func (Layout) Compare(a, b string) int { return CompareVersions(a, b) }

func (Layout) Prerelease(version string) bool { return strings.HasSuffix(version, SnapshotSuffix) }
