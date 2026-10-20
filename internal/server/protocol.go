package server

import "renju-game/internal/game"

// Msg is the single JSON message shape used in both directions over the WebSocket.
// Fields that do not apply to a message type are omitted.
//
// Client → server:
//
//	{"type":"create_room","color":"black","rule":"freestyle"}
//	{"type":"join_room","code":"K7QX2M"}
//	{"type":"start_ai","color":"white"}
//	{"type":"move","move":{"x":7,"y":7},"ply":4}   ply = moves the client has seen; optional
//	{"type":"reconnect","token":"…"}
//	{"type":"leave"}
//
// Server → client:
//
//	room_created          code, color, rule, token
//	game_start            mode, code, color (yours), rule, size, turn, token
//	move                  move, ply (its 0-based index), color (who played it), turn (next to move)
//	game_over             winner ("" for a draw), reason
//	state                 reply to reconnect: mode, code, color, rule, size, turn, moves, status
//	opponent_disconnected / opponent_reconnected
//	error                 error (code), message
type Msg struct {
	Type    string      `json:"type"`
	Mode    string      `json:"mode,omitempty"`
	Code    string      `json:"code,omitempty"`
	Token   string      `json:"token,omitempty"`
	Color   string      `json:"color,omitempty"`
	Rule    string      `json:"rule,omitempty"`
	Size    int         `json:"size,omitempty"`
	Turn    string      `json:"turn,omitempty"`
	Move    *game.Move  `json:"move,omitempty"`
	Moves   []game.Move `json:"moves,omitempty"`
	Ply     *int        `json:"ply,omitempty"`
	Status  string      `json:"status,omitempty"`
	Winner  string      `json:"winner,omitempty"`
	Reason  string      `json:"reason,omitempty"`
	Error   string      `json:"error,omitempty"`
	Message string      `json:"message,omitempty"`
}

// Error codes sent in Msg.Error.
const (
	ErrBadRequest      = "bad_request"
	ErrUnknownType     = "unknown_type"
	ErrAlreadyInGame   = "already_in_game"
	ErrRoomNotFound    = "room_not_found"
	ErrRoomFull        = "room_full"
	ErrNotInGame       = "not_in_game"
	ErrSessionNotFound = "session_not_found"
	ErrUnsupportedRule = "unsupported_rule"
	ErrOccupied        = "occupied"
	ErrOutOfBounds     = "out_of_bounds"
	ErrNotYourTurn     = "not_your_turn"
	ErrGameOver        = "game_over"
)

// Game-over reasons sent in Msg.Reason.
const (
	ReasonFive        = "five"
	ReasonDraw        = "draw"
	ReasonResign      = "resign"
	ReasonAbandoned   = "abandoned"
	ReasonEngineError = "engine_error"
)
