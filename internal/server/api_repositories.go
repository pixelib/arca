package server

import (
	"net/http"
	"net/url"
	"strings"

	"arca/internal/format"
	"arca/internal/proxy"
	"arca/internal/store"
	"arca/internal/store/models"
)

const (
	maxTTLSeconds    = 365 * 24 * 60 * 60
	maxRetentionDays = 3650
)

var (
	policies        = []string{models.PolicyRelease, models.PolicySnapshot, models.PolicyMixed}
	visibilities    = []string{models.VisibilityPublic, models.VisibilityPrivate}
	permissions     = []string{models.PermissionRead, models.PermissionWrite, models.PermissionAdmin}
	repositoryTypes = []string{models.TypeHosted, models.TypeProxy}
	formats         = []string{format.Maven2, format.NPM, format.P2}
)

type repositoryResponse struct {
	ID                 string                 `json:"id"`
	Name               string                 `json:"name"`
	Format             string                 `json:"format"`
	Type               string                 `json:"type"`
	CategoryID         *string                `json:"categoryId"`
	CategoryName       string                 `json:"categoryName"`
	Policy             string                 `json:"policy"`
	Visibility         string                 `json:"visibility"`
	Description        string                 `json:"description"`
	AllowRedeploy      bool                   `json:"allowRedeploy"`
	RemoteURL          string                 `json:"remoteUrl"`
	RemoteUsername     string                 `json:"remoteUsername"`
	MetadataTTLSeconds int64                  `json:"metadataTtlSeconds"`
	NegativeTTLSeconds int64                  `json:"negativeTtlSeconds"`
	CacheRetentionDays int64                  `json:"cacheRetentionDays"`
	CreatedAt          int64                  `json:"createdAt"`
	Permission         *string                `json:"permission"`
	Components         *int64                 `json:"components,omitempty"`
	UpdatedAt          *int64                 `json:"updatedAt,omitempty"`
	Stats              *store.RepositoryStats `json:"stats,omitempty"`
}

// serializeRepository never returns RemotePassword, which is write-only from
// the API's point of view.
func serializeRepository(repository models.Repository, permission, categoryName string) repositoryResponse {
	response := repositoryResponse{
		ID:                 repository.ID,
		Name:               repository.Name,
		Format:             repository.Format,
		Type:               repository.Type,
		CategoryID:         repository.CategoryID,
		CategoryName:       categoryName,
		Policy:             repository.Policy,
		Visibility:         repository.Visibility,
		Description:        repository.Description,
		AllowRedeploy:      repository.AllowRedeploy,
		RemoteURL:          repository.RemoteURL,
		RemoteUsername:     repository.RemoteUsername,
		MetadataTTLSeconds: repository.MetadataTTLSeconds,
		NegativeTTLSeconds: repository.NegativeTTLSeconds,
		CacheRetentionDays: repository.CacheRetentionDays,
		CreatedAt:          repository.CreatedAt,
	}
	if permission != "" {
		response.Permission = &permission
	}
	return response
}

func requireTTL(value *int64, fallback int64, field string) (int64, error) {
	if value == nil {
		return fallback, nil
	}
	if *value < 0 || *value > maxTTLSeconds {
		return 0, fail(http.StatusBadRequest, "%s must be between 0 and %d seconds", field, maxTTLSeconds)
	}
	return *value, nil
}

// requireRetention reads the eviction window in days, where zero keeps cached
// files until the repository is purged by hand.
func requireRetention(value *int64, repositoryType string) (int64, error) {
	if value == nil {
		return 0, nil
	}
	if repositoryType != models.TypeProxy && *value != 0 {
		return 0, fail(http.StatusBadRequest, "cacheRetentionDays is only valid on proxy repositories")
	}
	if *value < 0 || *value > maxRetentionDays {
		return 0, fail(http.StatusBadRequest, "cacheRetentionDays must be between 0 and %d days", maxRetentionDays)
	}
	return *value, nil
}

func (s *Server) loadAccessible(request *http.Request, required string) (*models.Repository, string, error) {
	repository, err := s.store.RepositoryByName(routeVar(request, "name"))
	if err != nil {
		return nil, "", fail(http.StatusNotFound, "Repository not found")
	}

	user := currentUser(request)
	permission, err := s.store.EffectivePermission(repository, user)
	if err != nil {
		return nil, "", err
	}
	if !store.Satisfies(permission, required) {
		status := http.StatusUnauthorized
		if user != nil {
			status = http.StatusForbidden
		}
		return nil, "", fail(status, "You do not have access to this repository")
	}

	return repository, permission, nil
}

