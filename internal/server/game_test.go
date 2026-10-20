package server

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/coder/websocket"

	"renju-game/internal/game"
)

// playToFive plays black on row by and white on row wy until black has five.
// Both clients must see every move, in order.
func playToFive(t *testing.T, black, white *testClient, by, wy int) {
	t.Helper()
	for x := 0; x < 5; x++ {
		black.play(x, by)
		black.expectMove(x, by)
		white.expectMove(x, by)
		if x == 4 {
			break
		}
		white.play(x, wy)
		white.expectMove(x, wy)
		black.expectMove(x, wy)
	}
}

func expectGameOver(t *testing.T, c *testClient, winner, reason string) {
	t.Helper()
	m := c.expect("game_over")
	if m.Winner != winner || m.Reason != reason {
		t.Fatalf("game_over = %+v, want winner %q reason %q", m, winner, reason)
	}
}

func TestFullGameUntilFive(t *testing.T) {
	s := newTestServer(t, Config{})
	black, white, _ := startPvP(t, s, "black")
	playToFive(t, black, white, 7, 9)
	expectGameOver(t, black, "black", ReasonFive)
	expectGameOver(t, white, "black", ReasonFive)
}

func TestMoveReachesOpponent(t *testing.T) {
	s := newTestServer(t, Config{})
	black, white, _ := startPvP(t, s, "black")
	black.play(3, 11)
	m := white.expectMove(3, 11)
	if m.Color != "black" || m.Turn != "white" {
		t.Fatalf("move = %+v, want color black, turn white", m)
	}
}

func TestIllegalMoveOnlyAffectsSender(t *testing.T) {
	s := newTestServer(t, Config{})
	black, white, _ := startPvP(t, s, "black")
	black.play(7, 7)
	black.expectMove(7, 7)
	white.expectMove(7, 7)

	white.play(7, 7)
	white.expectError(ErrOccupied)
	white.play(15, 0)
	white.expectError(ErrOutOfBounds)
	black.play(8, 8)
	black.expectError(ErrNotYourTurn)

	// The board is unchanged: white can still play 8,8, and black's next
	// message is that move, so it saw none of the errors.
	white.play(8, 8)
	black.expectMove(8, 8)
	white.expectMove(8, 8)
}

func TestSimultaneousMoves(t *testing.T) {
	s := newTestServer(t, Config{})
	black, white, _ := startPvP(t, s, "black")
	raw := func(c *testClient, x, y, ply int) {
		b, _ := json.Marshal(Msg{Type: "move", Move: &game.Move{X: x, Y: y}, Ply: &ply})
		_ = c.conn.Write(context.Background(), websocket.MessageText, b)
	}
	for round := 0; round < 6; round++ {
		turn, other := black, white
		if round%2 == 1 {
			turn, other = white, black
		}
		tx, ox := p(round, 0), p(round, 10)
		done := make(chan struct{}, 2)
		// Both have seen the same number of moves, so whichever arrives second is stale.
		go func() { raw(turn, tx.X, tx.Y, round); done <- struct{}{} }()
		go func() { raw(other, ox.X, ox.Y, round); done <- struct{}{} }()
		<-done
		<-done

		// The player whose turn it was sees only their own move.
		turn.expectMove(tx.X, tx.Y)
		// The other player sees that move and a not_your_turn error, in either order.
		var sawMove, sawErr bool
		for i := 0; i < 2; i++ {
			switch m := other.recv(); {
			case m.Type == "move" && *m.Move == tx:
				sawMove = true
			case m.Type == "error" && m.Error == ErrNotYourTurn:
				sawErr = true
			default:
				t.Fatalf("round %d: unexpected %+v", round, m)
			}
		}
		if !sawMove || !sawErr {
			t.Fatalf("round %d: move %v error %v", round, sawMove, sawErr)
		}
	}
}

