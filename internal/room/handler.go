package room

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"sync"
	"time"

	"github.com/Investorharry19/truth-or-dare-server/pkg/cloudinary"
	"github.com/Investorharry19/truth-or-dare-server/pkg/jwt"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

var clients = make(map[string]*Client)
var clientsMu sync.RWMutex

type Client struct {
	UserID   string          `json:"id"`
	Username string          `json:"username"`
	Conn     *websocket.Conn `json:"conn"`
	RoomID   string          `json:"roomId"`
	MicOn    bool            `json:"micOn"`

	Disconnected bool `json:"disconnected"`
}

type WSMessage struct {
	Type      string                 `json:"type"`
	Payload   map[string]interface{} `json:"payload"`
	AudioUri  string                 `json:"audio_uri"`
	AudioData string                 `json:"audio_data"`
	Timestamp int64                  `json:"timestamp"`
	MicOn     *bool                  `json:"mic_on"`
}

func WebSocketHandler(c *gin.Context) {

	token := c.Query("token")
	username := c.Query("username")

	userID, err := jwt.ValidateAccessToken(token)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "Invalid token",
		})
		return
	}

	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}

	clientsMu.Lock()

	// reconnect support
	if existingClient, exists := clients[userID]; exists {

		oldConn := existingClient.Conn

		existingClient.Conn = conn
		existingClient.Username = username
		existingClient.Disconnected = false

		clientsMu.Unlock()

		if oldConn != nil {
			oldConn.Close()
		}

		fmt.Printf("User reconnected: %s\n", username)

		go handleMessages(existingClient, conn)

		return
	}

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

