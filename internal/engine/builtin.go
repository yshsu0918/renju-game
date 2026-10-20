package engine

import (
	"context"
	"errors"
	"sort"
	"time"

	"renju-game/internal/game"
)

// Builtin is the in-process Minimax + Alpha-Beta engine, the default and the fallback.
type Builtin struct {
	board [game.Size][game.Size]game.Color
	moves []game.Move

	// MaxDepth caps the iterative deepening (plies). Zero means 6.
	MaxDepth int
	// Width is how many candidate moves are searched at each node. Zero means 10.
	Width int
}

// NewBuiltin returns a builtin engine with default settings.
func NewBuiltin() *Builtin { return &Builtin{} }

func (b *Builtin) Start(_ context.Context, size int, rule game.Rule) error {
	if size != game.Size {
		return errors.New("builtin engine only supports a 15×15 board")
	}
	if rule != game.Freestyle {
		return errors.New("builtin engine only supports freestyle")
	}
	b.board = [game.Size][game.Size]game.Color{}
	b.moves = nil
	return nil
}

func (b *Builtin) SetPosition(_ context.Context, moves []game.Move) error {
	b.board = [game.Size][game.Size]game.Color{}
	c := game.Black
	for _, m := range moves {
		if !m.InBounds() || b.board[m.X][m.Y] != game.Empty {
			return errors.New("invalid position")
		}
		b.board[m.X][m.Y] = c
		c = c.Opponent()
	}
	b.moves = append([]game.Move(nil), moves...)
	return nil
}

func (b *Builtin) Close() error { return nil }

const (
	winScore = 100_000_000
	center   = game.Size / 2
)

// Think picks a move: win now if possible, otherwise block the opponent's five,
// otherwise search with iterative deepening until half the time limit is used.
func (b *Builtin) Think(ctx context.Context, timeLimit time.Duration) (game.Move, error) {
	if len(b.moves) == 0 {
		return game.Move{X: center, Y: center}, nil
	}
	if len(b.moves) == game.Size*game.Size {
		return game.Move{}, errors.New("board is full")
	}
	me := game.Black
	if len(b.moves)%2 == 1 {
		me = game.White
	}
	s := &search{
		board:    b.board,
		width:    b.Width,
		deadline: time.Now().Add(timeLimit / 2),
		ctx:      ctx,
	}
	if s.width == 0 {
		s.width = 10
	}
	maxDepth := b.MaxDepth
	if maxDepth == 0 {
		maxDepth = 6
	}

	cands := s.candidates(me)
	if mv, ok := s.findFive(me, cands); ok {
		return mv, nil
	}
	if mv, ok := s.findFive(me.Opponent(), cands); ok {
		return mv, nil
	}

	best := cands[0]
	for depth := 2; depth <= maxDepth; depth += 2 {
		mv, ok := s.root(me, depth)
		if !ok {
			break // out of time: keep the last completed depth
		}
		best = mv
	}
	return best, ctx.Err()
}

type search struct {
	board    [game.Size][game.Size]game.Color
	width    int
	deadline time.Time
	ctx      context.Context
	nodes    int
	stopped  bool
}

func (s *search) timeUp() bool {
	if s.stopped {
		return true
	}
	s.nodes++
	if s.nodes&15 == 0 && (time.Now().After(s.deadline) || s.ctx.Err() != nil) {
		s.stopped = true
	}
	return s.stopped
}

// root searches all candidates to depth and returns the best move, or false if time ran out.
func (s *search) root(me game.Color, depth int) (game.Move, bool) {
	cands := s.candidates(me)
	if len(cands) > s.width {
		cands = cands[:s.width]
	}
	alpha, beta := -winScore*2, winScore*2
	best := cands[0]
	for _, mv := range cands {
		s.board[mv.X][mv.Y] = me
		var v int
		if game.IsFive(&s.board, mv, me) {
			v = winScore
		} else {
			v = -s.negamax(me.Opponent(), depth-1, -beta, -alpha)
		}
		s.board[mv.X][mv.Y] = game.Empty
		if s.stopped {
			return game.Move{}, false
		}
		if v > alpha {
			alpha, best = v, mv
		}
	}
	return best, true
}

