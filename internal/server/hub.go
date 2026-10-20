// Package server runs rooms and games and speaks the JSON protocol over WebSocket.
package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"time"

	"renju-game/internal/engine"
	"renju-game/internal/game"
)

// Config holds the hub's settings.
type Config struct {
	Engine         engine.Factory // engine for AI games
	Fallback       engine.Factory // used when Engine keeps failing; may be nil
	EngineTimeout  time.Duration  // think time per AI move
	MaxRestarts    int            // engine restarts per move before falling back
	ReconnectGrace time.Duration  // how long a disconnected player's seat is held
	Logger         *slog.Logger
}

type roomState int

const (
	waiting roomState = iota
	playing
	finished
)

func (s roomState) String() string {
	return [...]string{"waiting", "playing", "finished"}[s]
}

type room struct {
	code    string // join code; empty for AI games
	rule    game.Rule
	game    *game.Game
	state   roomState
	seats   [3]*seat   // indexed by game.Color; nil for the AI's side
	aiColor game.Color // Empty for human vs human
	engine  *engine.Managed
	aiBusy  bool
	ctx     context.Context
	cancel  context.CancelFunc
}

func (r *room) mode() string {
	if r.aiColor != game.Empty {
		return "ai"
	}
	return "pvp"
}

type seat struct {
	room   *room
	color  game.Color
	token  string
	client *client // nil while disconnected
	timer  *time.Timer
}

// Hub owns every room. One mutex guards all room, seat and client-seat state;
// each operation is short, and AI thinking runs outside the lock.
type Hub struct {
	cfg Config

	mu     sync.Mutex
	rooms  map[string]*room // human rooms by join code
	seats  map[string]*seat // by reconnect token
	active map[*room]struct{}
}

