package ceremony_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func (m *mobileTest) iosRun(t *testing.T, mode string, roster []byte) {
	t.Helper()
	cmd := exec.Command("xcrun", "simctl", "launch", "--terminate-running-process", m.serial, m.bundle)
	cmd.Env = append(os.Environ(), "SIMCTL_CHILD_NU54_P03_SIMULATOR_MODE="+mode, "SIMCTL_CHILD_NU54_P03_SIMULATOR_SUBJECT="+m.subject,
		"SIMCTL_CHILD_NU54_P03_SIMULATOR_ROSTER="+base64.StdEncoding.EncodeToString(roster))
	if err := cmd.Run(); err != nil {
		t.Fatalf("iOS P03 fixture could not launch: %v", err)
	}
}

func (m *mobileTest) report(t *testing.T, name, expected string) []byte {
	t.Helper()
	path := filepath.Join(m.container, "Documents", "p03-"+m.subject+"."+name)
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		if name == "csr" {
			status, err := os.ReadFile(filepath.Join(m.container, "Documents", "p03-"+m.subject+".status"))
			if err == nil && strings.HasPrefix(string(status), "failed") {
				t.Fatalf("iOS participant could not prepare its local identity (%s)", status)
			}
		}
		value, err := os.ReadFile(path)
		if err == nil && len(value) > 0 {
			if bytes.HasPrefix(value, []byte("failed")) {
				t.Fatalf("iOS participant rejected fixture (%s)", value)
			}
			if expected == "" || string(value) == expected {
				return value
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("iOS participant report %s timed out", name)
	return nil
}

func (m *mobileTest) iosPrepare(t *testing.T) {
	t.Helper()
	m.bundle = os.Getenv("NU54_P03_IOS_BUNDLE")
	expected := "org.nu54.p03fixture"
	if os.Getenv("P03_TEST_MARKET_PROFILE") != "" {
		expected = "org.nu54.p03marketfixture"
	}
	if m.bundle != expected {
		t.Fatal("dedicated P03 iOS bundle is required")
	}
	cmd := exec.Command("xcrun", "simctl", "get_app_container", m.serial, m.bundle, "data")
	raw, err := cmd.Output()
	if err != nil {
		t.Fatal("iOS fixture app container unavailable")
	}
	m.container = strings.TrimSpace(string(raw))
	if !filepath.IsAbs(m.container) {
		t.Fatal("invalid iOS fixture container")
	}
	t.Cleanup(func() { _ = exec.Command("xcrun", "simctl", "terminate", m.serial, m.bundle).Run() })
	m.iosRun(t, "prepare", nil)
	m.csr = string(m.report(t, "csr", ""))
}

func (m *mobileTest) iosServe(t *testing.T, roster []byte) {
	t.Helper()
	m.iosRun(t, "serve", roster)
	m.report(t, "status", "ready")
	m.restart = func() {
		t.Helper()
		var profile struct{ API, Identity string }
		encoded := os.Getenv("NU54_P03_IOS_PROFILE_B64")
		raw, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil || json.Unmarshal(raw, &profile) != nil || profile.API == "" || profile.Identity == "" {
			t.Fatal("signed fixture app profile is required")
		}
		digest := sha256.Sum256([]byte(profile.API + "|" + profile.Identity + "|" + m.subject))
		root := filepath.Join(m.container, "Library", "Application Support", "cloud-p03-participant-"+hex.EncodeToString(digest[:]))
		if info, err := os.Stat(filepath.Join(root, "device-tls.sealed")); err != nil || info.Mode().Perm() != 0600 || info.Size() != 60 {
			t.Fatal("iOS local TLS identity was not sealed")
		}
		if _, err := os.Stat(filepath.Join(root, "tls.key")); !os.IsNotExist(err) {
			t.Fatal("iOS plaintext TLS key appeared")
		}

		path := filepath.Join(m.container, "Documents", "p03-"+m.subject+".stop")
		if err := os.WriteFile(path, []byte("stop"), 0600); err != nil {
			t.Fatal("iOS fixture stop request failed")
		}
		m.report(t, "status", "stopped")
		m.iosRun(t, "serve", nil)
		m.report(t, "status", "ready")
	}
}
