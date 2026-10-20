package server

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/coder/websocket"
)

// client is one WebSocket connection. Its seat field is guarded by Hub.mu.
type client struct {
	hub       *Hub
	conn      *websocket.Conn
	out       chan []byte
	done      chan struct{}
	closeOnce sync.Once
	seat      *seat
}

const sendBuffer = 64

// send queues a message; a client too slow to drain its queue is dropped.
func (c *client) send(m Msg) {
	b, err := json.Marshal(m)
	if err != nil {
		return
	}
	select {
	case c.out <- b:
	default:
		c.close()
	}
}

func (c *client) sendError(code, message string) {
	c.send(Msg{Type: "error", Error: code, Message: message})
}

// close signals the writer to close the connection. Safe to call with Hub.mu held.
func (c *client) close() {
	c.closeOnce.Do(func() { close(c.done) })
}

func (c *client) writeLoop() {
	for {
		select {
		case b := <-c.out:
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			err := c.conn.Write(ctx, websocket.MessageText, b)
			cancel()
			if err != nil {
				c.close()
			}
		case <-c.done:
			_ = c.conn.Close(websocket.StatusNormalClosure, "")
			return
		}
	}
}

func (c *client) readLoop(ctx context.Context) {
	for {
		_, data, err := c.conn.Read(ctx)
		if err != nil {
			return
		}
		var m Msg
		if err := json.Unmarshal(data, &m); err != nil {
			c.sendError(ErrBadRequest, "invalid JSON")
			continue
		}
		c.hub.handle(c, m)
	}
}

// ServeWS upgrades the request and serves one client until it disconnects.
func (h *Hub) ServeWS(w http.ResponseWriter, r *http.Request, originPatterns []string) {
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{OriginPatterns: originPatterns})
	if err != nil {
		return
	}
	c := &client{hub: h, conn: conn, out: make(chan []byte, sendBuffer), done: make(chan struct{})}
	go c.writeLoop()
	c.readLoop(r.Context())
	h.disconnect(c)
	c.close()
}
