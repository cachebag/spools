// Package remote runs spools on another machine by shelling out to the user's
// own ssh, so their keys, agent, ~/.ssh/config and tailscale all just work.
package remote

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

// Spools runs `spools <args...>` on host, wiring stdin/stdout and passing the
// remote stderr straight through.
func Spools(host string, stdin io.Reader, stdout io.Writer, args ...string) error {
	if host == "" || strings.HasPrefix(host, "-") {
		return fmt.Errorf("invalid host %q", host)
	}
	cmd := exec.Command("ssh", "-T", host, Command(append([]string{"spools"}, args...)...))
	cmd.Stdin = stdin
	cmd.Stdout = stdout
	cmd.Stderr = os.Stderr

	err := cmd.Run()
	var exit *exec.ExitError
	if errors.As(err, &exit) && exit.ExitCode() == 127 {
		return fmt.Errorf("ssh %s: spools not found on the remote PATH (non-interactive ssh skips .zshrc; install it to /usr/local/bin or similar)", host)
	}
	if err != nil {
		return fmt.Errorf("ssh %s: %w", host, err)
	}
	return nil
}

// Command joins args into a single POSIX shell command line. ssh hands the
// remote command to the login shell as a string, so every arg is quoted.
func Command(args ...string) string {
	quoted := make([]string, len(args))
	for i, a := range args {
		quoted[i] = quote(a)
	}
	return strings.Join(quoted, " ")
}

func quote(s string) string {
	if s != "" && !strings.ContainsFunc(s, needsQuote) {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func needsQuote(r rune) bool {
	safe := r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("-_./=:@", r)
	return !safe
}
