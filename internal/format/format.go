package format

const (
	Maven2 = "maven2"
	NPM    = "npm"
)

type Coordinates struct {
	Namespace   string
	Name        string
	Version     string
	BaseVersion string
	IsSnapshot  bool
}

type Layout interface {
	Coordinates(path string) (Coordinates, bool)
	ComponentAt(prefix string) (Coordinates, bool)
	ContentType(filename string) string
	Mutable(path string) bool
	Compare(a, b string) int
	Prerelease(version string) bool
}