func (s *search) negamax(c game.Color, depth, alpha, beta int) int {
	if s.timeUp() {
		return 0
	}
	if depth == 0 {
		return s.evaluate(c)
	}
	cands := s.candidates(c)
	if len(cands) == 0 {
		return 0 // full board: draw
	}
	if len(cands) > s.width {
		cands = cands[:s.width]
	}
	best := -winScore * 2
	for _, mv := range cands {
		s.board[mv.X][mv.Y] = c
		var v int
		if game.IsFive(&s.board, mv, c) {
			v = winScore + depth // prefer faster wins
		} else {
			v = -s.negamax(c.Opponent(), depth-1, -beta, -alpha)
		}
		s.board[mv.X][mv.Y] = game.Empty
		if s.stopped {
			return 0
		}
		if v > best {
			best = v
		}
		if v > alpha {
			alpha = v
		}
		if alpha >= beta {
			break
		}
	}
	return best
}

func (s *search) findFive(c game.Color, cands []game.Move) (game.Move, bool) {
	for _, mv := range cands {
		s.board[mv.X][mv.Y] = c
		five := game.IsFive(&s.board, mv, c)
		s.board[mv.X][mv.Y] = game.Empty
		if five {
			return mv, true
		}
	}
	return game.Move{}, false
}

// windowWeight scores a five-cell window holding n stones of one color and none of the other.
var windowWeight = [6]int{0, 1, 12, 150, 2_000, winScore}

// evaluate scores the board from c's point of view by summing every five-cell
// window on every line. The side to move gets a bonus, since it acts first.
func (s *search) evaluate(c game.Color) int {
	var score [3]int
	for x := 0; x < game.Size; x++ {
		for y := 0; y < game.Size; y++ {
			for _, d := range game.Directions {
				ex, ey := x+4*d[0], y+4*d[1]
				if ex < 0 || ex >= game.Size || ey < 0 || ey >= game.Size {
					continue
				}
				var cnt [3]int
				for i := 0; i < 5; i++ {
					cnt[s.board[x+i*d[0]][y+i*d[1]]]++
				}
				switch {
				case cnt[game.Black] > 0 && cnt[game.White] == 0:
					score[game.Black] += windowWeight[cnt[game.Black]]
				case cnt[game.White] > 0 && cnt[game.Black] == 0:
					score[game.White] += windowWeight[cnt[game.White]]
				}
			}
		}
	}
	mine, theirs := score[c], score[c.Opponent()]
	return mine + mine/4 - theirs
}

// candidates lists empty points within two of a stone, best-first by a quick
// attack-plus-defense score for color c.
func (s *search) candidates(c game.Color) []game.Move {
	type scored struct {
		mv    game.Move
		score int
	}
	var list []scored
	any := false
	for x := 0; x < game.Size; x++ {
		for y := 0; y < game.Size; y++ {
			if s.board[x][y] != game.Empty {
				any = true
				continue
			}
			if !s.nearStone(x, y) {
				continue
			}
			mv := game.Move{X: x, Y: y}
			list = append(list, scored{mv, s.pointScore(mv, c)*5/4 + s.pointScore(mv, c.Opponent())})
		}
	}
	if !any {
		return []game.Move{{X: center, Y: center}}
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].score != list[j].score {
			return list[i].score > list[j].score
		}
		// Deterministic tie-break toward the center.
		return dist(list[i].mv) < dist(list[j].mv)
	})
	out := make([]game.Move, len(list))
	for i, e := range list {
		out[i] = e.mv
	}
	return out
}

func dist(m game.Move) int {
	return abs(m.X-center) + abs(m.Y-center)
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

func (s *search) nearStone(x, y int) bool {
	for dx := -2; dx <= 2; dx++ {
		for dy := -2; dy <= 2; dy++ {
			nx, ny := x+dx, y+dy
			if nx >= 0 && nx < game.Size && ny >= 0 && ny < game.Size && s.board[nx][ny] != game.Empty {
				return true
			}
		}
	}
	return false
}

// pointScore sums the window weights c would have through mv if it played there.
func (s *search) pointScore(mv game.Move, c game.Color) int {
	total := 0
	for _, d := range game.Directions {
		for off := -4; off <= 0; off++ {
			sx, sy := mv.X+off*d[0], mv.Y+off*d[1]
			ex, ey := sx+4*d[0], sy+4*d[1]
			if sx < 0 || sx >= game.Size || sy < 0 || sy >= game.Size ||
				ex < 0 || ex >= game.Size || ey < 0 || ey >= game.Size {
				continue
			}
			n, blocked := 1, false
			for i := 0; i < 5; i++ {
				x, y := sx+i*d[0], sy+i*d[1]
				if x == mv.X && y == mv.Y {
					continue
				}
				switch s.board[x][y] {
				case c:
					n++
				case c.Opponent():
					blocked = true
				}
			}
			if !blocked {
				total += windowWeight[n]
			}
		}
	}
	return total
}
