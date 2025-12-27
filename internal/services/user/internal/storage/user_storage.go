package storage

import (
	"context"
	"easyms/internal/shared/db"
	"easyms/internal/shared/models"
)

// UserStorage defines the interface for user data persistence.
type UserStorage interface {
	Create(ctx context.Context, user *models.User) error
	GetByID(ctx context.Context, id uint) (*models.User, error)
}

// userStorageImpl implements the UserStorage interface.
type userStorageImpl struct {
	db db.Database
}

// NewUserStorage creates a new user storage instance.
func NewUserStorage(db db.Database) UserStorage {
	return &userStorageImpl{
		db: db,
	}
}

// Create inserts a new user record into the database.
func (s *userStorageImpl) Create(ctx context.Context, user *models.User) error {
	return s.db.Insert(ctx, user)
}

// GetByID retrieves a user by their ID from the database.
func (s *userStorageImpl) GetByID(ctx context.Context, id uint) (*models.User, error) {
	var user models.User
	err := s.db.GetDB().WithContext(ctx).First(&user, id).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}
