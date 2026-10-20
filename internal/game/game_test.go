package game

import (
	"errors"
	"testing"
)

// playLine plays the given black moves, with white answering far away each time.
func playLine(t *testing.T, g *Game, black []Move) {
	t.Helper()
	whiteSpots := []Move{{14, 14}, {14, 12}, {12, 14}, {14, 10}, {10, 14}, {12, 10}}
	for i, m := range black {
		if err := g.Play(Black, m); err != nil {
			t.Fatalf("black %v: %v", m, err)
		}
		if g.Status().Over() || i == len(black)-1 {
			return
		}
		if err := g.Play(White, whiteSpots[i]); err != nil {
			t.Fatalf("white %v: %v", whiteSpots[i], err)
		}
	}
}

func TestFiveInEveryDirection(t *testing.T) {
	cases := map[string][]Move{
		"horizontal":    {{3, 7}, {4, 7}, {5, 7}, {6, 7}, {7, 7}},
		"vertical":      {{7, 3}, {7, 4}, {7, 5}, {7, 6}, {7, 7}},
		"diagonal":      {{3, 3}, {4, 4}, {5, 5}, {6, 6}, {7, 7}},
		"anti-diagonal": {{3, 9}, {4, 8}, {5, 7}, {6, 6}, {7, 5}},
		// The winning stone lands in the middle of the line.
		"gap filled": {{3, 7}, {4, 7}, {6, 7}, {7, 7}, {5, 7}},
	}
	for name, moves := range cases {
		t.Run(name, func(t *testing.T) {
			g := New(Freestyle)
			playLine(t, g, moves)
			if g.Status() != BlackWon {
				t.Fatalf("status = %v, want BlackWon", g.Status())
			}
		})
	}
}

func TestWhiteCanWin(t *testing.T) {
	g := New(Freestyle)
	blackSpots := []Move{{0, 0}, {0, 2}, {0, 4}, {0, 6}, {0, 8}}
	for i := 0; i < 5; i++ {
		must(t, g.Play(Black, blackSpots[i]))
		must(t, g.Play(White, Move{5, 3 + i}))
	}
	if g.Status() != WhiteWon {
		t.Fatalf("status = %v, want WhiteWon", g.Status())
	}
}

func TestOverlineWinsInFreestyle(t *testing.T) {
	g := New(Freestyle)
	// Two groups of black stones with a gap; filling it makes six.
	playLine(t, g, []Move{{1, 7}, {2, 7}, {3, 7}, {5, 7}, {6, 7}, {4, 7}})
	if g.Status() != BlackWon {
		t.Fatalf("status = %v, want BlackWon", g.Status())
	}
}

func TestFourIsNotAWin(t *testing.T) {
	g := New(Freestyle)
	playLine(t, g, []Move{{3, 7}, {4, 7}, {5, 7}, {6, 7}})
	if g.Status() != Playing {
		t.Fatalf("status = %v, want Playing", g.Status())
	}
}

func TestFiveOnEdgesAndCorners(t *testing.T) {
	cases := map[string][]Move{
		"top edge":           {{0, 0}, {1, 0}, {2, 0}, {3, 0}, {4, 0}},
		"bottom-right edge":  {{14, 10}, {14, 11}, {14, 12}, {14, 13}, {14, 9}},
		"corner diagonal":    {{0, 0}, {1, 1}, {2, 2}, {3, 3}, {4, 4}},
		"corner anti-diag":   {{0, 14}, {1, 13}, {2, 12}, {3, 11}, {4, 10}},
		"far corner to edge": {{10, 10}, {11, 11}, {12, 12}, {13, 13}, {14, 14}},
	}
	for name, moves := range cases {
		t.Run(name, func(t *testing.T) {
			g := New(Freestyle)
			// White answers in the middle so it never collides with the edge lines.
			for i, m := range moves {
				must(t, g.Play(Black, m))
				if g.Status().Over() {
					break
				}
				must(t, g.Play(White, Move{7, 3 + i}))
			}
			if g.Status() != BlackWon {
				t.Fatalf("status = %v, want BlackWon", g.Status())
			}
		})
	}
}

