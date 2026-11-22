package chainview

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const heldStream = "c5489488-1328-4874-a593-98b6292f9b02"
const heldGeneration = "01f4033c-5f65-4104-af39-313530f3bd06"

func verifyIngestCommand(t *testing.T, ctx context.Context, database, endpoint string, owner *pgxpool.Pool, scope Profile, tip int64) {
	t.Helper()
	binary := os.Getenv("P07_CHAINVIEW_TEST_INDEXER")
	if binary == "" {
		t.Fatal("normal fixture must supply a built indexer executable")
	}
	_, restrictedURL := fixtureLogin(t, database, owner, "nu54_p07_ingest")
	scope.GenerationID = "b0cabaf8-23e5-43d8-b73e-362fa0fb14a4"
	dir := t.TempDir()
	profile := filepath.Join(dir, "profiles.json")
	held := scope
	held.StreamID = heldStream
	held.GenerationID = heldGeneration
	held.ABIHash = fmt.Sprintf("%064d", 7)
	held.Manifest = fmt.Sprintf("%064d", 8)
	raw, err := json.Marshal([]Profile{held, scope})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(profile, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	log, err := os.Create(filepath.Join(dir, "producer.log"))
	if err != nil {
		t.Fatal(err)
	}
	defer log.Close()
	cmd := exec.CommandContext(ctx, binary, "chainview")
	cmd.Env = append(os.Environ(), "P07_CHAINVIEW_DATABASE_URL="+restrictedURL,
		"P07_CHAINVIEW_PROFILES="+profile, "P07_RPC="+endpoint, "P07_POLL=25ms")
	cmd.Stderr = log
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	exited := make(chan error, 1)
	go func() { exited <- cmd.Wait() }()
	stopped := false
	defer func() {
		if !stopped {
			_ = cmd.Process.Kill()
			<-exited
		}
	}()
	deadline := time.NewTimer(10 * time.Second)
	defer deadline.Stop()
	tick := time.NewTicker(30 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case err := <-exited:
			stopped = true
			raw, _ := os.ReadFile(log.Name())
			t.Fatalf("normal producer exited before reaching tip %d: %v, %s", tip, err, raw)
		case <-deadline.C:
			raw, _ := os.ReadFile(log.Name())
			t.Fatalf("normal producer did not reach tip %d: %s", tip, raw)
		case <-tick.C:
			var stored *int64
			err := owner.QueryRow(ctx, "SELECT tip_height FROM chainview.generation WHERE id=$1", scope.GenerationID).Scan(&stored)
			if err != nil || stored == nil || *stored != tip {
				continue
			}
			var visible int
			if err := owner.QueryRow(ctx, "SELECT count(*) FROM chainview.finalized_receipt WHERE generation_id=$1",
				scope.GenerationID).Scan(&visible); err != nil || visible != 0 {
				t.Fatal(fmt.Errorf("unapproved command generation became visible: %d, %v", visible, err))
			}
			var reason string
			if err := owner.QueryRow(ctx, "SELECT reason FROM chainview.profile_hold WHERE stream_id=$1 AND generation_id=$2",
				heldStream, heldGeneration).Scan(&reason); err != nil || reason != "unknown_abi" {
				t.Fatalf("held deployment missing from observation: %q %v", reason, err)
			}
			var registered bool
			if err := owner.QueryRow(ctx, "SELECT EXISTS(SELECT FROM chainview.generation WHERE id=$1)", heldGeneration).
				Scan(&registered); err != nil || registered {
				t.Fatalf("unknown deployment acquired a cursor: %v %v", registered, err)
			}
			return
		}
	}
}

func verifyHeldProfileResume(t *testing.T, ctx context.Context, database, endpoint string, owner *pgxpool.Pool, candidate Profile, tip int64) {
	t.Helper()
	_, login := fixtureLogin(t, database, owner, "nu54_p07_ingest")
	candidate.StreamID, candidate.GenerationID, candidate.Manifest = heldStream, heldGeneration, fmt.Sprintf("%064d", 8)
	key, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(candidate)
	if err != nil {
		t.Fatal(err)
	}
	signed := ed25519.Sign(private, append([]byte("NU54-chainview-profile-v1\x00"), body...))
	dir := t.TempDir()
	manifest, approvals := filepath.Join(dir, "profiles.json"), filepath.Join(dir, "signatures.json")
	if err := os.WriteFile(manifest, append(append([]byte("["), body...), ']'), 0o600); err != nil {
		t.Fatal(err)
	}
	observed := func(mode string, expect bool) {
		t.Helper()
		command := exec.CommandContext(ctx, os.Getenv("P07_CHAINVIEW_TEST_INDEXER"), "chainview")
		command.Env = append(os.Environ(), "P07_CHAINVIEW_DATABASE_URL="+login, "P07_CHAINVIEW_PROFILES="+manifest,
			"P07_RPC="+endpoint, "P07_POLL=25ms", "P07_CHAINVIEW_APPROVAL_KEY=", "P07_CHAINVIEW_SIGNATURES=")
		if mode != "unsigned" {
			sig := append([]byte(nil), signed...)
			if mode == "tampered" {
				sig[0] ^= 1
			}
			raw, _ := json.Marshal(map[string]string{candidate.GenerationID: hex.EncodeToString(sig)})
			if err := os.WriteFile(approvals, raw, 0o600); err != nil {
				t.Fatal(err)
			}
			command.Env = append(command.Env, "P07_CHAINVIEW_APPROVAL_KEY="+hex.EncodeToString(key),
				"P07_CHAINVIEW_SIGNATURES="+approvals)
		}
		log, err := os.Create(filepath.Join(dir, mode+".log"))
		if err != nil {
			t.Fatal(err)
		}
		defer log.Close()
		command.Stderr = log
		if err := command.Start(); err != nil {
			t.Fatal(err)
		}
		defer func() { _ = command.Process.Kill(); _ = command.Wait() }()
		end := time.Now().Add(4 * time.Second)
		for time.Now().Before(end) {
			var height *int64
			var held bool
			err := owner.QueryRow(ctx, `SELECT (SELECT tip_height FROM chainview.generation WHERE id=$1),
                EXISTS(SELECT FROM chainview.profile_hold WHERE stream_id=$2 AND generation_id=$1)`,
				candidate.GenerationID, candidate.StreamID).Scan(&height, &held)
			if err != nil {
				t.Fatal(err)
			}
			if expect && height != nil && *height == tip && !held {
				return
			}
			if !expect && (!held || height != nil) {
				t.Fatalf("held profile resumed without authority: %s", mode)
			}
			time.Sleep(40 * time.Millisecond)
		}
		if expect {
			raw, _ := os.ReadFile(log.Name())
			t.Fatalf("verified profile did not resume: %s", raw)
		}
	}
	observed("unsigned", false)
	observed("tampered", false)
	observed("signed", true)
	t.Log("unmapped deployment rejected unsigned and tampered retry; trusted signature restored only its isolated cursor")
}