func (s *Server) loadAdministered(request *http.Request) (*models.Repository, error) {
	if _, err := requireAdmin(request); err != nil {
		return nil, err
	}

	repository, err := s.store.RepositoryByName(routeVar(request, "name"))
	if err != nil {
		return nil, fail(http.StatusNotFound, "Repository not found")
	}
	return repository, nil
}

func (s *Server) routeListRepositories(writer http.ResponseWriter, request *http.Request) error {
	user := currentUser(request)

	repositories, err := s.store.VisibleRepositories(user)
	if err != nil {
		return err
	}

	identifiers := make([]string, 0, len(repositories))
	for _, repository := range repositories {
		identifiers = append(identifiers, repository.ID)
	}

	summaries, err := s.store.ComponentSummaries(identifiers)
	if err != nil {
		return err
	}

	categories, err := s.store.ListCategories()
	if err != nil {
		return err
	}
	names := make(map[string]string, len(categories))
	for _, category := range categories {
		names[category.ID] = category.Name
	}

	responses := make([]repositoryResponse, 0, len(repositories))
	for _, repository := range repositories {
		permission, err := s.store.EffectivePermission(&repository, user)
		if err != nil {
			return err
		}

		categoryName := ""
		if repository.CategoryID != nil {
			categoryName = names[*repository.CategoryID]
		}

		summary := summaries[repository.ID]
		response := serializeRepository(repository, permission, categoryName)
		response.Components = &summary.Components
		response.UpdatedAt = &summary.UpdatedAt
		responses = append(responses, response)
	}

	writeJSON(writer, http.StatusOK, map[string]any{"repositories": responses})
	return nil
}

func (s *Server) routeCreateRepository(writer http.ResponseWriter, request *http.Request) error {
	if _, err := requireAdmin(request); err != nil {
		return err
	}

	var body struct {
		Name               string `json:"name"`
		Format             string `json:"format"`
		Type               string `json:"type"`
		Policy             string `json:"policy"`
		Visibility         string `json:"visibility"`
		Description        string `json:"description"`
		AllowRedeploy      bool   `json:"allowRedeploy"`
		RemoteURL          string `json:"remoteUrl"`
		RemoteUsername     string `json:"remoteUsername"`
		RemotePassword     string `json:"remotePassword"`
		MetadataTTLSeconds *int64 `json:"metadataTtlSeconds"`
		NegativeTTLSeconds *int64 `json:"negativeTtlSeconds"`
		CacheRetentionDays *int64 `json:"cacheRetentionDays"`
		CategoryID         string `json:"categoryId"`
	}
	if err := decodeBody(request, &body); err != nil {
		return err
	}

	name, err := requireText(body.Name, "name", 64)
	if err != nil {
		return err
	}
	name = strings.ToLower(name)
	if !store.IsValidRepositoryName(name) {
		return fail(http.StatusBadRequest, "name must be lowercase alphanumeric with dots, dashes or underscores")
	}

	repositoryFormat, err := requireOneOf(orDefault(body.Format, format.Maven2), formats, "format")
	if err != nil {
		return err
	}
	repositoryType, err := requireOneOf(orDefault(body.Type, models.TypeHosted), repositoryTypes, "type")
	if err != nil {
		return err
	}
	policy, err := requireOneOf(orDefault(body.Policy, models.PolicyRelease), policies, "policy")
	if err != nil {
		return err
	}
	visibility, err := requireOneOf(orDefault(body.Visibility, models.VisibilityPrivate), visibilities, "visibility")
	if err != nil {
		return err
	}
	if len(body.Description) > 500 {
		return fail(http.StatusBadRequest, "description is too long")
	}

	metadataTTL, err := requireTTL(body.MetadataTTLSeconds, models.DefaultMetadataTTLSeconds, "metadataTtlSeconds")
	if err != nil {
		return err
	}
	negativeTTL, err := requireTTL(body.NegativeTTLSeconds, models.DefaultNegativeTTLSeconds, "negativeTtlSeconds")
	if err != nil {
		return err
	}

	retentionDays, err := requireRetention(body.CacheRetentionDays, repositoryType)
	if err != nil {
		return err
	}

	remoteURL, err := normalizeRemote(repositoryType, body.RemoteURL)
	if err != nil {
		return err
	}
	if err := validateP2(repositoryFormat, repositoryType, remoteURL); err != nil {
		return err
	}

	categoryID, err := s.resolveCategory(body.CategoryID)
	if err != nil {
		return err
	}

	if _, err := s.store.RepositoryByName(name); err == nil {
		return fail(http.StatusConflict, "A repository with that name already exists")
	}

	repository := &models.Repository{
		Name:               name,
		Format:             repositoryFormat,
		Type:               repositoryType,
		CategoryID:         categoryID,
		Policy:             policy,
		Visibility:         visibility,
		Description:        strings.TrimSpace(body.Description),
		AllowRedeploy:      body.AllowRedeploy || policy == models.PolicySnapshot,
		RemoteURL:          remoteURL,
		RemoteUsername:     strings.TrimSpace(body.RemoteUsername),
		RemotePassword:     body.RemotePassword,
		MetadataTTLSeconds: metadataTTL,
		NegativeTTLSeconds: negativeTTL,
		CacheRetentionDays: retentionDays,
	}
	if err := s.store.CreateRepository(repository); err != nil {
		return err
	}

	writeJSON(writer, http.StatusCreated, serializeRepository(*repository, models.PermissionAdmin, s.categoryNameFor(repository.CategoryID)))
	return nil
}

