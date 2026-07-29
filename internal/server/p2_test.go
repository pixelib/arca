package server

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

const (
	p2IndexPath  = "releases/2024-12/202412041000/p2.index"
	p2ArtifactsP = "releases/2024-12/202412041000/artifacts.jar"
	p2ContentP   = "releases/2024-12/202412041000/content.jar"
	p2BundlePath = "releases/2024-12/202412041000/plugins/org.eclipse.jgit_7.1.0.202411261347-r.jar"
	p2CompositeP = "releases/2024-12/compositeContent.xml"
	p2EppPath    = "technology/epp/packages/2024-12/compositeContent.xml"
)

const p2Index = "version=1\n" +
	"metadata.repository.factory.order=content.xml.xz,content.xml,!\n" +
	"artifact.repository.factory.order=artifacts.xml.xz,artifacts.xml,!\n"

const p2Artifacts = `<?xml version='1.0' encoding='UTF-8'?>
<?artifactRepository version='1.1.0'?>
<repository name='202412041000' type='org.eclipse.equinox.p2.artifact.repository.simpleRepository' version='1.0.0'>
  <properties size='3'>
    <property name='p2.timestamp' value='1732778560399'/>
    <property name='p2.mirrorsURL' value='https://www.eclipse.org/downloads/download.php?format=xml&amp;file=/releases/2024-12'/>
    <property name='p2.statsURI' value='https://download.eclipse.org/stats/releases/2024-12'/>
  </properties>
  <mappings size='1'>
    <rule filter='(&amp; (classifier=osgi.bundle))' output='${repoUrl}/plugins/${id}_${version}.jar'/>
  </mappings>
  <artifacts size='1'>
    <artifact classifier='osgi.bundle' id='org.eclipse.jgit' version='7.1.0.202411261347-r'/>
  </artifacts>
</repository>
`

// The Eclipse release train points at a child above its own directory, which is
// why a p2 proxy is pinned to a host root.
const p2Composite = `<?xml version='1.0' encoding='UTF-8'?>
<repository name='Eclipse SimRel 2024-12' type='org.eclipse.equinox.internal.p2.metadata.repository.CompositeMetadataRepository'>
  <children size='2'>
    <child location='../../technology/epp/packages/2024-12/'/>
    <child location='202412041000'/>
  </children>
</repository>
`

const p2BundleContent = "bundle-bytes"

type p2Upstream struct {
	server *httptest.Server

	mutex sync.Mutex
	hits  map[string]int
	files map[string][]byte
}

func newP2Upstream(t *testing.T) *p2Upstream {
	t.Helper()

	remote := &p2Upstream{
		hits: map[string]int{},
		files: map[string][]byte{
			p2IndexPath:  []byte(p2Index),
			p2ArtifactsP: jarContaining(t, "artifacts.xml", p2Artifacts),
			p2ContentP:   jarContaining(t, "content.xml", "<repository name='c'/>"),
			p2BundlePath: []byte(p2BundleContent),
			p2CompositeP: []byte(p2Composite),
			p2EppPath:    []byte("<repository name='epp'/>"),
		},
	}

	remote.server = httptest.NewServer(http.HandlerFunc(remote.serve))
	t.Cleanup(remote.server.Close)

	return remote
}

func (u *p2Upstream) serve(writer http.ResponseWriter, request *http.Request) {
	path := strings.TrimPrefix(request.URL.Path, "/")

	u.mutex.Lock()
	u.hits[path]++
	content, present := u.files[path]
	u.mutex.Unlock()

	if !present {
		http.NotFound(writer, request)
		return
	}
	writer.Write(content)
}

func (u *p2Upstream) hitsFor(path string) int {
	u.mutex.Lock()
	defer u.mutex.Unlock()
	return u.hits[path]
}

func jarContaining(t *testing.T, name, content string) []byte {
	t.Helper()

	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)

	entry, err := writer.Create(name)
	if err != nil {
		t.Fatalf("creating the jar entry: %v", err)
	}
	if _, err := entry.Write([]byte(content)); err != nil {
		t.Fatalf("writing the jar entry: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("closing the jar: %v", err)
	}

	return buffer.Bytes()
}

func entryInJar(t *testing.T, document []byte, name string) string {
	t.Helper()

	reader, err := zip.NewReader(bytes.NewReader(document), int64(len(document)))
	if err != nil {
		t.Fatalf("the served jar does not open: %v", err)
	}
	for _, file := range reader.File {
		if file.Name != name {
			continue
		}
		opened, err := file.Open()
		if err != nil {
			t.Fatalf("opening %s: %v", name, err)
		}
		defer opened.Close()

		content, err := io.ReadAll(opened)
		if err != nil {
			t.Fatalf("reading %s: %v", name, err)
		}
		return string(content)
	}

	t.Fatalf("%s is missing from the served jar", name)
	return ""
}

