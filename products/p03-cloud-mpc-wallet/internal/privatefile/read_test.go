package privatefile

import (
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"
)

func TestReadOwnerOnlySecretMount(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "key")
	link := filepath.Join(dir, "mounted")
	if err := os.WriteFile(path, []byte("fixture"), 0400); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(path, link); err != nil {
		t.Fatal(err)
	}
	if raw, err := Read(link, 7, 7); err != nil || string(raw) != "fixture" {
		t.Fatalf("read-only projected file unavailable: %v", err)
	}
	fifo := filepath.Join(dir, "fifo")
	if err := unix.Mkfifo(fifo, 0600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		path     string
		min, max int64
	}{
		{dir, 1, 32}, {fifo, 1, 32}, {link, 8, 32}, {link, 1, 6}, {filepath.Join(dir, "missing"), 1, 32},
	} {
		if raw, err := Read(tc.path, tc.min, tc.max); err != errUnavailable || raw != nil {
			t.Fatalf("unsafe input accepted: %v", err)
		}
	}
	if err := os.Chmod(path, 0640); err != nil {
		t.Fatal(err)
	}
	if raw, err := Read(link, 1, 32); err != errUnavailable || raw != nil {
		t.Fatalf("group-readable target accepted: %v", err)
	}
}
