package editor

import (
	"net"
	"time"
)

// Timeouts of research R-02. overallTimeout is a variable so a test can
// shorten it.
var (
	dialTimeout    = 500 * time.Millisecond
	callTimeout    = time.Second
	overallTimeout = 2 * time.Second
)

// Client sends msgpack-RPC requests to one Neovim instance. Every call is
// structured: a method name and its arguments as values, never a command
// string (F014-FR-005).
type Client struct {
	conn     net.Conn
	nextID   int64
	deadline time.Time
}

// CallError is an error response from Neovim, with its raw message.
type CallError struct{ Message string }

func (e *CallError) Error() string { return "neovim: " + e.Message }

var errNoAnswer = &EndpointError{Reason: ReasonNoAnswer}

// Dial connects to the Unix socket at endpoint. Every call through the
// client ends by deadline.
func Dial(endpoint string, deadline time.Time) (*Client, error) {
	conn, err := net.DialTimeout("unix", endpoint, min(dialTimeout, time.Until(deadline)))
	if err != nil {
		return nil, errNoAnswer
	}
	return &Client{conn: conn, deadline: deadline}, nil
}

// Close closes the connection.
func (c *Client) Close() error { return c.conn.Close() }

// Call sends one request and returns its result. It skips notifications and
// responses to other requests; a timeout, a closed connection or a response
// above the size limit is no_answer.
func (c *Client) Call(method string, params ...any) (any, error) {
	c.nextID++
	id := c.nextID
	if params == nil {
		params = []any{}
	}
	request, err := encode([]any{0, id, method, params})
	if err != nil {
		return nil, err
	}
	deadline := time.Now().Add(callTimeout)
	if c.deadline.Before(deadline) {
		deadline = c.deadline
	}
	if err := c.conn.SetDeadline(deadline); err != nil {
		return nil, errNoAnswer
	}
	if _, err := c.conn.Write(request); err != nil {
		return nil, errNoAnswer
	}
	for {
		v, err := decode(c.conn, maxMessage)
		if err != nil {
			return nil, errNoAnswer
		}
		message, ok := v.([]any)
		if !ok || len(message) != 4 || message[0] != int64(1) || message[1] != id {
			continue
		}
		if message[2] != nil {
			return nil, &CallError{Message: errorMessage(message[2])}
		}
		return message[3], nil
	}
}

// errorMessage extracts the text of Neovim's [type, message] error.
func errorMessage(v any) string {
	if pair, ok := v.([]any); ok && len(pair) == 2 {
		if text, ok := pair[1].(string); ok {
			return text
		}
	}
	return "unrecognized error response"
}
