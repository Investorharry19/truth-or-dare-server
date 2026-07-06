package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/Investorharry19/truth-or-dare-server/internal/user"
	jwtpkg "github.com/Investorharry19/truth-or-dare-server/pkg/jwt"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

func normalizeEmail(email string) string {
	return strings.TrimSpace(strings.ToLower(email))
}

var (
	ErrEmailOrUsernameTaken = errors.New("email or username already taken")
	ErrEmailNotVerified     = errors.New("email not verified")
	ErrEmailAlreadyVerified = errors.New("email already verified")
)

type Service struct {
	users      *user.Repository
	httpClient *http.Client
}

func NewService(users *user.Repository) *Service {
	httpClient := &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			TLSHandshakeTimeout:   10 * time.Second,
			ResponseHeaderTimeout: 10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
			IdleConnTimeout:       90 * time.Second,
			MaxIdleConns:          100,
			MaxIdleConnsPerHost:   10,
		},
	}
	return &Service{users: users, httpClient: httpClient}
}

func (s *Service) sendVerificationEmail(ctx context.Context, email, username, token string) error {
	apiKey := os.Getenv("RESEND_API_KEY")
	if apiKey == "" {
		return errors.New("resend API key is not configured")
	}

	from := os.Getenv("RESEND_FROM_EMAIL")
	if from == "" {
		from = "no-reply@haven.thukool.online"
	}

	frontendURL := os.Getenv("FRONTEND_URL")
	if frontendURL == "" {
		frontendURL = "http://localhost:3000"
	}

	verificationURL := fmt.Sprintf("%s/verify-email?token=%s", strings.TrimRight(frontendURL, "/"), url.QueryEscape(token))

	type resendPayload struct {
		From    string   `json:"from"`
		To      []string `json:"to"`
		Subject string   `json:"subject"`
		Html    string   `json:"html"`
	}

	payload := resendPayload{
		From:    from,
		To:      []string{email},
		Subject: "Confirm your email",
		Html: fmt.Sprintf(
			"<p>Hi %s,</p><p>Please verify your email address by clicking the link below:</p><p><a href=\"%s\">Verify email</a></p><p>If that link does not work, copy and paste the following URL into your browser:</p><p>%s</p>",
			username,
			verificationURL,
			verificationURL,
		),
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.resend.com/emails", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		payload, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("resend email failed: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(payload)))
	}
	fmt.Println("email sent")

	return nil
}

func (s *Service) sendPasswordResetEmail(ctx context.Context, email, username, token string) error {
	apiKey := os.Getenv("RESEND_API_KEY")
	if apiKey == "" {
		return errors.New("resend API key is not configured")
	}

	from := os.Getenv("RESEND_FROM_EMAIL")
	if from == "" {
		from = "no-reply@haven.thukool.online"
	}

	frontendURL := os.Getenv("FRONTEND_URL")
	if frontendURL == "" {
		frontendURL = "http://localhost:3000"
	}

	resetURL := fmt.Sprintf("%s/reset-password?token=%s", strings.TrimRight(frontendURL, "/"), url.QueryEscape(token))

	type resendPayload struct {
		From    string   `json:"from"`
		To      []string `json:"to"`
		Subject string   `json:"subject"`
		Html    string   `json:"html"`
	}

	payload := resendPayload{
		From:    from,
		To:      []string{email},
		Subject: "Reset your password",
		Html: fmt.Sprintf(
			"<p>Hi %s,</p><p>We received a request to reset your password. You can do so by clicking the link below:</p><p><a href=\"%s\">Reset Password</a></p><p>If you didn't request a password reset, you can safely ignore this email.</p><p>If that link does not work, copy and paste the following URL into your browser:</p><p>%s</p>",
			username,
			resetURL,
			resetURL,
		),
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.resend.com/emails", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		payload, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("resend email failed: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(payload)))
	}
	fmt.Println("password reset email sent")

	return nil
}

