package room

import (
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// User represents a participant in a room
type User struct {
	ID       string
	Username string
	Conn     *websocket.Conn
	MicOn    bool
}

// Room represents a game room
type Room struct {
	ID           string
	Name         string
	LastActivity time.Time
	HostID       string    `json:"hostId"`
	Participants []*Client `json:"participants"`
	MaxPlayers   int       `json:"maxPlayers"`
	Private      bool
	BannedWords  []string `json:"bannedWords"`
	TimeLimit    int      `json:"timeLimit"`
	Password     string   `json:"password"`

	// Game state
	CurrentPlayerID    string // player whose turn it is to pick truth/dare
	CommanderID        string // who is currently giving the prompt
	LastSelectedID     string // player the bottle landed on
	MicOverride        bool   // true if commander is speaking
	GameStarted        bool
	LastTargetResponse string     `json:"lastTargetResponse"`
	CurrentChoice      string     `json:"currentChoice"`
	CurrentChallenge   *Challenge `json:"currentChallenge"`

	RoundHistory []RoundHistoryItem `json:"roundHistory"`
	IsPaused     bool               `json:"isPaused"`

	CurrentCommanderID string
	CurrentTargetID    string
	RoundPhase         RoomPhase
	PausedPhase        RoomPhase `json:"pausedPhase"`

	// Cloudinary resource tracking
	UploadedResources []UploadedResource `json:"uploadedResources"`

	// Add mutex for thread safety
	mu sync.RWMutex
}

// UploadedResource tracks a file uploaded to Cloudinary for this room
type UploadedResource struct {
	PublicID     string `json:"publicId"`
	URL          string `json:"url"`
	ResourceType string `json:"resourceType"` // "image", "video", "raw"
}

type Challenge struct {
	Type        string `json:"type"`
	Text        string `json:"text"`
	TargetID    string `json:"targetId"`
	CommanderID string `json:"commanderId"`
}

type RoundHistoryItem struct {
	ID         string `json:"id"`
	Type       string `json:"type"`
	Text       string `json:"text"`
	ActorID    string `json:"actorId"`
	ActorName  string `json:"actorName"`
	TargetID   string `json:"targetId,omitempty"`
	TargetName string `json:"targetName,omitempty"`
	Timestamp  int64  `json:"timestamp"`
}

type RoomPhase string

const (
	PhaseSpinning        RoomPhase = "spinning"
	PhaseChoosing        RoomPhase = "choosing"
	PhaseCommanderPrompt RoomPhase = "commander_prompt"
	PhaseTargetReply     RoomPhase = "target_reply"
	PhaseReveal          RoomPhase = "reveal"
	PhasePaused          RoomPhase = "paused"
	PhaseWaiting         RoomPhase = "waiting"
)

// Map to store active rooms in memory .
var (
	Rooms   = make(map[string]*Room)
	roomsMu sync.RWMutex
)
