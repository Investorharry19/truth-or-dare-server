package user

import (
	"context"
	"errors"
	"time"

	"github.com/Investorharry19/truth-or-dare-server/pkg/db"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

var ErrUserNotFound = errors.New("user not found")

type Repository struct{}

func NewRepository() *Repository {
	return &Repository{}
}

func (r *Repository) Create(ctx context.Context, u *User) error {
	query := `
		INSERT INTO users (
			id, username, full_name, email, password_hash, email_verified,
			verification_token, verification_expires_at, paid_points, free_points, current_streak, longest_streak, last_active_at,
			last_reset_at, password_reset_token, password_reset_expires_at, refresh_token
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
	`

	_, err := db.Pool.Exec(ctx, query,
		u.ID,
		u.Username,
		u.FullName,
		u.Email,
		u.PasswordHash,
		u.EmailVerified,
		u.VerificationToken,
		u.VerificationExpiresAt,
		u.PaidPoints,
		u.FreePoints,
		u.CurrentStreak,
		u.LongestStreak,
		u.LastActiveAt,
		u.LastResetAt,
		u.PasswordResetToken,
		u.PasswordResetExpiresAt,
		u.RefreshToken,
	)

	return err
}

func (r *Repository) GetByEmail(ctx context.Context, email string) (*User, error) {
	query := `
		SELECT id, username, full_name, email, password_hash, email_verified,
		COALESCE(verification_token, ''), COALESCE(verification_expires_at, '0001-01-01'),
		paid_points, free_points, COALESCE(current_streak,0), COALESCE(longest_streak,0), COALESCE(last_active_at, '0001-01-01'),
		created_at, last_reset_at, COALESCE(refresh_token, '')
		FROM users
		WHERE email = $1
	`

	row := db.Pool.QueryRow(ctx, query, email)

	var u User
	err := row.Scan(
		&u.ID,
		&u.Username,
		&u.FullName,
		&u.Email,
		&u.PasswordHash,
		&u.EmailVerified,
		&u.VerificationToken,
		&u.VerificationExpiresAt,
		&u.PaidPoints,
		&u.FreePoints,
		&u.CurrentStreak,
		&u.LongestStreak,
		&u.LastActiveAt,
		&u.CreatedAt,
		&u.LastResetAt,
		&u.RefreshToken,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, err
	}

	return &u, nil
}

func (r *Repository) GetByID(ctx context.Context, id uuid.UUID) (*User, error) {
	query := `
		SELECT id, username, full_name, email, password_hash, email_verified,
		COALESCE(verification_token, ''), COALESCE(verification_expires_at, '0001-01-01'),
		paid_points, free_points, COALESCE(current_streak,0), COALESCE(longest_streak,0), COALESCE(last_active_at, '0001-01-01'),
		created_at, last_reset_at, COALESCE(refresh_token, '')
		FROM users
		WHERE id = $1
	`

	row := db.Pool.QueryRow(ctx, query, id)

	var u User
	err := row.Scan(
		&u.ID,
		&u.Username,
		&u.FullName,
		&u.Email,
		&u.PasswordHash,
		&u.EmailVerified,
		&u.VerificationToken,
		&u.VerificationExpiresAt,
		&u.PaidPoints,
		&u.FreePoints,
		&u.CurrentStreak,
		&u.LongestStreak,
		&u.LastActiveAt,
		&u.CreatedAt,
		&u.LastResetAt,
		&u.RefreshToken,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, err
	}

	return &u, nil
}

func (r *Repository) GetPointBalances(ctx context.Context, id uuid.UUID) (int, int, error) {
	query := `
		SELECT paid_points, free_points
		FROM users
		WHERE id = $1
	`
	row := db.Pool.QueryRow(ctx, query, id)
	var paidPoints, freePoints int
	err := row.Scan(&paidPoints, &freePoints)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, 0, ErrUserNotFound
		}
		return 0, 0, err
	}
	return paidPoints, freePoints, nil
}

func (r *Repository) SetPasswordResetToken(ctx context.Context, email, token string, expiresAt time.Time) error {
	query := `
		UPDATE users
		SET password_reset_token = $1, password_reset_expires_at = $2
		WHERE email = $3
	`

	_, err := db.Pool.Exec(ctx, query, token, expiresAt, email)
	return err
}

func (r *Repository) SetVerificationToken(ctx context.Context, email, token string, expiresAt time.Time) error {
	query := `
		UPDATE users
		SET verification_token = $1, verification_expires_at = $2
		WHERE email = $3
	`

	_, err := db.Pool.Exec(ctx, query, token, expiresAt, email)
	return err
}

func (r *Repository) MarkEmailVerified(ctx context.Context, email string) error {
	query := `
		UPDATE users
		SET email_verified = TRUE, verification_token = NULL, verification_expires_at = NULL
		WHERE email = $1
	`

	_, err := db.Pool.Exec(ctx, query, email)
	return err
}

