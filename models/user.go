package models

type User struct {
	ID                int    `json:"id"`
	Email             string `json:"email"`
	PasswordHash      string `json:"-"`
	Verified          bool   `json:"verified"`
	VerificationToken string `json:"-"`
}
