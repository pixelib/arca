package server

import (
	"fmt"
	"html"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/log"

	"arca/internal/format"
	"arca/internal/store"
	"arca/internal/store/models"
)

func plain(writer http.ResponseWriter, status int, message string) {
	writer.Header().Set("Content-Type", "text/plain; charset=utf-8")
	writer.WriteHeader(status)
	fmt.Fprintln(writer, message)
}

func (s *Server) challenge(writer http.ResponseWriter) {
	realm := fmt.Sprintf("Basic realm=%q, charset=\"UTF-8\"", s.store.InstanceName())
	writer.Header().Set("WWW-Authenticate", realm)
	plain(writer, http.StatusUnauthorized, "Unauthorized")
}

func (s *Server) handleRepository(writer http.ResponseWriter, request *http.Request) {
	name := routeVar(request, "repository")

	repository, err := s.store.RepositoryByName(name)
	if err != nil {
		plain(writer, http.StatusNotFound, "Repository not found")
		return
	}

	path := strings.TrimPrefix(strings.TrimPrefix(request.URL.Path, "/repository/"+name), "/")

	switch repository.Format {
	case format.NPM:
		s.serveNPM(writer, request, repository, path)
	case format.P2:
		s.serveP2(writer, request, repository, path)
	default:
		s.serveMaven(writer, request, repository, path)
	}
}

func (s *Server) authorize(writer http.ResponseWriter, request *http.Request, repository *models.Repository, required string) bool {
	user := currentUser(request)

	permission, err := s.store.EffectivePermission(repository, user)
	if err != nil {
		log.Errorf("resolving repository permission failed: %v", err)
		plain(writer, http.StatusInternalServerError, "Internal Server Error")
		return false
	}
	if store.Satisfies(permission, required) {
		return true
	}

	if user == nil {
		s.challenge(writer)
	} else {
		plain(writer, http.StatusForbidden, "Forbidden")
	}
	return false
}

func (s *Server) writeAsset(writer http.ResponseWriter, request *http.Request, asset *models.Asset) bool {
	file, info, err := s.blobs.Open(asset.StorageKey)
	if err != nil {
		return false
	}
	defer file.Close()

	header := writer.Header()
	header.Set("Content-Type", asset.ContentType)
	header.Set("X-Checksum-MD5", asset.MD5)
	header.Set("X-Checksum-SHA1", asset.SHA1)
	header.Set("X-Checksum-SHA256", asset.SHA256)
	header.Set("ETag", strconv.Quote(asset.SHA256))

	http.ServeContent(writer, request, asset.Path, info.ModTime(), file)
	return true
}

type uploadDetails struct {
	UploadedBy         *string
	FetchedAt          int64
	ExpiresAt          int64
	LastAccessedAt     int64
	RemoteETag         string
	RemoteLastModified string
}

// storeUpload writes the body to the blob store and records the asset, together
// with the component it belongs to when the path carries coordinates.
func (s *Server) storeUpload(repository *models.Repository, path string, body io.Reader, details uploadDetails) (*models.Asset, error) {
	layout := layoutFor(repository)
	key := repository.ID + "/" + path

	size, digests, err := s.blobs.Put(key, body, s.config.MaxUploadBytes)
	if err != nil {
		return nil, err
	}

	var componentID *string
	if coordinates, ok := layout.Coordinates(path); ok {
		identifier, err := s.store.UpsertComponent(&models.Component{
			RepositoryID: repository.ID,
			Format:       repository.Format,
			Namespace:    coordinates.Namespace,
			Name:         coordinates.Name,
			Version:      coordinates.Version,
			BaseVersion:  coordinates.BaseVersion,
			IsSnapshot:   coordinates.IsSnapshot,
		})
		if err != nil {
			return nil, err
		}
		componentID = &identifier
	}

	segments := strings.Split(path, "/")

	asset := &models.Asset{
		RepositoryID:       repository.ID,
		ComponentID:        componentID,
		Path:               path,
		StorageKey:         key,
		Size:               size,
		ContentType:        layout.ContentType(segments[len(segments)-1]),
		MD5:                digests.MD5,
		SHA1:               digests.SHA1,
		SHA256:             digests.SHA256,
		SHA512:             digests.SHA512,
		UploadedBy:         details.UploadedBy,
		FetchedAt:          details.FetchedAt,
		ExpiresAt:          details.ExpiresAt,
		LastAccessedAt:     details.LastAccessedAt,
		RemoteETag:         details.RemoteETag,
		RemoteLastModified: details.RemoteLastModified,
	}
	if err := s.store.UpsertAsset(asset); err != nil {
		return nil, err
	}
	return asset, nil
}

func (s *Server) writeDirectoryIndex(writer http.ResponseWriter, repository *models.Repository, path string) {
	entries, err := s.store.BrowseDirectory(repository.ID, path)
	if err != nil {
		log.Errorf("listing a directory failed: %v", err)
		plain(writer, http.StatusInternalServerError, "Internal Server Error")
		return
	}

	title := html.EscapeString("/" + repository.Name + "/" + path)

	var rows strings.Builder
	if path != "" {
		rows.WriteString(`<tr><td><a href="../">../</a></td><td>-</td><td></td></tr>`)
	}
	for _, entry := range entries {
		name := entry.Name
		size := "-"
		updated := ""
		if entry.Type == store.EntryDirectory {
			name += "/"
		} else {
			if entry.Size != nil {
				size = strconv.FormatInt(*entry.Size, 10)
			}
			if entry.UpdatedAt != nil {
				updated = time.UnixMilli(*entry.UpdatedAt).UTC().Format(time.RFC3339)
			}
		}
		escaped := html.EscapeString(name)
		fmt.Fprintf(&rows, "<tr><td><a href=\"%s\">%s</a></td><td>%s</td><td>%s</td></tr>\n", escaped, escaped, size, updated)
	}

	writer.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(writer, `<!doctype html>
<html><head><meta charset="utf-8"><title>Index of %s</title>
<style>body{font-family:ui-monospace,SFMono-Regular,Menlo,monospace;margin:2rem;color:#111}table{border-collapse:collapse}td{padding:.15rem 1.5rem .15rem 0}a{color:#0b57d0}</style>
</head><body><h1>Index of %s</h1><table>
%s
</table></body></html>
`, title, title, rows.String())
}
