package p2

import (
	"archive/zip"
	"bytes"
	"io"
	"strings"
	"testing"
)

const eclipseArtifacts = `<?xml version='1.0' encoding='UTF-8'?>
<?artifactRepository version='1.1.0'?>
<repository name='2024-12' type='org.eclipse.equinox.p2.artifact.repository.simpleRepository' version='1.0.0'>
  <properties size='4'>
    <property name='p2.compressed' value='true'/>
    <property name='p2.timestamp' value='1732778560399'/>
    <property name='p2.mirrorsURL' value='https://www.eclipse.org/downloads/download.php?format=xml&amp;file=/releases/2024-12'/>
    <property name='p2.statsURI' value='https://download.eclipse.org/stats/releases/2024-12'/>
  </properties>
  <mappings size='1'>
    <rule filter='(&amp; (classifier=osgi.bundle))' output='${repoUrl}/plugins/${id}_${version}.jar'/>
  </mappings>
  <artifacts size='1'>
    <artifact classifier='osgi.bundle' id='org.eclipse.jgit' version='7.1.0'>
      <properties size='2'>
        <property name='artifact.size' value='317955'/>
        <property name='p2.statsURI' value='https://example.invalid/per-artifact'/>
      </properties>
    </artifact>
  </artifacts>
</repository>
`

func TestStripMirrors(t *testing.T) {
	stripped, err := StripMirrors([]byte(eclipseArtifacts))
	if err != nil {
		t.Fatalf("StripMirrors() failed: %v", err)
	}
	document := string(stripped)

	if strings.Contains(document, "p2.mirrorsURL") {
		t.Error("the mirrors property survived")
	}
	if strings.Contains(document, "download.php") {
		t.Error("the mirrors URL survived")
	}
	if !strings.Contains(document, "p2.timestamp") {
		t.Error("an unrelated repository property was dropped")
	}
	if !strings.Contains(document, `${repoUrl}/plugins/${id}_${version}.jar`) {
		t.Error("the mapping rule was altered")
	}
	// Only the repository's own properties are stripped; the per-artifact block
	// is data about the artifact and gets copied through.
	if !strings.Contains(document, "https://example.invalid/per-artifact") {
		t.Error("a per-artifact property was dropped")
	}
	if !strings.Contains(document, `size="2"`) {
		t.Errorf("the repository properties size was not corrected from 4 to 2:\n%s", document)
	}
}

func TestStripMirrorsLeavesACleanDocumentAlone(t *testing.T) {
	clean := `<?xml version='1.0' encoding='UTF-8'?>
<repository name='local'>
  <properties size='1'>
    <property name='p2.timestamp' value='1'/>
  </properties>
</repository>
`
	stripped, err := StripMirrors([]byte(clean))
	if err != nil {
		t.Fatalf("StripMirrors() failed: %v", err)
	}
	if !strings.Contains(string(stripped), `size="1"`) {
		t.Errorf("the size was changed with nothing removed:\n%s", stripped)
	}
}

func TestRewriteArtifactDocument(t *testing.T) {
	cases := []struct {
		name string
		path string
		jar  bool
	}{
		{name: "bare xml", path: "releases/2024-12/artifacts.xml"},
		{name: "composite xml", path: "releases/2024-12/compositeArtifacts.xml"},
		{name: "jar", path: "releases/2024-12/artifacts.jar", jar: true},
		{name: "composite jar", path: "releases/2024-12/compositeArtifacts.jar", jar: true},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			document := []byte(eclipseArtifacts)
			if test.jar {
				document = jarOf(t, "artifacts.xml", eclipseArtifacts)
			}

			rewritten, err := RewriteArtifactDocument(test.path, document)
			if err != nil {
				t.Fatalf("RewriteArtifactDocument() failed: %v", err)
			}

			inner := string(rewritten)
			if test.jar {
				inner = unjarOf(t, rewritten, "artifacts.xml")
			}
			if strings.Contains(inner, "p2.mirrorsURL") {
				t.Error("the mirrors property survived")
			}
			if !strings.Contains(inner, "org.eclipse.jgit") {
				t.Error("the artifact list was lost")
			}
		})
	}
}

func jarOf(t *testing.T, name, content string) []byte {
	t.Helper()

	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)

	entry, err := writer.Create(name)
	if err != nil {
		t.Fatalf("creating the jar entry failed: %v", err)
	}
	if _, err := entry.Write([]byte(content)); err != nil {
		t.Fatalf("writing the jar entry failed: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("closing the jar failed: %v", err)
	}

	return buffer.Bytes()
}

func unjarOf(t *testing.T, document []byte, name string) string {
	t.Helper()

	reader, err := zip.NewReader(bytes.NewReader(document), int64(len(document)))
	if err != nil {
		t.Fatalf("the rewritten jar does not open: %v", err)
	}
	for _, file := range reader.File {
		if file.Name != name {
			continue
		}
		opened, err := file.Open()
		if err != nil {
			t.Fatalf("opening %s failed: %v", name, err)
		}
		defer opened.Close()

		content, err := io.ReadAll(opened)
		if err != nil {
			t.Fatalf("reading %s failed: %v", name, err)
		}
		return string(content)
	}

	t.Fatalf("%s is missing from the rewritten jar", name)
	return ""
}
