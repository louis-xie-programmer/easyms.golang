package service

import (
	"context"
	"errors"
	"github.com/louis-xie-programmer/easyms/cmd/auth-svc/model"
	"github.com/louis-xie-programmer/easyms/pkg/db"
)

var (
	ErrUserNotExist = errors.New("username is not exist")
	ErrPassword     = errors.New("invalid password")
)

// Service Define a service interface
type UserDetailsService interface {
	// Get UserDetails By username
	GetUserDetailByUsername(ctx context.Context, username, password string) (*model.UserDetails, error)
}

type PostgresUserDetailsService struct {
	db *db.EasyDatabase
}

func NewPostgresUserDetailsService(db *db.EasyDatabase) UserDetailsService {
	return &PostgresUserDetailsService{
		db: db,
	}
}

func (service *PostgresUserDetailsService) GetUserDetailByUsername(ctx context.Context, username, password string) (*model.UserDetails, error) {
	var userDetails model.UserDetails
	err := service.db.Query(&userDetails, "select * from user_details where username = ?", username)
	if err != nil {
		return nil, err
	}

	if userDetails.Password != password {
		return nil, ErrPassword
	}

	return &userDetails, nil
}

// UserService implement Service interface
type InMemoryUserDetailsService struct {
	userDetailsDict map[string]*model.UserDetails
}

func (service *InMemoryUserDetailsService) GetUserDetailByUsername(ctx context.Context, username, password string) (*model.UserDetails, error) {
	// 根据 username 获取用户信息
	userDetails, ok := service.userDetailsDict[username]
	if ok {
		// 比较 password 是否匹配
		if userDetails.Password == password {
			return userDetails, nil
		} else {
			return nil, ErrPassword
		}
	} else {
		return nil, ErrUserNotExist
	}

}

func NewInMemoryUserDetailsService(userDetailsList []*model.UserDetails) UserDetailsService {
	userDetailsDict := make(map[string]*model.UserDetails)

	if userDetailsList != nil {
		for _, value := range userDetailsList {
			userDetailsDict[value.Username] = value
		}
	}

	return &InMemoryUserDetailsService{
		userDetailsDict: userDetailsDict,
	}
}