func handleMessages(client *Client, conn *websocket.Conn) {

	defer cleanupClient(client, conn)

	for {

		_, msg, err := conn.ReadMessage()
		if err != nil {
			break
		}

		var wsMsg WSMessage

		if err := json.Unmarshal(msg, &wsMsg); err != nil {
			continue
		}

		// Update room activity
		if r := currentRoom(client); r != nil {
			r.mu.Lock()
			r.LastActivity = time.Now()
			r.mu.Unlock()
		}

		payload := wsMsg.Payload

		fmt.Println("message type: ", wsMsg.Type)
		switch wsMsg.Type {

		// =====================================================
		// JOIN ROOM
		// =====================================================

		case "join_room":

			roomID, ok := payload["room_id"].(string)

			if !ok || roomID == "" {
				sendError(client, "Invalid room ID")
				continue
			}

			room := GetRoom(roomID)

			if room == nil {
				sendError(client, "Room not found")
				continue
			}

			// If joining a different room, leave current first
			if client.RoomID != "" && client.RoomID != roomID {
				leaveRoom(client)
			}

			room.mu.Lock()

			if len(room.Participants) >= room.MaxPlayers {
				room.mu.Unlock()
				sendError(client, "Room is full")
				continue
			}

			// Private room passcode check
			if room.Private && client.UserID != room.HostID {
				code, okCode := payload["passcode"].(string)
				if !okCode || code == "" {
					room.mu.Unlock()
					sendError(client, "Passcode required for private room")
					continue
				}
				if !room.consumePasscodeLocked(code) {
					room.mu.Unlock()
					sendError(client, "Invalid passcode")
					continue
				}
			}

			// ✅ Check if already in the room
			alreadyIn := false
			for _, p := range room.Participants {
				if p.UserID == client.UserID {
					alreadyIn = true
					break
				}
			}

			if alreadyIn {
				room.mu.Unlock()

				client.Conn.WriteJSON(gin.H{
					"type": "joined_room",
					"payload": gin.H{
						"room_id": roomID,
					},
				})

				sendRoomUpdate(room)
				continue
			}

			room.Participants = append(room.Participants, client)
			client.RoomID = roomID

			room.mu.Unlock()

			fmt.Printf("%s joined room %s\n", client.Username, roomID)

			client.Conn.WriteJSON(gin.H{
				"type": "joined_room",
				"payload": gin.H{
					"room_id": roomID,
				},
			})

			Broadcast(room, "user_joined", gin.H{
				"id":       client.UserID,
				"username": client.Username,
			})
			fmt.Println("join_room complete, client.RoomID:", client.RoomID)

			sendRoomUpdate(room)

		// =====================================================
		// LEAVE ROOM
		// =====================================================

		case "generate_passcode":
			fmt.Println("Received generate_passcode from client:", client.UserID)
			// Host can generate a one‑time passcode for a private room
			room := currentRoom(client)
			if room == nil {
				fmt.Println("generate_passcode failed: Room not found")
				sendError(client, "Room not found")
				continue
			}
			if client.UserID != room.HostID {
				fmt.Println("generate_passcode failed: Only the host can generate a passcode")
				sendError(client, "Only the host can generate a passcode")
				continue
			}
			if !room.Private {
				fmt.Println("generate_passcode failed: Passcode only needed for private rooms")
				sendError(client, "Passcode only needed for private rooms")
				continue
			}
			
			room.mu.Lock()
			passcode := room.generatePasscodeLocked()
			fmt.Println("generate_passcode: generated new passcode:", passcode)
			room.mu.Unlock()

			fmt.Println("generate_passcode success. Sending passcode:", passcode)
			// Return the room's persistent password
			client.Conn.WriteJSON(gin.H{
				"type": "passcode_generated",
				"payload": gin.H{"passcode": passcode},
			})
			continue

		case "leave_room":

			leaveRoom(client)

		// =====================================================
		// END ROOM
		// =====================================================

		case "end_room":
			room := currentRoom(client)
			if room == nil {
				continue
			}

			if client.UserID != room.HostID {
				sendError(client, "Only the host can end the room.")
				continue
			}

			Broadcast(room, "room_closed", gin.H{
				"message": "The host has ended the room.",
			})

			// Clean up Cloudinary resources
			cleanupRoomResources(room)

			// Close all participant connections
			room.mu.Lock()
			for _, p := range room.Participants {
				if p.Conn != nil {
					p.Conn.Close()
				}
			}
			room.mu.Unlock()

			roomsMu.Lock()
			delete(Rooms, room.ID)
			roomsMu.Unlock()

		// =====================================================
		// START GAME
		// =====================================================

		case "start_game":
			room := currentRoom(client)
			if room == nil {
				fmt.Println("room is nil, skipping")
				continue
			}

			room.mu.Lock()

			isHost := client.UserID == room.HostID
			playerCount := len(room.Participants)

			if !isHost {
				room.mu.Unlock()
				sendError(client, "Only host can start game")
				continue
			}

			if playerCount < 2 {
				room.mu.Unlock()
				sendError(client, "Need at least 2 players")
				continue
			}

			room.GameStarted = true
			room.CurrentPlayerID = room.HostID
			room.CommanderID = room.HostID
			room.CurrentCommanderID = room.HostID
			room.RoundPhase = PhaseSpinning

			room.mu.Unlock()

			fmt.Println("send game started")

			Broadcast(room, "game_started", gin.H{
				"current_player_id": room.HostID,
			})
			sendRoomUpdate(room)

		// =====================================================
		// SPIN BOTTLE
		// =====================================================

		case "spin_bottle":

			room := currentRoom(client)
			if room == nil {
				continue
			}

			room.mu.RLock()

			if room.CurrentPlayerID != client.UserID &&
				room.CurrentPlayerID != "" {

				room.mu.RUnlock()

				sendError(client, "Not your turn")
				continue
			}

			var candidates []string

			for _, p := range room.Participants {
				if p.UserID != client.UserID {
					candidates = append(candidates, p.UserID)
				}
			}

			room.mu.RUnlock()

			if len(candidates) == 0 {
				sendError(client, "No candidates")
				continue
			}

			selectedID := candidates[rand.Intn(len(candidates))]

			room.mu.Lock()

			room.LastSelectedID = selectedID
			room.CommanderID = client.UserID
			room.CurrentPlayerID = selectedID
			room.CurrentTargetID = selectedID
			room.RoundPhase = PhaseChoosing

			room.mu.Unlock()

			addRoundHistory(room, RoundHistoryItem{
				ID:         fmt.Sprintf("spin_%d", time.Now().UnixNano()),
				Type:       "bottle_spun",
				Text:       fmt.Sprintf("%s spun the bottle and it landed on %s.", client.Username, getUsername(room, selectedID)),
				ActorID:    client.UserID,
				ActorName:  client.Username,
				TargetID:   selectedID,
				TargetName: getUsername(room, selectedID),
				Timestamp:  time.Now().UnixMilli(),
			})

			Broadcast(room, "bottle_spun", gin.H{
				"spinner_id":  client.UserID,
				"selected_id": selectedID,
			})
			sendRoomUpdate(room)

		// =====================================================
		// MAKE CHOICE
		// =====================================================

		case "make_choice", "truth_or_dare_choice":
			room := currentRoom(client)
			if room == nil {
				sendError(client, "Room not found")
				continue
			}
			choice, _ := payload["choice"].(string)
			room.mu.Lock()
			room.RoundPhase = PhaseCommanderPrompt
			room.CurrentChoice = choice
			room.mu.Unlock()

			Broadcast(room, "choice_made", gin.H{
				"choice":    choice,
				"target_id": client.UserID,
			})

			addRoundHistory(room, RoundHistoryItem{
				ID:         fmt.Sprintf("choice_%d", time.Now().UnixNano()),
				Type:       "choice_made",
				Text:       fmt.Sprintf("%s chose %s.", client.Username, choice),
				ActorID:    client.UserID,
				ActorName:  client.Username,
				TargetID:   room.LastSelectedID,
				TargetName: client.Username,
				Timestamp:  time.Now().UnixMilli(),
			})

			sendRoomUpdate(room)

			// =====================================================
			// ISSUE CHALLENGE
			// =====================================================

		case "issue_challenge", "prompt_broadcast":
			room := currentRoom(client)
			if room == nil {
				sendError(client, "Room not found")
				continue
			}
			text, _ := payload["prompt_text"].(string)
			choice, _ := payload["choice"].(string)
			targetID := room.CurrentPlayerID

			room.mu.Lock()
			room.CurrentChoice = choice
			room.CurrentChallenge = &Challenge{
				Type:        choice,
				Text:        text,
				TargetID:    targetID,
				CommanderID: client.UserID,
			}
			room.RoundPhase = PhaseTargetReply
			room.mu.Unlock()

			Broadcast(room, "prompt_broadcast", gin.H{
				"commander_id": client.UserID,
				"selected_id":  targetID,
				"prompt_text":  text,
				"choice":       choice,
			})

			// Also broadcast as challenge_issued for frontend compatibility
			Broadcast(room, "challenge_issued", gin.H{
				"commander_id": client.UserID,
				"target_id":    targetID,
				"text":         text,
				"choice":       choice,
			})

			addRoundHistory(room, RoundHistoryItem{
				ID:         fmt.Sprintf("prompt_%d", time.Now().UnixNano()),
				Type:       "prompt_broadcast",
				Text:       fmt.Sprintf("%s asked %s: %s", client.Username, getUsername(room, targetID), text),
				ActorID:    client.UserID,
				ActorName:  client.Username,
				TargetID:   targetID,
				TargetName: getUsername(room, targetID),
				Timestamp:  time.Now().UnixMilli(),
			})

			sendRoomUpdate(room)

			// =====================================================
			// TARGET REPLY
			// =====================================================

		case "target_reply", "player_response":
			room := currentRoom(client)
			if room == nil {
				sendError(client, "Room not found")
				continue
			}
			reply, _ := payload["response_text"].(string)
			if reply == "" {
				reply, _ = payload["text"].(string)
			}

			room.mu.Lock()
			room.CurrentCommanderID = client.UserID
			room.CurrentTargetID = ""
			room.RoundPhase = PhaseReveal
			room.LastTargetResponse = reply
			room.mu.Unlock()

			Broadcast(room, "player_response", gin.H{
				"text":      reply,
				"target_id": client.UserID,
			})

			// Also broadcast as target_replied for frontend compatibility
			Broadcast(room, "target_replied", gin.H{
				"text":      reply,
				"target_id": client.UserID,
			})

			addRoundHistory(room, RoundHistoryItem{
				ID:         fmt.Sprintf("response_%d", time.Now().UnixNano()),
				Type:       "player_response",
				Text:       fmt.Sprintf("%s replied: %s", client.Username, reply),
				ActorID:    client.UserID,
				ActorName:  client.Username,
				TargetID:   client.UserID,
				TargetName: getUsername(room, client.UserID),
				Timestamp:  time.Now().UnixMilli(),
			})

			sendRoomUpdate(room)

			Broadcast(room, "next_turn", gin.H{
				"current_player_id": room.CurrentPlayerID,
				"commander_id":      room.CommanderID,
			})

		// =====================================================
		// PAUSE GAME
		// =====================================================

		case "pause_game":
			room := currentRoom(client)
			if room == nil {
				sendError(client, "Room not found")
				continue
			}

			room.mu.Lock()

			if client.UserID != room.HostID {
				room.mu.Unlock()
				sendError(client, "Only the host can pause the game")
				continue
			}

			room.PausedPhase = room.RoundPhase
			room.RoundPhase = PhasePaused
			room.IsPaused = true

			room.mu.Unlock()

			Broadcast(room, "game_paused", gin.H{
				"message": "Game paused by host.",
			})
			sendRoomUpdate(room)

		// =====================================================
		// RESUME GAME
		// =====================================================

		case "resume_game":
			room := currentRoom(client)
			if room == nil {
				sendError(client, "Room not found")
				continue
			}

			room.mu.Lock()

			if client.UserID != room.HostID {
				room.mu.Unlock()
				sendError(client, "Only the host can resume the game")
				continue
			}

			room.RoundPhase = room.PausedPhase
			room.PausedPhase = ""
			room.IsPaused = false

			room.mu.Unlock()

			Broadcast(room, "game_resumed", gin.H{
				"restored_phase": room.RoundPhase,
			})
			sendRoomUpdate(room)

		// =====================================================
		// RESET GAME
		// =====================================================

		case "reset_game":
			room := currentRoom(client)
			if room == nil {
				sendError(client, "Room not found")
				continue
			}

			room.mu.Lock()

			if client.UserID != room.HostID {
				room.mu.Unlock()
				sendError(client, "Only the host can reset the game")
				continue
			}

			room.RoundPhase = PhaseWaiting
			room.IsPaused = false
			room.PausedPhase = ""
			room.CommanderID = room.HostID
			room.CurrentCommanderID = room.HostID
			room.CurrentPlayerID = room.HostID
			room.CurrentTargetID = ""
			room.LastSelectedID = ""
			room.CurrentChoice = ""
			room.CurrentChallenge = nil
			room.LastTargetResponse = ""
			room.GameStarted = false

			room.mu.Unlock()

			Broadcast(room, "game_reset", gin.H{
				"message": "Game has been reset by the host.",
			})
			sendRoomUpdate(room)

		// =====================================================
		// CHAT
		// =====================================================

		case "chat_message":

			room := currentRoom(client)
			if room == nil {
				continue
			}

			text, _ := payload["text"].(string)
			fileURL, _ := payload["file_url"].(string)
			fileType, _ := payload["file_type"].(string)

			msg := gin.H{
				"id":       client.UserID,
				"username": client.Username,
				"text":     text,
			}
			if fileURL != "" {
				msg["file_url"] = fileURL
				msg["file_type"] = fileType
			}

			Broadcast(room, "chat_message", msg)

		// =====================================================
		// MIC TOGGLE
		// =====================================================

		case "mic_toggle":

			room := currentRoom(client)
			if room == nil {
				continue
			}

			var micOn bool

			if wsMsg.MicOn != nil {
				micOn = *wsMsg.MicOn
			}

			client.MicOn = micOn

			Broadcast(room, "mic_state_changed", gin.H{
				"id":     client.UserID,
				"mic_on": micOn,
			})
			sendRoomUpdate(room)

		// =====================================================
		// AUDIO
		// =====================================================

		case "audio_chunk":

			room := currentRoom(client)
			if room == nil {
				continue
			}

			Broadcast(room, "audio_chunk", gin.H{
				"id":         client.UserID,
				"audio_data": wsMsg.AudioData,
				"timestamp":  wsMsg.Timestamp,
			})
		}
	}
}

