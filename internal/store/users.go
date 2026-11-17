package store

import (
	"arca/internal/store/models"
)

func (s *Store) CreateUser(user *models.User) error {
	if user.ID == "" {
		user.ID = NewID()
	}
	if user.CreatedAt == 0 {
		user.CreatedAt = NowMillis()
	}
	return s.db.Create(user).Error
}

func (s *Store) UserByID(id string) (*models.User, error) {
	var user models.User
	if err := s.db.First(&user, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &user, nil
}

func (s *Store) UserByEmail(email string) (*models.User, error) {
	var user models.User
	if err := s.db.First(&user, "email = ? COLLATE NOCASE", email).Error; err != nil {
		return nil, err
	}
	return &user, nil
}

func (s *Store) EmailTaken(email string) (bool, error) {
	var count int64
	err := s.db.Model(&models.User{}).Where("email = ? COLLATE NOCASE", email).Count(&count).Error
	return count > 0, err
}

func (s *Store) ListUsers() ([]models.User, error) {
	var users []models.User
	err := s.db.Order("created_at").Find(&users).Error
	return users, err
}

func (s *Store) CountUsers() (int64, error) {
	var count int64
	err := s.db.Model(&models.User{}).Count(&count).Error
	return count, err
}

func (s *Store) CountEnabledAdmins() (int64, error) {
	var count int64
	err := s.db.Model(&models.User{}).Where("role = ? AND disabled = ?", models.RoleAdmin, false).Count(&count).Error
	return count, err
}

func (s *Store) UpdateUser(id string, changes map[string]any) (int64, error) {
	result := s.db.Model(&models.User{}).Where("id = ?", id).Updates(changes)
	return result.RowsAffected, result.Error
}

func (s *Store) DeleteUser(id string) (int64, error) {
	var deleted int64

	err := s.Transaction(func(tx *Store) error {
		if err := tx.query().Where("user_id = ?", id).Delete(&models.Session{}).Error; err != nil {
			return err
		}
		if err := tx.query().Where("user_id = ?", id).Delete(&models.APIToken{}).Error; err != nil {
			return err
		}
		if err := tx.query().Where("user_id = ?", id).Delete(&models.Grant{}).Error; err != nil {
			return err
		}
		if err := tx.query().Where("user_id = ?", id).Delete(&models.Identity{}).Error; err != nil {
			return err
		}
		if err := tx.query().Model(&models.Asset{}).Where("uploaded_by = ?", id).Update("uploaded_by", nil).Error; err != nil {
			return err
		}
		result := tx.query().Where("id = ?", id).Delete(&models.User{})
		deleted = result.RowsAffected
		return result.Error
	})

	return deleted, err
}
