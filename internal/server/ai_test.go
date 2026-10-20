package server

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"renju-game/internal/engine"
	"renju-game/internal/game"
)

func TestAIWaitsWhenPlayerIsBlack(t *testing.T) {
	s := newTestServer(t, Config{})
	c := s.dial(t)
	c.sendMsg(Msg{Type: "start_ai", Color: "black"})
	if m := c.expect("game_start"); m.Mode != "ai" || m.Color != "black" || m.Turn != "black" {
		t.Fatalf("game_start = %+v", m)
	}
	c.expectSilence(300 * time.Millisecond)
	c.play(7, 7)
	c.expectMove(7, 7)
	if m := c.expect("move"); m.Color != "white" {
		t.Fatalf("AI reply = %+v, want a white move", m)
	}
}

func TestAIMovesFirstWhenPlayerIsWhite(t *testing.T) {
	s := newTestServer(t, Config{})
	c := s.dial(t)
	c.sendMsg(Msg{Type: "start_ai", Color: "white"})
	c.expect("game_start")
	m := c.expect("move")
	if m.Color != "black" || m.Turn != "white" {
		t.Fatalf("AI first move = %+v", m)
	}
}

func TestAIReplyWithinTimeout(t *testing.T) {
	const timeout = 300 * time.Millisecond
	s := newTestServer(t, Config{EngineTimeout: timeout})
	c := s.dial(t)
	c.sendMsg(Msg{Type: "start_ai", Color: "black"})
	c.expect("game_start")
	for _, mv := range []game.Move{p(7, 7), p(8, 8), p(6, 9)} {
		start := time.Now()
		c.play(mv.X, mv.Y)
		c.expectMove(mv.X, mv.Y)
		reply := c.expect("move")
		if el := time.Since(start); el > timeout {
			t.Fatalf("AI took %v, timeout %v", el, timeout)
		}
		// Keep the scripted moves legal if the AI picked one of them.
		if *reply.Move == p(8, 8) || *reply.Move == p(6, 9) {
			t.Skip("AI took a scripted point; timing already checked")
		}
	}
}

// TestFullAIGame plays a whole game against the AI, with the player driven by a
// second builtin engine, checking every AI move is legal.
func TestFullAIGame(t *testing.T) {
	for _, color := range []string{"black", "white"} {
		t.Run(color, func(t *testing.T) {
			s := newTestServer(t, Config{EngineTimeout: 100 * time.Millisecond})
			c := s.dial(t)
			c.sendMsg(Msg{Type: "start_ai", Color: color})
			c.expect("game_start")
			me, _ := game.ParseColor(color)

			local := game.New(game.Freestyle)
			player := &engine.Managed{Factory: func() (engine.Engine, error) { return engine.NewBuiltin(), nil },
				Rule: game.Freestyle, Timeout: 100 * time.Millisecond}
			defer player.Close()
			for {
				if local.Turn() == me && !local.Status().Over() {
					mv, err := player.NextMove(context.Background(), local.Moves())
					if err != nil {
						t.Fatal(err)
					}
					c.play(mv.X, mv.Y)
				}
				m := c.recv()
				switch m.Type {
				case "move":
					col, _ := game.ParseColor(m.Color)
					if err := local.Play(col, *m.Move); err != nil {
						t.Fatalf("server sent illegal %s move %v: %v", m.Color, *m.Move, err)
					}
				case "game_over":
					if m.Reason != ReasonFive && m.Reason != ReasonDraw {
						t.Fatalf("game_over = %+v", m)
					}
					if !local.Status().Over() || local.Status().Winner().String() != m.Winner {
						t.Fatalf("server says %+v, local status %v", m, local.Status())
					}
					s.waitStats(t, Stats{})
					return
				default:
					t.Fatalf("unexpected %+v", m)
				}
			}
		})
	}
}

// firstEmpty is a mock GomokuEngine that plays the first empty point, scanning x then y.
type firstEmpty struct {
	moves  []game.Move
	closed *atomic.Int32
	crash  bool
}

func (e *firstEmpty) Start(context.Context, int, game.Rule) error { return nil }
func (e *firstEmpty) SetPosition(_ context.Context, m []game.Move) error {
	e.moves = m
	return nil
}
func (e *firstEmpty) Think(context.Context, time.Duration) (game.Move, error) {
	if e.crash {
		return game.Move{}, errors.New("mock crash")
	}
	used := map[game.Move]bool{}
	for _, m := range e.moves {
		used[m] = true
	}
	for x := 0; x < game.Size; x++ {
		for y := 0; y < game.Size; y++ {
			if !used[p(x, y)] {
				return p(x, y), nil
			}
		}
	}
	return game.Move{}, errors.New("board full")
}
func (e *firstEmpty) Close() error {
	e.closed.Add(1)
	return nil
}

