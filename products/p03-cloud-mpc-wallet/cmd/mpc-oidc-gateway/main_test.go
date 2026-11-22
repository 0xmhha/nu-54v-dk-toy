package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestPrivateConfigForStartupAndDigest(t *testing.T) {
	dir := t.TempDir()
	lookup := filepath.Join(dir, "lookup")
	configPath := filepath.Join(dir, "bindings.json")
	if err := os.WriteFile(lookup, bytes.Repeat([]byte{1}, 32), 0400); err != nil {
		t.Fatal(err)
	}
	valid := `{"LookupKey":` + strconv.Quote(lookup) + `,"Providers":[{"Identity":{"id":"qa","name":"QA","issuer":"https://issuer.example"},"ClientID":"client"}]}`
	for _, tc := range []struct {
		name, content string
		mode          os.FileMode
	}{
		{"public", valid, 0644},
		{"null", "null", 0600},
		{"invalid utf8", valid[:len(valid)-1] + `,"Origin":"` + string([]byte{0xff}) + `"}`, 0600},
		{"unknown", valid[:len(valid)-1] + `,"unrecognized":"secret-marker"}`, 0600},
		{"oversized", valid + strings.Repeat(" ", 1<<20) + "secret-marker", 0600},
		{"trailing", valid + ` {"secret-marker":true}`, 0600},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := os.WriteFile(configPath, []byte(tc.content), 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.Chmod(configPath, tc.mode); err != nil {
				t.Fatal(err)
			}
			for _, args := range [][]string{{"-config", configPath}, {"-config", configPath, "-subject-digest", "qa"}} {
				var out bytes.Buffer
				err := run(args, strings.NewReader("external-subject"), &out)
				if err == nil || err.Error() != "registered identity configuration unavailable" || out.Len() != 0 {
					t.Fatalf("unsafe config accepted or disclosed: err=%v output=%q", err, out.String())
				}
			}
		})
	}
	if err := os.Chmod(configPath, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, []byte(valid), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(configPath, 0400); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	err := run([]string{"-config", configPath, "-subject-digest", "qa"}, strings.NewReader("external-subject\n"), &out)
	if err != nil || len(strings.TrimSpace(out.String())) != 64 {
		t.Fatalf("read-only config digest failed: %v", err)
	}
}

func TestGatewayRejectsUnsafeKeysBeforeUsingThem(t *testing.T) {
	dir := t.TempDir()
	lookup, seed, path := filepath.Join(dir, "lookup"), filepath.Join(dir, "seed"), filepath.Join(dir, "config")
	for _, key := range []string{lookup, seed} {
		if err := os.WriteFile(key, bytes.Repeat([]byte{42}, 32), 0600); err != nil {
			t.Fatal(err)
		}
	}
	raw := `{"LookupKey":` + strconv.Quote(lookup) + `,"Seed":` + strconv.Quote(seed) +
		`,"Providers":[{"Identity":{"id":"qa","issuer":"https://issuer.example"},"ClientID":"fixture"}]}`
	if err := os.WriteFile(path, []byte(raw), 0600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	for _, key := range []string{lookup, seed} {
		if err := os.Chmod(key, 0644); err != nil {
			t.Fatal(err)
		}
		err := run([]string{"-config", path}, strings.NewReader(""), &out)
		if err == nil || err.Error() != "private identity key unavailable" || out.Len() != 0 {
			t.Fatalf("unsafe startup key accepted: %v", err)
		}
		err = run([]string{"-config", path, "-subject-digest", "qa"}, strings.NewReader("subject"), &out)
		if key == lookup && (err == nil || err.Error() != "private identity key unavailable" || out.Len() != 0) {
			t.Fatalf("unsafe digest key accepted: %v", err)
		}
		if key == seed && (err != nil || len(strings.TrimSpace(out.String())) != 64) {
			t.Fatalf("offline digest required the signing key: %v", err)
		}
		if err := os.Chmod(key, 0600); err != nil {
			t.Fatal(err)
		}
	}
}
