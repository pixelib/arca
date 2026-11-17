package server

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"arca/internal/config"
)

const (
	administratorEmail    = "admin@example.com"
	administratorPassword = "supersecret123"
)

type testInstance struct {
	t       *testing.T
	app     *Server
	server  *httptest.Server
	client  *http.Client
	storage string
}

func newTestInstance(t *testing.T) *testInstance {
	t.Helper()
	return newConfiguredInstance(t, config.Config{})
}

// newConfiguredInstance starts an instance from the given configuration, with
// its database and storage placed in a temporary directory.
func newConfiguredInstance(t *testing.T, configuration config.Config) *testInstance {
	t.Helper()

	directory := t.TempDir()
	storage := filepath.Join(directory, "storage")

	configuration.DatabasePath = filepath.Join(directory, "arca.db")
	configuration.StorageDir = storage
	configuration.MaxUploadBytes = 1 << 20
	instance, err := New(configuration)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	server := httptest.NewServer(instance.router)
	t.Cleanup(func() {
		server.Close()
		instance.traffic.close()
		instance.store.Close()
	})

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("cookiejar.New: %v", err)
	}

	return &testInstance{
		t:       t,
		app:     instance,
		server:  server,
		storage: storage,
		client: &http.Client{
			Jar:           jar,
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
	}
}

func (i *testInstance) do(request *http.Request) *http.Response {
	i.t.Helper()

	response, err := i.client.Do(request)
	if err != nil {
		i.t.Fatalf("%s %s: %v", request.Method, request.URL.Path, err)
	}
	return response
}

func (i *testInstance) doAnonymously(request *http.Request) *http.Response {
	i.t.Helper()

	response, err := (&http.Client{
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}).Do(request)
	if err != nil {
		i.t.Fatalf("%s %s: %v", request.Method, request.URL.Path, err)
	}
	return response
}

func (i *testInstance) request(method, path, body string) *http.Request {
	i.t.Helper()

	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}

	request, err := http.NewRequest(method, i.server.URL+path, reader)
	if err != nil {
		i.t.Fatalf("building a request: %v", err)
	}
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	return request
}

func (i *testInstance) api(method, path, body string) *http.Response {
	i.t.Helper()
	return i.do(i.request(method, path, body))
}

func (i *testInstance) maven(method, path, body string, authenticated bool) *http.Response {
	i.t.Helper()

	request := i.request(method, path, body)
	if !authenticated {
		return i.doAnonymously(request)
	}

	request.SetBasicAuth(administratorEmail, administratorPassword)
	return i.do(request)
}

func (i *testInstance) setup() {
	i.t.Helper()

	response := i.api(http.MethodPost, "/api/setup", `{
		"setupToken": "`+i.app.SetupToken()+`",
		"instanceName": "test-nexus",
		"name": "Admin",
		"email": "`+administratorEmail+`",
		"password": "`+administratorPassword+`",
		"seedRepositories": true
	}`)
	defer response.Body.Close()

	if response.StatusCode != http.StatusCreated {
		i.t.Fatalf("setup returned %d: %s", response.StatusCode, readBody(i.t, response))
	}
}

func readBody(t *testing.T, response *http.Response) string {
	t.Helper()

	content, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("reading a response body: %v", err)
	}
	return string(content)
}

func expectStatus(t *testing.T, response *http.Response, want int) string {
	t.Helper()

	body := readBody(t, response)
	response.Body.Close()

	if response.StatusCode != want {
		t.Fatalf("status = %d, want %d (body: %s)", response.StatusCode, want, body)
	}
	return body
}

func TestBootstrapReportsUninitializedInstance(t *testing.T) {
	instance := newTestInstance(t)

	body := expectStatus(t, instance.api(http.MethodGet, "/api/bootstrap", ""), http.StatusOK)

	var payload struct {
		Initialized  bool   `json:"initialized"`
		InstanceName string `json:"instanceName"`
		User         any    `json:"user"`
	}
	if err := json.Unmarshal([]byte(body), &payload); err != nil {
		t.Fatalf("decoding bootstrap: %v", err)
	}
	if payload.Initialized {
		t.Fatal("expected a fresh instance to report initialized = false")
	}
	if payload.User != nil {
		t.Fatalf("expected no user, got %v", payload.User)
	}
}

