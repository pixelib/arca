package p2

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// A p2 artifact repository advertises a mirror list and a statistics endpoint.
// Clients that honour them download bundles from an Eclipse mirror instead of
// the repository they loaded, which walks straight past this cache and tells
// eclipse.org what an internal network installs.
var strippedProperties = map[string]bool{
	"p2.mirrorsURL": true,
	"p2.statsURI":   true,
}

// RewriteArtifactDocument strips those properties from an artifact document in
// whichever encoding it arrived in.
func RewriteArtifactDocument(path string, document []byte) ([]byte, error) {
	if IsJar(path) {
		return rewriteJar(document, StripMirrors)
	}
	return StripMirrors(document)
}

func StripMirrors(document []byte) ([]byte, error) {
	decoder := xml.NewDecoder(bytes.NewReader(document))

	var out bytes.Buffer
	encoder := xml.NewEncoder(&out)

	depth := 0
	done := false

	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}

		if start, ok := token.(xml.StartElement); ok {
			depth++
			// Only the repository's own properties carry these; the per-artifact
			// blocks further down are left alone.
			if !done && depth == 2 && start.Name.Local == "properties" {
				filtered, err := filterProperties(decoder, start)
				if err != nil {
					return nil, err
				}
				for _, filteredToken := range filtered {
					if err := encoder.EncodeToken(filteredToken); err != nil {
						return nil, err
					}
				}
				depth--
				done = true
				continue
			}
		}
		if _, ok := token.(xml.EndElement); ok {
			depth--
		}

		if err := encoder.EncodeToken(token); err != nil {
			return nil, err
		}
	}

	if err := encoder.Flush(); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// filterProperties consumes a properties element and returns the tokens that
// should replace it, with the size attribute corrected for what was dropped.
func filterProperties(decoder *xml.Decoder, start xml.StartElement) ([]xml.Token, error) {
	body := []xml.Token{}
	removed := 0
	depth := 0

	for {
		token, err := decoder.Token()
		if err != nil {
			return nil, err
		}

		switch typed := token.(type) {
		case xml.StartElement:
			if depth == 0 && typed.Name.Local == "property" && strippedProperties[attribute(typed, "name")] {
				if err := decoder.Skip(); err != nil {
					return nil, err
				}
				removed++
				continue
			}
			depth++

		case xml.EndElement:
			if depth == 0 {
				head := []xml.Token{resized(start, removed)}
				return append(append(head, body...), xml.CopyToken(token)), nil
			}
			depth--
		}

		body = append(body, xml.CopyToken(token))
	}
}

func attribute(start xml.StartElement, name string) string {
	for _, candidate := range start.Attr {
		if candidate.Name.Local == name {
			return candidate.Value
		}
	}
	return ""
}

// resized keeps the size hint honest. p2's parser reads it to presize its own
// collections and complains when it does not match what follows.
func resized(start xml.StartElement, removed int) xml.StartElement {
	copied := start.Copy()
	if removed == 0 {
		return copied
	}
	for index, candidate := range copied.Attr {
		if candidate.Name.Local != "size" {
			continue
		}
		size, err := strconv.Atoi(candidate.Value)
		if err != nil {
			continue
		}
		copied.Attr[index].Value = strconv.Itoa(max(size-removed, 0))
	}
	return copied
}

func rewriteJar(document []byte, rewrite func([]byte) ([]byte, error)) ([]byte, error) {
	reader, err := zip.NewReader(bytes.NewReader(document), int64(len(document)))
	if err != nil {
		return nil, fmt.Errorf("read the artifact jar: %w", err)
	}

	var out bytes.Buffer
	writer := zip.NewWriter(&out)

	for _, file := range reader.File {
		content, err := readEntry(file)
		if err != nil {
			return nil, err
		}
		if strings.HasSuffix(file.Name, ".xml") {
			if content, err = rewrite(content); err != nil {
				return nil, err
			}
		}

		entry, err := writer.Create(file.Name)
		if err != nil {
			return nil, err
		}
		if _, err := entry.Write(content); err != nil {
			return nil, err
		}
	}

	if err := writer.Close(); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func readEntry(file *zip.File) ([]byte, error) {
	opened, err := file.Open()
	if err != nil {
		return nil, fmt.Errorf("open %s in the artifact jar: %w", file.Name, err)
	}
	defer opened.Close()

	return io.ReadAll(opened)
}
