package p2

import "strings"

const IndexFilename = "p2.index"

const (
	ClassifierBundle  = "osgi.bundle"
	ClassifierFeature = "org.eclipse.update.feature"
	ClassifierBinary  = "binary"
)

// artifactDirectories mirrors the mapping rules every simple p2 repository
// publishes, which address artifacts as ${repoUrl}/<directory>/${id}_${version}.
var artifactDirectories = map[string]string{
	"plugins":  ClassifierBundle,
	"features": ClassifierFeature,
	"binary":   ClassifierBinary,
}

var metadataFilenames = map[string]bool{
	IndexFilename:            true,
	"content.jar":            true,
	"content.xml":            true,
	"content.xml.xz":         true,
	"artifacts.jar":          true,
	"artifacts.xml":          true,
	"artifacts.xml.xz":       true,
	"compositeContent.jar":   true,
	"compositeContent.xml":   true,
	"compositeArtifacts.jar": true,
	"compositeArtifacts.xml": true,
}

var artifactDocuments = map[string]bool{
	"artifacts.jar":          true,
	"artifacts.xml":          true,
	"compositeArtifacts.jar": true,
	"compositeArtifacts.xml": true,
}

// compressedArtifactDocuments are the encodings we refuse to serve, because
// stripping the mirror properties out of them would mean carrying an xz codec.
// RewriteIndex steers clients onto the encodings we can rewrite.
var compressedArtifactDocuments = map[string]bool{
	"artifacts.xml.xz": true,
}

func Filename(path string) string {
	if index := strings.LastIndex(path, "/"); index >= 0 {
		return path[index+1:]
	}
	return path
}

// A repository can sit at any depth of a proxied host, so every classification
// below looks at the tail of the path rather than its root.
func IsMetadata(path string) bool { return metadataFilenames[Filename(path)] }

func IsIndex(path string) bool { return Filename(path) == IndexFilename }

func IsArtifactDocument(path string) bool { return artifactDocuments[Filename(path)] }

func IsCompressedArtifactDocument(path string) bool {
	return compressedArtifactDocuments[Filename(path)]
}

func IsJar(path string) bool { return strings.HasSuffix(Filename(path), ".jar") }

type Artifact struct {
	Classifier string
	ID         string
	Version    string
}

// ParseArtifact reads the coordinates out of a bundle, feature or binary path.
// Ids may contain dots and dashes but never underscores, so the last underscore
// is the separator.
func ParseArtifact(path string) (Artifact, bool) {
	segments := strings.Split(path, "/")
	if len(segments) < 2 {
		return Artifact{}, false
	}

	classifier, ok := artifactDirectories[segments[len(segments)-2]]
	if !ok {
		return Artifact{}, false
	}

	filename := segments[len(segments)-1]
	if classifier != ClassifierBinary {
		trimmed, isJar := strings.CutSuffix(filename, ".jar")
		if !isJar {
			return Artifact{}, false
		}
		filename = trimmed
	}

	separator := strings.LastIndex(filename, "_")
	if separator <= 0 || separator == len(filename)-1 {
		return Artifact{}, false
	}

	return Artifact{
		Classifier: classifier,
		ID:         filename[:separator],
		Version:    filename[separator+1:],
	}, true
}
