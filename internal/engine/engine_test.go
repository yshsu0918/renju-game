package engine

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"renju-game/internal/game"
)

func think(t *testing.T, moves []game.Move, limit time.Duration) game.Move {
	t.Helper()
	b := NewBuiltin()
	ctx := context.Background()
	if err := b.Start(ctx, game.Size, game.Freestyle); err != nil {
		t.Fatal(err)
	}
	if err := b.SetPosition(ctx, moves); err != nil {
		t.Fatal(err)
	}
	mv, err := b.Think(ctx, limit)
	if err != nil {
		t.Fatal(err)
	}
	return mv
}

func TestBuiltinOpensInCenter(t *testing.T) {
	if mv := think(t, nil, time.Second); mv != (game.Move{X: 7, Y: 7}) {
		t.Fatalf("first move = %v, want 7,7", mv)
	}
}

func TestBuiltinCompletesOwnFour(t *testing.T) {
	// Black: (3..6, 7). White: scattered. Black to move must play (2,7) or (7,7).
	moves := []game.Move{
		p(3, 7), p(0, 0), p(4, 7), p(0, 14), p(5, 7), p(14, 0), p(6, 7), p(14, 14),
	}
	mv := think(t, moves, time.Second)
	if mv != (game.Move{X: 2, Y: 7}) && mv != (game.Move{X: 7, Y: 7}) {
		t.Fatalf("move = %v, want a five at 2,7 or 7,7", mv)
	}
}

func TestBuiltinPrefersWinOverBlock(t *testing.T) {
	// Both sides have four; black to move should win rather than block.
	moves := []game.Move{
		p(3, 3), p(3, 10), p(4, 3), p(4, 10), p(5, 3), p(5, 10), p(6, 3), p(6, 10),
	}
	mv := think(t, moves, time.Second)
	if mv != (game.Move{X: 2, Y: 3}) && mv != (game.Move{X: 7, Y: 3}) {
		t.Fatalf("move = %v, want a five on row 3", mv)
	}
}

func TestBuiltinBlocksOpponentFour(t *testing.T) {
	// White has a closed four on column 5 (blocked at y=1 by black); black must block y=6.
	moves := []game.Move{
		p(5, 1), p(5, 2), p(10, 10), p(5, 3), p(12, 12), p(5, 4), p(0, 14), p(5, 5),
	}
	mv := think(t, moves, time.Second)
	if mv != (game.Move{X: 5, Y: 6}) {
		t.Fatalf("move = %v, want block at 5,6", mv)
	}
}

func TestBuiltinBlocksOpenThree(t *testing.T) {
	// White has an open three on row 7; black must stop it from becoming an open four.
	moves := []game.Move{
		p(0, 0), p(6, 7), p(0, 14), p(7, 7), p(14, 0), p(8, 7),
	}
	mv := think(t, moves, 2*time.Second)
	if mv != (game.Move{X: 5, Y: 7}) && mv != (game.Move{X: 9, Y: 7}) &&
		mv != (game.Move{X: 4, Y: 7}) && mv != (game.Move{X: 10, Y: 7}) {
		t.Fatalf("move = %v, want a block next to the open three", mv)
	}
}

func TestBuiltinRespectsTimeLimit(t *testing.T) {
	moves := []game.Move{p(7, 7), p(7, 8), p(8, 7), p(6, 8), p(8, 8), p(8, 9)}
	start := time.Now()
	think(t, moves, 300*time.Millisecond)
	if el := time.Since(start); el > 300*time.Millisecond {
		t.Fatalf("think took %v, limit 300ms", el)
	}
}

// TestSelfPlayFinishes plays builtin against itself until the game ends.
func TestSelfPlayFinishes(t *testing.T) {
	g := game.New(game.Freestyle)
	m := &Managed{Factory: builtinFactory, Rule: game.Freestyle, Timeout: 100 * time.Millisecond}
	defer m.Close()
	for !g.Status().Over() {
		mv, err := m.NextMove(context.Background(), g.Moves())
		if err != nil {
			t.Fatal(err)
		}
		if err := g.Play(g.Turn(), mv); err != nil {
			t.Fatalf("engine played illegal %v: %v", mv, err)
		}
	}
}

func builtinFactory() (Engine, error) { return NewBuiltin(), nil }

// fake is a scripted engine for testing Managed.
type fake struct {
	onThink func(ctx context.Context) (game.Move, error)
	closed  *atomic.Int32
	moves   []game.Move
}