func cleanupClient(client *Client, conn *websocket.Conn) {

	clientsMu.Lock()

	currentClient, exists := clients[client.UserID]

	if !exists || currentClient.Conn != conn {
		clientsMu.Unlock()
		return
	}

	client.Disconnected = true

	clientsMu.Unlock()

	conn.Close()

	fmt.Printf("Temporarily disconnected: %s\n", client.Username)

	go func() {
		// wait for reconnect
		time.Sleep(10 * time.Second)

		clientsMu.Lock()
		defer clientsMu.Unlock()

		currentClient, exists := clients[client.UserID]

		// user reconnected
		if exists && !currentClient.Disconnected {
			return
		}

		// fully remove now
		if exists {

			if currentClient.RoomID != "" {
				leaveRoom(currentClient)
			}

			delete(clients, client.UserID)

			fmt.Printf("Fully disconnected: %s\n", client.Username)
		}
	}()
}

func leaveRoom(client *Client) {
	if client.RoomID == "" {
		return
	}

	room := GetRoom(client.RoomID)
	if room == nil {
		client.RoomID = ""
		return
	}

	// If the host leaves, dissolve the entire room
	if client.UserID == room.HostID {
		roomID := client.RoomID
		client.RoomID = ""
		Broadcast(room, "room_closed", gin.H{
			"message": "The host has left the game.",
		})
		// Clean up Cloudinary resources
		cleanupRoomResources(room)
		roomsMu.Lock()
		defer roomsMu.Unlock()
		delete(Rooms, roomID)
		return
	}

	room.mu.Lock()
	for i, p := range room.Participants {
		if p.UserID == client.UserID {
			room.Participants = append(
				room.Participants[:i],
				room.Participants[i+1:]...,
			)
			break
		}
	}
	room.mu.Unlock()

	client.RoomID = ""

	Broadcast(room, "user_left", gin.H{
		"id":       client.UserID,
		"username": client.Username,
	})

	sendRoomUpdate(room)
}

