package livekit

import (
	"fmt"
	"net/http"

	"github.com/Investorharry19/truth-or-dare-server/pkg/jwt"
	"github.com/gin-gonic/gin"
)

// TokenRequest is the request struct for token endpoint
type TokenRequest struct {
	RoomID string `json:"room_id" binding:"required"`
}

// TokenHandler handles LiveKit token generation
type TokenHandler struct {
	tokenConfig *TokenConfig
}

// NewTokenHandler creates a new token handler
func NewTokenHandler(cfg *TokenConfig) *TokenHandler {
	return &TokenHandler{
		tokenConfig: cfg,
	}
}

// GetToken generates and returns a LiveKit token for a user
func (h *TokenHandler) GetToken(c *gin.Context) {
	// Extract user ID from JWT token (via auth middleware)
	userID, ok := c.Get("user_id")
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	userIDStr, ok := userID.(string)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid user ID"})
		return
	}

	var req TokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}

	if req.RoomID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Room ID is required"})
		return
	}

	if !h.tokenConfig.IsConfigured() {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "LiveKit not configured"})
		return
	}

	// Generate token with publish/subscribe permissions
	token, err := h.tokenConfig.GenerateToken(
		userIDStr,
		userIDStr, // username = userID for now, adjust as needed
		req.RoomID,
		true, // canPublish
		true, // canPublishData
		true, // canSubscribe
	)
	if err != nil {
		fmt.Printf("Token generation error: %v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate token"})
		return
	}

	resp := TokenResponse{
		Token:  token,
		URL:    h.tokenConfig.ServerURL,
		Room:   req.RoomID,
		UserID: userIDStr,
	}

	c.JSON(http.StatusOK, resp)
}

// GetTokenWithAuth is an alternative handler that validates token in query params
// Useful for WebSocket connections where headers might not be available
func (h *TokenHandler) GetTokenWithAuth(c *gin.Context) {
	token := c.Query("token")
	roomID := c.Query("room_id")

	if token == "" || roomID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Missing token or room_id"})
		return
	}

	// Validate JWT token and extract user ID
	userID, err := jwt.ValidateAccessToken(token)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid token"})
		return
	}

	if !h.tokenConfig.IsConfigured() {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "LiveKit not configured"})
		return
	}

	// Generate LiveKit token
	lkToken, err := h.tokenConfig.GenerateToken(
		userID,
		userID,
		roomID,
		true,
		true,
		true,
	)
	if err != nil {
		fmt.Printf("Token generation error: %v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate token"})
		return
	}

	resp := TokenResponse{
		Token:  lkToken,
		URL:    h.tokenConfig.ServerURL,
		Room:   roomID,
		UserID: userID,
	}

	c.JSON(http.StatusOK, resp)
}
