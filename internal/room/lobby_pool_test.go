package room

import (
	"testing"
	"time"
)

func TestEnsureLobbyPoolMaintainsFiveOpenPublicLobbies(t *testing.T) {
	roomsMu.Lock()
	originalRooms := Rooms
	Rooms = make(map[string]*Room)
	Rooms["public"] = &Room{
		ID:           "public",
		Name:         "Player lobby",
		MaxPlayers:   8,
		Participants: make([]*Client, 0),
	}
	Rooms["private"] = &Room{
		ID:           "private",
		MaxPlayers:   8,
		Private:      true,
		Participants: make([]*Client, 0),
	}
	Rooms["full"] = &Room{
		ID:           "full",
		MaxPlayers:   1,
		Participants: []*Client{{UserID: "player"}},
	}
	roomsMu.Unlock()
	t.Cleanup(func() {
		roomsMu.Lock()
		Rooms = originalRooms
		roomsMu.Unlock()
	})

	EnsureLobbyPool()
	lobbies := ListOpenLobbies()
	if len(lobbies) != LobbyPoolTarget {
		t.Fatalf("expected %d open lobbies, got %d", LobbyPoolTarget, len(lobbies))
	}

	generatedCount := 0
	for _, lobby := range lobbies {
		if lobby.Generated {
			generatedCount++
			if !lobby.Ownerless {
				t.Errorf("new generated lobby %q should be ownerless", lobby.RoomID)
			}
		}
	}
	if generatedCount != LobbyPoolTarget-1 {
		t.Errorf("expected %d generated lobbies alongside the player lobby, got %d", LobbyPoolTarget-1, generatedCount)
	}

	EnsureLobbyPool()
	if got := len(ListOpenLobbies()); got != LobbyPoolTarget {
		t.Errorf("second ensure should keep the target at %d, got %d", LobbyPoolTarget, got)
	}
}

func TestOwnerlessGeneratedLobbiesDoNotExpire(t *testing.T) {
	roomsMu.Lock()
	originalRooms := Rooms
	placeholder := &Room{
		ID:           "placeholder",
		Generated:    true,
		MaxPlayers:   8,
		LastActivity: time.Now().Add(-2 * InactivityTimeout),
		Participants: make([]*Client, 0),
	}
	Rooms = map[string]*Room{placeholder.ID: placeholder}
	roomsMu.Unlock()
	t.Cleanup(func() {
		roomsMu.Lock()
		Rooms = originalRooms
		roomsMu.Unlock()
	})

	toStart, toClose, changed := processInactiveRooms(time.Now(), InactivityTimeout)
	if len(toStart) != 0 || len(toClose) != 0 || changed {
		t.Fatalf("ownerless generated lobby should stay available: start=%d close=%d changed=%t", len(toStart), len(toClose), changed)
	}
	if Rooms[placeholder.ID] != placeholder {
		t.Fatal("ownerless generated lobby was removed")
	}
}
