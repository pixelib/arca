package p2

import "arca/internal/format"

type Layout struct{}

// Coordinates names a bundle, feature or binary by its classifier and id. The
// same version can appear in more than one site of a repository, and both files
// resolve to the one component, which is what they are.
func (Layout) Coordinates(path string) (format.Coordinates, bool) {
	artifact, ok := ParseArtifact(path)
	if !ok {
		return format.Coordinates{}, false
	}
	return format.Coordinates{
		Namespace:   artifact.Classifier,
		Name:        artifact.ID,
		Version:     artifact.Version,
		BaseVersion: artifact.Version,
	}, true
}

// ComponentAt reports nothing. A p2 directory holds every bundle of a site
// rather than the versions of one, so no browse prefix names a component.
func (Layout) ComponentAt(string) (format.Coordinates, bool) {
	return format.Coordinates{}, false
}

func (Layout) ContentType(filename string) string { return ContentTypeFor(filename) }

// Mutable reports whether an upstream can change a path under a stable name.
// Every p2 metadata document is republished in place, which is how a site like
// releases/latest moves; a bundle cannot be, because its version is part of the
// filename.
func (Layout) Mutable(path string) bool { return IsMetadata(path) }

func (Layout) Compare(a, b string) int { return CompareVersions(a, b) }

func (Layout) Prerelease(version string) bool { return Prerelease(version) }
