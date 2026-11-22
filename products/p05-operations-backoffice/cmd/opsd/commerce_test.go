package main

import (
	"context"
	"crypto/sha256"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSignerExecUsesVerifiedMaterialAcrossFileReplacement(t *testing.T) {
	path := filepath.Join(t.TempDir(), "candidate")
	first := []byte("#!/bin/sh\nprintf '%s\\n' '" + strings.Repeat("11", 65) + "'\n")
	second := []byte("#!/bin/sh\nprintf '%s\\n' '" + strings.Repeat("22", 65) + "'\n")
	if err := os.WriteFile(path, first, 0o700); err != nil {
		t.Fatal(err)
	}
	signer := processSigner{path, sha256.Sum256(first)}
	verified, err := signer.material()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, second, 0o700); err != nil {
		t.Fatal(err)
	}
	got, err := signVerified(context.Background(), verified, [32]byte{1})
	if err != nil || len(got) != 65 || got[0] != 0x11 {
		t.Fatalf("verified invocation ran replacement bytes: %x %v", got, err)
	}
	if _, err := signer.SignDigest(context.Background(), [32]byte{1}); err == nil {
		t.Fatal("a new invocation accepted a replaced executable")
	}
}
