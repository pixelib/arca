package server

import (
	"net/http"
	"slices"
	"sort"
	"strings"

	"arca/internal/format"
	"arca/internal/store"
	"arca/internal/store/models"
)

type artifactSummary struct {
	Repository   string  `json:"repository"`
	Format       string  `json:"format"`
	Namespace    string  `json:"namespace"`
	Name         string  `json:"name"`
	VersionCount int     `json:"versionCount"`
	Latest       *string `json:"latest"`
	Release      *string `json:"release"`
	UpdatedAt    int64   `json:"updatedAt"`
}

func nullable(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func groupComponents(records []store.ComponentRecord) []artifactSummary {
	type bucket struct {
		record   store.ComponentRecord
		versions []string
		updated  int64
	}

	order := make([]string, 0, len(records))
	buckets := map[string]*bucket{}

	for _, record := range records {
		key := record.Repository + "|" + record.Namespace + "|" + record.Name
		existing, ok := buckets[key]
		if !ok {
			buckets[key] = &bucket{record: record, versions: []string{record.Version}, updated: record.UpdatedAt}
			order = append(order, key)
			continue
		}
		existing.versions = append(existing.versions, record.Version)
		existing.updated = max(existing.updated, record.UpdatedAt)
	}

	summaries := make([]artifactSummary, 0, len(order))
	for _, key := range order {
		entry := buckets[key]
		latest, release := format.LatestAndRelease(layoutForFormat(entry.record.Format), entry.versions)
		summaries = append(summaries, artifactSummary{
			Repository:   entry.record.Repository,
			Format:       entry.record.Format,
			Namespace:    entry.record.Namespace,
			Name:         entry.record.Name,
			VersionCount: len(entry.versions),
			Latest:       nullable(latest),
			Release:      nullable(release),
			UpdatedAt:    entry.updated,
		})
	}

	sort.SliceStable(summaries, func(i, j int) bool {
		return summaries[i].UpdatedAt > summaries[j].UpdatedAt
	})
	return summaries
}

func limitSummaries(summaries []artifactSummary, limit int) []artifactSummary {
	if len(summaries) > limit {
		return summaries[:limit]
	}
	return summaries
}

func normalizeBrowsePath(raw string) string {
	trimmed := strings.Trim(raw, "/")
	if trimmed == "" {
		return ""
	}
	return trimmed + "/"
}

func (s *Server) routeBrowse(writer http.ResponseWriter, request *http.Request) error {
	repository, _, err := s.loadAccessible(request, models.PermissionRead)
	if err != nil {
		return err
	}

	path := normalizeBrowsePath(request.URL.Query().Get("path"))

	exists, err := s.store.DirectoryExists(repository.ID, path)
	if err != nil {
		return err
	}
	if !exists {
		return fail(http.StatusNotFound, "Directory not found")
	}

	entries, err := s.store.BrowseDirectory(repository.ID, path)
	if err != nil {
		return err
	}

	response := map[string]any{"path": path, "entries": entries, "artifact": nil}

	if coordinates, ok := layoutFor(repository).ComponentAt(path); ok {
		known, err := s.store.ComponentExists(repository.ID, coordinates.Namespace, coordinates.Name)
		if err != nil {
			return err
		}
		if known {
			response["artifact"] = map[string]string{
				"namespace": coordinates.Namespace,
				"name":      coordinates.Name,
			}
		}
	}

	writeJSON(writer, http.StatusOK, response)
	return nil
}

type artifactVersion struct {
	Version    string `json:"version"`
	IsSnapshot bool   `json:"isSnapshot"`
	UpdatedAt  int64  `json:"updatedAt"`
	CreatedAt  int64  `json:"createdAt"`
}

func (s *Server) routeArtifact(writer http.ResponseWriter, request *http.Request) error {
	repository, _, err := s.loadAccessible(request, models.PermissionRead)
	if err != nil {
		return err
	}

	namespace, name, err := requireCoordinates(request)
	if err != nil {
		return err
	}

	components, err := s.store.ComponentVersions(repository.ID, namespace, name)
	if err != nil {
		return err
	}
	if len(components) == 0 {
		return fail(http.StatusNotFound, "Artifact not found")
	}

	layout := layoutFor(repository)

	versions := make([]artifactVersion, 0, len(components))
	names := make([]string, 0, len(components))
	for _, component := range components {
		versions = append(versions, artifactVersion{
			Version:    component.Version,
			IsSnapshot: component.IsSnapshot,
			UpdatedAt:  component.UpdatedAt,
			CreatedAt:  component.CreatedAt,
		})
		names = append(names, component.Version)
	}

	sort.SliceStable(versions, func(i, j int) bool {
		return layout.Compare(versions[i].Version, versions[j].Version) > 0
	})
	latest, release := format.LatestAndRelease(layout, names)

	writeJSON(writer, http.StatusOK, map[string]any{
		"repository": repository.Name,
		"format":     repository.Format,
		"namespace":  namespace,
		"name":       name,
		"latest":     nullable(latest),
		"release":    nullable(release),
		"versions":   versions,
	})
	return nil
}

// requireCoordinates reads the namespace and name a content route addresses. A
// namespace is optional because not every format has one: npm leaves it empty
// for unscoped packages.
func requireCoordinates(request *http.Request) (namespace string, name string, err error) {
	query := request.URL.Query()

	namespace = strings.TrimSpace(query.Get("namespace"))
	if len(namespace) > 300 {
		return "", "", fail(http.StatusBadRequest, "namespace is too long")
	}

	name, err = requireText(query.Get("name"), "name", 200)
	if err != nil {
		return "", "", err
	}
	return namespace, name, nil
}

func (s *Server) routeArtifactFiles(writer http.ResponseWriter, request *http.Request) error {
	repository, _, err := s.loadAccessible(request, models.PermissionRead)
	if err != nil {
		return err
	}

	namespace, name, err := requireCoordinates(request)
	if err != nil {
		return err
	}
	version, err := requireText(request.URL.Query().Get("version"), "version", 200)
	if err != nil {
		return err
	}

	files, err := s.store.ComponentFiles(repository.ID, namespace, name, version)
	if err != nil {
		return err
	}

	writeJSON(writer, http.StatusOK, map[string]any{
		"repository": repository.Name,
		"format":     repository.Format,
		"namespace":  namespace,
		"name":       name,
		"version":    version,
		"files":      files,
	})
	return nil
}

func (s *Server) routeDeleteArtifact(writer http.ResponseWriter, request *http.Request) error {
	repository, _, err := s.loadAccessible(request, models.PermissionAdmin)
	if err != nil {
		return err
	}

	namespace, name, err := requireCoordinates(request)
	if err != nil {
		return err
	}

	components, err := s.store.ComponentVersions(repository.ID, namespace, name)
	if err != nil {
		return err
	}
	if version := strings.TrimSpace(request.URL.Query().Get("version")); version != "" {
		components = slices.DeleteFunc(components, func(component models.Component) bool {
			return component.Version != version
		})
	}
	if len(components) == 0 {
		return fail(http.StatusNotFound, "Artifact not found")
	}

	versions := make([]string, 0, len(components))
	for _, component := range components {
		versions = append(versions, component.Version)
	}

	keys, err := s.store.DeleteComponentVersions(repository.ID, namespace, name, versions)
	if err != nil {
		return err
	}
	if err := s.blobs.Delete(keys...); err != nil {
		return err
	}

	writeJSON(writer, http.StatusOK, map[string]any{"versions": len(components), "files": len(keys)})
	return nil
}

func (s *Server) routeRecent(writer http.ResponseWriter, request *http.Request) error {
	repository, _, err := s.loadAccessible(request, models.PermissionRead)
	if err != nil {
		return err
	}

	records, err := s.store.RecentComponents(repository, 200)
	if err != nil {
		return err
	}

	writeJSON(writer, http.StatusOK, map[string]any{"artifacts": limitSummaries(groupComponents(records), 25)})
	return nil
}

func (s *Server) routeSearch(writer http.ResponseWriter, request *http.Request) error {
	query := strings.TrimSpace(request.URL.Query().Get("q"))
	if len(query) < 2 {
		writeJSON(writer, http.StatusOK, map[string]any{"results": []artifactSummary{}})
		return nil
	}

	repositories, err := s.store.VisibleRepositories(currentUser(request))
	if err != nil {
		return err
	}

	if scoped := request.URL.Query().Get("repository"); scoped != "" {
		targets := make([]models.Repository, 0, 1)
		for _, repository := range repositories {
			if repository.Name == scoped {
				targets = append(targets, repository)
			}
		}
		repositories = targets
	}

	records, err := s.store.SearchComponents(repositories, query, 500)
	if err != nil {
		return err
	}

	writeJSON(writer, http.StatusOK, map[string]any{"results": limitSummaries(groupComponents(records), 50)})
	return nil
}
