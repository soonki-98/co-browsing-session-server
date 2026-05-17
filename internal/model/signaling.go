package model

type SignalingMessage struct {
	Type    string `json:"type"`
	Payload any    `json:"payload"`
}

type JoinRoomPayload struct {
	RoomID   string `json:"room_id"`
	UserID   string `json:"user_id"`
	UserRole string `json:"user_role"`
}
