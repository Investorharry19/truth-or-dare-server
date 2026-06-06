package user

import (
	"time"

	"github.com/google/uuid"
)

type User struct {
	ID                     uuid.UUID
	Username               string
	FullName               string
	Email                  string
	PasswordHash           string
	EmailVerified          bool
	VerificationToken      string
	VerificationExpiresAt  time.Time
	FreePoints             int
	PaidPoints             int
	CreatedAt              time.Time
	LastResetAt            time.Time
	PasswordResetToken     string
	PasswordResetExpiresAt time.Time
	RefreshToken           string
}
