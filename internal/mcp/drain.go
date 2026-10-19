package mcp

import (
	"context"
	"sync"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// drainTransport connects through another transport and wraps the connection in a drainConn.
type drainTransport struct {
	// inner is the transport that carries the messages.
	inner mcpsdk.Transport
}

// Connect connects the inner transport and wraps its connection so an end of input waits
// for the replies already owed.
func (t *drainTransport) Connect(ctx context.Context) (mcpsdk.Connection, error) {
	conn, err := t.inner.Connect(ctx)
	if err != nil {
		return nil, err
	}
	return newDrainConn(conn), nil
}

// drainConn holds back the end of input until every call already read has been answered.
// The SDK treats a failed read as shutdown and refuses any reply still in flight, so without
// it a client that writes its requests and then closes the pipe gets nothing back.
type drainConn struct {
	// Connection is the wrapped connection.
	mcpsdk.Connection
	// mu guards owed and asked.
	mu sync.Mutex
	// owed holds the IDs of calls read from the client and not yet answered.
	owed map[jsonrpc.ID]struct{}
	// asked holds the IDs of calls sent to the client and not yet answered. An owed reply
	// may wait on one, and no answer can arrive once input has ended.
	asked map[jsonrpc.ID]struct{}
	// changed wakes a Read that is holding back the end of input. It buffers one signal, so
	// a change that lands before the Read starts waiting is not lost.
	changed chan struct{}
	// closed is closed by Close, releasing a held Read.
	closed chan struct{}
	// closeOnce guards closing closed.
	closeOnce sync.Once
}

// newDrainConn wraps conn.
func newDrainConn(conn mcpsdk.Connection) *drainConn {
	return &drainConn{
		Connection: conn,
		owed:       make(map[jsonrpc.ID]struct{}),
		asked:      make(map[jsonrpc.ID]struct{}),
		changed:    make(chan struct{}, 1),
		closed:     make(chan struct{}),
	}
}

// Read returns the next message from the client. When the inner read fails, Read returns
// the error only once nothing is owed, an owed reply waits on the client, ctx is done, or
// the connection is closed.
func (c *drainConn) Read(ctx context.Context) (jsonrpc.Message, error) {
	msg, err := c.Connection.Read(ctx)
	if err != nil {
		c.drain(ctx)
		return nil, err
	}
	c.update(func() {
		switch m := msg.(type) {
		case *jsonrpc.Request:
			if m.IsCall() {
				c.owed[m.ID] = struct{}{}
			}
		case *jsonrpc.Response:
			delete(c.asked, m.ID)
		}
	})
	return msg, nil
}

// Write sends msg to the client. A call is recorded before it goes out, so an answer read
// back at once finds it, and a reply is settled only after its bytes are written, so a held
// Read never releases ahead of them.
func (c *drainConn) Write(ctx context.Context, msg jsonrpc.Message) error {
	req, ok := msg.(*jsonrpc.Request)
	asking := ok && req.IsCall()
	if asking {
		c.update(func() { c.asked[req.ID] = struct{}{} })
	}
	err := c.Connection.Write(ctx, msg)
	if asking && err != nil {
		c.update(func() { delete(c.asked, req.ID) })
	}
	if resp, ok := msg.(*jsonrpc.Response); ok {
		c.update(func() { delete(c.owed, resp.ID) })
	}
	return err
}

// Close releases a held Read and closes the inner connection.
func (c *drainConn) Close() error {
	c.closeOnce.Do(func() { close(c.closed) })
	return c.Connection.Close()
}

// update applies f under the lock and wakes a held Read to look again.
func (c *drainConn) update(f func()) {
	c.mu.Lock()
	f()
	c.mu.Unlock()
	select {
	case c.changed <- struct{}{}:
	default:
	}
}

// drain blocks until the server owes the client nothing it can still deliver, ctx is done,
// or the connection is closed.
func (c *drainConn) drain(ctx context.Context) {
	for {
		c.mu.Lock()
		settled := len(c.owed) == 0 || len(c.asked) > 0
		c.mu.Unlock()
		if settled {
			return
		}
		select {
		case <-c.changed:
		case <-ctx.Done():
			return
		case <-c.closed:
			return
		}
	}
}
