package chainview_test

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func (s *combinedGenerationOpsd) chromeWithdrawal(t *testing.T, phase, closeHash string) string {
	t.Helper()
	if os.Getenv("P05_O09_RETRY_WITHDRAWAL") == "third-party" {
		return ""
	}
	return s.chromeWithdrawalAt(t, phase, closeHash, nil)
}

func (s *combinedGenerationOpsd) chromeWithdrawalAt(t *testing.T, phase, closeHash string, advance func()) string {
	t.Helper()
	if os.Getenv("P05_O09_CHROME_DIST") == "" {
		return ""
	}
	script, err := filepath.Abs("../../../p05-operations-backoffice/test/finance/protected-withdrawal-browser.cjs")
	if err != nil {
		t.Fatal(err)
	}
	var gate string
	if phase == "revoked" || advance != nil {
		gate = filepath.Join(t.TempDir(), "scope")
	}
	browserCtx, cancel := context.WithTimeout(s.ctx, 45*time.Second)
	defer cancel()
	command := exec.CommandContext(browserCtx, "node", script, s.origin, s.input.expected.Branch, s.input.expected.Board,
		s.input.expected.Generation, phase, closeHash, s.input.operatorCertFile, s.input.operatorKeyFile, gate)
	command.Cancel = func() error { return command.Process.Signal(syscall.SIGTERM) }
	command.WaitDelay = 6 * time.Second
	// Inherit the fixture's ephemeral TMPDIR and never copy the synthetic keys.
	type finished struct {
		out []byte
		err error
	}
	done := make(chan finished, 1)
	go func() {
		out, err := command.CombinedOutput()
		done <- finished{out: out, err: err}
	}()
	if gate != "" {
		for deadline := time.Now().Add(25 * time.Second); ; {
			if _, err := os.Stat(gate + ".ready"); err == nil {
				break
			}
			if time.Now().After(deadline) {
				t.Fatal("same-tab fixture did not render the authorized evidence")
			}
			select {
			case result := <-done:
				t.Fatalf("O09 Chrome %s exited before the authorized render: %v\n%s", phase, result.err, result.out)
			case <-time.After(50 * time.Millisecond):
			}
		}
		if advance != nil {
			advance()
		} else {
			s.setIdentity("abcdef12-0000-4000-8000-000000000091", s.actor)
			s.restart(t)
		}
		if err := os.WriteFile(gate+".revoked", nil, 0600); err != nil {
			t.Fatal(err)
		}
	}
	result := <-done
	out, err := result.out, result.err
	if err != nil {
		t.Fatalf("O09 Chrome %s: %v\n%s", phase, err, out)
	}
	const marker = "O09_PROTECTED_FLEET "
	for _, line := range strings.Split(string(out), "\n") {
		if value, ok := strings.CutPrefix(line, marker); ok {
			var trace struct {
				Phase, RequestKey string
				Writes            int
			}
			if err := json.Unmarshal([]byte(value), &trace); err != nil || trace.Phase != phase {
				t.Fatal("invalid protected Fleet evidence")
			}
			t.Logf("%sphase=%s writes=%d", marker, trace.Phase, trace.Writes)
			return trace.RequestKey
		}
	}
	t.Fatal("missing protected Fleet evidence")
	return ""
}