func (f *fake) Start(context.Context, int, game.Rule) error { return nil }
func (f *fake) SetPosition(_ context.Context, m []game.Move) error {
	f.moves = m
	return nil
}
func (f *fake) Think(ctx context.Context, _ time.Duration) (game.Move, error) {
	return f.onThink(ctx)
}
func (f *fake) Close() error {
	f.closed.Add(1)
	return nil
}

func TestManagedRestartsAfterCrash(t *testing.T) {
	var created, closed atomic.Int32
	factory := func() (Engine, error) {
		n := created.Add(1)
		return &fake{closed: &closed, onThink: func(context.Context) (game.Move, error) {
			if n == 1 {
				return game.Move{}, errors.New("engine crashed")
			}
			return game.Move{X: 1, Y: 1}, nil
		}}, nil
	}
	m := &Managed{Factory: factory, Rule: game.Freestyle, Timeout: time.Second, MaxRestarts: 2}
	mv, err := m.NextMove(context.Background(), []game.Move{p(7, 7)})
	if err != nil || mv != (game.Move{X: 1, Y: 1}) {
		t.Fatalf("got %v %v", mv, err)
	}
	if created.Load() != 2 || closed.Load() != 1 {
		t.Fatalf("created %d closed %d, want 2 and 1", created.Load(), closed.Load())
	}
}

func TestManagedRestartsAfterTimeout(t *testing.T) {
	var created, closed atomic.Int32
	factory := func() (Engine, error) {
		n := created.Add(1)
		return &fake{closed: &closed, onThink: func(ctx context.Context) (game.Move, error) {
			if n == 1 {
				<-ctx.Done() // hang until the deadline
				return game.Move{}, ctx.Err()
			}
			return game.Move{X: 2, Y: 2}, nil
		}}, nil
	}
	m := &Managed{Factory: factory, Rule: game.Freestyle, Timeout: 50 * time.Millisecond, MaxRestarts: 1}
	mv, err := m.NextMove(context.Background(), nil)
	if err != nil || mv != (game.Move{X: 2, Y: 2}) {
		t.Fatalf("got %v %v", mv, err)
	}
}

func TestManagedRestoresPositionAfterRestart(t *testing.T) {
	var closed atomic.Int32
	var engines []*fake
	factory := func() (Engine, error) {
		f := &fake{closed: &closed}
		first := len(engines) == 0
		f.onThink = func(context.Context) (game.Move, error) {
			if first {
				return game.Move{}, errors.New("crash")
			}
			return game.Move{X: 0, Y: 0}, nil
		}
		engines = append(engines, f)
		return f, nil
	}
	m := &Managed{Factory: factory, Rule: game.Freestyle, Timeout: time.Second, MaxRestarts: 1}
	pos := []game.Move{p(7, 7), p(7, 8), p(8, 8)}
	if _, err := m.NextMove(context.Background(), pos); err != nil {
		t.Fatal(err)
	}
	if got := engines[1].moves; len(got) != 3 || got[2] != pos[2] {
		t.Fatalf("restarted engine got position %v, want %v", got, pos)
	}
}

func TestManagedRejectsIllegalMove(t *testing.T) {
	var closed atomic.Int32
	bad := func() (Engine, error) {
		return &fake{closed: &closed, onThink: func(context.Context) (game.Move, error) {
			return game.Move{X: 7, Y: 7}, nil // occupied
		}}, nil
	}
	m := &Managed{Factory: bad, Rule: game.Freestyle, Timeout: time.Second, MaxRestarts: 1}
	if _, err := m.NextMove(context.Background(), []game.Move{p(7, 7)}); !errors.Is(err, ErrEngineFailed) {
		t.Fatalf("err = %v, want ErrEngineFailed", err)
	}
}

func TestManagedFallsBackToBuiltin(t *testing.T) {
	var closed atomic.Int32
	broken := func() (Engine, error) {
		return &fake{closed: &closed, onThink: func(context.Context) (game.Move, error) {
			return game.Move{}, errors.New("always crashes")
		}}, nil
	}
	m := &Managed{Factory: broken, Fallback: builtinFactory, Rule: game.Freestyle, Timeout: 200 * time.Millisecond, MaxRestarts: 1}
	mv, err := m.NextMove(context.Background(), []game.Move{p(7, 7)})
	if err != nil {
		t.Fatal(err)
	}
	if !legal([]game.Move{p(7, 7)}, mv) {
		t.Fatalf("fallback move %v is illegal", mv)
	}
}

func TestNewFactory(t *testing.T) {
	if _, err := NewFactory("builtin"); err != nil {
		t.Fatal(err)
	}
	if _, err := NewFactory("piskvork"); err == nil {
		t.Fatal("piskvork is not implemented yet and should be rejected")
	}
}

func p(x, y int) game.Move { return game.Move{X: x, Y: y} }
