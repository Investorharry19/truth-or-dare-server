package room

import (
	"fmt"
	"net/http"

	"github.com/Investorharry19/truth-or-dare-server/pkg/jwt"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

// Upgrader allows HTTP connections to become WebSockets
var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

// upgradeConnection upgrades HTTP connection to WebSocket
func upgradeConnection(c *gin.Context) (*websocket.Conn, error) {
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return nil, err
	}
	return conn, nil
}

// ServeWs is a gin handler for WebSocket connections
func ServeWs(c *gin.Context) {

	token := c.Query("token")
	username := c.Query("username")

	if token == "" {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "Missing token",
		})
		return
	}

	userID, err := jwt.ValidateAccessToken(token)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "Invalid token",
		})
		return
	}

	conn, err := upgradeConnection(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to upgrade connection",
		})
		return
	}

	clientsMu.Lock()

	// =====================================
	// RECONNECT FLOW
	// =====================================

	if existingClient, exists := clients[userID]; exists {

		oldConn := existingClient.Conn

		existingClient.Conn = conn
		existingClient.Username = username
		existingClient.Disconnected = false

		clientsMu.Unlock()

		// kill old socket
		if oldConn != nil {
			oldConn.Close()
		}

		fmt.Printf("User reconnected: %s\n", username)

		// notify room if user is in one
		if existingClient.RoomID != "" {

			room := GetRoom(existingClient.RoomID)

			if room != nil {

				Broadcast(room, "user_reconnected", gin.H{
					"user_id":  existingClient.UserID,
					"username": existingClient.Username,
				})

				sendRoomUpdate(room)
			}
		}

		go handleMessages(existingClient, conn)

		return
	}

	// =====================================
	// NEW CONNECTION
	// =====================================

	client := &Client{
		UserID:   userID,
		Username: username,
		Conn:     conn,
		MicOn:    true,
	}

	clients[userID] = client

	clientsMu.Unlock()

	fmt.Printf("User connected: %s\n", username)

	go handleMessages(client, conn)
}