func TestReconnectRestoresGame(t *testing.T) {
	s := newTestServer(t, Config{})
	host, guest := s.dial(t), s.dial(t)
	host.sendMsg(Msg{Type: "create_room", Color: "black"})
	code := host.expect("room_created").Code
	guest.sendMsg(Msg{Type: "join_room", Code: code})
	host.expect("game_start")
	token := guest.expect("game_start").Token

	host.play(7, 7)
	host.expectMove(7, 7)
	guest.expectMove(7, 7)
	guest.play(7, 8)
	host.expectMove(7, 8)
	guest.expectMove(7, 8)

	guest.conn.CloseNow()
	host.expect("opponent_disconnected")

	back := s.dial(t)
	back.sendMsg(Msg{Type: "reconnect", Token: token})
	st := back.expect("state")
	if st.Color != "white" || st.Status != "playing" || st.Turn != "black" ||
		len(st.Moves) != 2 || st.Moves[0] != p(7, 7) || st.Moves[1] != p(7, 8) {
		t.Fatalf("state = %+v", st)
	}
	host.expect("opponent_reconnected")

	host.play(8, 8)
	back.expectMove(8, 8)
}

func TestAbandonedAfterGrace(t *testing.T) {
	s := newTestServer(t, Config{ReconnectGrace: 100 * time.Millisecond})
	black, white, _ := startPvP(t, s, "black")
	white.conn.CloseNow()
	black.expect("opponent_disconnected")
	expectGameOver(t, black, "black", ReasonAbandoned)
	s.waitStats(t, Stats{})
}

func TestResign(t *testing.T) {
	s := newTestServer(t, Config{})
	black, white, _ := startPvP(t, s, "black")
	white.sendMsg(Msg{Type: "leave"})
	expectGameOver(t, black, "black", ReasonResign)
	expectGameOver(t, white, "black", ReasonResign)
	s.waitStats(t, Stats{})
}

func TestTenGamesAtOnce(t *testing.T) {
	s := newTestServer(t, Config{})
	t.Run("games", func(t *testing.T) {
		for i := 0; i < 10; i++ {
			t.Run(fmt.Sprint(i), func(t *testing.T) {
				t.Parallel()
				// Hosts alternate colors; each game uses its own rows, so a leaked
				// move from another game would fail expectMove.
				hostColor := []string{"black", "white"}[i%2]
				host, guest, _ := startPvP(t, s, hostColor)
				black, white := host, guest
				if hostColor == "white" {
					black, white = guest, host
				}
				playToFive(t, black, white, i, (i+7)%15)
				expectGameOver(t, black, "black", ReasonFive)
				expectGameOver(t, white, "black", ReasonFive)
			})
		}
	})
	s.waitStats(t, Stats{})
}

func TestGameEndCleansUp(t *testing.T) {
	s := newTestServer(t, Config{})
	host, guest := s.dial(t), s.dial(t)
	host.sendMsg(Msg{Type: "create_room", Color: "black"})
	code := host.expect("room_created").Code
	guest.sendMsg(Msg{Type: "join_room", Code: code})
	token := host.expect("game_start").Token
	guest.expect("game_start")
	s.waitStats(t, Stats{Rooms: 1, Sessions: 2, Active: 1})

	playToFive(t, host, guest, 7, 9)
	expectGameOver(t, host, "black", ReasonFive)
	expectGameOver(t, guest, "black", ReasonFive)
	s.waitStats(t, Stats{})

	// The code, the token and the seat are all gone.
	host.play(10, 10)
	host.expectError(ErrNotInGame)
	host.sendMsg(Msg{Type: "reconnect", Token: token})
	host.expectError(ErrSessionNotFound)
	third := s.dial(t)
	third.sendMsg(Msg{Type: "join_room", Code: code})
	third.expectError(ErrRoomNotFound)

	// Both players can start a new game on the same connection.
	host.sendMsg(Msg{Type: "create_room", Color: "white"})
	host.expect("room_created")
}
