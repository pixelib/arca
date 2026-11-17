package server

import (
	"net/http"
	"time"

	"github.com/charmbracelet/log"
	"github.com/gorilla/mux"
)

func (s *Server) routes() {
	s.router.Use(recoverMiddleware, loggingMiddleware, s.resolver.Middleware)

	api := s.router.PathPrefix("/api").Subrouter()

	api.Handle("/bootstrap", handler(s.routeBootstrap)).Methods(http.MethodGet)
	api.Handle("/setup", handler(s.routeSetup)).Methods(http.MethodPost)
	api.Handle("/branding/logo", handler(s.routeLogo)).Methods(http.MethodGet)
	api.Handle("/auth/login", handler(s.routeLogin)).Methods(http.MethodPost)
	api.Handle("/auth/logout", handler(s.routeLogout)).Methods(http.MethodPost)
	api.Handle("/auth/me", handler(s.routeMe)).Methods(http.MethodGet)
	api.Handle("/auth/password", handler(s.routeChangePassword)).Methods(http.MethodPost)
	api.Handle("/auth/oidc/login", handler(s.routeOIDCLogin)).Methods(http.MethodGet)
	api.Handle("/auth/oidc/callback", handler(s.routeOIDCCallback)).Methods(http.MethodGet)

	api.Handle("/tokens", handler(s.routeListTokens)).Methods(http.MethodGet)
	api.Handle("/tokens", handler(s.routeCreateToken)).Methods(http.MethodPost)
	api.Handle("/tokens/{id}", handler(s.routeDeleteToken)).Methods(http.MethodDelete)

	api.Handle("/users", handler(s.routeListUsers)).Methods(http.MethodGet)
	api.Handle("/users", handler(s.routeCreateUser)).Methods(http.MethodPost)
	api.Handle("/users/{id}", handler(s.routeUpdateUser)).Methods(http.MethodPatch)
	api.Handle("/users/{id}", handler(s.routeDeleteUser)).Methods(http.MethodDelete)

	api.Handle("/categories", handler(s.routeListCategories)).Methods(http.MethodGet)
	api.Handle("/categories", handler(s.routeCreateCategory)).Methods(http.MethodPost)
	api.Handle("/categories/order", handler(s.routeReorderCategories)).Methods(http.MethodPut)
	api.Handle("/categories/{id}", handler(s.routeUpdateCategory)).Methods(http.MethodPatch)
	api.Handle("/categories/{id}", handler(s.routeDeleteCategory)).Methods(http.MethodDelete)

	api.Handle("/repositories", handler(s.routeListRepositories)).Methods(http.MethodGet)
	api.Handle("/repositories", handler(s.routeCreateRepository)).Methods(http.MethodPost)
	api.Handle("/repositories/{name}", handler(s.routeRepository)).Methods(http.MethodGet)
	api.Handle("/repositories/{name}", handler(s.routeUpdateRepository)).Methods(http.MethodPatch)
	api.Handle("/repositories/{name}", handler(s.routeDeleteRepository)).Methods(http.MethodDelete)
	api.Handle("/repositories/{name}/cache", handler(s.routePurgeCache)).Methods(http.MethodDelete)
	api.Handle("/repositories/{name}/grants", handler(s.routeListGrants)).Methods(http.MethodGet)
	api.Handle("/repositories/{name}/grants", handler(s.routeReplaceGrants)).Methods(http.MethodPut)
	api.Handle("/repositories/{name}/aliases", handler(s.routeListAliases)).Methods(http.MethodGet)
	api.Handle("/repositories/{name}/aliases", handler(s.routeCreateAlias)).Methods(http.MethodPost)
	api.Handle("/repositories/{name}/aliases/{alias}", handler(s.routeDeleteAlias)).Methods(http.MethodDelete)
	api.Handle("/repositories/{name}/links", handler(s.routeListLinks)).Methods(http.MethodGet)
	api.Handle("/repositories/{name}/links", handler(s.routeCreateLink)).Methods(http.MethodPost)
	api.Handle("/repositories/{name}/links/{id}", handler(s.routeUpdateLink)).Methods(http.MethodPatch)
	api.Handle("/repositories/{name}/links/{id}", handler(s.routeDeleteLink)).Methods(http.MethodDelete)

	api.Handle("/repositories/{name}/browse", handler(s.routeBrowse)).Methods(http.MethodGet)
	api.Handle("/repositories/{name}/artifacts", handler(s.routeArtifact)).Methods(http.MethodGet)
	api.Handle("/repositories/{name}/artifacts", handler(s.routeDeleteArtifact)).Methods(http.MethodDelete)
	api.Handle("/repositories/{name}/artifacts/files", handler(s.routeArtifactFiles)).Methods(http.MethodGet)
	api.Handle("/repositories/{name}/recent", handler(s.routeRecent)).Methods(http.MethodGet)
	api.Handle("/repositories/{name}/docker/manifests", handler(s.routeDockerManifest)).Methods(http.MethodGet)
	api.Handle("/search", handler(s.routeSearch)).Methods(http.MethodGet)

	api.Handle("/admin/health", handler(s.routeHealth)).Methods(http.MethodGet)
	api.Handle("/admin/traffic", handler(s.routeTraffic)).Methods(http.MethodGet)
	api.Handle("/admin/storage", handler(s.routeStorage)).Methods(http.MethodGet)
	api.Handle("/admin/components", handler(s.routeComponents)).Methods(http.MethodGet)
	api.Handle("/admin/settings", handler(s.routeSettings)).Methods(http.MethodGet)
	api.Handle("/admin/settings", handler(s.routeUpdateSettings)).Methods(http.MethodPatch)
	api.Handle("/admin/branding/logo", handler(s.routeUploadLogo)).Methods(http.MethodPut)
	api.Handle("/admin/branding/logo", handler(s.routeDeleteLogo)).Methods(http.MethodDelete)

	api.Handle("/admin/migration", handler(s.routeMigrationStatus)).Methods(http.MethodGet)
	api.Handle("/admin/migration", handler(s.routeStartMigration)).Methods(http.MethodPost)
	api.Handle("/admin/migration", handler(s.routeCancelMigration)).Methods(http.MethodDelete)
	api.Handle("/admin/migration/preview", handler(s.routeMigrationPreview)).Methods(http.MethodPost)

	api.Handle("/admin/docker/sweep", handler(s.routeDockerSweepPreview)).Methods(http.MethodGet)
	api.Handle("/admin/docker/sweep", handler(s.routeDockerSweep)).Methods(http.MethodPost)
	api.Handle("/admin/docker/reindex", handler(s.routeDockerReindex)).Methods(http.MethodPost)

	api.PathPrefix("/").HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writeJSON(writer, http.StatusNotFound, map[string]string{"error": "Not found"})
	})

	// Deployment links sit at the host root rather than under /repository,
	// because the slug is the whole address: the repository a link reads from is
	// private more often than not and has no business being in the URL.
	s.router.Handle("/download/{slug}", contentHeaders(http.HandlerFunc(s.handleDownload))).
		Methods(http.MethodGet, http.MethodHead)

	repositoryMethods := []string{http.MethodGet, http.MethodHead, http.MethodPut, http.MethodPost, http.MethodDelete}
	repository := contentHeaders(http.HandlerFunc(s.handleRepository))
	s.router.Handle("/repository/{repository}", repository).Methods(repositoryMethods...)
	s.router.PathPrefix("/repository/{repository}/").Handler(repository).Methods(repositoryMethods...)

	// The registry API lives at the host root because a Docker client builds its
	// URLs from the image reference and cannot be pointed at a path prefix. PATCH
	// appears here and nowhere else: it is how a chunked blob push appends.
	registryMethods := []string{
		http.MethodGet, http.MethodHead, http.MethodPost,
		http.MethodPut, http.MethodPatch, http.MethodDelete,
	}
	registry := contentHeaders(http.HandlerFunc(s.handleDockerRegistry))
	s.router.Handle("/v2", registry).Methods(registryMethods...)
	s.router.PathPrefix("/v2/").Handler(registry).Methods(registryMethods...)

	s.mountFrontend()
}