func newP2Instance(t *testing.T, extra string) (*testInstance, *p2Upstream) {
	t.Helper()

	instance := newTestInstance(t)
	instance.setup()

	remote := newP2Upstream(t)
	body := fmt.Sprintf(
		`{"name":"eclipse","format":"p2","type":"proxy","visibility":"public","remoteUrl":%q%s}`,
		remote.server.URL, extra,
	)
	expectStatus(t, instance.api(http.MethodPost, "/api/repositories", body), http.StatusCreated)

	return instance, remote
}

func (i *testInstance) p2Get(path string) *http.Response {
	i.t.Helper()
	return i.maven(http.MethodGet, "/repository/eclipse/"+path, "", true)
}

func (i *testInstance) p2Bytes(path string) []byte {
	i.t.Helper()

	response := i.p2Get(path)
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		i.t.Fatalf("GET %s = %d, want 200", path, response.StatusCode)
	}
	body, err := io.ReadAll(response.Body)
	if err != nil {
		i.t.Fatalf("reading %s: %v", path, err)
	}
	return body
}

func TestP2StripsMirrorPropertiesFromArtifacts(t *testing.T) {
	instance, _ := newP2Instance(t, "")

	document := entryInJar(t, instance.p2Bytes(p2ArtifactsP), "artifacts.xml")

	if strings.Contains(document, "p2.mirrorsURL") {
		t.Error("the mirrors property was served, so clients would download around the cache")
	}
	if strings.Contains(document, "p2.statsURI") {
		t.Error("the stats property was served")
	}
	if !strings.Contains(document, "p2.timestamp") {
		t.Error("an unrelated repository property was dropped")
	}
	if !strings.Contains(document, "org.eclipse.jgit") {
		t.Error("the artifact list was lost")
	}
}

func TestP2RewritesTheIndexOntoServeableEncodings(t *testing.T) {
	instance, _ := newP2Instance(t, "")

	index := string(instance.p2Bytes(p2IndexPath))

	if strings.Contains(index, "artifacts.xml.xz") {
		t.Errorf("the xz encoding is still offered:\n%s", index)
	}
	// The remaining '.xml' entry names a factory that already looks for the jar,
	// so nothing is added.
	if !strings.Contains(index, "artifact.repository.factory.order=artifacts.xml,!") {
		t.Errorf("the artifact order was not reduced to the serveable candidate:\n%s", index)
	}
	// Only artifact documents need rewriting, so the metadata order is untouched.
	if !strings.Contains(index, "metadata.repository.factory.order=content.xml.xz,content.xml,!") {
		t.Errorf("the metadata order was altered:\n%s", index)
	}
}

func TestP2RefusesEncodingsItCannotStrip(t *testing.T) {
	instance, remote := newP2Instance(t, "")

	response := instance.p2Get("releases/2024-12/202412041000/artifacts.xml.xz")
	defer response.Body.Close()

	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("artifacts.xml.xz = %d, want 404", response.StatusCode)
	}
	if hits := remote.hitsFor("releases/2024-12/202412041000/artifacts.xml.xz"); hits != 0 {
		t.Fatalf("the upstream was asked %d times for an encoding we refuse to serve", hits)
	}
}

func TestP2ServesBundlesUntouchedAndCachesThem(t *testing.T) {
	instance, remote := newP2Instance(t, "")

	if body := string(instance.p2Bytes(p2BundlePath)); body != p2BundleContent {
		t.Fatalf("bundle body = %q, want %q", body, p2BundleContent)
	}
	if body := string(instance.p2Bytes(p2BundlePath)); body != p2BundleContent {
		t.Fatalf("cached bundle body = %q, want %q", body, p2BundleContent)
	}

	if hits := remote.hitsFor(p2BundlePath); hits != 1 {
		t.Fatalf("the upstream was asked %d times for an immutable bundle, want 1", hits)
	}
}

// Metadata is republished under a stable name, which is how a site like
// releases/latest moves. Before the p2 layout every path was cached forever.
func TestP2RevalidatesMetadataButNotBundles(t *testing.T) {
	instance, remote := newP2Instance(t, `,"metadataTtlSeconds":0`)

	cases := []struct {
		name      string
		path      string
		wantHits  int
		mutable   bool
		unwrapped bool
	}{
		{name: "index", path: p2IndexPath, wantHits: 2, mutable: true},
		{name: "artifacts", path: p2ArtifactsP, wantHits: 2, mutable: true},
		{name: "content", path: p2ContentP, wantHits: 2, mutable: true},
		{name: "composite", path: p2CompositeP, wantHits: 2, mutable: true},
		{name: "bundle", path: p2BundlePath, wantHits: 1},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			instance.p2Bytes(test.path)
			instance.p2Bytes(test.path)

			if hits := remote.hitsFor(test.path); hits != test.wantHits {
				t.Fatalf("upstream hits for %s = %d, want %d", test.path, hits, test.wantHits)
			}
		})
	}
}