func (s *Server) routeRepository(writer http.ResponseWriter, request *http.Request) error {
	repository, permission, err := s.loadAccessible(request, models.PermissionRead)
	if err != nil {
		return err
	}

	stats, err := s.store.RepositoryStats(repository.ID)
	if err != nil {
		return err
	}

	response := serializeRepository(*repository, permission, s.categoryNameFor(repository.CategoryID))
	response.Stats = &stats

	writeJSON(writer, http.StatusOK, response)
	return nil
}

func (s *Server) routeUpdateRepository(writer http.ResponseWriter, request *http.Request) error {
	repository, err := s.loadAdministered(request)
	if err != nil {
		return err
	}

	var body struct {
		Format             *string `json:"format"`
		Policy             *string `json:"policy"`
		Visibility         *string `json:"visibility"`
		Description        *string `json:"description"`
		AllowRedeploy      *bool   `json:"allowRedeploy"`
		RemoteURL          *string `json:"remoteUrl"`
		RemoteUsername     *string `json:"remoteUsername"`
		RemotePassword     *string `json:"remotePassword"`
		MetadataTTLSeconds *int64  `json:"metadataTtlSeconds"`
		NegativeTTLSeconds *int64  `json:"negativeTtlSeconds"`
		CacheRetentionDays *int64  `json:"cacheRetentionDays"`
		CategoryID         *string `json:"categoryId"`
	}
	if err := decodeBody(request, &body); err != nil {
		return err
	}

	// Format is fixed at creation: the stored paths and coordinates mean one
	// thing per format, and nothing migrates them between two.
	if body.Format != nil && *body.Format != repository.Format {
		return fail(http.StatusBadRequest, "format cannot be changed after a repository is created")
	}

	changes := map[string]any{}
	if body.Policy != nil {
		policy, err := requireOneOf(*body.Policy, policies, "policy")
		if err != nil {
			return err
		}
		changes["policy"] = policy
	}
	if body.Visibility != nil {
		visibility, err := requireOneOf(*body.Visibility, visibilities, "visibility")
		if err != nil {
			return err
		}
		changes["visibility"] = visibility
	}
	if body.Description != nil {
		if len(*body.Description) > 500 {
			return fail(http.StatusBadRequest, "description is too long")
		}
		changes["description"] = strings.TrimSpace(*body.Description)
	}
	if body.AllowRedeploy != nil {
		changes["allow_redeploy"] = *body.AllowRedeploy
	}
	if body.RemoteURL != nil {
		remoteURL, err := normalizeRemote(repository.Type, *body.RemoteURL)
		if err != nil {
			return err
		}
		changes["remote_url"] = remoteURL
	}
	if body.RemoteUsername != nil {
		changes["remote_username"] = strings.TrimSpace(*body.RemoteUsername)
	}
	if body.RemotePassword != nil {
		changes["remote_password"] = *body.RemotePassword
	}
	if body.MetadataTTLSeconds != nil {
		ttl, err := requireTTL(body.MetadataTTLSeconds, 0, "metadataTtlSeconds")
		if err != nil {
			return err
		}
		changes["metadata_ttl_seconds"] = ttl
	}
	if body.NegativeTTLSeconds != nil {
		ttl, err := requireTTL(body.NegativeTTLSeconds, 0, "negativeTtlSeconds")
		if err != nil {
			return err
		}
		changes["negative_ttl_seconds"] = ttl
	}
	if body.CacheRetentionDays != nil {
		retentionDays, err := requireRetention(body.CacheRetentionDays, repository.Type)
		if err != nil {
			return err
		}
		changes["cache_retention_days"] = retentionDays
	}
	// An empty categoryId moves the repository back out of any category, which
	// is why this reads a string rather than relying on a JSON null.
	if body.CategoryID != nil {
		categoryID, err := s.resolveCategory(*body.CategoryID)
		if err != nil {
			return err
		}
		changes["category_id"] = categoryID
	}
	if len(changes) == 0 {
		return fail(http.StatusBadRequest, "No changes supplied")
	}

	if err := s.store.UpdateRepository(repository.ID, changes); err != nil {
		return err
	}

	updated, err := s.store.RepositoryByName(repository.Name)
	if err != nil {
		return err
	}

	writeJSON(writer, http.StatusOK, serializeRepository(*updated, models.PermissionAdmin, s.categoryNameFor(updated.CategoryID)))
	return nil
}

