package auth

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

func TestClassifyRegistrationErrorDuplicate(t *testing.T) {
	err := classifyRegistrationError(&pgconn.PgError{Code: "23505", Message: "duplicate key value violates unique constraint"})
	if !errors.Is(err, ErrEmailOrUsernameTaken) {
		t.Fatalf("expected duplicate key error to map to ErrEmailOrUsernameTaken, got %v", err)
	}
}

func TestClassifyRegistrationErrorOther(t *testing.T) {
	baseErr := errors.New("boom")
	err := classifyRegistrationError(baseErr)
	if !errors.Is(err, baseErr) {
		t.Fatalf("expected original error to be preserved, got %v", err)
	}
}
