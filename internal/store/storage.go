package store

import (
	"strings"

	"arca/internal/store/models"
)

var knownExtensions = []string{
	"jar", "pom", "module", "war", "ear", "aar", "zip", "tgz", "gz", "xml", "json", "txt", "asc",
}

const otherExtension = "other"

type StorageTotals struct {
	Repositories  int64 `json:"repositories"`
	Components    int64 `json:"components"`
	Assets        int64 `json:"assets"`
	Bytes         int64 `json:"bytes"`
	SnapshotBytes int64 `json:"snapshotBytes"`
	ReleaseBytes  int64 `json:"releaseBytes"`
}

type RepositoryUsage struct {
	Repository    string `json:"repository"`
	Components    int64  `json:"components"`
	Assets        int64  `json:"assets"`
	Bytes         int64  `json:"bytes"`
	SnapshotBytes int64  `json:"snapshotBytes"`
	ReleaseBytes  int64  `json:"releaseBytes"`
}

type ExtensionUsage struct {
	Extension string `json:"extension"`
	Files     int64  `json:"files"`
	Bytes     int64  `json:"bytes"`
}

type ComponentUsage struct {
	Repository string `json:"repository"`
	Format     string `json:"format"`
	Namespace  string `json:"namespace"`
	Name       string `json:"name"`
	Version    string `json:"version"`
	IsSnapshot bool   `json:"isSnapshot"`
	Files      int64  `json:"files"`
	Bytes      int64  `json:"bytes"`
	UpdatedAt  int64  `json:"updatedAt"`
}

const snapshotBytes = `
	COALESCE(SUM(CASE WHEN components.is_snapshot THEN assets.size ELSE 0 END), 0) AS snapshot_bytes,
	COALESCE(SUM(CASE WHEN components.is_snapshot THEN 0 ELSE assets.size END), 0) AS release_bytes`

func extensionCase(column string) string {
	var expression strings.Builder
	expression.WriteString("CASE")
	for _, extension := range knownExtensions {
		expression.WriteString(" WHEN " + column + " LIKE '%." + extension + "' THEN '" + extension + "'")
	}
	expression.WriteString(" ELSE '" + otherExtension + "' END")
	return expression.String()
}

func (s *Store) StorageTotals() (StorageTotals, error) {
	var totals StorageTotals
	err := s.db.Raw(
		`SELECT
		   (SELECT COUNT(*) FROM repositories) AS repositories,
		   (SELECT COUNT(*) FROM components) AS components,
		   (SELECT COUNT(*) FROM assets) AS assets,
		   (SELECT COALESCE(SUM(size), 0) FROM assets) AS bytes,
		   (SELECT COALESCE(SUM(assets.size), 0) FROM assets
		      JOIN components ON components.id = assets.component_id WHERE components.is_snapshot) AS snapshot_bytes,
		   (SELECT COALESCE(SUM(assets.size), 0) FROM assets
		      LEFT JOIN components ON components.id = assets.component_id
		      WHERE components.id IS NULL OR NOT components.is_snapshot) AS release_bytes`,
	).Scan(&totals).Error
	return totals, err
}

func (s *Store) StorageByRepository() ([]RepositoryUsage, error) {
	usage := []RepositoryUsage{}
	err := s.db.Raw(
		`SELECT repositories.name AS repository,
		        COUNT(assets.id) AS assets,
		        COALESCE(SUM(assets.size), 0) AS bytes,` + snapshotBytes + `,
		        (SELECT COUNT(*) FROM components WHERE components.repository_id = repositories.id) AS components
		 FROM repositories
		 LEFT JOIN assets ON assets.repository_id = repositories.id
		 LEFT JOIN components ON components.id = assets.component_id
		 GROUP BY repositories.id
		 ORDER BY bytes DESC`,
	).Scan(&usage).Error
	return usage, err
}

func (s *Store) StorageByExtension() ([]ExtensionUsage, error) {
	usage := []ExtensionUsage{}
	err := s.db.Raw(
		`SELECT ` + extensionCase("path") + ` AS extension, COUNT(*) AS files, COALESCE(SUM(size), 0) AS bytes
		 FROM assets GROUP BY extension ORDER BY bytes DESC`,
	).Scan(&usage).Error
	return usage, err
}

func (s *Store) LargestComponents(limit int) ([]ComponentUsage, error) {
	usage := []ComponentUsage{}
	err := s.db.Raw(
		`SELECT repositories.name AS repository, components.format, components.namespace, components.name, components.version,
		        components.is_snapshot, components.updated_at,
		        COUNT(assets.id) AS files, COALESCE(SUM(assets.size), 0) AS bytes
		 FROM components
		 JOIN repositories ON repositories.id = components.repository_id
		 LEFT JOIN assets ON assets.component_id = components.id
		 GROUP BY components.id
		 ORDER BY bytes DESC
		 LIMIT ?`,
		limit,
	).Scan(&usage).Error
	return usage, err
}

func (s *Store) ComponentsByRepository(repositoryID string, search string, limit int) ([]ComponentUsage, error) {
	usage := []ComponentUsage{}

	query := s.db.Table("components").
		Select(`repositories.name AS repository, components.format, components.namespace, components.name, components.version,
		        components.is_snapshot, components.updated_at,
		        COUNT(assets.id) AS files, COALESCE(SUM(assets.size), 0) AS bytes`).
		Joins("JOIN repositories ON repositories.id = components.repository_id").
		Joins("LEFT JOIN assets ON assets.component_id = components.id").
		Group("components.id").
		Order("components.updated_at DESC").
		Limit(limit)

	if repositoryID != "" {
		query = query.Where("components.repository_id = ?", repositoryID)
	}
	if search != "" {
		needle := strings.ToLower(search)
		query = query.Where(
			"instr(lower(components.namespace), ?) > 0 OR instr(lower(components.name), ?) > 0",
			needle, needle,
		)
	}

	return usage, query.Scan(&usage).Error
}

// DeleteComponentVersions removes the assets belonging to the named versions of
// one component, following the component link rather than a path prefix.
func (s *Store) DeleteComponentVersions(repositoryID, namespace, name string, versions []string) ([]string, error) {
	var keys []string

	err := s.Transaction(func(tx *Store) error {
		var ids []string
		err := tx.db.Model(&models.Component{}).
			Where("repository_id = ? AND namespace = ? AND name = ? AND version IN ?",
				repositoryID, namespace, name, versions).
			Pluck("id", &ids).Error
		if err != nil {
			return err
		}
		if len(ids) == 0 {
			return nil
		}

		keys, err = tx.deleteMatchingAssets(repositoryID, ofComponents(ids))
		if err != nil || len(keys) == 0 {
			return err
		}
		return tx.PruneOrphanComponents(repositoryID)
	})

	return keys, err
}
