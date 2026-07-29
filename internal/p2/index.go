package p2

import "strings"

const (
	artifactOrderKey = "artifact.repository.factory.order"
	orderTerminator  = "!"
)

// RewriteIndex removes the encodings this server refuses to serve from the
// artifact factory order.
//
// The entries name repository factories rather than files, and the factory
// '<name>.xml' already looks for '<name>.jar' before '<name>.xml'. So dropping
// the compressed candidate is the whole job: the remaining '.xml' factory still
// reaches the jar, and naming a jar directly would name a factory that does not
// exist.
func RewriteIndex(document []byte) []byte {
	lines := strings.Split(string(document), "\n")
	rewritten := make([]string, 0, len(lines))

	for _, line := range lines {
		body := strings.TrimSuffix(line, "\r")
		carriage := body != line

		key, value, found := strings.Cut(body, "=")
		if !found || strings.TrimSpace(key) != artifactOrderKey {
			rewritten = append(rewritten, line)
			continue
		}

		order := serveableOrder(value)
		if order == "" {
			// Nothing serveable is left to name, so the override goes entirely
			// and the client falls back on its own defaults.
			continue
		}

		replaced := strings.TrimSpace(key) + "=" + order
		if carriage {
			replaced += "\r"
		}
		rewritten = append(rewritten, replaced)
	}

	return []byte(strings.Join(rewritten, "\n"))
}

func serveableOrder(value string) string {
	kept := make([]string, 0, len(value))
	factories := 0

	for _, candidate := range strings.Split(value, ",") {
		trimmed := strings.TrimSpace(candidate)
		if compressedArtifactDocuments[trimmed] {
			continue
		}
		if !isTerminator(trimmed) {
			factories++
		}
		kept = append(kept, trimmed)
	}

	if factories == 0 {
		return ""
	}
	return strings.Join(kept, ",")
}

// isTerminator matches both spellings of the stop marker. p2.index is a Java
// properties file, where '!' opens a comment, so it is often escaped.
func isTerminator(candidate string) bool {
	return candidate == orderTerminator || candidate == `\`+orderTerminator
}
