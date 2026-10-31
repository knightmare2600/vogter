// Package onboard connects a new user to Zabbix: it tests the server, turns a
// username and password into an API token, and saves the profile. The password
// is used once and never stored.
package onboard

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/knightmare2600/vogter/internal/config"
	"github.com/knightmare2600/vogter/internal/zabbix"
)

// nameTries bounds the search for a free token name.
const nameTries = 20

var notTokenName = regexp.MustCompile(`[^a-z0-9-]+`)

// Backend does the network and file work of the welcome flow.
type Backend struct {
	// Path is the config file to write.
	Path      string
	UserAgent string
	// Hostname names the token; tests replace it.
	Hostname func() (string, error)
}

// New builds a Backend.
func New(path, userAgent string) *Backend {
	return &Backend{Path: path, UserAgent: userAgent, Hostname: os.Hostname}
}

// Normalise tidies what a user typed into a frontend URL, assuming https when
// no scheme is given.
func Normalise(raw string) (string, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "", errors.New("enter the address of your Zabbix frontend")
	}
	if !strings.Contains(s, "://") {
		s = "https://" + s
	}
	u, err := url.Parse(s)
	if err != nil || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return "", fmt.Errorf("%q is not an http or https address", strings.TrimSpace(raw))
	}
	return strings.TrimRight((&config.Profile{URL: s}).WebURL(), "/"), nil
}

func (b *Backend) client(serverURL string, verifyTLS bool) *zabbix.Client {
	p := config.Profile{URL: serverURL}
	return zabbix.New(zabbix.Options{URL: p.APIURL(), VerifyTLS: verifyTLS, UserAgent: b.UserAgent})
}

// Probe checks that the address is a Zabbix that Vægter supports, and returns
// a line describing it.
func (b *Backend) Probe(ctx context.Context, serverURL string, verifyTLS bool) (string, error) {
	raw, err := b.client(serverURL, verifyTLS).APIVersion(ctx)
	if err != nil {
		return "", err
	}
	v, err := zabbix.ParseVersion(raw)
	if err != nil {
		return "", err
	}
	s := zabbix.SupportFor(v)
	if s == zabbix.Unsupported {
		return "", errors.New(s.Describe(v))
	}
	line := "Zabbix " + v.String()
	if note := s.Describe(v); note != "" {
		line += " · " + note
	}
	return line, nil
}

// UseToken saves a token the user already has, after testing it.
func (b *Backend) UseToken(ctx context.Context, serverURL string, verifyTLS bool, token string) (string, error) {
	cl := b.client(serverURL, verifyTLS)
	if err := check(ctx, cl, token); err != nil {
		return "", err
	}
	return config.SaveProfile(b.Path, serverURL, token, verifyTLS)
}

// SignIn creates an API token with the user's password, tests it and saves
// it. Any failure after creation removes the token again, and the session
// always closes, so a failed attempt leaves nothing behind on the server.
func (b *Backend) SignIn(ctx context.Context, serverURL string, verifyTLS bool, username, password string) (string, error) {
	cl := b.client(serverURL, verifyTLS)
	if err := cl.Login(ctx, username, password); err != nil {
		return "", err
	}
	defer func() {
		lctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = cl.Logout(lctx)
	}()
	id, err := b.createToken(ctx, cl)
	if err != nil {
		return "", err
	}
	profile, err := b.finish(ctx, cl, serverURL, verifyTLS, id)
	if err != nil {
		lctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = cl.DeleteToken(lctx, id)
		return "", err
	}
	return profile, nil
}

func (b *Backend) finish(ctx context.Context, cl *zabbix.Client, serverURL string, verifyTLS bool, id string) (string, error) {
	token, err := cl.GenerateToken(ctx, id)
	if err != nil {
		return "", err
	}
	if err := check(ctx, b.client(serverURL, verifyTLS), token); err != nil {
		return "", err
	}
	return config.SaveProfile(b.Path, serverURL, token, verifyTLS)
}

// createToken picks the first free name, because Zabbix refuses a duplicate
// for the same user and generating an existing token would break whichever
// machine already uses it.
func (b *Backend) createToken(ctx context.Context, cl *zabbix.Client) (string, error) {
	base := b.tokenName()
	for i := 1; i <= nameTries; i++ {
		name := base
		if i > 1 {
			name += "-" + strconv.Itoa(i)
		}
		id, err := cl.CreateToken(ctx, name)
		if err == nil {
			return id, nil
		}
		if !strings.Contains(strings.ToLower(err.Error()), "already exists") {
			return "", err
		}
	}
	return "", fmt.Errorf("could not find a free token name after %q", base)
}

func (b *Backend) tokenName() string {
	host, err := b.Hostname()
	if err != nil {
		return "vaegter"
	}
	slug := strings.Trim(notTokenName.ReplaceAllString(strings.ToLower(host), "-"), "-")
	if slug == "" {
		return "vaegter"
	}
	return "vaegter-" + slug
}

func check(ctx context.Context, cl *zabbix.Client, token string) error {
	cl.SetToken(token)
	_, err := cl.Count(ctx, "host.get")
	return err
}
