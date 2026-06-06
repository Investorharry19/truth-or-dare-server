package room

import (
	"fmt"
	"time"

	"github.com/gin-gonic/gin"
)

// GetRoom fetches a room by ID
func GetRoom(id string) *Room {
	if room, ok := Rooms[id]; ok {
		return room
	}
	return nil
}

// CreateRoom creates a new room and adds it to the map
func CreateRoom(id, name, hostID, hostUsername string, maxPlayers int, private bool) *Room {
	room := &Room{
		ID:           id,
		Name:         name,
		HostID:       hostID,
		MaxPlayers:   maxPlayers,
		Private:      private,
		Participants: make([]*Client, 0),
		TimeLimit:    30,
		LastActivity: time.Now(),
	}

	Rooms[id] = room

	fmt.Println("Room created with host:", id, hostID)

	return room
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
		if client == nil || client.Conn == nil {
			continue
		}
		fmt.Println(68)
		fmt.Println(client)

		err := client.Conn.WriteJSON(message)

		if err != nil {
			fmt.Printf("Broadcast error to %s: %v\n", client.UserID, err)
		}
	}
}

// StartInactivityCleanup starts a background ticker that removes rooms inactive for more than the timeout duration
func StartInactivityCleanup(timeout time.Duration) {
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()

		for range ticker.C {
			roomsMu.Lock()
			now := time.Now()
			for id, r := range Rooms {
				r.mu.RLock()
				inactive := now.Sub(r.LastActivity) > timeout
				r.mu.RUnlock()

				if inactive {
					// Dissolve the room
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

					delete(Rooms, id)
					fmt.Printf("Room %s closed due to inactivity\n", id)
				}
			}
			roomsMu.Unlock()
		}
	}()
}
