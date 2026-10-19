package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"testing"
	"testing/synctest"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestServeClosedInput drives the server the way a shell pipe does: every request is written
// and then the input ends before any reply goes out. Each call must still be answered, and
// the end of input must be a clean return.
func TestServeClosedInput(t *testing.T) {
	t.Parallel()
	const (
		initialize  = `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"test","version":"0"}}}`
		initialized = `{"jsonrpc":"2.0","method":"notifications/initialized"}`
		listTools   = `{"jsonrpc":"2.0","id":2,"method":"tools/list"}`
		chop        = `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"chop","arguments":{"text":"We leverage synergy."}}}`
	)
	tests := []struct {
		Input   []string
		WantIDs []int64
	}{{ // Test 0: The handshake and a tools listing are both answered.
		Input: []string{initialize, initialized, listTools}, WantIDs: []int64{1, 2},
	}, { // Test 1: A tool call sent right before the input ends is answered.
		Input: []string{initialize, initialized, chop}, WantIDs: []int64{1, 3},
	}, { // Test 2: Notifications alone are owed nothing.
		Input: []string{initialized}, WantIDs: nil,
	}, { // Test 3: Empty input ends at once.
		Input: nil, WantIDs: nil,
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			var in strings.Builder
			for _, line := range test.Input {
				in.WriteString(line + "\n")
			}
			var out bytes.Buffer
			srv := NewServer(testSanitizers(), nil, nil, "test")
			err := srv.serve(t.Context(), &mcpsdk.IOTransport{
				Reader: io.NopCloser(strings.NewReader(in.String())),
				Writer: testWriteCloser{&out},
			})
			if err != nil {
				t.Fatalf("serve: %v", err)
			}

			var gotIDs []int64
			for line := range strings.Lines(out.String()) {
				var resp struct {
					ID    int64           `json:"id"`
					Error json.RawMessage `json:"error"`
				}
				if err := json.Unmarshal([]byte(line), &resp); err != nil {
					t.Fatalf("unmarshal %q: %v", line, err)
				}
				if resp.Error != nil {
					t.Errorf("call %d failed: %s", resp.ID, resp.Error)
				}
				gotIDs = append(gotIDs, resp.ID)
			}
			slices.Sort(gotIDs)
			if diff := cmp.Diff(test.WantIDs, gotIDs, cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("answered IDs mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// TestDrainConn checks when a read that hit the end of input is held back and what releases
// it. Each case runs steps before the input ends, then reads the end of input, then runs
// steps one at a time, checking after each whether the read is still held.
func TestDrainConn(t *testing.T) {
	t.Parallel()
	var (
		call1    = &jsonrpc.Request{ID: testID(1), Method: "tools/list"}
		call2    = &jsonrpc.Request{ID: testID(2), Method: "tools/list"}
		reply1   = &jsonrpc.Response{ID: testID(1), Result: json.RawMessage(`{}`)}
		reply2   = &jsonrpc.Response{ID: testID(2), Result: json.RawMessage(`{}`)}
		ask9     = &jsonrpc.Request{ID: testID(9), Method: "roots/list"}
		answer9  = &jsonrpc.Response{ID: testID(9), Result: json.RawMessage(`{}`)}
		progress = &jsonrpc.Request{Method: "notifications/progress"}
		errWrite = errors.New("write broke")
	)
	tests := []struct {
		Before   []drainStep
		After    []drainStep
		WantHeld []bool
		WriteErr error
	}{{ // Test 0: Nothing owed lets the end of input through at once.
		Before: []drainStep{{Read: progress}}, WantHeld: []bool{false},
	}, { // Test 1: An owed call holds the end of input until its reply is written.
		Before: []drainStep{{Read: call1}}, After: []drainStep{{Write: reply1}},
		WantHeld: []bool{true, false},
	}, { // Test 2: Every owed call must be answered, not just the first.
		Before: []drainStep{{Read: call1}, {Read: call2}},
		After:  []drainStep{{Write: reply1}, {Write: reply2}}, WantHeld: []bool{true, true, false},
	}, { // Test 3: A reply written before the input ends leaves nothing owed.
		Before: []drainStep{{Read: call1}, {Write: reply1}}, WantHeld: []bool{false},
	}, { // Test 4: A notification does not settle an owed call.
		Before: []drainStep{{Read: call1}}, After: []drainStep{{Write: progress}, {Write: reply1}},
		WantHeld: []bool{true, true, false},
	}, { // Test 5: A call to the client still unanswered can never be answered, so it releases.
		Before: []drainStep{{Read: call1}, {Write: ask9}}, WantHeld: []bool{false},
	}, { // Test 6: A call to the client sent while held releases.
		Before: []drainStep{{Read: call1}}, After: []drainStep{{Write: ask9}},
		WantHeld: []bool{true, false},
	}, { // Test 7: A call to the client that was answered no longer counts.
		Before: []drainStep{{Write: ask9}, {Read: answer9}, {Read: call1}},
		After:  []drainStep{{Write: reply1}}, WantHeld: []bool{true, false},
	}, { // Test 8: A failed call to the client is forgotten, and a failed reply still settles.
		Before: []drainStep{{Read: call1}, {Write: ask9}}, After: []drainStep{{Write: reply1}},
		WantHeld: []bool{true, false}, WriteErr: errWrite,
	}, { // Test 9: Closing the connection releases a held read.
		Before: []drainStep{{Read: call1}}, After: []drainStep{{Close: true}},
		WantHeld: []bool{true, false},
	}, { // Test 10: Canceling the read's context releases it.
		Before: []drainStep{{Read: call1}}, After: []drainStep{{Cancel: true}},
		WantHeld: []bool{true, false},
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			if len(test.WantHeld) != len(test.After)+1 || test.WantHeld[len(test.WantHeld)-1] {
				t.Fatalf("WantHeld needs one entry per step plus one, ending released")
			}
			synctest.Test(t, func(t *testing.T) {
				ctx, cancel := context.WithCancel(t.Context())
				defer cancel()
				inner := &testConn{writeErr: test.WriteErr}
				c := newDrainConn(inner)
				for _, step := range test.Before {
					step.run(t, ctx, c, inner, cancel)
				}

				done := make(chan error, 1)
				go func() {
					_, err := c.Read(ctx)
					done <- err
				}()
				var readErr error
				held := func() bool {
					synctest.Wait()
					select {
					case readErr = <-done:
						return false
					default:
						return true
					}
				}
				if got := held(); got != test.WantHeld[0] {
					t.Fatalf("at end of input: held = %v, want %v", got, test.WantHeld[0])
				}
				for i, step := range test.After {
					step.run(t, ctx, c, inner, cancel)
					if got := held(); got != test.WantHeld[i+1] {
						t.Fatalf("after step %d: held = %v, want %v", i, got, test.WantHeld[i+1])
					}
				}
				if !errors.Is(readErr, io.EOF) {
					t.Errorf("read error = %v, want %v", readErr, io.EOF)
				}
			})
		})
	}
}

// drainStep is one action in a TestDrainConn case. Exactly one field is set.
type drainStep struct {
	// Read is a message the client sends, read through the connection.
	Read jsonrpc.Message
	// Write is a message the server sends.
	Write jsonrpc.Message
	// Close closes the connection.
	Close bool
	// Cancel cancels the context of the held read.
	Cancel bool
}

// run performs the step against c, whose inner connection is inner.
func (s drainStep) run(t *testing.T, ctx context.Context, c *drainConn, inner *testConn, cancel context.CancelFunc) {
	t.Helper()
	switch {
	case s.Read != nil:
		inner.next = s.Read
		if _, err := c.Read(ctx); err != nil {
			t.Fatalf("read: %v", err)
		}
	case s.Write != nil:
		if err := c.Write(ctx, s.Write); !errors.Is(err, inner.writeErr) {
			t.Fatalf("write error = %v, want %v", err, inner.writeErr)
		}
	case s.Close:
		if err := c.Close(); err != nil {
			t.Fatalf("close: %v", err)
		}
	case s.Cancel:
		cancel()
	}
}

// testConn is a scripted Connection. Read returns next once and then the end of input.
type testConn struct {
	// next is what the following Read returns. It is cleared once read.
	next jsonrpc.Message
	// writeErr is what every Write returns.
	writeErr error
}

// Read returns next if one is set, and io.EOF otherwise.
func (c *testConn) Read(context.Context) (jsonrpc.Message, error) {
	if c.next == nil {
		return nil, io.EOF
	}
	msg := c.next
	c.next = nil
	return msg, nil
}

// Write returns writeErr.
func (c *testConn) Write(context.Context, jsonrpc.Message) error { return c.writeErr }

// Close does nothing.
func (c *testConn) Close() error { return nil }

// SessionID returns no session.
func (c *testConn) SessionID() string { return "" }

// testWriteCloser adds a no-op Close to a writer.
type testWriteCloser struct {
	// Writer receives the bytes.
	io.Writer
}

// Close does nothing.
func (testWriteCloser) Close() error { return nil }

// testID builds a numeric request ID.
func testID(n float64) jsonrpc.ID {
	id, err := jsonrpc.MakeID(n)
	if err != nil {
		panic(fmt.Sprintf("testID: %v", err))
	}
	return id
}