// NewHub returns a hub with cfg; zero durations get defaults.
func NewHub(cfg Config) *Hub {
	if cfg.EngineTimeout == 0 {
		cfg.EngineTimeout = 3 * time.Second
	}
	if cfg.ReconnectGrace == 0 {
		cfg.ReconnectGrace = 60 * time.Second
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	return &Hub{
		cfg:    cfg,
		rooms:  map[string]*room{},
		seats:  map[string]*seat{},
		active: map[*room]struct{}{},
	}
}

// Stats reports live counts, for tests and monitoring.
type Stats struct {
	Rooms    int // joinable or in-progress human rooms
	Sessions int // reconnect tokens held
	Active   int // all unfinished games, AI included
}

func (h *Hub) Stats() Stats {
	h.mu.Lock()
	defer h.mu.Unlock()
	return Stats{Rooms: len(h.rooms), Sessions: len(h.seats), Active: len(h.active)}
}

func (h *Hub) handle(c *client, m Msg) {
	h.mu.Lock()
	defer h.mu.Unlock()
	switch m.Type {
	case "create_room":
		h.createRoom(c, m)
	case "join_room":
		h.joinRoom(c, m)
	case "start_ai":
		h.startAI(c, m)
	case "move":
		h.move(c, m)
	case "reconnect":
		h.reconnect(c, m)
	case "leave":
		h.leave(c)
	default:
		c.sendError(ErrUnknownType, "unknown message type "+m.Type)
	}
}

func (h *Hub) newRoom(rule game.Rule) *room {
	ctx, cancel := context.WithCancel(context.Background())
	r := &room{rule: rule, ctx: ctx, cancel: cancel}
	h.active[r] = struct{}{}
	return r
}

func (h *Hub) newSeat(r *room, color game.Color, c *client) *seat {
	s := &seat{room: r, color: color, token: newToken(), client: c}
	r.seats[color] = s
	h.seats[s.token] = s
	c.seat = s
	return s
}

// parseSetup reads the color and rule shared by create_room and start_ai.
func parseSetup(c *client, m Msg) (game.Color, game.Rule, bool) {
	color, err := game.ParseColor(m.Color)
	if err != nil {
		c.sendError(ErrBadRequest, "color must be black or white")
		return 0, "", false
	}
	rule, err := game.ParseRule(m.Rule)
	if err != nil {
		c.sendError(ErrUnsupportedRule, err.Error())
		return 0, "", false
	}
	return color, rule, true
}

func (h *Hub) createRoom(c *client, m Msg) {
	if c.seat != nil {
		c.sendError(ErrAlreadyInGame, "leave the current room first")
		return
	}
	color, rule, ok := parseSetup(c, m)
	if !ok {
		return
	}
	r := h.newRoom(rule)
	r.code = h.newCode()
	h.rooms[r.code] = r
	s := h.newSeat(r, color, c)
	c.send(Msg{Type: "room_created", Code: r.code, Color: color.String(), Rule: string(rule), Token: s.token})
}

func (h *Hub) joinRoom(c *client, m Msg) {
	if c.seat != nil {
		c.sendError(ErrAlreadyInGame, "leave the current room first")
		return
	}
	r := h.rooms[strings.ToUpper(strings.TrimSpace(m.Code))]
	if r == nil {
		c.sendError(ErrRoomNotFound, "no room with that code")
		return
	}
	if r.state != waiting {
		c.sendError(ErrRoomFull, "room already has two players")
		return
	}
	var hostColor game.Color
	for _, col := range []game.Color{game.Black, game.White} {
		if r.seats[col] != nil {
			hostColor = col
		}
	}
	h.newSeat(r, hostColor.Opponent(), c)
	r.state = playing
	r.game = game.New(r.rule)
	h.sendStart(r)
}

func (h *Hub) startAI(c *client, m Msg) {
	if c.seat != nil {
		c.sendError(ErrAlreadyInGame, "leave the current room first")
		return
	}
	color, rule, ok := parseSetup(c, m)
	if !ok {
		return
	}
	r := h.newRoom(rule)
	r.aiColor = color.Opponent()
	r.engine = &engine.Managed{
		Factory:     h.cfg.Engine,
		Fallback:    h.cfg.Fallback,
		Rule:        rule,
		Timeout:     h.cfg.EngineTimeout,
		MaxRestarts: h.cfg.MaxRestarts,
	}
	h.newSeat(r, color, c)
	r.state = playing
	r.game = game.New(rule)
	h.sendStart(r)
	if r.aiColor == game.Black {
		h.runAI(r)
	}
}

func (h *Hub) sendStart(r *room) {
	for _, s := range r.seats {
		if s != nil && s.client != nil {
			s.client.send(Msg{
				Type: "game_start", Mode: r.mode(), Code: r.code, Color: s.color.String(),
				Rule: string(r.rule), Size: game.Size, Turn: r.game.Turn().String(), Token: s.token,
			})
		}
	}
}

func (h *Hub) move(c *client, m Msg) {
	s := c.seat
	if s == nil || s.room.state == waiting {
		c.sendError(ErrNotInGame, "no game in progress")
		return
	}
	if m.Move == nil {
		c.sendError(ErrBadRequest, "move is required")
		return
	}
	r := s.room
	// A move sent before the client saw the opponent's last move is stale, even
	// if it would now be legal: when both players send at once, only the player
	// whose turn it was gets their move in.
	if m.Ply != nil && *m.Ply != r.game.MoveCount() && !r.game.Status().Over() {
		c.sendError(ErrNotYourTurn, "move was sent for an earlier position")
		return
	}
	if err := r.game.Play(s.color, *m.Move); err != nil {
		c.sendError(moveError(err), err.Error())
		return
	}
	h.afterMove(r, s.color, *m.Move)
}

// afterMove broadcasts a move that was just played and ends the game or hands the turn to the AI.
func (h *Hub) afterMove(r *room, color game.Color, mv game.Move) {
	ply := r.game.MoveCount() - 1
	h.broadcast(r, Msg{Type: "move", Move: &mv, Ply: &ply, Color: color.String(), Turn: r.game.Turn().String()})
	switch st := r.game.Status(); st {
	case game.BlackWon, game.WhiteWon:
		h.finish(r, st.Winner(), ReasonFive)
	case game.Draw:
		h.finish(r, game.Empty, ReasonDraw)
	default:
		if r.game.Turn() == r.aiColor {
			h.runAI(r)
		}
	}
}

func moveError(err error) string {
	switch {
	case errors.Is(err, game.ErrOccupied):
		return ErrOccupied
	case errors.Is(err, game.ErrOutOfBounds):
		return ErrOutOfBounds
	case errors.Is(err, game.ErrNotYourTurn):
		return ErrNotYourTurn
	case errors.Is(err, game.ErrGameOver):
		return ErrGameOver
	}
	return ErrBadRequest
}

// runAI asks the engine for a move without holding the lock, then plays it.
// Called with h.mu held.
func (h *Hub) runAI(r *room) {
	r.aiBusy = true
	moves := r.game.Moves()
	go func() {
		mv, err := r.engine.NextMove(r.ctx, moves)
		h.mu.Lock()
		defer h.mu.Unlock()
		r.aiBusy = false
		if r.state != playing {
			_ = r.engine.Close() // the game ended while the engine was thinking
			return
		}
		if err == nil {
			err = r.game.Play(r.aiColor, mv)
		}
		if err != nil {
			h.cfg.Logger.Error("engine failed", "err", err)
			h.finish(r, game.Empty, ReasonEngineError)
			return
		}
		h.afterMove(r, r.aiColor, mv)
	}()
}

func (h *Hub) reconnect(c *client, m Msg) {
	s := h.seats[m.Token]
	if s == nil {
		c.sendError(ErrSessionNotFound, "no game for that token")
		return
	}
	if c.seat != nil && c.seat != s {
		c.sendError(ErrAlreadyInGame, "leave the current room first")
		return
	}
	if old := s.client; old != nil && old != c {
		old.seat = nil
		old.close()
	}
	s.client = c
	c.seat = s
	if s.timer != nil {
		s.timer.Stop()
		s.timer = nil
	}
	r := s.room
	st := Msg{Type: "state", Mode: r.mode(), Code: r.code, Color: s.color.String(),
		Rule: string(r.rule), Size: game.Size, Status: r.state.String(), Token: s.token}
	if r.game != nil {
		st.Turn = r.game.Turn().String()
		st.Moves = r.game.Moves()
	}
	c.send(st)
	h.notifyOpponent(s, "opponent_reconnected")
}

func (h *Hub) leave(c *client) {
	s := c.seat
	if s == nil {
		return
	}
	switch s.room.state {
	case waiting:
		h.cleanup(s.room)
	case playing:
		h.finish(s.room, s.color.Opponent(), ReasonResign)
	}
	c.seat = nil
}

// disconnect runs when a client's connection closes.
func (h *Hub) disconnect(c *client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	s := c.seat
	if s == nil || s.client != c {
		return
	}
	c.seat = nil
	s.client = nil
	r := s.room
	switch r.state {
	case waiting:
		// The host left before anyone joined: the room and its code go away.
		h.cleanup(r)
	case playing:
		h.notifyOpponent(s, "opponent_disconnected")
		s.timer = time.AfterFunc(h.cfg.ReconnectGrace, func() {
			h.mu.Lock()
			defer h.mu.Unlock()
			if s.client == nil && r.state == playing {
				h.finish(r, s.color.Opponent(), ReasonAbandoned)
			}
		})
	}
}

func (h *Hub) notifyOpponent(s *seat, typ string) {
	if o := s.room.seats[s.color.Opponent()]; o != nil && o.client != nil && s.room.state == playing {
		o.client.send(Msg{Type: typ})
	}
}

func (h *Hub) broadcast(r *room, m Msg) {
	for _, s := range r.seats {
		if s != nil && s.client != nil {
			s.client.send(m)
		}
	}
}

func (h *Hub) finish(r *room, winner game.Color, reason string) {
	h.broadcast(r, Msg{Type: "game_over", Winner: winner.String(), Reason: reason})
	h.cleanup(r)
}

// cleanup ends a room and releases its code, tokens, timers and engine.
func (h *Hub) cleanup(r *room) {
	r.state = finished
	if r.code != "" {
		delete(h.rooms, r.code)
	}
	for _, s := range r.seats {
		if s == nil {
			continue
		}
		delete(h.seats, s.token)
		if s.timer != nil {
			s.timer.Stop()
		}
		if s.client != nil {
			s.client.seat = nil
		}
	}
	r.cancel()
	if r.engine != nil && !r.aiBusy {
		_ = r.engine.Close()
	}
	delete(h.active, r)
}

// codeAlphabet leaves out 0/O and 1/I so codes are easy to read aloud.
const codeAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"

func (h *Hub) newCode() string {
	for {
		b := make([]byte, 6)
		_, _ = rand.Read(b)
		for i := range b {
			b[i] = codeAlphabet[int(b[i])%len(codeAlphabet)]
		}
		if code := string(b); h.rooms[code] == nil {
			return code
		}
	}
}

func newToken() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
