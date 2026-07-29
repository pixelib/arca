package store

import (
	"strings"

	"gorm.io/gorm/clause"

	"arca/internal/store/models"
)

type ComponentRecord struct {
	Repository string `json:"repository"`
	Format     string `json:"format"`
	Namespace  string `json:"namespace"`
	Name       string `json:"name"`
	Version    string `json:"version"`
	IsSnapshot bool   `json:"isSnapshot"`
	CreatedAt  int64  `json:"createdAt"`
	UpdatedAt  int64  `json:"updatedAt"`
}

func (s *Store) UpsertComponent(component *models.Component) (string, error) {
	now := NowMillis()
	component.ID = NewID()
	component.CreatedAt = now
	component.UpdatedAt = now

	err := s.db.Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "repository_id"}, {Name: "namespace"}, {Name: "name"}, {Name: "version"},
		},
		DoUpdates: clause.Assignments(map[string]any{"updated_at": now}),
	}).Create(component).Error
	if err != nil {
		return "", err
	}

	var existing models.Component
	err = s.db.
		Where("repository_id = ? AND namespace = ? AND name = ? AND version = ?",
			component.RepositoryID, component.Namespace, component.Name, component.Version).
		First(&existing).Error
	if err != nil {
		return "", err
	}
	return existing.ID, nil
}

func (s *Store) ComponentVersions(repositoryID, namespace, name string) ([]models.Component, error) {
	var components []models.Component
	err := s.db.
		Where("repository_id = ? AND namespace = ? AND name = ?", repositoryID, namespace, name).
		Find(&components).Error
	return components, err
}

// ComponentFiles lists the assets that make up a component version. It follows
// the component link rather than a path prefix, because a version occupies a
// directory in some formats and a single file in others.
func (s *Store) ComponentFiles(repositoryID, namespace, name, version string) ([]BrowseEntry, error) {
	files := []FileEntry{}
	err := s.db.Raw(
		`SELECT assets.path, assets.size, assets.updated_at
		 FROM assets
		 JOIN components ON components.id = assets.component_id
		 WHERE assets.repository_id = ? AND components.namespace = ? AND components.name = ? AND components.version = ?
		 ORDER BY assets.path`,
		repositoryID, namespace, name, version,
	).Scan(&files).Error
	if err != nil {
		return nil, err
	}

	entries := make([]BrowseEntry, 0, len(files))
	for _, file := range files {
		size, updated := file.Size, file.UpdatedAt
		entries = append(entries, BrowseEntry{
			Name:      file.Path[strings.LastIndex(file.Path, "/")+1:],
			Path:      file.Path,
			Type:      EntryFile,
			Size:      &size,
			UpdatedAt: &updated,
		})
	}
	return entries, nil
}

func (s *Store) ComponentExists(repositoryID, namespace, name string) (bool, error) {
	var count int64
	err := s.db.Model(&models.Component{}).
		Where("repository_id = ? AND namespace = ? AND name = ?", repositoryID, namespace, name).
		Limit(1).
		Count(&count).Error
	return count > 0, err
}

func (s *Store) RecentComponents(repository *models.Repository, limit int) ([]ComponentRecord, error) {
	records := []ComponentRecord{}
	err := s.db.Model(&models.Component{}).
		Select("? AS repository, format, namespace, name, version, is_snapshot, created_at, updated_at", repository.Name).
		Where("repository_id = ?", repository.ID).
		Order("updated_at DESC").
		Limit(limit).
		Scan(&records).Error
	return records, err
}

func (s *Store) SearchComponents(repositories []models.Repository, query string, limit int) ([]ComponentRecord, error) {
	records := []ComponentRecord{}
	if len(repositories) == 0 {
		return records, nil
	}

	ids := make([]string, 0, len(repositories))
	for _, repository := range repositories {
		ids = append(ids, repository.ID)
	}

	needle := strings.ToLower(query)
	err := s.db.Model(&models.Component{}).
		Select("repositories.name AS repository, components.format, components.namespace, components.name, components.version, components.is_snapshot, components.created_at, components.updated_at").
		Joins("JOIN repositories ON repositories.id = components.repository_id").
		Where("components.repository_id IN ?", ids).
		Where("instr(lower(components.namespace), ?) > 0 OR instr(lower(components.name), ?) > 0", needle, needle).
		Order("components.updated_at DESC").
		Limit(limit).
		Scan(&records).Error

	return records, err
}

func (s *Store) PruneOrphanComponents(repositoryID string) error {
	return s.db.
		Where("repository_id = ?", repositoryID).
		Where("id NOT IN (SELECT component_id FROM assets WHERE component_id IS NOT NULL)").
		Delete(&models.Component{}).Error
}
