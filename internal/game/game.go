// Package game holds the board and the rules. The server is the only place
// moves are validated and results decided; clients only display them.
package game

import (
	"errors"
	"fmt"
)

// Size is the board size. The spec fixes the board at 15×15.
const Size = 15

// Color is a stone color. Black always moves first.
type Color int

const (
	Empty Color = iota
	Black
	White
)

func (c Color) String() string {
	switch c {
	case Black:
		return "black"
	case White:
		return "white"
	}
	return ""
}

// Opponent returns the other color.
func (c Color) Opponent() Color {
	if c == Black {
		return White
	}
	return Black
}

// ParseColor parses "black" / "white".
func ParseColor(s string) (Color, error) {
	switch s {
	case "black":
		return Black, nil
	case "white":
		return White, nil
	}
	return Empty, fmt.Errorf("unknown color %q", s)
}

// Rule selects the rule set.
type Rule string

const (
	Freestyle Rule = "freestyle"
	// Renju (forbidden moves for black) is planned for a later version.
	Renju Rule = "renju"
)

// ParseRule accepts the rules this version supports. An empty string means freestyle.
func ParseRule(s string) (Rule, error) {
	switch Rule(s) {
	case "", Freestyle:
		return Freestyle, nil
	case Renju:
		return "", errors.New("renju rule is not supported yet")
	}
	return "", fmt.Errorf("unknown rule %q", s)
}

// Move is a 0-based x,y coordinate, matching the Piskvork protocol.
type Move struct {
	X int `json:"x"`
	Y int `json:"y"`
}

// InBounds reports whether the move is on the board.
func (m Move) InBounds() bool {
	return m.X >= 0 && m.X < Size && m.Y >= 0 && m.Y < Size
}

// Status is the state of a game.
type Status int

const (
	Playing Status = iota
	BlackWon
	WhiteWon
	Draw
)

// Over reports whether the game has ended.
func (s Status) Over() bool { return s != Playing }

// Winner returns the winning color, or Empty for a draw or an unfinished game.
func (s Status) Winner() Color {
	switch s {
	case BlackWon:
		return Black
	case WhiteWon:
		return White
	}
	return Empty
}

func wonBy(c Color) Status {
	if c == Black {
		return BlackWon
	}
	return WhiteWon
}

var (
	ErrOutOfBounds = errors.New("move is outside the board")
	ErrOccupied    = errors.New("intersection is already occupied")
	ErrNotYourTurn = errors.New("not your turn")
	ErrGameOver    = errors.New("game is already over")
)

// Game is one game of Gomoku. It is not safe for concurrent use; callers lock around it.
type Game struct {
	Rule   Rule
	board  [Size][Size]Color
	moves  []Move
	turn   Color
	status Status
}

// New starts an empty game with black to move.
func New(rule Rule) *Game {
	return &Game{Rule: rule, turn: Black}
}

// Turn returns the color to move.
func (g *Game) Turn() Color { return g.turn }

// Status returns the current status.
func (g *Game) Status() Status { return g.status }

// At returns the stone at x,y.
func (g *Game) At(x, y int) Color { return g.board[x][y] }

// MoveCount returns how many moves have been played.
func (g *Game) MoveCount() int { return len(g.moves) }

// Moves returns a copy of the moves played so far, in order.
func (g *Game) Moves() []Move {
	return append([]Move(nil), g.moves...)
}

// Play places a stone of color c at m. On error the board is unchanged.
func (g *Game) Play(c Color, m Move) error {
	if g.status.Over() {
		return ErrGameOver
	}
	if c != g.turn {
		return ErrNotYourTurn
	}
	if !m.InBounds() {
		return ErrOutOfBounds
	}
	if g.board[m.X][m.Y] != Empty {
		return ErrOccupied
	}
	g.board[m.X][m.Y] = c
	g.moves = append(g.moves, m)
	switch {
	case IsFive(&g.board, m, c):
		g.status = wonBy(c)
	case len(g.moves) == Size*Size:
		g.status = Draw
	default:
		g.turn = c.Opponent()
	}
	return nil
}

// Directions are the four line directions: horizontal, vertical and the two diagonals.
var Directions = [4][2]int{{1, 0}, {0, 1}, {1, 1}, {1, -1}}

// IsFive reports whether the stone at m forms five or more in a row for color c.
// Under freestyle an overline (six or more) also wins.
func IsFive(b *[Size][Size]Color, m Move, c Color) bool {
	for _, d := range Directions {
		if 1+count(b, m, c, d[0], d[1])+count(b, m, c, -d[0], -d[1]) >= 5 {
			return true
		}
	}
	return false
}

func count(b *[Size][Size]Color, m Move, c Color, dx, dy int) int {
	n := 0
	for x, y := m.X+dx, m.Y+dy; x >= 0 && x < Size && y >= 0 && y < Size && b[x][y] == c; x, y = x+dx, y+dy {
		n++
	}
	return n
}