func (r *Repository) GetByVerificationToken(ctx context.Context, token string) (*User, error) {
	query := `
		SELECT id, username, full_name, email, password_hash, email_verified,
		COALESCE(verification_token, ''), COALESCE(verification_expires_at, '0001-01-01'),
		paid_points, free_points, COALESCE(current_streak,0), COALESCE(longest_streak,0), COALESCE(last_active_at, '0001-01-01'),
		created_at, last_reset_at, COALESCE(refresh_token, '')
		FROM users
		WHERE verification_token = $1
	`

	row := db.Pool.QueryRow(ctx, query, token)

	var u User
	err := row.Scan(
		&u.ID,
		&u.Username,
		&u.FullName,
		&u.Email,
		&u.PasswordHash,
		&u.EmailVerified,
		&u.VerificationToken,
		&u.VerificationExpiresAt,
		&u.PaidPoints,
		&u.FreePoints,
		&u.CurrentStreak,
		&u.LongestStreak,
		&u.LastActiveAt,
		&u.CreatedAt,
		&u.LastResetAt,
		&u.RefreshToken,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, err
	}

	return &u, nil
}

func (r *Repository) GetByResetToken(ctx context.Context, token string) (*User, error) {
	query := `
		SELECT id, username, full_name, email, password_hash, email_verified,
		COALESCE(verification_token, ''), COALESCE(verification_expires_at, '0001-01-01'),
		paid_points, free_points, COALESCE(current_streak,0), COALESCE(longest_streak,0), COALESCE(last_active_at, '0001-01-01'),
		created_at, last_reset_at,
		COALESCE(password_reset_token, ''), COALESCE(password_reset_expires_at, '0001-01-01')
		FROM users
		WHERE password_reset_token = $1
	`

	row := db.Pool.QueryRow(ctx, query, token)

	var u User
	err := row.Scan(
		&u.ID,
		&u.Username,
		&u.FullName,
		&u.Email,
		&u.PasswordHash,
		&u.EmailVerified,
		&u.VerificationToken,
		&u.VerificationExpiresAt,
		&u.PaidPoints,
		&u.FreePoints,
		&u.CurrentStreak,
		&u.LongestStreak,
		&u.LastActiveAt,
		&u.CreatedAt,
		&u.LastResetAt,
		&u.PasswordResetToken,
		&u.PasswordResetExpiresAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, err
	}

	return &u, nil
}

func (r *Repository) UpdatePassword(ctx context.Context, id uuid.UUID, passwordHash string) error {
	query := `
		UPDATE users
		SET password_hash = $1, password_reset_token = NULL, password_reset_expires_at = NULL
		WHERE id = $2
	`

	_, err := db.Pool.Exec(ctx, query, passwordHash, id)
	return err
}

func (r *Repository) SetRefreshToken(ctx context.Context, id uuid.UUID, token string) error {
	query := `
		UPDATE users
		SET refresh_token = $1
		WHERE id = $2
	`

	_, err := db.Pool.Exec(ctx, query, token, id)
	return err
}

// UpdateStreak increments or resets a user's streak based on provided date
func (r *Repository) UpdateStreak(ctx context.Context, id uuid.UUID, now time.Time) (int, int, error) {
	// Load current streak fields
	query := `SELECT COALESCE(current_streak,0), COALESCE(longest_streak,0), COALESCE(last_active_at,'0001-01-01') FROM users WHERE id = $1`
	row := db.Pool.QueryRow(ctx, query, id)
	var current, longest int
	var lastActive time.Time
	if err := row.Scan(&current, &longest, &lastActive); err != nil {
		return 0, 0, err
	}

	// Determine if now is the same day, previous day, or later
	y := now.UTC()
	today := time.Date(y.Year(), y.Month(), y.Day(), 0, 0, 0, 0, time.UTC)
	yesterday := today.AddDate(0, 0, -1)
	lastDate := time.Date(lastActive.UTC().Year(), lastActive.UTC().Month(), lastActive.UTC().Day(), 0, 0, 0, 0, time.UTC)

	nextStreak := 1
	if !lastActive.IsZero() {
		if lastDate.Equal(today) {
			// already counted today
			return current, longest, nil
		}
		if lastDate.Equal(yesterday) {
			nextStreak = current + 1
		}
	}

	if nextStreak > longest {
		longest = nextStreak
	}

	update := `UPDATE users SET current_streak = $1, longest_streak = $2, last_active_at = $3 WHERE id = $4`
	if _, err := db.Pool.Exec(ctx, update, nextStreak, longest, now.UTC(), id); err != nil {
		return 0, 0, err
	}

	return nextStreak, longest, nil
}

func (r *Repository) ClearRefreshToken(ctx context.Context, id uuid.UUID) error {
	query := `
		UPDATE users
		SET refresh_token = NULL
		WHERE id = $1
	`

	_, err := db.Pool.Exec(ctx, query, id)
	return err
}

func (r *Repository) GetByRefreshToken(ctx context.Context, token string) (*User, error) {
	query := `
		SELECT id, username, full_name, email, password_hash, email_verified,
		COALESCE(verification_token, ''), COALESCE(verification_expires_at, '0001-01-01'),
		paid_points, free_points, COALESCE(current_streak,0), COALESCE(longest_streak,0), COALESCE(last_active_at, '0001-01-01'),
		created_at, last_reset_at, COALESCE(refresh_token, '')
		FROM users
		WHERE refresh_token = $1
	`

	row := db.Pool.QueryRow(ctx, query, token)

	var u User
	err := row.Scan(
		&u.ID,
		&u.Username,
		&u.FullName,
		&u.Email,
		&u.PasswordHash,
		&u.EmailVerified,
		&u.VerificationToken,
		&u.VerificationExpiresAt,
		&u.PaidPoints,
		&u.FreePoints,
		&u.CurrentStreak,
		&u.LongestStreak,
		&u.LastActiveAt,
		&u.CreatedAt,
		&u.LastResetAt,
		&u.RefreshToken,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, err
	}

	return &u, nil
}