func TestSetupSeedsRepositoriesAndSignsIn(t *testing.T) {
	instance := newTestInstance(t)
	token := instance.app.SetupToken()
	instance.setup()

	body := expectStatus(t, instance.api(http.MethodGet, "/api/repositories", ""), http.StatusOK)

	var payload struct {
		Repositories []struct {
			Name       string `json:"name"`
			Policy     string `json:"policy"`
			Permission string `json:"permission"`
		} `json:"repositories"`
	}
	if err := json.Unmarshal([]byte(body), &payload); err != nil {
		t.Fatalf("decoding repositories: %v", err)
	}

	seeded := map[string]string{}
	for _, repository := range payload.Repositories {
		seeded[repository.Name] = repository.Policy
	}

	cases := []struct {
		name   string
		policy string
	}{
		{"maven-releases", "release"},
		{"maven-snapshots", "snapshot"},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if seeded[testCase.name] != testCase.policy {
				t.Fatalf("policy = %q, want %q", seeded[testCase.name], testCase.policy)
			}
		})
	}

	// Once set up, the route is gone for everyone, token or not.
	expectStatus(t, instance.api(http.MethodPost, "/api/setup", `{"setupToken":"`+token+`","instanceName":"x","name":"y","email":"a@b.co","password":"0123456789"}`), http.StatusNotFound)
}

func TestRepositoryFormatDefaultsAndIsImmutable(t *testing.T) {
	instance := newTestInstance(t)
	instance.setup()

	t.Run("a repository created without a format is maven2", func(t *testing.T) {
		body := expectStatus(t, instance.api(http.MethodPost, "/api/repositories", `{"name":"implicit"}`), http.StatusCreated)
		if !strings.Contains(body, `"format":"maven2"`) {
			t.Fatalf("format was not defaulted: %s", body)
		}
	})

	t.Run("an explicit supported format is accepted", func(t *testing.T) {
		body := expectStatus(t, instance.api(http.MethodPost, "/api/repositories", `{"name":"explicit","format":"maven2"}`), http.StatusCreated)
		if !strings.Contains(body, `"format":"maven2"`) {
			t.Fatalf("format was not stored: %s", body)
		}
	})

	cases := []struct {
		name   string
		body   string
		status int
	}{
		{"unchanged format alongside a real change", `{"format":"maven2","description":"unchanged"}`, http.StatusOK},
		{"different supported format", `{"format":"npm"}`, http.StatusBadRequest},
		{"unknown format", `{"format":"rubygems"}`, http.StatusBadRequest},
		{"omitted format", `{"description":"still fine"}`, http.StatusOK},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			expectStatus(t, instance.api(http.MethodPatch, "/api/repositories/explicit", testCase.body), testCase.status)
		})
	}
}

func TestSetupRejectsInvalidInput(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{"missing instance name", `{"name":"Admin","email":"a@b.co","password":"0123456789"}`},
		{"invalid email", `{"instanceName":"n","name":"Admin","email":"nope","password":"0123456789"}`},
		{"short password", `{"instanceName":"n","name":"Admin","email":"a@b.co","password":"short"}`},
		{"malformed json", `{`},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			instance := newTestInstance(t)
			body := testCase.body
			if json.Valid([]byte(body)) {
				body = `{"setupToken":"` + instance.app.SetupToken() + `",` + body[1:]
			}
			expectStatus(t, instance.api(http.MethodPost, "/api/setup", body), http.StatusBadRequest)
		})
	}
}

