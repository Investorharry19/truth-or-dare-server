package room

import "encoding/json"

// WSMessageTyped represents the future strict-typed websocket message format
// to replace the current WSMessage that uses map[string]interface{}.
type WSMessageTyped struct {
	Type      string          `json:"type"`
	Payload   json.RawMessage `json:"payload"`
	AudioUri  string          `json:"audio_uri"`
	AudioData string          `json:"audio_data"`
	Timestamp int64           `json:"timestamp"`
	MicOn     *bool           `json:"mic_on"`
}

// JoinRoomPayload defines the expected payload for "join_room" events
type JoinRoomPayload struct {
	RoomID   string `json:"room_id"`
	Passcode string `json:"passcode,omitempty"`
}

// ChoicePayload defines the expected payload for "make_choice" events
type ChoicePayload struct {
	Choice string `json:"choice"`
}

// ChallengePayload defines the expected payload for "issue_challenge" events
type ChallengePayload struct {
	PromptText string `json:"prompt_text"`
	Choice     string `json:"choice"`
}

// ReplyPayload defines the expected payload for "target_reply" events
type ReplyPayload struct {
	ResponseText string `json:"response_text"`
	Text         string `json:"text,omitempty"` // Fallback support
}

// ChatPayload defines the expected payload for "chat_message" events
type ChatPayload struct {
	Text     string `json:"text"`
	FileURL  string `json:"file_url,omitempty"`
	FileType string `json:"file_type,omitempty"`
}