func (s *Service) Register(
	ctx context.Context,
	username, email, fullName, password string,
) (uuid.UUID, error) {

	email = normalizeEmail(email)

	hash, err := bcrypt.GenerateFromPassword([]byte(password), 12)
	if err != nil {
		return uuid.Nil, err
	}

	token := uuid.NewString()
	expiresAt := time.Now().Add(24 * time.Hour)

	u := &user.User{
		ID:                    uuid.New(),
		Username:              username,
		Email:                 email,
		FullName:              fullName,
		PasswordHash:          string(hash),
		EmailVerified:         false,
		VerificationToken:     token,
		VerificationExpiresAt: expiresAt,
		PaidPoints:            0,
		FreePoints:            30,
		LastResetAt:           ctx.Value("now").(time.Time),
	}

	err = s.users.Create(ctx, u)
	if err != nil {
		fmt.Println(err)
		return uuid.Nil, ErrEmailOrUsernameTaken
	}

	if err := s.sendVerificationEmail(ctx, u.Email, u.Username, token); err != nil {
		return uuid.Nil, err
	}

	fmt.Printf("DEBUG register password : %v\n", password)
	fmt.Printf("DEBUG register password bytes: %v\n", []byte(password))

	return u.ID, nil
}

func (s *Service) Login(
	ctx context.Context,
	email, password string,
) (uuid.UUID, string, error) {

	fmt.Printf("DEBUG login password : %v\n", password)
	fmt.Printf("DEBUG login password bytes: %v\n", []byte(password))
	email = normalizeEmail(email)

	fmt.Println("156")
	u, err := s.users.GetByEmail(ctx, email)
	if err != nil {
		fmt.Println("Login error: GetByEmail failed", err)
		return uuid.Nil, "", errors.New("invalid credentials")
	}

	err = bcrypt.CompareHashAndPassword(
		[]byte(u.PasswordHash),
		[]byte(password),
	)

	fmt.Println("167")
	if err != nil {
		fmt.Println("Password mismatch for email:", email)
		fmt.Println("Password mismatch for email sent:", u.Email)
		fmt.Println("Password mismatch for password set:", u.PasswordHash)
		fmt.Println("Password mismatch for password sent:", password)
		return uuid.Nil, "", errors.New("invalid credentials")
	}
	fmt.Print("Password match")

	fmt.Println("172")
	if !u.EmailVerified {
		err = s.ResendVerification(ctx, u.Email)
		if err != nil {
			return uuid.Nil, "", err
		}
		return uuid.Nil, "", ErrEmailNotVerified
	}

	fmt.Println("181")
	refreshToken, err := jwtpkg.GenerateRefreshToken(u.ID)
	if err != nil {
		return uuid.Nil, "", err
	}

	fmt.Println("187")
	if err := s.users.SetRefreshToken(ctx, u.ID, refreshToken); err != nil {
		return uuid.Nil, "", err
	}

	return u.ID, refreshToken, nil
}

func (s *Service) VerifyEmail(ctx context.Context, token string) error {
	u, err := s.users.GetByVerificationToken(ctx, token)
	if err != nil {
		return err
	}

	if u.EmailVerified {
		return ErrEmailAlreadyVerified
	}

	if u.VerificationExpiresAt.IsZero() || u.VerificationExpiresAt.Before(time.Now()) {
		return errors.New("verification token is invalid or expired")
	}

	return s.users.MarkEmailVerified(ctx, u.Email)
}

func (s *Service) ResendVerification(ctx context.Context, email string) error {
	email = normalizeEmail(email)
	u, err := s.users.GetByEmail(ctx, email)
	if err != nil {
		return err
	}

	if u.EmailVerified {
		return ErrEmailAlreadyVerified
	}

	token := uuid.NewString()
	expiresAt := time.Now().Add(24 * time.Hour)

	if err := s.users.SetVerificationToken(ctx, u.Email, token, expiresAt); err != nil {
		return err
	}

	return s.sendVerificationEmail(ctx, u.Email, u.Username, token)
}

func (s *Service) Refresh(ctx context.Context, refreshToken string) (uuid.UUID, string, error) {
	userID, err := jwtpkg.ValidateRefreshToken(refreshToken)
	if err != nil {
		return uuid.Nil, "", err
	}

	id, err := uuid.Parse(userID)
	if err != nil {
		return uuid.Nil, "", err
	}

	u, err := s.users.GetByRefreshToken(ctx, refreshToken)
	if err != nil {
		return uuid.Nil, "", err
	}

	if u.ID != id {
		return uuid.Nil, "", errors.New("invalid refresh token")
	}

	newRefreshToken, err := jwtpkg.GenerateRefreshToken(id)
	if err != nil {
		return uuid.Nil, "", err
	}

	if err := s.users.SetRefreshToken(ctx, id, newRefreshToken); err != nil {
		return uuid.Nil, "", err
	}

	return id, newRefreshToken, nil
}

