package hub

import (
	"context"
	"time"

	"github.com/coder/websocket"
)

// Client is one WebSocket connection inside a room. Two goroutines serve
// it: readPump (socket -> room.broadcast) and writePump (send -> socket).
// They are separate because both Read and Write block, and a client must
// be able to receive while it is sending.
type Client struct {
	conn *websocket.Conn
	send chan []byte
	room *room
}

// writePump drains c.send onto the socket. It exits when c.send is closed
// (by room.run on unregister) or when a write fails. Ranging over a channel
// ends cleanly when the channel is closed, which is the idiomatic way to
// stop a worker goroutine.
func (c *Client) writePump(ctx context.Context) {
	// Whatever ends this loop, close the socket so readPump's Read returns
	// an error and unwinds too. Without this, a client whose send channel
	// was closed by the room would leave readPump blocked forever.
	defer c.conn.Close(websocket.StatusNormalClosure, "")

	for data := range c.send {
		wctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		err := c.conn.Write(wctx, websocket.MessageText, data)
		cancel()
		if err != nil {
			return
		}
	}
}

func (c *Client) readPump(ctx context.Context) {
	c.conn.SetReadLimit(1 << 20)
	for {
		_, data, err := c.conn.Read(ctx)
		if err != nil {
			return
		}
		c.room.broadcast <- message{from: c, data: data}
	}
}
