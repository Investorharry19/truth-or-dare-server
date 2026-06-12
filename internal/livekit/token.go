package livekit

import (
	"fmt"
	"os"
	"time"

	"github.com/livekit/protocol/auth"
)

// TokenConfig holds LiveKit configuration
type TokenConfig struct {
	APIKey    string
	APISecret string
	ServerURL string
}

// NewTokenConfig creates a new LiveKit token config from environment
func NewTokenConfig() *TokenConfig {
	return &TokenConfig{
		APIKey:    os.Getenv("LIVEKIT_API_KEY"),
		APISecret: os.Getenv("LIVEKIT_API_SECRET"),
		ServerURL: os.Getenv("LIVEKIT_SERVER_URL"),
	}
}

// TokenResponse is the response struct for token endpoint
type TokenResponse struct {
	Token  string `json:"token"`
	URL    string `json:"url"`
	Room   string `json:"room"`
	UserID string `json:"user_id"`
}

// GenerateToken generates a LiveKit access token for a user
func (tc *TokenConfig) GenerateToken(userID, username, roomID string, canPublish, canPublishData, canSubscribe bool) (string, error) {
	if tc.APIKey == "" || tc.APISecret == "" {
		return "", fmt.Errorf("LiveKit credentials not configured")
	}

	at := auth.NewAccessToken(tc.APIKey, tc.APISecret)

	grant := &auth.VideoGrant{
		RoomJoin:       true,
		Room:           roomID,
		CanPublish:     &canPublish,
		CanSubscribe:   &canSubscribe,
		CanPublishData: &canPublishData,
	}

	at.AddGrant(grant).
		SetIdentity(userID).
		SetName(username).
		SetMetadata(fmt.Sprintf(`{"username": "%s"}`, username)).
		SetValidFor(time.Hour) // ✅ use SetValidFor instead of setting ExpiresAt directly

	token, err := at.ToJWT() // ✅ ToJWT() not ToJwt()
	if err != nil {
		return "", fmt.Errorf("failed to generate token: %w", err)
	}

	return token, nil
}

// IsConfigured checks if LiveKit is properly configured
func (tc *TokenConfig) IsConfigured() bool {
	return tc.APIKey != "" && tc.APISecret != "" && tc.ServerURL != ""
}
