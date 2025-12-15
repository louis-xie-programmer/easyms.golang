package storage

import (
	"context"
	"easyms/internal/shared/models"
	"errors"
)

type InMemoryStore struct {
	Clients map[string]*model.ClientDetails
	Users   map[string]*model.UserDetails
}

func (s *InMemoryStore) GetClient(_ context.Context, id string) (*model.ClientDetails, error) {
	c, ok := s.Clients[id]
	if !ok {
		return nil, errors.New("client not found")
	}
	return c, nil
}

func (s *InMemoryStore) GetUserByPassword(_ context.Context, username, password string) (*model.UserDetails, error) {
	user := s.Users[username]
	if user == nil {
		return nil, errors.New("invalid credentials")
	}
	if user.Password != password {
		return nil, errors.New("invalid credentials")
	}
	return user, nil
}