func (s *Server) routeDeleteRepository(writer http.ResponseWriter, request *http.Request) error {
	repository, err := s.loadAdministered(request)
	if err != nil {
		return err
	}

	keys, err := s.store.DeleteRepository(repository.ID)
	if err != nil {
		return err
	}
	if err := s.blobs.Delete(keys...); err != nil {
		return err
	}

	return noContent(writer)
}

func (s *Server) routePurgeCache(writer http.ResponseWriter, request *http.Request) error {
	repository, err := s.loadAdministered(request)
	if err != nil {
		return err
	}
	if !repository.IsProxy() {
		return fail(http.StatusBadRequest, "Only proxy repositories have a cache to purge")
	}

	keys, err := s.store.DeleteAssetTree(repository.ID, "")
	if err != nil {
		return err
	}
	if err := s.store.ClearRemoteMisses(repository.ID); err != nil {
		return err
	}
	if err := s.blobs.Delete(keys...); err != nil {
		return err
	}

	writeJSON(writer, http.StatusOK, map[string]any{"purged": len(keys)})
	return nil
}

func normalizeRemote(repositoryType, raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)

	if repositoryType != models.TypeProxy {
		if trimmed != "" {
			return "", fail(http.StatusBadRequest, "remoteUrl is only valid on proxy repositories")
		}
		return "", nil
	}

	if trimmed == "" {
		return "", fail(http.StatusBadRequest, "remoteUrl is required for proxy repositories")
	}

	normalized, err := proxy.NormalizeRemoteURL(trimmed)
	if err != nil {
		return "", fail(http.StatusBadRequest, "remoteUrl must be an absolute http or https URL")
	}
	return normalized, nil
}

func (s *Server) routeListGrants(writer http.ResponseWriter, request *http.Request) error {
	repository, err := s.loadAdministered(request)
	if err != nil {
		return err
	}

	grants, err := s.store.ListGrants(repository.ID)
	if err != nil {
		return err
	}

	writeJSON(writer, http.StatusOK, map[string]any{"grants": grants})
	return nil
}

func (s *Server) routeReplaceGrants(writer http.ResponseWriter, request *http.Request) error {
	repository, err := s.loadAdministered(request)
	if err != nil {
		return err
	}

	var body struct {
		Grants *[]struct {
			UserID     string `json:"userId"`
			Permission string `json:"permission"`
		} `json:"grants"`
	}
	if err := decodeBody(request, &body); err != nil {
		return err
	}
	if body.Grants == nil {
		return fail(http.StatusBadRequest, "grants must be an array")
	}

	grants := make([]models.Grant, 0, len(*body.Grants))
	for _, entry := range *body.Grants {
		userID, err := requireText(entry.UserID, "userId", 64)
		if err != nil {
			return err
		}
		permission, err := requireOneOf(entry.Permission, permissions, "permission")
		if err != nil {
			return err
		}
		grants = append(grants, models.Grant{RepositoryID: repository.ID, UserID: userID, Permission: permission})
	}

	if err := s.store.ReplaceGrants(repository.ID, grants); err != nil {
		return err
	}

	return noContent(writer)
}

func orDefault(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

// validateP2 pins a p2 proxy to a host root. Composite sites routinely point at
// children above their own directory, as the Eclipse release train does with
// '../../technology/epp/packages/', so a remote pinned to one update site loses
// half its children. Proxying the host root keeps those resolvable inside this
// repository. A hosted p2 repository has no remote and no such constraint.
func validateP2(repositoryFormat, repositoryType, remoteURL string) error {
	if repositoryFormat != format.P2 || repositoryType != models.TypeProxy {
		return nil
	}

	parsed, err := url.Parse(remoteURL)
	if err != nil || strings.Trim(parsed.Path, "/") != "" {
		return fail(http.StatusBadRequest, "remoteUrl for a p2 proxy must be a host root, such as https://download.eclipse.org")
	}
	return nil
}
