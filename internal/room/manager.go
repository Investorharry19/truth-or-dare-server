package room

import (
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const InactivityTimeout = 5 * time.Minute
const LobbyPoolTarget = 5

var lobbyBroadcastMu sync.Mutex

type LobbySummary struct {
	RoomID       string `json:"roomId"`
	Name         string `json:"name"`
	HostID       string `json:"hostId"`
	HostUsername string `json:"hostUsername"`
	PlayerCount  int    `json:"playerCount"`
	MaxPlayers   int    `json:"maxPlayers"`
	Generated    bool   `json:"generated"`
	Ownerless    bool   `json:"ownerless"`
}

// GetRoom fetches a room by ID
func GetRoom(id string) *Room {
	if room, ok := Rooms[id]; ok {
		return room
	}
	return nil
}

// CreateRoom creates a new room and adds it to the map
func CreateRoom(id, name, hostID, hostUsername string, maxPlayers int, private bool, password string) *Room {
	room := &Room{
		ID:           id,
		Name:         name,
		HostID:       hostID,
		HostUsername: hostUsername,
		MaxPlayers:   maxPlayers,
		Private:      private,
		Participants: make([]*Client, 0),
		Password:     password,
		TimeLimit:    30,
		LastActivity: time.Now(),
	}

	Rooms[id] = room

	fmt.Println("Room created with host:", id, hostID)

	return room
}

func InitializeLobbyPool() {
	ensureLobbyPool()
}

func EnsureLobbyPool() {
	ensureLobbyPool()
}

func ensureLobbyPool() {
	roomsMu.Lock()
	defer roomsMu.Unlock()

	openCount := 0
	for _, existing := range Rooms {
		existing.mu.RLock()
		isOpen := !existing.Private &&
			!existing.GameStarted &&
			len(existing.Participants) < existing.MaxPlayers
		existing.mu.RUnlock()
		if isOpen {
			openCount++
		}
	}

	for openCount < LobbyPoolTarget {
		id := uuid.New().String()[:6]
		for Rooms[id] != nil {
			id = uuid.New().String()[:6]
		}

		name := fmt.Sprintf("%s %s", lobbyNamePrefixes[openCount%len(lobbyNamePrefixes)], lobbyNameSuffixes[openCount%len(lobbyNameSuffixes)])
		generated := CreateRoom(id, name, "", "", 8, false, "")
		generated.Generated = true
		openCount++
	}
}

var lobbyNamePrefixes = []string{
	"After Hours",
	"Chaos Crew",
	"Truth Seekers",
	"Dare Squad",
	"Wildcard",
}

var lobbyNameSuffixes = []string{
	"Hangout",
	"Showdown",
	"Party",
	"Spot",
	"Arena",
}

func ListOpenLobbies() []LobbySummary {
	roomsMu.RLock()
	defer roomsMu.RUnlock()

	lobbies := make([]LobbySummary, 0, len(Rooms))
	for _, current := range Rooms {
		current.mu.RLock()
		isOpen := !current.Private &&
			!current.GameStarted &&
			len(current.Participants) < current.MaxPlayers
		if isOpen {
			lobbies = append(lobbies, LobbySummary{
				RoomID:       current.ID,
				Name:         current.Name,
				HostID:       current.HostID,
				HostUsername: current.HostUsername,
				PlayerCount:  len(current.Participants),
				MaxPlayers:   current.MaxPlayers,
				Generated:    current.Generated,
				Ownerless:    current.Generated && current.HostID == "",
			})
		}
		current.mu.RUnlock()
	}

	sort.Slice(lobbies, func(i, j int) bool {
		return lobbies[i].RoomID < lobbies[j].RoomID
	})
	return lobbies
}

func RefreshLobbyPool() {
	ensureLobbyPool()
	broadcastLobbySnapshot()
}

func sendLobbySnapshot(client *Client) {
	if client == nil {
		return
	}
	if err := client.writeJSON(gin.H{
		"type":    "lobbies_snapshot",
		"payload": gin.H{"lobbies": ListOpenLobbies()},
	}); err != nil {
		fmt.Printf("Failed to send lobby snapshot to %s: %v\n", client.UserID, err)
	}
}

func broadcastLobbySnapshot() {
	lobbyBroadcastMu.Lock()
	defer lobbyBroadcastMu.Unlock()

	clientsMu.RLock()
	connections := make([]*Client, 0, len(clients))
	for _, client := range clients {
		connections = append(connections, client)
	}
	clientsMu.RUnlock()

	message := gin.H{
		"type":    "lobbies_updated",
		"payload": gin.H{"lobbies": ListOpenLobbies()},
	}
	for _, client := range connections {
		if err := client.writeJSON(message); err != nil {
			fmt.Printf("Failed to broadcast lobby update to %s: %v\n", client.UserID, err)
		}
	}
}

func Broadcast(room *Room, event string, payload interface{}) {

	fmt.Println("Sending room update")
	if room == nil {
		return
	}

	room.mu.RLock()

	participants := make([]*Client, 0)

	for _, p := range room.Participants {
		participants = append(participants, p)
	}

	room.mu.RUnlock()

	message := gin.H{
		"type":    event,
		"payload": payload,
	}
	fmt.Println("61")
	fmt.Println(participants)

	for _, client := range participants {
		if client == nil {
			continue
		}
		fmt.Println(68)
		fmt.Println(client)

		err := client.writeJSON(message)

		if err != nil {
			fmt.Printf("Broadcast error to %s: %v\n", client.UserID, err)
		}
	}
}

// StartInactivityCleanup starts a background ticker that removes rooms inactive for more than the timeout duration
func StartInactivityCleanup(timeout time.Duration) {
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()

		for range ticker.C {
			now := time.Now()

			toStart, toClose, lobbyListChanged := processInactiveRooms(now, timeout)

			for _, r := range toStart {
				Broadcast(r, "game_started", gin.H{
					"current_player_id": r.HostID,
				})
				sendRoomUpdate(r)
				fmt.Printf("Room %s started automatically after inactivity\n", r.ID)
			}

			for _, r := range toClose {
				Broadcast(r, "room_closed", gin.H{
					"message": "The room has been closed due to inactivity.",
				})

				// Clean up Cloudinary resources
				cleanupRoomResources(r)

				// Close all websocket connections of participants
				r.mu.Lock()
				for _, p := range r.Participants {
					if p.Conn != nil {
						p.Conn.Close()
					}
				}
				r.mu.Unlock()

				fmt.Printf("Room %s closed due to inactivity\n", r.ID)
			}

			if lobbyListChanged {
				RefreshLobbyPool()
			}
		}
	}()
}

func processInactiveRooms(now time.Time, timeout time.Duration) ([]*Room, []*Room, bool) {
	var toStart []*Room
	var toClose []*Room
	lobbyListChanged := false

	roomsMu.Lock()
	defer roomsMu.Unlock()

	for id, r := range Rooms {
		r.mu.Lock()
		if r.Generated && r.HostID == "" {
			r.mu.Unlock()
			continue
		}
		inactive := !r.GameStarted && now.Sub(r.LastActivity) >= timeout
		participantCount := len(r.Participants)

		if inactive {
			if participantCount >= 2 {
				r.GameStarted = true
				r.CurrentPlayerID = r.HostID
				r.CommanderID = r.HostID
				r.CurrentCommanderID = r.HostID
				r.RoundPhase = PhaseSpinning
				toStart = append(toStart, r)
			} else {
				toClose = append(toClose, r)
				delete(Rooms, id)
			}
			lobbyListChanged = true
		}
		r.mu.Unlock()
	}

	return toStart, toClose, lobbyListChanged
}
