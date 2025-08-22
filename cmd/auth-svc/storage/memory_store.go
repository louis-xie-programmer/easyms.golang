package storage

import (
	"context"
	"errors"
	model2 "github.com/louis-xie-programmer/easyms/cmd/auth-svc/model"
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
	for _, u := range s.Users {
		if u.Username == username && u.Password == password {
			return u, nil
		}
	}
	return nil, errors.New("invalid credentials")
}
