package store

import (
	"arca/internal/store/models"
)

// UserByIdentity finds the account an issuer and subject were linked to, or nil
// when there is none yet.
func (s *Store) UserByIdentity(issuer, subject string) (*models.User, error) {
	var user models.User
	err := s.db.
		Joins("JOIN user_identities ON user_identities.user_id = users.id").
		Where("user_identities.issuer = ? AND user_identities.subject = ?", issuer, subject).
		First(&user).Error
	if isNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func (s *Store) CreateIdentity(identity *models.Identity) error {
	return s.db.Create(identity).Error
}

// IdentityUserIDs returns the accounts linked to an identity at the issuer.
func (s *Store) IdentityUserIDs(issuer string) ([]string, error) {
	var ids []string
	err := s.db.Model(&models.Identity{}).Where("issuer = ?", issuer).Pluck("user_id", &ids).Error
	return ids, err
}

// HasIdentity reports whether the account is linked to an identity at the issuer.
func (s *Store) HasIdentity(userID, issuer string) (bool, error) {
	var count int64
	err := s.db.Model(&models.Identity{}).Where("user_id = ? AND issuer = ?", userID, issuer).Count(&count).Error
	return count > 0, err
}
