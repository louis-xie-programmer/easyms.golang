package service

import (
	"context"
	"easyms/internal/services/user/internal/storage" // Import the new storage package
	"easyms/internal/shared/models"
	"errors"
	"fmt"
	"strings"

	"gorm.io/gorm"
)

// --- Business Errors ---

var (
	ErrUserNotFound   = errors.New("user not found")
	ErrUsernameExists = errors.New("username already exists")
	ErrEmailExists    = errors.New("email already exists")
)

// --- Service Definition ---

type UserService interface {
	Create(ctx context.Context, username, password, email string) (*models.User, error)
	GetByID(ctx context.Context, id uint) (*models.User, error)
}

// userServiceImpl now depends on UserStorage instead of db.Database
type userServiceImpl struct {
	storage storage.UserStorage
}

// NewUserService now requires a UserStorage dependency.
func NewUserService(storage storage.UserStorage) UserService {
	return &userServiceImpl{
		storage: storage,
	}
}

// Create creates a new user.
func (s *userServiceImpl) Create(ctx context.Context, username, password, email string) (*models.User, error) {
	user := &models.User{
		Username: username,
		Email:    email,
	}
	if err := user.SetPassword(password); err != nil {
		return nil, fmt.Errorf("failed to hash password: %w", err)
	}

	// Delegate persistence to the storage layer
	if err := s.storage.Create(ctx, user); err != nil {
		if strings.Contains(err.Error(), "Duplicate entry") || strings.Contains(err.Error(), "unique constraint") {
			if strings.Contains(err.Error(), "username") {
				return nil, ErrUsernameExists
			}
			if strings.Contains(err.Error(), "email") {
				return nil, ErrEmailExists
			}
		}
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	return user, nil
}

// GetByID retrieves a user by their ID.
func (s *userServiceImpl) GetByID(ctx context.Context, id uint) (*models.User, error) {
	// Delegate lookup to the storage layer
	user, err := s.storage.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrUserNotFound
		}
		return nil, err
	}
	return user, nil
}
