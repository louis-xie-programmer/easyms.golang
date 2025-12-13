package storage

import (
	"context"
	model2 "easyms/cmd/auth-svc/model"
	"errors"
)

type InMemoryStore struct {
	Clients map[string]*model2.ClientDetails
	Users   map[string]*model2.UserDetails
}

func (s *InMemoryStore) GetClient(_ context.Context, id string) (*model2.ClientDetails, error) {
	c, ok := s.Clients[id]
	if !ok {
		return nil, errors.New("client not found")
	}
	return c, nil
}

func (s *InMemoryStore) GetUserByPassword(_ context.Context, username, password string) (*model2.UserDetails, error) {
	user := s.Users[username]
	if user == nil {
		return nil, errors.New("invalid credentials")
	}
	if user.Password != password {
		return nil, errors.New("invalid credentials")
	}
	return user, nil
}