func TestMockEngineSwapsIn(t *testing.T) {
	var created, closed atomic.Int32
	s := newTestServer(t, Config{Engine: func() (engine.Engine, error) {
		created.Add(1)
		return &firstEmpty{closed: &closed}, nil
	}})
	c := s.dial(t)
	c.sendMsg(Msg{Type: "start_ai", Color: "black"})
	c.expect("game_start")
	// The mock answers at 0,0 0,1 0,2 0,3, so black's row 7 is never blocked.
	for x := 3; x < 8; x++ {
		c.play(x, 7)
		c.expectMove(x, 7)
		if x < 7 {
			c.expectMove(0, x-3)
		}
	}
	expectGameOver(t, c, "black", ReasonFive)
	s.waitStats(t, Stats{})
	if created.Load() != 1 || closed.Load() != 1 {
		t.Fatalf("engines created %d closed %d, want 1 and 1", created.Load(), closed.Load())
	}
}

func TestEngineCrashRecovers(t *testing.T) {
	var created, closed atomic.Int32
	s := newTestServer(t, Config{MaxRestarts: 1, Engine: func() (engine.Engine, error) {
		n := created.Add(1)
		// The first instance crashes on its first move; its replacement works.
		return &firstEmpty{closed: &closed, crash: n == 1}, nil
	}})
	c := s.dial(t)
	c.sendMsg(Msg{Type: "start_ai", Color: "black"})
	c.expect("game_start")
	c.play(7, 7)
	c.expectMove(7, 7)
	c.expectMove(0, 0)
	c.play(7, 8)
	c.expectMove(7, 8)
	// The restarted engine was given the whole position, so it skips 0,0.
	c.expectMove(0, 1)
	if created.Load() != 2 {
		t.Fatalf("engines created %d, want 2", created.Load())
	}
}

func TestEngineFailureEndsGame(t *testing.T) {
	var closed atomic.Int32
	s := newTestServer(t, Config{MaxRestarts: 1, Engine: func() (engine.Engine, error) {
		return &firstEmpty{closed: &closed, crash: true}, nil
	}})
	c := s.dial(t)
	c.sendMsg(Msg{Type: "start_ai", Color: "white"})
	c.expect("game_start")
	expectGameOver(t, c, "", ReasonEngineError)
	s.waitStats(t, Stats{})
}

func TestEngineFallsBackToBuiltin(t *testing.T) {
	var closed atomic.Int32
	s := newTestServer(t, Config{
		MaxRestarts: 1,
		Engine: func() (engine.Engine, error) {
			return &firstEmpty{closed: &closed, crash: true}, nil
		},
		Fallback: func() (engine.Engine, error) { return engine.NewBuiltin(), nil },
	})
	c := s.dial(t)
	c.sendMsg(Msg{Type: "start_ai", Color: "white"})
	c.expect("game_start")
	// Builtin opens in the center.
	c.expectMove(7, 7)
}

func TestPlayerLeavesWhileAIThinks(t *testing.T) {
	var closed atomic.Int32
	slow := func() (engine.Engine, error) { return &slowEngine{closed: &closed}, nil }
	s := newTestServer(t, Config{Engine: slow, EngineTimeout: time.Second})
	c := s.dial(t)
	c.sendMsg(Msg{Type: "start_ai", Color: "white"})
	c.expect("game_start")
	c.sendMsg(Msg{Type: "leave"})
	expectGameOver(t, c, "black", ReasonResign)
	s.waitStats(t, Stats{})
	// The engine is closed once its in-flight think returns.
	deadline := time.Now().Add(3 * time.Second)
	for closed.Load() == 0 {
		if time.Now().After(deadline) {
			t.Fatal("engine was never closed")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

type slowEngine struct{ closed *atomic.Int32 }

func (e *slowEngine) Start(context.Context, int, game.Rule) error    { return nil }
func (e *slowEngine) SetPosition(context.Context, []game.Move) error { return nil }
func (e *slowEngine) Close() error                                   { e.closed.Add(1); return nil }
func (e *slowEngine) Think(ctx context.Context, _ time.Duration) (game.Move, error) {
	<-ctx.Done()
	return game.Move{}, ctx.Err()
}
