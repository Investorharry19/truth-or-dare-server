package room

import (
	"testing"
	"time"
)

func TestProcessInactiveRoomsStartsRoomWithTwoPlayers(t *testing.T) {
	originalRooms := Rooms
	now := time.Now()
	room := &Room{
		ID:           "active",
		HostID:       "host",
		LastActivity: now.Add(-InactivityTimeout),
		Participants: []*Client{{UserID: "host"}, {UserID: "guest"}},
	}

	roomsMu.Lock()
	Rooms = map[string]*Room{room.ID: room}
	roomsMu.Unlock()
	t.Cleanup(func() {
		roomsMu.Lock()
		Rooms = originalRooms
		roomsMu.Unlock()
	})

	toStart, toClose, lobbyListChanged := processInactiveRooms(now, InactivityTimeout)

	if len(toStart) != 1 || toStart[0] != room {
		t.Fatalf("expected room to start, got %d rooms", len(toStart))
	}
	if len(toClose) != 0 {
		t.Fatalf("expected no rooms to close, got %d", len(toClose))
	}
	if !lobbyListChanged {
		t.Fatal("expected lobby list to be refreshed")
	}
	if !room.GameStarted ||
		room.CurrentPlayerID != room.HostID ||
		room.CommanderID != room.HostID ||
		room.CurrentCommanderID != room.HostID ||
		room.RoundPhase != PhaseSpinning {
		t.Fatal("expected room to be initialized for game start")
	}
}

func TestProcessInactiveRoomsClosesRoomWithFewerThanTwoPlayers(t *testing.T) {
	originalRooms := Rooms
	now := time.Now()
	room := &Room{
		ID:           "waiting",
		LastActivity: now.Add(-InactivityTimeout),
		Participants: []*Client{{UserID: "host"}},
	}
	activeRoom := &Room{
		ID:           "recent",
		LastActivity: now,
		Participants: []*Client{{UserID: "host"}},
	}

	roomsMu.Lock()
	Rooms = map[string]*Room{
		room.ID:       room,
		activeRoom.ID: activeRoom,
	}
	roomsMu.Unlock()
	t.Cleanup(func() {
		roomsMu.Lock()
		Rooms = originalRooms
		roomsMu.Unlock()
	})

	toStart, toClose, lobbyListChanged := processInactiveRooms(now, InactivityTimeout)

	if len(toStart) != 0 {
		t.Fatalf("expected no rooms to start, got %d", len(toStart))
	}
	if len(toClose) != 1 || toClose[0] != room {
		t.Fatalf("expected only the inactive single-player room to close, got %d rooms", len(toClose))
	}
	if !lobbyListChanged {
		t.Fatal("expected lobby list to be refreshed")
	}
	if _, exists := Rooms[room.ID]; exists {
		t.Fatal("expected inactive room to be removed")
	}
	if Rooms[activeRoom.ID] != activeRoom {
		t.Fatal("expected recently active room to remain")
	}
}
