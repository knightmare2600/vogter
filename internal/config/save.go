package config

import (
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/BurntSushi/toml"
)

var (
	notSlug         = regexp.MustCompile(`[^a-z0-9-]+`)
	defaultProfileK = regexp.MustCompile(`(?m)^default_profile\s*=.*$`)
)

// startingBlock is the profile in Template that a first profile replaces.
const startingBlock = `[profiles.local]
url = "http://localhost:8080"
# Create one in Zabbix: User settings -> API tokens.
token = "` + placeholder + `"
`

// SaveProfile stores a new profile named after the server and returns its
// name. A first profile starts a commented config from Template. Otherwise it
// is appended, keeping the user's comments. The result is parsed back before
// it replaces the file, so a bad write cannot damage a working config.
func SaveProfile(path, serverURL, token string, verifyTLS bool) (string, error) {
	raw, err := os.ReadFile(path)
	exists := err == nil
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return "", fmt.Errorf("reading %s: %w", path, err)
	}
	var current Config
	if exists {
		if _, err := toml.Decode(string(raw), &current); err != nil {
			return "", fmt.Errorf("parsing %s: %w", path, err)
		}
	}
	name := profileName(serverURL, current.Profiles)
	block := fmt.Sprintf("[profiles.%s]\nurl = %s\ntoken = %s\n", name, strconv.Quote(serverURL), strconv.Quote(token))
	if !verifyTLS {
		block += "verify_tls = false\n"
	}

	var text string
	if exists {
		text = strings.TrimRight(string(raw), "\n") + "\n\n" + block
		// A default that `vaegter init` left unfilled can never work.
		if d := current.Profiles[current.DefaultProfile]; d != nil && d.Token.isPlaceholder() {
			text = defaultProfileK.ReplaceAllString(text, "default_profile = "+strconv.Quote(name))
		}
	} else {
		text = strings.Replace(Template, startingBlock, block, 1)
		text = defaultProfileK.ReplaceAllString(text, "default_profile = "+strconv.Quote(name))
	}

	var check Config
	if _, err := toml.Decode(text, &check); err != nil {
		return "", fmt.Errorf("the new config did not parse: %w", err)
	}
	p := check.Profiles[name]
	if p == nil || p.URL != serverURL || p.Token.Reveal() != token {
		return "", errors.New("the new config did not read back correctly")
	}
	if err := writeNew(path, []byte(text), true); err != nil {
		return "", err
	}
	return name, nil
}

// profileName is the server's short host name, made unique among existing.
func profileName(serverURL string, existing map[string]*Profile) string {
	host := "zabbix"
	if u, err := url.Parse(serverURL); err == nil && u.Hostname() != "" {
		host = u.Hostname()
	}
	if dot := strings.IndexByte(host, '.'); dot > 0 && net.ParseIP(host) == nil {
		host = host[:dot]
	}
	base := strings.Trim(notSlug.ReplaceAllString(strings.ToLower(host), "-"), "-")
	if base == "" {
		base = "zabbix"
	}
	name := base
	for i := 2; existing[name] != nil; i++ {
		name = base + "-" + strconv.Itoa(i)
	}
	return name
}
