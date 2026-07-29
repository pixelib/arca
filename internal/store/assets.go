package store

import (
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"arca/internal/store/models"
)

type assetSelection func(*gorm.DB) *gorm.DB

func (s *Store) FindAsset(repositoryID, path string) (*models.Asset, error) {
	var asset models.Asset
	if err := s.db.First(&asset, "repository_id = ? AND path = ?", repositoryID, path).Error; err != nil {
		return nil, err
	}
	return &asset, nil
}

func (s *Store) UpsertAsset(asset *models.Asset) error {
	now := NowMillis()
	asset.ID = NewID()
	asset.CreatedAt = now
	asset.UpdatedAt = now

	return s.db.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "repository_id"}, {Name: "path"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"component_id", "storage_key", "size", "content_type",
			"md5", "sha1", "sha256", "sha512", "uploaded_by", "updated_at",
			"fetched_at", "expires_at", "last_accessed_at", "remote_etag", "remote_last_modified",
		}),
	}).Create(asset).Error
}

// MarkAssetRefreshed and MarkAssetAccessed write through UpdateColumns so the
// cache bookkeeping never bumps updated_at, which dates the artifact itself.
func (s *Store) MarkAssetRefreshed(id string, expiresAt int64, etag, lastModified string) error {
	now := NowMillis()

	return s.db.Model(&models.Asset{}).Where("id = ?", id).UpdateColumns(map[string]any{
		"fetched_at":           now,
		"expires_at":           expiresAt,
		"last_accessed_at":     now,
		"remote_etag":          etag,
		"remote_last_modified": lastModified,
	}).Error
}

func (s *Store) MarkAssetAccessed(id string) error {
	return s.db.Model(&models.Asset{}).Where("id = ?", id).
		UpdateColumn("last_accessed_at", NowMillis()).Error
}

func exactPath(path string) assetSelection {
	return func(query *gorm.DB) *gorm.DB {
		return query.Where("path = ?", path)
	}
}

func underPrefix(prefix string) assetSelection {
	bounds, bounded := RangeForPrefix(prefix)

	return func(query *gorm.DB) *gorm.DB {
		if !bounded {
			return query
		}
		return query.Where("path >= ? AND path < ?", bounds.Lower, bounds.Upper)
	}
}

func ofComponents(componentIDs []string) assetSelection {
	return func(query *gorm.DB) *gorm.DB {
		return query.Where("component_id IN ?", componentIDs)
	}
}

// idleSince falls back to the creation time so an asset stored before access
// tracking existed is aged from when it arrived rather than treated as ancient.
func idleSince(cutoff int64) assetSelection {
	return func(query *gorm.DB) *gorm.DB {
		return query.Where("max(last_accessed_at, created_at) < ?", cutoff)
	}
}

func (s *Store) DeleteAsset(repositoryID, path string) ([]string, error) {
	return s.deleteAssets(repositoryID, exactPath(path))
}

func (s *Store) DeleteIdleAssets(repositoryID string, cutoff int64) ([]string, error) {
	return s.deleteAssets(repositoryID, idleSince(cutoff))
}

func (s *Store) DeleteAssetTree(repositoryID, prefix string) ([]string, error) {
	return s.deleteAssets(repositoryID, underPrefix(prefix))
}

func (s *Store) deleteAssets(repositoryID string, selection assetSelection) ([]string, error) {
	var keys []string

	err := s.Transaction(func(tx *Store) error {
		removed, err := tx.deleteMatchingAssets(repositoryID, selection)
		if err != nil {
			return err
		}
		keys = removed
		if len(keys) == 0 {
			return nil
		}
		return tx.PruneOrphanComponents(repositoryID)
	})

	return keys, err
}

func (s *Store) deleteMatchingAssets(repositoryID string, selection assetSelection) ([]string, error) {
	matching := func() *gorm.DB {
		return selection(s.query().Where("repository_id = ?", repositoryID))
	}

	var keys []string
	if err := matching().Model(&models.Asset{}).Pluck("storage_key", &keys).Error; err != nil {
		return nil, err
	}
	if len(keys) == 0 {
		return nil, nil
	}
	return keys, matching().Delete(&models.Asset{}).Error
}