// GoogleLogin verifies a Google ID token, creates a user if necessary,
// and returns the user ID and a new refresh token.
func (s *Service) GoogleLogin(ctx context.Context, idToken string) (uuid.UUID, string, error) {
	// Verify token with Google's tokeninfo endpoint
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://oauth2.googleapis.com/tokeninfo", nil)
	if err != nil {
		return uuid.Nil, "", err
	}

	q := req.URL.Query()
	q.Add("id_token", idToken)
	req.URL.RawQuery = q.Encode()

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return uuid.Nil, "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return uuid.Nil, "", errors.New("invalid google token")
	}

	var payload struct {
		Email         string `json:"email"`
		EmailVerified string `json:"email_verified"`
		Name          string `json:"name"`
		Sub           string `json:"sub"`
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return uuid.Nil, "", err
	}

	if err := json.Unmarshal(body, &payload); err != nil {
		return uuid.Nil, "", err
	}

	if payload.Email == "" {
		return uuid.Nil, "", errors.New("google token missing email")
	}

	// Try to find an existing user by email
	u, err := s.users.GetByEmail(ctx, payload.Email)
	if err != nil {
		if errors.Is(err, user.ErrUserNotFound) {
			// Create a new user
			username := strings.Split(payload.Email, "@")[0]
			// sanitize username a bit
			username = strings.ToLower(strings.ReplaceAll(username, " ", ""))

			newUser := &user.User{
				ID:            uuid.New(),
				Username:      username,
				FullName:      payload.Name,
				Email:         payload.Email,
				PasswordHash:  "",
				EmailVerified: true,
				PaidPoints:    0,
				FreePoints:    30,
				LastResetAt:   time.Now(),
			}

			// Ensure username uniqueness: if create fails due to constraint, append suffix
			if err := s.users.Create(ctx, newUser); err != nil {
				// try with a random suffix
				newUser.Username = newUser.Username + "-" + uuid.NewString()[:8]
				if err := s.users.Create(ctx, newUser); err != nil {
					return uuid.Nil, "", err
				}
			}

			refreshToken, err := jwtpkg.GenerateRefreshToken(newUser.ID)
			if err != nil {
				return uuid.Nil, "", err
			}

			if err := s.users.SetRefreshToken(ctx, newUser.ID, refreshToken); err != nil {
				return uuid.Nil, "", err
			}

			return newUser.ID, refreshToken, nil
		}
		return uuid.Nil, "", err
	}

	// Existing user: issue a new refresh token
	refreshToken, err := jwtpkg.GenerateRefreshToken(u.ID)
	if err != nil {
		return uuid.Nil, "", err
	}

	if err := s.users.SetRefreshToken(ctx, u.ID, refreshToken); err != nil {
		return uuid.Nil, "", err
	}

	return u.ID, refreshToken, nil
}

func (s *Service) Logout(ctx context.Context, userID uuid.UUID) error {
	return s.users.ClearRefreshToken(ctx, userID)
}

func (s *Service) ForgotPassword(ctx context.Context, email string) (string, error) {
	email = normalizeEmail(email)
	u, err := s.users.GetByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, user.ErrUserNotFound) {
			return "", nil
		}
		return "", err
	}

	token := uuid.NewString()
	expiresAt := time.Now().Add(1 * time.Hour)

	if err := s.users.SetPasswordResetToken(ctx, u.Email, token, expiresAt); err != nil {
		return "", err
	}
	fmt.Printf("Password reset token for %s: %s\n", email, token)

	if err := s.sendPasswordResetEmail(ctx, u.Email, u.Username, token); err != nil {
		return "", err
	}

	return token, nil
}

func (s *Service) ResetPassword(ctx context.Context, token, password string) error {
	u, err := s.users.GetByResetToken(ctx, token)
	if err != nil {
		return err
	}

	if u.PasswordResetExpiresAt.IsZero() || u.PasswordResetExpiresAt.Before(time.Now()) {
		return errors.New("reset token is invalid or expired")
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), 12)
	if err != nil {
		return err
	}

	if err := s.users.UpdatePassword(ctx, u.ID, string(hash)); err != nil {
		return err
	}

	return nil
}

func (s *Service) GetUserPoints(ctx context.Context, userID uuid.UUID) (int, int, error) {
	paid, free, err := s.users.GetPointBalances(ctx, userID)
	if err != nil {
		return 0, 0, err
	}
	return free, paid, nil
}