// A composite child of ../../ resolves above the update site, so the proxy is
// pinned to the host root and the escaped path still lands in this repository.
func TestP2ResolvesCompositeChildrenAboveTheSite(t *testing.T) {
	instance, _ := newP2Instance(t, "")

	if body := string(instance.p2Bytes(p2CompositeP)); !strings.Contains(body, "../../technology/epp") {
		t.Fatalf("the composite was altered: %s", body)
	}
	if body := string(instance.p2Bytes(p2EppPath)); !strings.Contains(body, "epp") {
		t.Fatalf("the escaped child did not resolve: %s", body)
	}
}

func TestP2RepositoryValidation(t *testing.T) {
	instance := newTestInstance(t)
	instance.setup()

	cases := []struct {
		name string
		body string
		want int
	}{
		{
			name: "a host root proxy is accepted",
			body: `{"name":"p2-ok","format":"p2","type":"proxy","remoteUrl":"https://download.eclipse.org"}`,
			want: http.StatusCreated,
		},
		{
			name: "a remote pinned to one update site is rejected",
			body: `{"name":"p2-deep","format":"p2","type":"proxy","remoteUrl":"https://download.eclipse.org/releases/2024-12"}`,
			want: http.StatusBadRequest,
		},
		{
			name: "hosted p2 is accepted",
			body: `{"name":"p2-hosted","format":"p2","type":"hosted"}`,
			want: http.StatusCreated,
		},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			expectStatus(t, instance.api(http.MethodPost, "/api/repositories", test.body), test.want)
		})
	}
}

func TestP2ProxyRejectsWrites(t *testing.T) {
	instance, _ := newP2Instance(t, "")

	for _, method := range []string{http.MethodPut, http.MethodPost, http.MethodDelete} {
		t.Run(method, func(t *testing.T) {
			response := instance.maven(method, "/repository/eclipse/"+p2BundlePath, "body", true)
			defer response.Body.Close()

			if response.StatusCode != http.StatusMethodNotAllowed {
				t.Fatalf("%s = %d, want 405", method, response.StatusCode)
			}
		})
	}
}

// A Tycho build writes target/repository and the publish walks it file by file,
// which is what wagon-maven-plugin:upload does against a raw repository. These
// are the paths that walk produces.
var hostedSite = map[string]string{
	"beinformed/commons/25.2.5/artifacts.jar":                                      "artifacts-jar-bytes",
	"beinformed/commons/25.2.5/content.jar":                                        "content-jar-bytes",
	"beinformed/commons/25.2.5/plugins/com.beinformed.commons_25.2.5.jar":          "bundle-bytes",
	"beinformed/commons/25.2.5/plugins/com.beinformed.commons.source_25.2.5.jar":   "source-bundle-bytes",
	"beinformed/commons/25.2.5/features/com.beinformed.commons.feature_25.2.5.jar": "feature-bytes",
	"beinformed/commons/25.2.5/binary/com.beinformed.product_root_25.2.5":          "binary-bytes",
}

func newHostedP2Instance(t *testing.T) *testInstance {
	t.Helper()

	instance := newTestInstance(t)
	instance.setup()

	body := `{"name":"p2-releases","format":"p2","type":"hosted","visibility":"public"}`
	expectStatus(t, instance.api(http.MethodPost, "/api/repositories", body), http.StatusCreated)

	return instance
}

func (i *testInstance) p2Put(path, body string) *http.Response {
	i.t.Helper()
	return i.maven(http.MethodPut, "/repository/p2-releases/"+path, body, true)
}

func TestHostedP2TakesAWholeSiteAndServesItBack(t *testing.T) {
	instance := newHostedP2Instance(t)

	for path, content := range hostedSite {
		expectStatus(t, instance.p2Put(path, content), http.StatusCreated)
	}

	for path, content := range hostedSite {
		t.Run(path, func(t *testing.T) {
			response := instance.maven(http.MethodGet, "/repository/p2-releases/"+path, "", true)
			if body := expectStatus(t, response, http.StatusOK); body != content {
				t.Fatalf("body = %q, want %q", body, content)
			}
		})
	}
}

// Republishing rewrites the metadata documents under the same names, so an
// overwrite has to succeed rather than conflict the way a Maven release does.
func TestHostedP2AllowsRepublishing(t *testing.T) {
	instance := newHostedP2Instance(t)

	const path = "beinformed/commons/25.2.5/artifacts.jar"

	expectStatus(t, instance.p2Put(path, "first"), http.StatusCreated)
	expectStatus(t, instance.p2Put(path, "second"), http.StatusOK)

	response := instance.maven(http.MethodGet, "/repository/p2-releases/"+path, "", true)
	if body := expectStatus(t, response, http.StatusOK); body != "second" {
		t.Fatalf("body = %q, want the republished content", body)
	}
}

