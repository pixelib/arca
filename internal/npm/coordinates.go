package npm

import (
	"strings"

	"arca/internal/format"
)

const (
	ManifestFilename  = "package.json"
	PackumentFilename = "packument.json"
	TarballSuffix     = ".tgz"
	reservedSegment   = "-"
)

// SplitPackage separates "@types/node" into its scope and bare name. An
// unscoped package has an empty scope.
func SplitPackage(full string) (namespace string, name string, ok bool) {
	if !strings.HasPrefix(full, "@") {
		if full == "" || strings.Contains(full, "/") {
			return "", "", false
		}
		return "", full, isValidSegment(full)
	}

	rest := full[1:]
	slash := strings.IndexByte(rest, '/')
	if slash <= 0 {
		return "", "", false
	}

	namespace, name = rest[:slash], rest[slash+1:]
	if !isValidSegment(namespace) || !isValidSegment(name) {
		return "", "", false
	}
	return namespace, name, true
}

func PackageName(namespace, name string) string {
	if namespace == "" {
		return name
	}
	return "@" + namespace + "/" + name
}

func isValidSegment(segment string) bool {
	if segment == "" || len(segment) > 200 || strings.HasPrefix(segment, ".") {
		return false
	}
	if segment == reservedSegment || strings.HasPrefix(segment, reservedSegment) {
		return false
	}
	return !strings.ContainsAny(segment, "/\\@ \t\r\n")
}

// TarballFilename is what npm asks for over HTTP, which drops the scope.
func TarballFilename(name, version string) string {
	return name + "-" + version + TarballSuffix
}

// ComponentDirectory holds every file of one published version. Storage is laid
// out by version rather than by npm's "-" tarball convention so that deleting,
// browsing and measuring a version stay single path-prefix operations.
func ComponentDirectory(namespace, name, version string) string {
	return PackageName(namespace, name) + "/" + version
}

func TarballPath(namespace, name, version string) string {
	return ComponentDirectory(namespace, name, version) + "/" + TarballFilename(name, version)
}

func ManifestPath(namespace, name, version string) string {
	return ComponentDirectory(namespace, name, version) + "/" + ManifestFilename
}

// PackumentPath caches a proxied upstream document beside the version
// directories of the package it describes. A version can never be named
// packument.json, so the cache and the published layout cannot collide.
func PackumentPath(namespace, name string) string {
	return PackageName(namespace, name) + "/" + PackumentFilename
}

func RemotePackumentPath(namespace, name string) string {
	return PackageName(namespace, name)
}

// RemoteTarballPath assumes the upstream lays tarballs out the way the registry
// API specifies. Registries that publish tarballs under some other path are
// reachable only through the URLs in their packument, which this server
// replaces with its own.
func RemoteTarballPath(namespace, name, version string) string {
	return PackageName(namespace, name) + "/" + reservedSegment + "/" + TarballFilename(name, version)
}

func splitStoragePath(path string) (namespace, name string, rest []string, ok bool) {
	segments := strings.Split(strings.TrimSuffix(path, "/"), "/")

	if strings.HasPrefix(segments[0], "@") {
		if len(segments) < 2 {
			return "", "", nil, false
		}
		namespace, name, ok = SplitPackage(segments[0] + "/" + segments[1])
		return namespace, name, segments[2:], ok
	}

	namespace, name, ok = SplitPackage(segments[0])
	return namespace, name, segments[1:], ok
}

type Layout struct{}

func (Layout) Coordinates(path string) (format.Coordinates, bool) {
	namespace, name, rest, ok := splitStoragePath(path)
	if !ok || len(rest) != 2 {
		return format.Coordinates{}, false
	}

	version := rest[0]
	if !IsValidVersion(version) {
		return format.Coordinates{}, false
	}
	if rest[1] != ManifestFilename && rest[1] != TarballFilename(name, version) {
		return format.Coordinates{}, false
	}

	return format.Coordinates{
		Namespace:   namespace,
		Name:        name,
		Version:     version,
		BaseVersion: Core(version),
		IsSnapshot:  IsPrerelease(version),
	}, true
}

func (Layout) ComponentAt(prefix string) (format.Coordinates, bool) {
	namespace, name, rest, ok := splitStoragePath(prefix)
	if !ok || len(rest) != 0 {
		return format.Coordinates{}, false
	}
	return format.Coordinates{Namespace: namespace, Name: name}, true
}

func (Layout) ContentType(filename string) string { return ContentTypeFor(filename) }

// Mutable covers only cached upstream documents. Everything with coordinates is
// a published version, which npm treats as immutable.
func (l Layout) Mutable(path string) bool {
	_, ok := l.Coordinates(path)
	return !ok
}

func (Layout) Compare(a, b string) int { return Compare(a, b) }

func (Layout) Prerelease(version string) bool { return IsPrerelease(version) }
