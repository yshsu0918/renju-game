// Package engine defines the GomokuEngine interface the server talks to and
// the implementations behind it. The server never depends on a concrete engine.
package engine

import (
	"context"
	"errors"
	"fmt"
	"time"

	"renju-game/internal/game"
)

// Engine is the GomokuEngine interface from the spec. Implementations need not be
// safe for concurrent use; each game owns its own instance.
type Engine interface {
	// Start prepares a new game on a size×size board.
	Start(ctx context.Context, size int, rule game.Rule) error
	// SetPosition replaces the position with the given moves, black first.
	SetPosition(ctx context.Context, moves []game.Move) error
	// Think returns the engine's move for the side to move.
	Think(ctx context.Context, timeLimit time.Duration) (game.Move, error)
	// Close releases the engine (the spec's dispose).
	Close() error
}

// Factory creates a fresh engine instance.
type Factory func() (Engine, error)

// NewFactory returns the factory for a configured engine kind.
func NewFactory(kind string) (Factory, error) {
	switch kind {
	case "", "builtin":
		return func() (Engine, error) { return NewBuiltin(), nil }, nil
	case "piskvork", "http":
		return nil, fmt.Errorf("engine %q is planned but not implemented yet", kind)
	}
	return nil, fmt.Errorf("unknown engine %q", kind)
}

// Managed wraps one game's engine: it enforces the per-move timeout, checks that
// moves are legal, and on a timeout, crash or illegal move restarts the engine and
// restores the position. After MaxRestarts failed restarts it switches to Fallback.
type Managed struct {
	Factory     Factory
	Fallback    Factory // may be nil
	Rule        game.Rule
	Timeout     time.Duration
	MaxRestarts int

	eng         Engine
	usingBackup bool
}

// ErrEngineFailed is returned when neither the engine nor its fallback produced a legal move.
var ErrEngineFailed = errors.New("engine failed to produce a move")

// NextMove asks the engine for a move in the position given by moves.
func (m *Managed) NextMove(ctx context.Context, moves []game.Move) (game.Move, error) {
	var lastErr error
	for attempt := 0; attempt <= m.MaxRestarts; attempt++ {
		mv, err := m.try(ctx, moves)
		if err == nil {
			return mv, nil
		}
		if ctx.Err() != nil {
			return game.Move{}, ctx.Err()
		}
		lastErr = err
		m.reset()
	}
	if m.Fallback != nil && !m.usingBackup {
		m.usingBackup = true
		if mv, err := m.try(ctx, moves); err == nil {
			return mv, nil
		} else {
			lastErr = err
		}
		m.reset()
	}
	return game.Move{}, fmt.Errorf("%w: %v", ErrEngineFailed, lastErr)
}

func (m *Managed) try(ctx context.Context, moves []game.Move) (game.Move, error) {
	if m.eng == nil {
		f := m.Factory
		if m.usingBackup {
			f = m.Fallback
		}
		e, err := f()
		if err != nil {
			return game.Move{}, err
		}
		m.eng = e
		if err := e.Start(ctx, game.Size, m.Rule); err != nil {
			return game.Move{}, fmt.Errorf("start: %w", err)
		}
	}
	// Hard deadline a little past the think limit, so a hung engine cannot stall the game.
	tctx, cancel := context.WithTimeout(ctx, m.Timeout+m.Timeout/2)
	defer cancel()
	type result struct {
		mv  game.Move
		err error
	}
	done := make(chan result, 1)
	eng := m.eng
	go func() {
		if err := eng.SetPosition(tctx, moves); err != nil {
			done <- result{err: fmt.Errorf("set position: %w", err)}
			return
		}
		mv, err := eng.Think(tctx, m.Timeout)
		done <- result{mv, err}
	}()
	select {
	case r := <-done:
		if r.err != nil {
			return game.Move{}, r.err
		}
		if !legal(moves, r.mv) {
			return game.Move{}, fmt.Errorf("illegal move %v", r.mv)
		}
		return r.mv, nil
	case <-tctx.Done():
		return game.Move{}, fmt.Errorf("think timed out after %v", m.Timeout)
	}
}

func (m *Managed) reset() {
	if m.eng != nil {
		_ = m.eng.Close()
		m.eng = nil
	}
}

// Close releases the engine.
func (m *Managed) Close() error {
	m.reset()
	return nil
}

func legal(moves []game.Move, mv game.Move) bool {
	if !mv.InBounds() {
		return false
	}
	for _, p := range moves {
		if p == mv {
			return false
		}
	}
	return true
}