func TestBrokenLineAcrossEdgeIsNotFive(t *testing.T) {
	// Stones at x=12..14 on row 0 and x=0..1 on row 1 must not wrap around into five.
	g := New(Freestyle)
	playLine(t, g, []Move{{12, 0}, {13, 0}, {14, 0}, {0, 1}, {1, 1}})
	if g.Status() != Playing {
		t.Fatalf("status = %v, want Playing", g.Status())
	}
}

func TestRejectsOccupied(t *testing.T) {
	g := New(Freestyle)
	must(t, g.Play(Black, Move{7, 7}))
	before := g.Moves()
	if err := g.Play(White, Move{7, 7}); !errors.Is(err, ErrOccupied) {
		t.Fatalf("err = %v, want ErrOccupied", err)
	}
	if g.At(7, 7) != Black || len(g.Moves()) != len(before) || g.Turn() != White {
		t.Fatal("board changed after rejected move")
	}
}

func TestRejectsOutOfBounds(t *testing.T) {
	for _, m := range []Move{{-1, 0}, {0, -1}, {15, 0}, {0, 15}, {-5, 20}} {
		g := New(Freestyle)
		if err := g.Play(Black, m); !errors.Is(err, ErrOutOfBounds) {
			t.Errorf("%v: err = %v, want ErrOutOfBounds", m, err)
		}
		if len(g.Moves()) != 0 {
			t.Errorf("%v: board changed", m)
		}
	}
}

func TestRejectsWrongTurn(t *testing.T) {
	g := New(Freestyle)
	if err := g.Play(White, Move{7, 7}); !errors.Is(err, ErrNotYourTurn) {
		t.Fatalf("white first: err = %v, want ErrNotYourTurn", err)
	}
	must(t, g.Play(Black, Move{7, 7}))
	if err := g.Play(Black, Move{7, 8}); !errors.Is(err, ErrNotYourTurn) {
		t.Fatalf("black twice: err = %v, want ErrNotYourTurn", err)
	}
}

func TestRejectsMoveAfterGameOver(t *testing.T) {
	g := New(Freestyle)
	playLine(t, g, []Move{{3, 7}, {4, 7}, {5, 7}, {6, 7}, {7, 7}})
	if err := g.Play(White, Move{0, 0}); !errors.Is(err, ErrGameOver) {
		t.Fatalf("err = %v, want ErrGameOver", err)
	}
}

func TestFullBoardIsDraw(t *testing.T) {
	// Color by (x/2 + y) parity: x pairs make horizontal runs of at most 2,
	// vertical lines alternate, and both diagonals stay short.
	colorAt := func(x, y int) Color {
		if (x/2+y)%2 == 0 {
			return Black
		}
		return White
	}
	var blacks, whites []Move
	for x := 0; x < Size; x++ {
		for y := 0; y < Size; y++ {
			if colorAt(x, y) == Black {
				blacks = append(blacks, Move{x, y})
			} else {
				whites = append(whites, Move{x, y})
			}
		}
	}
	// Black has one more stone than white on a 225-point board only if the split is 113/112.
	if len(blacks) != 113 || len(whites) != 112 {
		t.Fatalf("pattern split %d/%d, want 113/112", len(blacks), len(whites))
	}
	g := New(Freestyle)
	for i := range blacks {
		must(t, g.Play(Black, blacks[i]))
		if i < len(whites) {
			must(t, g.Play(White, whites[i]))
		}
	}
	if g.Status() != Draw {
		t.Fatalf("status = %v, want Draw", g.Status())
	}
	if err := g.Play(White, Move{0, 0}); !errors.Is(err, ErrGameOver) {
		t.Fatalf("move after draw: err = %v, want ErrGameOver", err)
	}
}

func TestBlackMovesFirst(t *testing.T) {
	g := New(Freestyle)
	if g.Turn() != Black {
		t.Fatalf("first turn = %v, want black", g.Turn())
	}
}

func TestParseRule(t *testing.T) {
	if r, err := ParseRule(""); err != nil || r != Freestyle {
		t.Fatalf("empty: %v %v", r, err)
	}
	if _, err := ParseRule("renju"); err == nil {
		t.Fatal("renju should be rejected until it is implemented")
	}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
