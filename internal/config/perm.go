package config

import (
	"fmt"
	"os"
	"runtime"
)

// permissionIssues warns when the file holds a plain-text secret and other
// users can read it. Windows ACLs are not inspected.
func (c *Config) permissionIssues() []Issue {
	if runtime.GOOS == "windows" {
		return nil
	}
	plain := false
	for _, p := range c.Profiles {
		if (!p.Token.Empty() && !p.Token.isPlaceholder()) || !p.Password.Empty() {
			plain = true
		}
	}
	if !plain {
		return nil
	}
	st, err := os.Stat(c.Path)
	if err != nil || st.Mode().Perm()&0o077 == 0 {
		return nil
	}
	return []Issue{{Msg: fmt.Sprintf("%s holds a token or password and is readable by other users (mode %04o); run: chmod 600 %s",
		c.Path, st.Mode().Perm(), c.Path)}}
}