func currentRoom(client *Client) *Room {

	if client.RoomID == "" {
		return nil
	}

	return GetRoom(client.RoomID)
}

func getUsername(room *Room, userID string) string {
	room.mu.RLock()
	defer room.mu.RUnlock()

	for _, p := range room.Participants {
		if p.UserID == userID {
			return p.Username
		}
	}

	return "Player"
}

func sendError(client *Client, msg string) {
	if client == nil || client.Conn == nil {
		fmt.Printf("sendError skipped for nil client or connection: %s\n", msg)
		return
	}

	if err := client.Conn.WriteJSON(gin.H{
		"type": "error",
		"payload": gin.H{
			"message": msg,
		},
	}); err != nil {
		fmt.Printf("sendError failed for %s: %v\n", client.UserID, err)
	}
}

func addRoundHistory(room *Room, entry RoundHistoryItem) {
	room.mu.Lock()
	defer room.mu.Unlock()

	room.RoundHistory = append(room.RoundHistory, entry)
	if len(room.RoundHistory) > 100 {
		room.RoundHistory = room.RoundHistory[len(room.RoundHistory)-100:]
	}
}

func sendRoomUpdate(room *Room) {
	if room == nil {
		return
	}

	room.mu.RLock()
	defer room.mu.RUnlock()

	players := make([]gin.H, 0, len(room.Participants))
	for _, p := range room.Participants {
		players = append(players, gin.H{
			"id":       p.UserID,
			"username": p.Username,
			"isHost":   p.UserID == room.HostID,
			"micOn":    p.MicOn,
		})
	}

	payload := gin.H{
		"players":           players,
		"game_started":      room.GameStarted,
		"current_player_id": room.CurrentPlayerID,
		"commander_id":      room.CommanderID,
		"selected_id":       room.LastSelectedID,
		"phase":             room.RoundPhase,
		"choice":            room.CurrentChoice,
		"current_challenge": room.CurrentChallenge,
		"target_response":   room.LastTargetResponse,
		"is_paused":         room.IsPaused,
		"round_history":     room.RoundHistory,
	}

	Broadcast(room, "room_update", payload)
}