func TestHostedP2DeletesASiteTree(t *testing.T) {
	instance := newHostedP2Instance(t)

	for path, content := range hostedSite {
		expectStatus(t, instance.p2Put(path, content), http.StatusCreated)
	}

	deletion := instance.maven(http.MethodDelete, "/repository/p2-releases/beinformed/commons/25.2.5/", "", true)
	expectStatus(t, deletion, http.StatusNoContent)

	for path := range hostedSite {
		response := instance.maven(http.MethodGet, "/repository/p2-releases/"+path, "", true)
		defer response.Body.Close()

		if response.StatusCode != http.StatusNotFound {
			t.Fatalf("%s survived the tree delete with %d", path, response.StatusCode)
		}
	}

	if remaining := instance.cachedFiles(); remaining != 0 {
		t.Fatalf("%d files are still on disk after the tree delete", remaining)
	}
}

func TestHostedP2SurfacesBundlesAsArtifacts(t *testing.T) {
	instance := newHostedP2Instance(t)

	for path, content := range hostedSite {
		expectStatus(t, instance.p2Put(path, content), http.StatusCreated)
	}

	cases := []struct {
		name      string
		namespace string
		artifact  string
	}{
		{name: "bundle", namespace: "osgi.bundle", artifact: "com.beinformed.commons"},
		{name: "source bundle", namespace: "osgi.bundle", artifact: "com.beinformed.commons.source"},
		{name: "feature", namespace: "org.eclipse.update.feature", artifact: "com.beinformed.commons.feature"},
		{name: "binary", namespace: "binary", artifact: "com.beinformed.product_root"},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			query := "?namespace=" + test.namespace + "&name=" + test.artifact
			body := expectStatus(t,
				instance.api(http.MethodGet, "/api/repositories/p2-releases/artifacts"+query, ""),
				http.StatusOK)

			if !strings.Contains(body, `"version":"25.2.5"`) {
				t.Fatalf("the artifact page does not list the version: %s", body)
			}

			files := expectStatus(t,
				instance.api(http.MethodGet, "/api/repositories/p2-releases/artifacts/files"+query+"&version=25.2.5", ""),
				http.StatusOK)

			if !strings.Contains(files, test.artifact+"_25.2.5") {
				t.Fatalf("the files tab does not list the bundle: %s", files)
			}
		})
	}

	t.Run("metadata documents are not artifacts", func(t *testing.T) {
		body := expectStatus(t,
			instance.api(http.MethodGet, "/api/repositories/p2-releases/recent", ""),
			http.StatusOK)

		if strings.Contains(body, "artifacts.jar") || strings.Contains(body, "content.jar") {
			t.Fatalf("a metadata document was recorded as a component: %s", body)
		}
	})
}

func TestHostedP2DeletesOneBundleVersion(t *testing.T) {
	instance := newHostedP2Instance(t)

	for path, content := range hostedSite {
		expectStatus(t, instance.p2Put(path, content), http.StatusCreated)
	}

	expectStatus(t, instance.api(http.MethodDelete,
		"/api/repositories/p2-releases/artifacts?namespace=osgi.bundle&name=com.beinformed.commons&version=25.2.5", ""),
		http.StatusOK)

	gone := instance.maven(http.MethodGet,
		"/repository/p2-releases/beinformed/commons/25.2.5/plugins/com.beinformed.commons_25.2.5.jar", "", true)
	defer gone.Body.Close()
	if gone.StatusCode != http.StatusNotFound {
		t.Fatalf("the deleted bundle still resolves with %d", gone.StatusCode)
	}

	// Deleting one bundle must not touch its siblings or the site metadata.
	for _, path := range []string{
		"beinformed/commons/25.2.5/plugins/com.beinformed.commons.source_25.2.5.jar",
		"beinformed/commons/25.2.5/artifacts.jar",
	} {
		survivor := instance.maven(http.MethodGet, "/repository/p2-releases/"+path, "", true)
		defer survivor.Body.Close()
		if survivor.StatusCode != http.StatusOK {
			t.Fatalf("%s was removed with its sibling (%d)", path, survivor.StatusCode)
		}
	}
}

func TestHostedP2NeedsWritePermission(t *testing.T) {
	instance := newHostedP2Instance(t)

	response := instance.maven(http.MethodPut, "/repository/p2-releases/a/artifacts.jar", "body", false)
	defer response.Body.Close()

	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("anonymous publish = %d, want 401", response.StatusCode)
	}
}
