package presence

// PhotonRoomDetails holds optional/session-specific game state
type PhotonRoomDetails struct {
	Region      string `json:"reg" validate:"required,max=32"`
	Lobby       string `json:"lob" validate:"required,max=64"`
	Room        string `json:"rom" validate:"required,max=64"`
	Mode        string `json:"mod" validate:"required,max=32"`
	Difficulty  string `json:"dif" validate:"required,max=32"`
	Map         string `json:"map" validate:"required,max=64"`
	PlayerCount int    `json:"plc" validate:"gte=1,lte=100"`
	PrivateRoom bool   `json:"pri"`
}

// PlayerPrivacySettings holds optional privacy preferences
type PlayerPrivacySettings struct {
	AllowFriendsWithoutCode *bool   `json:"awc"`
	Visibility              *string `json:"vis" validate:"omitempty,oneof=online offline"`
}

func (p *PlayerPrivacySettings) SetDefaults() {
	defaultTrue := true
	defaultOffline := "offline"

	if p.AllowFriendsWithoutCode == nil {
		p.AllowFriendsWithoutCode = &defaultTrue
	}

	if p.Visibility == nil || *p.Visibility == "" {
		p.Visibility = &defaultOffline
	}
}

// HeartbeatPayload embeds the sub-structs while preserving the top-level JSON structure
type HeartbeatPayload struct {
	AppVersion            string `json:"ver" validate:"required,max=32"`
	PhotonRoomDetails     `validate:"structonly"`
	PlayerPrivacySettings `validate:"structonly"`
}
