package server

import (
	"errors"
	"net/http"
	"strings"

	"github.com/charmbracelet/log"

	"arca/internal/blob"
	"arca/internal/format"
	"arca/internal/p2"
	"arca/internal/store/models"
)

func (s *Server) serveP2(writer http.ResponseWriter, request *http.Request, repository *models.Repository, path string) {
	required := models.PermissionWrite
	if request.Method == http.MethodGet || request.Method == http.MethodHead {
		required = models.PermissionRead
	}
	if !s.authorize(writer, request, repository, required) {
		return
	}
	if repository.IsProxy() && required != models.PermissionRead {
		plain(writer, http.StatusMethodNotAllowed, "Proxy repositories are read-only")
		return
	}

	switch request.Method {
	case http.MethodGet, http.MethodHead:
		s.serveP2Read(writer, request, repository, path)
	case http.MethodPut, http.MethodPost:
		s.serveP2Upload(writer, request, repository, path)
	case http.MethodDelete:
		s.serveP2Delete(writer, repository, path)
	default:
		plain(writer, http.StatusMethodNotAllowed, "Method Not Allowed")
	}
}

func (s *Server) serveP2Read(writer http.ResponseWriter, request *http.Request, repository *models.Repository, path string) {
	if path == "" || strings.HasSuffix(path, "/") {
		exists, err := s.store.DirectoryExists(repository.ID, path)
		if err != nil {
			log.Errorf("directory lookup failed: %v", err)
			plain(writer, http.StatusInternalServerError, "Internal Server Error")
			return
		}
		if !exists {
			plain(writer, http.StatusNotFound, "Not Found")
			return
		}
		s.writeDirectoryIndex(writer, repository, path)
		return
	}

	if !repository.IsProxy() {
		s.serveP2Hosted(writer, request, repository, path)
		return
	}

	// An upstream encoding whose mirror properties cannot be stripped is never
	// passed on. RewriteIndex has already steered the client onto one that can
	// be, and a client reaching here without an index falls back on its own.
	// A hosted site is served exactly as it was published, so this is a proxy
	// concern only.
	if p2.IsCompressedArtifactDocument(path) {
		plain(writer, http.StatusNotFound, "Not Found")
		return
	}

	s.serveProxyFetch(writer, request, repository, p2Fetch(path))
}

func (s *Server) serveP2Hosted(writer http.ResponseWriter, request *http.Request, repository *models.Repository, path string) {
	asset, err := s.store.FindAsset(repository.ID, path)
	if err != nil {
		plain(writer, http.StatusNotFound, "Not Found")
		return
	}

	if request.Method == http.MethodGet {
		s.recordAssetTraffic(request, models.TrafficDownload, repository, asset)
	}
	if !s.writeAsset(writer, request, asset) {
		plain(writer, http.StatusNotFound, "Not Found")
	}
}

// serveP2Upload takes a p2 site a file at a time, which is how Tycho's output
// is published: the build writes target/repository and the upload walks it.
// Overwriting is always allowed, because republishing a site rewrites its
// metadata documents under the same names by design.
func (s *Server) serveP2Upload(writer http.ResponseWriter, request *http.Request, repository *models.Repository, path string) {
	if !format.IsSafe(path) {
		plain(writer, http.StatusBadRequest, "Invalid path")
		return
	}
	if request.ContentLength > s.config.MaxUploadBytes {
		plain(writer, http.StatusRequestEntityTooLarge, "Artifact exceeds upload limit")
		return
	}

	_, lookupErr := s.store.FindAsset(repository.ID, path)
	replacing := lookupErr == nil

	var uploadedBy *string
	if user := currentUser(request); user != nil {
		uploadedBy = &user.ID
	}

	asset, err := s.storeUpload(repository, path, request.Body, uploadDetails{UploadedBy: uploadedBy})
	if errors.Is(err, blob.ErrTooLarge) {
		plain(writer, http.StatusRequestEntityTooLarge, "Artifact exceeds upload limit")
		return
	}
	if err != nil {
		log.Errorf("storing an upload failed: %v", err)
		plain(writer, http.StatusInternalServerError, "Internal Server Error")
		return
	}

	s.recordAssetTraffic(request, models.TrafficUpload, repository, asset)

	status := http.StatusCreated
	if replacing {
		status = http.StatusOK
	}

	writer.Header().Set("X-Checksum-SHA1", asset.SHA1)
	writer.Header().Set("X-Checksum-MD5", asset.MD5)
	writer.WriteHeader(status)
}

// serveP2Delete removes a file or a whole site, which is what a republish of a
// snapshot needs before it uploads again.
func (s *Server) serveP2Delete(writer http.ResponseWriter, repository *models.Repository, path string) {
	var keys []string
	var err error

	if path == "" || strings.HasSuffix(path, "/") {
		keys, err = s.store.DeleteAssetTree(repository.ID, path)
	} else {
		keys, err = s.store.DeleteAsset(repository.ID, path)
	}
	if err != nil {
		log.Errorf("deleting assets failed: %v", err)
		plain(writer, http.StatusInternalServerError, "Internal Server Error")
		return
	}
	if len(keys) == 0 {
		plain(writer, http.StatusNotFound, "Not Found")
		return
	}

	if err := s.blobs.Delete(keys...); err != nil {
		log.Errorf("removing stored objects failed: %v", err)
	}

	writer.WriteHeader(http.StatusNoContent)
}

func p2Fetch(path string) proxyFetch {
	fetch := pathFetch(path)

	switch {
	case p2.IsIndex(path):
		fetch.Rewrite = func(document []byte) ([]byte, error) { return p2.RewriteIndex(document), nil }
	case p2.IsArtifactDocument(path):
		fetch.Rewrite = func(document []byte) ([]byte, error) { return p2.RewriteArtifactDocument(path, document) }
	}

	return fetch
}