func TestMavenReleaseLifecycle(t *testing.T) {
	instance := newTestInstance(t)
	instance.setup()

	const (
		artifactPath = "/repository/maven-releases/com/example/app/1.0.0/app-1.0.0.jar"
		content      = "artifact-bytes"
	)

	expectStatus(t, instance.maven(http.MethodPut, artifactPath, content, true), http.StatusCreated)

	t.Run("download", func(t *testing.T) {
		response := instance.maven(http.MethodGet, artifactPath, "", true)
		body := expectStatus(t, response, http.StatusOK)

		if body != content {
			t.Fatalf("body = %q, want %q", body, content)
		}
		if got := response.Header.Get("X-Checksum-SHA1"); got == "" {
			t.Fatal("expected a sha1 checksum header")
		}
		if got := response.Header.Get("Content-Type"); got != "application/java-archive" {
			t.Fatalf("content type = %q, want application/java-archive", got)
		}
	})

	t.Run("checksums", func(t *testing.T) {
		cases := []struct {
			extension string
			length    int
		}{
			{"md5", 32},
			{"sha1", 40},
			{"sha256", 64},
			{"sha512", 128},
		}

		for _, testCase := range cases {
			t.Run(testCase.extension, func(t *testing.T) {
				body := expectStatus(t, instance.maven(http.MethodGet, artifactPath+"."+testCase.extension, "", true), http.StatusOK)
				if len(body) != testCase.length {
					t.Fatalf("checksum = %q, want %d hex characters", body, testCase.length)
				}
			})
		}
	})

	t.Run("generated metadata", func(t *testing.T) {
		body := expectStatus(t, instance.maven(http.MethodGet, "/repository/maven-releases/com/example/app/maven-metadata.xml", "", true), http.StatusOK)

		for _, fragment := range []string{"<groupId>com.example</groupId>", "<artifactId>app</artifactId>", "<version>1.0.0</version>", "<release>1.0.0</release>"} {
			if !strings.Contains(body, fragment) {
				t.Fatalf("metadata is missing %s:\n%s", fragment, body)
			}
		}
	})

	t.Run("redeploy is rejected", func(t *testing.T) {
		expectStatus(t, instance.maven(http.MethodPut, artifactPath, content, true), http.StatusConflict)
	})

	t.Run("uploaded checksums are verified", func(t *testing.T) {
		expectStatus(t, instance.maven(http.MethodPut, artifactPath+".sha1", "0000000000000000000000000000000000000000", true), http.StatusBadRequest)
	})

	t.Run("anonymous access is challenged", func(t *testing.T) {
		response := instance.maven(http.MethodGet, artifactPath, "", false)
		expectStatus(t, response, http.StatusUnauthorized)

		if !strings.HasPrefix(response.Header.Get("WWW-Authenticate"), "Basic realm=") {
			t.Fatal("expected a basic authentication challenge")
		}
	})

	t.Run("directory listing", func(t *testing.T) {
		body := expectStatus(t, instance.maven(http.MethodGet, "/repository/maven-releases/com/example/app/1.0.0/", "", true), http.StatusOK)
		if !strings.Contains(body, "app-1.0.0.jar") {
			t.Fatalf("listing is missing the artifact:\n%s", body)
		}
	})

	t.Run("delete", func(t *testing.T) {
		expectStatus(t, instance.maven(http.MethodDelete, artifactPath, "", true), http.StatusNoContent)
		expectStatus(t, instance.maven(http.MethodGet, artifactPath, "", true), http.StatusNotFound)
	})
}

func TestSnapshotMetadata(t *testing.T) {
	instance := newTestInstance(t)
	instance.setup()

	uploads := []string{
		"/repository/maven-snapshots/com/example/app/1.0.0-SNAPSHOT/app-1.0.0-20240115.101500-1.jar",
		"/repository/maven-snapshots/com/example/app/1.0.0-SNAPSHOT/app-1.0.0-20240115.101500-1.pom",
		"/repository/maven-snapshots/com/example/app/1.0.0-SNAPSHOT/app-1.0.0-20240116.120000-2.jar",
	}

	for _, path := range uploads {
		t.Run(path, func(t *testing.T) {
			expectStatus(t, instance.maven(http.MethodPut, path, "payload", true), http.StatusCreated)
		})
	}

	body := expectStatus(t, instance.maven(http.MethodGet, "/repository/maven-snapshots/com/example/app/1.0.0-SNAPSHOT/maven-metadata.xml", "", true), http.StatusOK)

	fragments := []string{
		"<version>1.0.0-SNAPSHOT</version>",
		"<timestamp>20240116.120000</timestamp>",
		"<buildNumber>2</buildNumber>",
		"<extension>pom</extension>",
	}

	for _, fragment := range fragments {
		t.Run(fragment, func(t *testing.T) {
			if !strings.Contains(body, fragment) {
				t.Fatalf("metadata is missing %s:\n%s", fragment, body)
			}
		})
	}
}

func TestPolicyRejectsMismatchedVersions(t *testing.T) {
	instance := newTestInstance(t)
	instance.setup()

	cases := []struct {
		name string
		path string
	}{
		{"snapshot into releases", "/repository/maven-releases/com/example/app/2.0.0-SNAPSHOT/app-2.0.0-SNAPSHOT.jar"},
		{"release into snapshots", "/repository/maven-snapshots/com/example/app/2.0.0/app-2.0.0.jar"},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			expectStatus(t, instance.maven(http.MethodPut, testCase.path, "payload", true), http.StatusBadRequest)
		})
	}
}

func TestUploadRejectsUnsafePaths(t *testing.T) {
	instance := newTestInstance(t)
	instance.setup()

	response := instance.maven(http.MethodPut, "/repository/maven-releases/com/example/./app.jar", "payload", true)
	body := readBody(t, response)
	response.Body.Close()

	if response.StatusCode == http.StatusCreated {
		t.Fatalf("expected the upload to be rejected, got 201 (body: %s)", body)
	}
}

func TestApiRequiresAuthentication(t *testing.T) {
	instance := newTestInstance(t)

	cases := []struct {
		name   string
		method string
		path   string
		want   int
	}{
		{"me", http.MethodGet, "/api/auth/me", http.StatusUnauthorized},
		{"tokens", http.MethodGet, "/api/tokens", http.StatusUnauthorized},
		{"users", http.MethodGet, "/api/users", http.StatusUnauthorized},
		{"unknown route", http.MethodGet, "/api/nope", http.StatusNotFound},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			expectStatus(t, instance.api(testCase.method, testCase.path, ""), testCase.want)
		})
	}
}

