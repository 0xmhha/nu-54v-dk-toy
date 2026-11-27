package main

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/0xmhha/nu-54v-dk-toy/products/p05-operations-backoffice/internal/ble"
)

func TestCollectorRequiresPrivateEnrollmentBeforeOpeningBLE(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "enrollment.json")
	if err := os.WriteFile(path, []byte(`{"RegisteredPeer":"unqualified"}`), 0644); err != nil {
		t.Fatal(err)
	}
	connect := func(string, bool) (ble.Transport, error) {
		t.Fatal("opened BLE without a complete private enrolled scope")
		return nil, nil
	}
	if err := run(context.Background(), path, "", connect, io.Discard); err == nil {
		t.Fatal("accepted an enrollment readable by other users")
	}
	if err := os.Chmod(path, 0600); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(root, "other.json")
	if err := os.Symlink(path, alias); err != nil {
		t.Fatal(err)
	}
	if err := run(context.Background(), alias, "", connect, io.Discard); err == nil {
		t.Fatal("accepted an enrollment symlink")
	}
	if err := run(context.Background(), path, "", connect, io.Discard); err == nil {
		t.Fatal("accepted an incomplete enrollment")
	}
}