// contentHeaders wraps every route that hands back bytes somebody else uploaded.
// Those bytes are served on the same origin as the UI, whose session cookie is sent
// on a top-level navigation, so a stored file must never be allowed to run as a
// page: nosniff keeps the browser from second-guessing the type, and the policy
// forbids scripts and any other resource even for a type it does render, XML and
// SVG included. Set before the handler runs so an error body is covered too.
func contentHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		header := writer.Header()
		header.Set("X-Content-Type-Options", "nosniff")
		header.Set("Content-Security-Policy", contentPolicy)
		next.ServeHTTP(writer, request)
	})
}

const (
	contentPolicy = "default-src 'none'; sandbox"
	// indexPolicy is what the generated directory listing runs under. It is this
	// server's own markup with every name escaped, and it needs its one inline
	// stylesheet; it still loads no script and no remote resource.
	indexPolicy = "default-src 'none'; style-src 'unsafe-inline'"
)

func recoverMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		defer func() {
			if recovered := recover(); recovered != nil {
				log.Errorf("panic serving %s %s: %v", request.Method, request.URL.Path, recovered)
				writeJSON(writer, http.StatusInternalServerError, map[string]string{"error": "Internal server error"})
			}
		}()
		next.ServeHTTP(writer, request)
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

func (r *statusRecorder) Unwrap() http.ResponseWriter { return r.ResponseWriter }

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		started := time.Now()
		recorder := &statusRecorder{ResponseWriter: writer, status: http.StatusOK}

		next.ServeHTTP(recorder, request)

		log.Infof("%s %s %d (%s)", request.Method, request.URL.Path, recorder.status, time.Since(started).Round(time.Millisecond))
	})
}

func routeVar(request *http.Request, name string) string {
	return mux.Vars(request)[name]
}
