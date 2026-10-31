package zabbix

import (
	"errors"
	"fmt"
	"strings"
)

// Kind classifies a failed call so callers can react to the cause rather
// than to message text.
type Kind int

const (
	// KindTransport covers dial, DNS, TLS, timeout and cancellation errors.
	KindTransport Kind = iota + 1
	// KindHTTP is a non-2xx response, for example a proxy's 502.
	KindHTTP
	// KindDecode is a body that is not a JSON-RPC response.
	KindDecode
	// KindRPC is a JSON-RPC error object not covered by a narrower kind.
	KindRPC
	// KindAuth is a rejected or expired token or session.
	KindAuth
	// KindPermission is a call the account is not allowed to make.
	KindPermission
	// KindShape is a response missing a field Vægter needs.
	KindShape
)

func (k Kind) String() string {
	switch k {
	case KindTransport:
		return "transport"
	case KindHTTP:
		return "http"
	case KindDecode:
		return "decode"
	case KindRPC:
		return "rpc"
	case KindAuth:
		return "auth"
	case KindPermission:
		return "permission"
	case KindShape:
		return "shape"
	}
	return "unknown"
}

// Error is returned by every client call that fails.
type Error struct {
	Kind   Kind
	Method string
	// Status is the HTTP status for KindHTTP.
	Status int
	// Code, Message and Data come from a JSON-RPC error object.
	Code    int
	Message string
	Data    string
	Err     error
}

func (e *Error) Error() string {
	switch e.Kind {
	case KindHTTP:
		return fmt.Sprintf("%s: HTTP %d", e.Method, e.Status)
	case KindRPC, KindAuth, KindPermission:
		return strings.TrimSpace(fmt.Sprintf("%s: %s %s", e.Method, e.Message, e.Data))
	}
	if e.Err != nil {
		return fmt.Sprintf("%s: %v", e.Method, e.Err)
	}
	return e.Method + ": " + e.Kind.String() + " error"
}

func (e *Error) Unwrap() error { return e.Err }

// KindOf returns the kind of a client error, or 0 for other errors.
func KindOf(err error) Kind {
	var e *Error
	if errors.As(err, &e) {
		return e.Kind
	}
	return 0
}

// classify narrows a JSON-RPC error. Zabbix reports both authentication and
// permission failures as code -32602 or -32500 with the reason in data, so
// the data text is the only signal; the phrases are those of the Zabbix 7.0
// frontend (unverified against a live server, see porting-audit §16).
func classify(code int, message, data string) Kind {
	d := strings.ToLower(data + " " + message)
	switch {
	case strings.Contains(d, "not authorized"),
		strings.Contains(d, "not authorised"),
		strings.Contains(d, "session terminated"),
		strings.Contains(d, "re-login"),
		strings.Contains(d, "api token expired"),
		strings.Contains(d, "incorrect user name or password"),
		strings.Contains(d, "login name or password is incorrect"):
		return KindAuth
	case strings.Contains(d, "no permissions"),
		strings.Contains(d, "permission denied"),
		strings.Contains(d, "no permission"):
		return KindPermission
	}
	return KindRPC
}
