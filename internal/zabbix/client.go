// Package zabbix is a small JSON-RPC client for the Zabbix API methods
// Vogter needs. It has no knowledge of the terminal UI; every call takes a
// context and returns *Error on failure.
package zabbix

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

// DefaultTimeout bounds a whole request, as zabterm's 20 s client timeout did.
const DefaultTimeout = 20 * time.Second

// Options configures a Client.
type Options struct {
	// URL is the full api_jsonrpc.php endpoint.
	URL string
	// VerifyTLS checks server certificates. Turn it off only for
	// self-signed frontends.
	VerifyTLS bool
	// Timeout bounds each request; DefaultTimeout when zero.
	Timeout   time.Duration
	UserAgent string
	// Transport replaces the HTTP transport, for tests.
	Transport http.RoundTripper
}

// Client talks to one Zabbix frontend. It is safe for concurrent use.
type Client struct {
	http *http.Client
	url  string
	ua   string
	id   atomic.Uint64

	mu      sync.RWMutex
	token   string
	session bool // token came from user.login and needs user.logout
}

// New builds a client. It does not contact the server.
func New(o Options) *Client {
	if o.Timeout == 0 {
		o.Timeout = DefaultTimeout
	}
	tr := o.Transport
	if tr == nil {
		tr = &http.Transport{
			Proxy:                 http.ProxyFromEnvironment,
			DialContext:           (&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
			TLSClientConfig:       &tls.Config{InsecureSkipVerify: !o.VerifyTLS},
			TLSHandshakeTimeout:   10 * time.Second,
			ResponseHeaderTimeout: o.Timeout,
			IdleConnTimeout:       90 * time.Second,
			MaxIdleConnsPerHost:   8,
			ForceAttemptHTTP2:     true,
		}
	}
	return &Client{http: &http.Client{Transport: tr, Timeout: o.Timeout}, url: o.URL, ua: o.UserAgent}
}

// SetToken sets an API token for Bearer authentication.
func (c *Client) SetToken(token string) {
	c.mu.Lock()
	c.token, c.session = token, false
	c.mu.Unlock()
}

// HasSession reports whether the client holds a user.login session.
func (c *Client) HasSession() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.session
}

// noAuth lists the methods that Zabbix rejects when credentials are sent.
var noAuth = map[string]bool{"apiinfo.version": true, "user.login": true}

type request struct {
	JSONRPC string `json:"jsonrpc"`
	Method  string `json:"method"`
	Params  any    `json:"params"`
	ID      uint64 `json:"id"`
}

type response struct {
	Result json.RawMessage `json:"result"`
	Error  *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    string `json:"data"`
	} `json:"error"`
}

// Call invokes method with params and decodes the result into out (which
// may be nil to discard it).
func (c *Client) Call(ctx context.Context, method string, params, out any) error {
	body, err := json.Marshal(request{JSONRPC: "2.0", Method: method, Params: params, ID: c.id.Add(1)})
	if err != nil {
		return &Error{Kind: KindDecode, Method: method, Err: err}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url, bytes.NewReader(body))
	if err != nil {
		return &Error{Kind: KindTransport, Method: method, Err: err}
	}
	req.Header.Set("Content-Type", "application/json-rpc")
	if c.ua != "" {
		req.Header.Set("User-Agent", c.ua)
	}
	if !noAuth[method] {
		c.mu.RLock()
		tok := c.token
		c.mu.RUnlock()
		if tok != "" {
			req.Header.Set("Authorization", "Bearer "+tok)
		}
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return &Error{Kind: KindTransport, Method: method, Err: err}
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return &Error{Kind: KindHTTP, Method: method, Status: resp.StatusCode}
	}

	var r response
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		if ctx.Err() != nil {
			return &Error{Kind: KindTransport, Method: method, Err: ctx.Err()}
		}
		return &Error{Kind: KindDecode, Method: method, Err: fmt.Errorf("invalid JSON: %w", err)}
	}
	if r.Error != nil {
		return &Error{
			Kind:    classify(r.Error.Code, r.Error.Message, r.Error.Data),
			Method:  method,
			Code:    r.Error.Code,
			Message: r.Error.Message,
			Data:    r.Error.Data,
		}
	}
	if r.Result == nil {
		return &Error{Kind: KindDecode, Method: method, Err: fmt.Errorf("no result")}
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(r.Result, out); err != nil {
		return &Error{Kind: KindShape, Method: method, Err: fmt.Errorf("unexpected response shape: %w", err)}
	}
	if v, ok := out.(validator); ok {
		if err := v.validate(); err != nil {
			return &Error{Kind: KindShape, Method: method, Err: err}
		}
	}
	return nil
}

// validator is implemented by result types with fields that must be
// present; a failed request must never look like an empty or zero result.
type validator interface {
	validate() error
}

// Login opens a session with user.login and uses it for later calls.
func (c *Client) Login(ctx context.Context, username, password string) error {
	var tok string
	if err := c.Call(ctx, "user.login", map[string]string{"username": username, "password": password}, &tok); err != nil {
		return err
	}
	if tok == "" {
		return &Error{Kind: KindShape, Method: "user.login", Err: fmt.Errorf("empty session id")}
	}
	c.mu.Lock()
	c.token, c.session = tok, true
	c.mu.Unlock()
	return nil
}

// Logout closes a user.login session. It does nothing for API tokens,
// which must not be logged out. The Zabbix manual asks API users to log out
// so sessions do not pile up (zabterm never did, D3).
func (c *Client) Logout(ctx context.Context) error {
	if !c.HasSession() {
		return nil
	}
	err := c.Call(ctx, "user.logout", []any{}, nil)
	c.mu.Lock()
	c.token, c.session = "", false
	c.mu.Unlock()
	return err
}
