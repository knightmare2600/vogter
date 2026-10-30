package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/BurntSushi/toml"
)

// ZabtermPath is where zabterm keeps its config.
func ZabtermPath() string {
	if x := os.Getenv("XDG_CONFIG_HOME"); filepath.IsAbs(x) {
		return filepath.Join(x, "zabterm", "config.toml")
	}
	return filepath.Join(Home(), ".config", "zabterm", "config.toml")
}

// Import copies a zabterm config to dest after checking it parses and has
// usable profiles. The schema is the same, so the text, comments included,
// is kept. The source is never changed, and dest is only replaced with
// force. It returns the profile names.
func Import(from, dest string, force bool) ([]string, error) {
	raw, err := os.ReadFile(from)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", from, err)
	}
	c := defaults()
	if _, err := toml.Decode(string(raw), &c); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", from, err)
	}
	if len(c.Profiles) == 0 {
		return nil, fmt.Errorf("%s has no profiles", from)
	}
	names := make([]string, 0, len(c.Profiles))
	for n, p := range c.Profiles {
		if p == nil || p.URL == "" {
			return nil, fmt.Errorf("profile %q has no url", n)
		}
		names = append(names, n)
	}
	sort.Strings(names)
	if err := writeNew(dest, raw, force); err != nil {
		if errors.Is(err, ErrExists) {
			return nil, fmt.Errorf("%s %w (use --force to overwrite)", dest, ErrExists)
		}
		return nil, err
	}
	return names, nil
}
