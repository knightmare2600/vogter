package config

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// TokenCmdTimeout bounds token_cmd. It is long because a password manager
// may prompt for a passphrase first.
const TokenCmdTimeout = 2 * time.Minute

// Auth is a resolved credential: either an API token, or a username and
// password for user.login.
type Auth struct {
	Token    Secret
	Username string
	Password Secret
	// Source names where the credential came from, for diagnostics.
	Source string
}

// IsLogin reports whether the credential needs user.login.
func (a Auth) IsLogin() bool { return a.Token.Empty() }

// ResolveAuth finds the profile's credential, using the first that is set
// of: VOGTER_TOKEN, token_env, token_cmd, token, username and password.
// An empty value counts as unset (zabterm accepted empty tokens, D14).
func (p *Profile) ResolveAuth(ctx context.Context) (Auth, error) {
	if t := os.Getenv(EnvToken); t != "" {
		return Auth{Token: Secret(t), Source: EnvToken}, nil
	}
	if p.TokenEnv != "" {
		t := os.Getenv(p.TokenEnv)
		if t == "" {
			return Auth{}, fmt.Errorf("token_env: $%s is not set or empty", p.TokenEnv)
		}
		return Auth{Token: Secret(t), Source: "token_env $" + p.TokenEnv}, nil
	}
	if p.TokenCmd != "" {
		t, err := runTokenCmd(ctx, p.TokenCmd)
		if err != nil {
			return Auth{}, err
		}
		return Auth{Token: Secret(t), Source: "token_cmd"}, nil
	}
	if !p.Token.Empty() && !p.Token.isPlaceholder() {
		return Auth{Token: p.Token, Source: "token"}, nil
	}
	if p.Username != "" && !p.Password.Empty() {
		return Auth{Username: p.Username, Password: p.Password, Source: "username/password"}, nil
	}
	return Auth{}, fmt.Errorf("profile %q has no credentials: set token, token_env, token_cmd or username/password", p.Name)
}

// ShellCommand returns the shell used for token_cmd: sh -c, or cmd /C on
// Windows.
func ShellCommand(ctx context.Context, line string) *exec.Cmd {
	if runtime.GOOS == "windows" {
		return exec.CommandContext(ctx, "cmd", "/C", line)
	}
	return exec.CommandContext(ctx, "sh", "-c", line)
}

func runTokenCmd(ctx context.Context, line string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, TokenCmdTimeout)
	defer cancel()
	cmd := ShellCommand(ctx, line)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		// stderr is the command's diagnostics, never the token (stdout).
		return "", fmt.Errorf("token_cmd failed: %v: %s", err, strings.TrimSpace(stderr.String()))
	}
	t := strings.TrimSpace(stdout.String())
	if t == "" {
		return "", fmt.Errorf("token_cmd printed nothing")
	}
	return t, nil
}
