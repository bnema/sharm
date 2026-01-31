package port

import "github.com/bnema/sharm/internal/domain"

type UserStore interface {
	HasUser() (bool, error)
	GetUser(username string) (*domain.User, error)
	GetUserByID(id int64) (*domain.User, error)
	GetFirstUser() (*domain.User, error)
	CreateUser(username, passwordHash string) error
	UpdatePassword(id int64, passwordHash string) error
}