func CreateRoomHandler(c *gin.Context) {
	roomsMu.Lock()
	defer roomsMu.Unlock()

	type requestBody struct {
		Name        string   `json:"name"`
		MaxPlayers  int      `json:"max_players"`
		Private     bool     `json:"private"`
		Password    string   `json:"password"`
		BannedWords []string `json:"banned_words"`
		Username    string   `json:"username"`
	}

	var body requestBody

	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": err.Error(),
		})
		return
	}

	hostID := c.GetString("user_id")

	roomID := uuid.New().String()[:6]

	room := CreateRoom(
		roomID,
		body.Name,
		hostID,
		body.Username,
		body.MaxPlayers,
		body.Private,
		body.Password,
	)

	room.BannedWords = body.BannedWords

	c.JSON(http.StatusOK, gin.H{
		"roomId":       room.ID,
		"name":         room.Name,
		"hostId":       room.HostID,
		"maxPlayers":   room.MaxPlayers,
		"private":      room.Private,
		"bannedWords":  room.BannedWords,
		"participants": room.Participants,
	})
}

func GetRoomHandler(c *gin.Context) {
	roomsMu.RLock()
	defer roomsMu.RUnlock()
	roomID := c.Param("id")

	room := GetRoom(roomID)

	if room == nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "Room not found",
		})
		return
	}

	room.mu.RLock()

	participants := make([]gin.H, 0)

	for _, p := range room.Participants {

		participants = append(participants, gin.H{
			"id":       p.UserID,
			"username": p.Username,
			"micOn":    p.MicOn,
		})
	}

	room.mu.RUnlock()

	c.JSON(http.StatusOK, gin.H{
		"roomId":       room.ID,
		"name":         room.Name,
		"hostId":       room.HostID,
		"participants": participants,
		"maxPlayers":   room.MaxPlayers,
		"private":      room.Private,
		"gameStarted":  room.GameStarted,

		"phase":            room.RoundPhase,
		"commanderId":      room.CommanderID,
		"selectedId":       room.LastSelectedID,
		"currentChallenge": room.CurrentChallenge,
		"currentChoice":    room.CurrentChoice,
		"targetResponse":   room.LastTargetResponse,
		"isPaused":         room.IsPaused,
	})
}

