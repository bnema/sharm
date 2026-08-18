package domain

type User struct {
	ID             int64
	Username       string
	PasswordHash   string
	SessionVersion int64
	CreatedAt      string
	UpdatedAt      string
}