func TestTokenIssuedTokenAuthenticatesMaven(t *testing.T) {
	instance := newTestInstance(t)
	instance.setup()

	body := expectStatus(t, instance.api(http.MethodPost, "/api/tokens", `{"name":"ci","expiresInDays":0}`), http.StatusCreated)

	var issued struct {
		Token    string `json:"token"`
		Username string `json:"username"`
	}
	if err := json.Unmarshal([]byte(body), &issued); err != nil {
		t.Fatalf("decoding the token: %v", err)
	}
	if !strings.HasPrefix(issued.Token, "cnx_") {
		t.Fatalf("token = %q, want a cnx_ prefix", issued.Token)
	}

	const path = "/repository/maven-releases/com/example/app/3.0.0/app-3.0.0.jar"

	request := instance.request(http.MethodPut, path, "payload")
	request.SetBasicAuth(issued.Username, issued.Token)
	expectStatus(t, instance.do(request), http.StatusCreated)

	listing := expectStatus(t, instance.api(http.MethodGet, "/api/tokens", ""), http.StatusOK)
	if !strings.Contains(listing, `"name":"ci"`) {
		t.Fatalf("token listing is missing the issued token: %s", listing)
	}
}

func TestBrowseAndSearch(t *testing.T) {
	instance := newTestInstance(t)
	instance.setup()

	expectStatus(t, instance.maven(http.MethodPut, "/repository/maven-releases/com/example/app/1.0.0/app-1.0.0.jar", "payload", true), http.StatusCreated)

	t.Run("browse root", func(t *testing.T) {
		body := expectStatus(t, instance.api(http.MethodGet, "/api/repositories/maven-releases/browse?path=", ""), http.StatusOK)
		if !strings.Contains(body, `"name":"com"`) {
			t.Fatalf("browse is missing the com directory: %s", body)
		}
	})

	t.Run("browse artifact directory", func(t *testing.T) {
		body := expectStatus(t, instance.api(http.MethodGet, "/api/repositories/maven-releases/browse?path=com/example/app", ""), http.StatusOK)
		if !strings.Contains(body, `"name":"app","namespace":"com.example"`) {
			t.Fatalf("browse did not resolve the artifact: %s", body)
		}
	})

	t.Run("artifact versions", func(t *testing.T) {
		body := expectStatus(t, instance.api(http.MethodGet, "/api/repositories/maven-releases/artifacts?namespace=com.example&name=app", ""), http.StatusOK)
		if !strings.Contains(body, `"latest":"1.0.0"`) {
			t.Fatalf("artifact detail is missing the latest version: %s", body)
		}
	})

	t.Run("search", func(t *testing.T) {
		body := expectStatus(t, instance.api(http.MethodGet, "/api/search?q=app", ""), http.StatusOK)
		if !strings.Contains(body, `"name":"app"`) {
			t.Fatalf("search did not find the artifact: %s", body)
		}
	})

	t.Run("short queries return nothing", func(t *testing.T) {
		body := expectStatus(t, instance.api(http.MethodGet, "/api/search?q=a", ""), http.StatusOK)
		if !strings.Contains(body, `"results":[]`) {
			t.Fatalf("expected an empty result set: %s", body)
		}
	})
}

func TestPublicRepositoryAllowsAnonymousReads(t *testing.T) {
	instance := newTestInstance(t)
	instance.setup()

	expectStatus(t, instance.api(http.MethodPost, "/api/repositories", `{"name":"public-releases","policy":"release","visibility":"public"}`), http.StatusCreated)

	const path = "/repository/public-releases/com/example/app/1.0.0/app-1.0.0.jar"
	expectStatus(t, instance.maven(http.MethodPut, path, "payload", true), http.StatusCreated)

	anonymous := &http.Client{}
	response, err := anonymous.Get(instance.server.URL + path)
	if err != nil {
		t.Fatalf("anonymous read: %v", err)
	}
	expectStatus(t, response, http.StatusOK)

	writeResponse, err := anonymous.Do(mustRequest(t, http.MethodPut, instance.server.URL+"/repository/public-releases/com/example/app/2.0.0/app-2.0.0.jar", "payload"))
	if err != nil {
		t.Fatalf("anonymous write: %v", err)
	}
	expectStatus(t, writeResponse, http.StatusUnauthorized)
}

func mustRequest(t *testing.T, method, url, body string) *http.Request {
	t.Helper()

	request, err := http.NewRequest(method, url, strings.NewReader(body))
	if err != nil {
		t.Fatalf("building a request: %v", err)
	}
	return request
}