// UploadFileHandler handles file uploads for a room and stores them on Cloudinary
func UploadFileHandler(c *gin.Context) {
	roomID := c.Param("id")
	userID := c.GetString("user_id")

	room := GetRoom(roomID)
	if room == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Room not found"})
		return
	}

	// Verify user is a participant
	room.mu.RLock()
	isParticipant := false
	for _, p := range room.Participants {
		if p.UserID == userID {
			isParticipant = true
			break
		}
	}
	room.mu.RUnlock()

	if !isParticipant {
		c.JSON(http.StatusForbidden, gin.H{"error": "You are not in this room"})
		return
	}

	// Parse the uploaded file (10MB max)
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 10<<20)
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to read file: " + err.Error()})
		return
	}
	defer file.Close()

	result, err := cloudinary.UploadFile(file, header.Filename, roomID)
	if err != nil {
		fmt.Printf("[Upload] Cloudinary error: %v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Upload failed"})
		return
	}

	// Track the resource on the room
	room.mu.Lock()
	room.UploadedResources = append(room.UploadedResources, UploadedResource{
		PublicID:     result.PublicID,
		URL:          result.SecureURL,
		ResourceType: result.ResourceType,
	})
	room.mu.Unlock()

	fmt.Printf("[Upload] File uploaded for room %s: %s (%s)\n", roomID, result.PublicID, result.ResourceType)

	c.JSON(http.StatusOK, gin.H{
		"url":       result.SecureURL,
		"publicId":  result.PublicID,
		"file_type": result.ResourceType,
	})
}

// cleanupRoomResources deletes all Cloudinary resources associated with a room
func cleanupRoomResources(room *Room) {
	if room == nil {
		return
	}

	room.mu.RLock()
	resources := make([]UploadedResource, len(room.UploadedResources))
	copy(resources, room.UploadedResources)
	roomID := room.ID
	room.mu.RUnlock()

	if len(resources) == 0 {
		return
	}

	// Fire-and-forget in a goroutine
	go func() {
		publicIDs := make([]string, len(resources))
		resourceTypes := make([]string, len(resources))
		for i, r := range resources {
			publicIDs[i] = r.PublicID
			resourceTypes[i] = r.ResourceType
		}

		fmt.Printf("[Cleanup] Deleting %d Cloudinary resources for room %s\n", len(publicIDs), roomID)

		if err := cloudinary.DeleteResources(publicIDs, resourceTypes); err != nil {
			fmt.Printf("[Cleanup] Error deleting resources for room %s: %v\n", roomID, err)
		}

		// Try to clean up the folder too
		_ = cloudinary.DeleteFolder(roomID)

		fmt.Printf("[Cleanup] Done cleaning up room %s\n", roomID)
	}()
}
