package server

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"renju-game/internal/engine"
	"renju-game/internal/game"
)

type testServer struct {
	hub *Hub
	url string
}

func newTestServer(t *testing.T, cfg Config) *testServer {
	t.Helper()
	if cfg.Engine == nil {
		cfg.Engine = func() (engine.Engine, error) { return engine.NewBuiltin(), nil }
	}
	if cfg.EngineTimeout == 0 {
		cfg.EngineTimeout = 200 * time.Millisecond
	}
	hub := NewHub(cfg)
	srv := httptest.NewServer(NewHandler(hub, "", nil))
	t.Cleanup(srv.Close)
	return &testServer{hub: hub, url: "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws"}
}

// waitStats polls until the hub reaches want, since disconnects are handled asynchronously.
func (s *testServer) waitStats(t *testing.T, want Stats) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for {
		got := s.hub.Stats()
		if got == want {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("stats = %+v, want %+v", got, want)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

type testClient struct {
	t    *testing.T
	conn *websocket.Conn
	in   chan Msg
}

func (s *testServer) dial(t *testing.T) *testClient {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, s.url, nil)
	if err != nil {
		t.Fatal(err)
	}
	c := &testClient{t: t, conn: conn, in: make(chan Msg, 256)}
	go func() {
		defer close(c.in)
		for {
			_, data, err := conn.Read(context.Background())
			if err != nil {
				return
			}
			var m Msg
			if json.Unmarshal(data, &m) == nil {
				c.in <- m
			}
		}
	}()
	t.Cleanup(func() { c.conn.CloseNow() })
	return c
}

func (c *testClient) sendMsg(m Msg) {
	c.t.Helper()
	b, _ := json.Marshal(m)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := c.conn.Write(ctx, websocket.MessageText, b); err != nil {
		c.t.Fatal(err)
	}
}

func (c *testClient) recv() Msg {
	c.t.Helper()
	select {
	case m, ok := <-c.in:
		if !ok {
			c.t.Fatal("connection closed")
		}
		return m
	case <-time.After(5 * time.Second):
		c.t.Fatal("timed out waiting for a message")
	}
	return Msg{}
}

// expect reads the next message and checks its type.
func (c *testClient) expect(typ string) Msg {
	c.t.Helper()
	m := c.recv()
	if m.Type != typ {
		c.t.Fatalf("got %+v, want type %q", m, typ)
	}
	return m
}

func (c *testClient) expectError(code string) {
	c.t.Helper()
	if m := c.expect("error"); m.Error != code {
		c.t.Fatalf("error = %q (%s), want %q", m.Error, m.Message, code)
	}
}

// expectSilence checks that nothing arrives for d.
func (c *testClient) expectSilence(d time.Duration) {
	c.t.Helper()
	select {
	case m := <-c.in:
		c.t.Fatalf("unexpected message %+v", m)
	case <-time.After(d):
	}
}

func (c *testClient) play(x, y int) {
	c.t.Helper()
	c.sendMsg(Msg{Type: "move", Move: &game.Move{X: x, Y: y}})
}

// startPvP opens a room with the host as hostColor and has guest join it.
func startPvP(t *testing.T, s *testServer, hostColor string) (host, guest *testClient, code string) {
	t.Helper()
	host, guest = s.dial(t), s.dial(t)
	host.sendMsg(Msg{Type: "create_room", Color: hostColor})
	code = host.expect("room_created").Code
	guest.sendMsg(Msg{Type: "join_room", Code: code})
	host.expect("game_start")
	guest.expect("game_start")
	return host, guest, code
}

// expectMove reads a move broadcast and checks its coordinate.
func (c *testClient) expectMove(x, y int) Msg {
	c.t.Helper()
	m := c.expect("move")
	if m.Move == nil || m.Move.X != x || m.Move.Y != y {
		c.t.Fatalf("move = %+v, want %d,%d", m.Move, x, y)
	}
	return m
}

func p(x, y int) game.Move { return game.Move{X: x, Y: y} }
