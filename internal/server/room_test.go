package server

import (
	"strings"
	"testing"
)

func TestCreateRoomReturnsCode(t *testing.T) {
	s := newTestServer(t, Config{})
	c := s.dial(t)
	c.sendMsg(Msg{Type: "create_room", Color: "black"})
	m := c.expect("room_created")
	if len(m.Code) != 6 || m.Token == "" || m.Color != "black" || m.Rule != "freestyle" {
		t.Fatalf("room_created = %+v", m)
	}
	s.waitStats(t, Stats{Rooms: 1, Sessions: 1, Active: 1})
}

func TestRoomCodesAreUnique(t *testing.T) {
	s := newTestServer(t, Config{})
	seen := map[string]bool{}
	for i := 0; i < 50; i++ {
		c := s.dial(t)
		c.sendMsg(Msg{Type: "create_room", Color: "black"})
		code := c.expect("room_created").Code
		if seen[code] {
			t.Fatalf("duplicate code %s", code)
		}
		seen[code] = true
	}
	s.waitStats(t, Stats{Rooms: 50, Sessions: 50, Active: 50})
}

func TestJoinWithCorrectCode(t *testing.T) {
	s := newTestServer(t, Config{})
	host, guest := s.dial(t), s.dial(t)
	host.sendMsg(Msg{Type: "create_room", Color: "black"})
	code := host.expect("room_created").Code
	// Codes are matched case-insensitively.
	guest.sendMsg(Msg{Type: "join_room", Code: " " + strings.ToLower(code) + " "})
	for _, c := range []*testClient{host, guest} {
		m := c.expect("game_start")
		if m.Mode != "pvp" || m.Code != code || m.Size != 15 || m.Turn != "black" || m.Token == "" {
			t.Fatalf("game_start = %+v", m)
		}
	}
}

func TestJoinUnknownCode(t *testing.T) {
	s := newTestServer(t, Config{})
	c := s.dial(t)
	c.sendMsg(Msg{Type: "join_room", Code: "ZZZZZZ"})
	c.expectError(ErrRoomNotFound)
}

func TestThirdPlayerCannotJoin(t *testing.T) {
	s := newTestServer(t, Config{})
	_, _, code := startPvP(t, s, "black")
	third := s.dial(t)
	third.sendMsg(Msg{Type: "join_room", Code: code})
	third.expectError(ErrRoomFull)
}

func TestHostChoosesColor(t *testing.T) {
	for _, hostColor := range []string{"black", "white"} {
		t.Run(hostColor, func(t *testing.T) {
			s := newTestServer(t, Config{})
			host, guest := s.dial(t), s.dial(t)
			host.sendMsg(Msg{Type: "create_room", Color: hostColor})
			code := host.expect("room_created").Code
			guest.sendMsg(Msg{Type: "join_room", Code: code})
			h, g := host.expect("game_start"), guest.expect("game_start")
			if h.Color != hostColor || g.Color == hostColor || g.Color == "" {
				t.Fatalf("host %q guest %q, host chose %q", h.Color, g.Color, hostColor)
			}
		})
	}
}

func TestRoomOptionsReachBothPlayers(t *testing.T) {
	s := newTestServer(t, Config{})
	host, guest := s.dial(t), s.dial(t)
	host.sendMsg(Msg{Type: "create_room", Color: "black", Rule: "freestyle"})
	code := host.expect("room_created").Code
	guest.sendMsg(Msg{Type: "join_room", Code: code})
	h, g := host.expect("game_start"), guest.expect("game_start")
	if h.Rule != "freestyle" || g.Rule != h.Rule {
		t.Fatalf("rules differ: host %q guest %q", h.Rule, g.Rule)
	}
}

func TestRenjuNotYetOffered(t *testing.T) {
	s := newTestServer(t, Config{})
	c := s.dial(t)
	c.sendMsg(Msg{Type: "create_room", Color: "black", Rule: "renju"})
	c.expectError(ErrUnsupportedRule)
}

func TestHostLeavesBeforeJoin(t *testing.T) {
	for _, how := range []string{"disconnect", "leave"} {
		t.Run(how, func(t *testing.T) {
			s := newTestServer(t, Config{})
			host := s.dial(t)
			host.sendMsg(Msg{Type: "create_room", Color: "black"})
			code := host.expect("room_created").Code
			if how == "leave" {
				host.sendMsg(Msg{Type: "leave"})
			} else {
				host.conn.CloseNow()
			}
			s.waitStats(t, Stats{})
			guest := s.dial(t)
			guest.sendMsg(Msg{Type: "join_room", Code: code})
			guest.expectError(ErrRoomNotFound)
		})
	}
}

func TestBadMessages(t *testing.T) {
	s := newTestServer(t, Config{})
	c := s.dial(t)
	c.sendMsg(Msg{Type: "dance"})
	c.expectError(ErrUnknownType)
	c.sendMsg(Msg{Type: "create_room", Color: "purple"})
	c.expectError(ErrBadRequest)
	c.play(7, 7)
	c.expectError(ErrNotInGame)
}
