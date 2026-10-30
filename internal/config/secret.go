package config

import "log/slog"

// placeholder is the token written by `vogter init`; it counts as unset.
const placeholder = "paste-your-api-token-here"

// Secret holds a credential. Every way of formatting it prints a redaction
// marker, so a secret cannot leak through %v, %+v, %#v or structured logs.
type Secret string

// Reveal returns the secret itself. Call it only where the value is sent
// to Zabbix.
func (s Secret) Reveal() string { return string(s) }

// Empty reports whether no value is set.
func (s Secret) Empty() bool { return s == "" }

func (s Secret) isPlaceholder() bool { return s == placeholder }

func (s Secret) String() string {
	if s == "" {
		return "<unset>"
	}
	return "<redacted>"
}

// GoString covers %#v.
func (s Secret) GoString() string { return s.String() }

// LogValue covers log/slog.
func (s Secret) LogValue() slog.Value { return slog.StringValue(s.String()) }
