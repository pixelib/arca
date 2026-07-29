package p2

import "strings"

func ContentTypeFor(filename string) string {
	switch {
	case filename == IndexFilename:
		return "text/plain"
	case strings.HasSuffix(filename, ".jar"):
		return "application/java-archive"
	case strings.HasSuffix(filename, ".xml"):
		return "application/xml"
	case strings.HasSuffix(filename, ".xz"):
		return "application/x-xz"
	case strings.HasSuffix(filename, ".zip"):
		return "application/zip"
	case strings.HasSuffix(filename, ".gz"):
		return "application/gzip"
	default:
		return "application/octet-stream"
	}
}
