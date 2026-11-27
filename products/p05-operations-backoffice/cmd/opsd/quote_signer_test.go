package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestEnrolledQuoteSignerKeepsVerifiedBytes(t *testing.T) {
	signature := bytes.Repeat([]byte{0x42}, 65)
	executable := []byte(fmt.Sprintf("#!/bin/sh\nread -r digest\nprintf '%%s\\n' '%x'\n", signature))
	source := filepath.Join(t.TempDir(), "signer")
	if err := os.WriteFile(source, executable, 0700); err != nil {
		t.Fatal(err)
	}
	signer, err := enrollProcessSigner(source, sha256.Sum256(executable))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(signer.Close)
	if err := os.WriteFile(source, []byte("#!/bin/sh\nexit 1\n"), 0700); err != nil {
		t.Fatal(err)
	}
	if untrusted, err := enrollProcessSigner(source, sha256.Sum256(executable)); err == nil {
		untrusted.Close()
		t.Fatal("accepted modified enrollment bytes")
	}
	if mode, err := os.Stat(signer.path); err != nil || mode.Mode().Perm() != 0500 {
		t.Fatalf("snapshot must be read-only and owner-executable: %v, %v", mode, err)
	}
	var running sync.WaitGroup
	for i := range 32 {
		running.Go(func() {
			got, err := signer.SignDigest(context.Background(), [32]byte{byte(i)})
			if err != nil || !bytes.Equal(got, signature) {
				t.Errorf("sign with enrolled bytes %d: %x, %v", i, got, err)
			}
		})
	}
	running.Wait()
	signer.Close()
	if _, err := os.Stat(filepath.Dir(signer.path)); !os.IsNotExist(err) {
		t.Fatalf("snapshot directory survives shutdown: %v", err)
	}
	if strings.HasPrefix(signer.path, filepath.Dir(source)+string(os.PathSeparator)) {
		t.Fatal("snapshot must live outside the original source directory")
	}
}
