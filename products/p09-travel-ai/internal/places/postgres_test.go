package places

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

func TestPostgresSignedPublication(t *testing.T) {
	readURL, writeURL := os.Getenv("P09_TEST_READ_URL"), os.Getenv("P09_TEST_WRITE_URL")
	if readURL == "" || writeURL == "" {
		t.Skip("uses an isolated P09 test database")
	}
	for _, connection := range []string{readURL, writeURL} {
		config, err := pgx.ParseConfig(connection)
		if err != nil || !strings.HasPrefix(config.Database, "nu54_p09_") ||
			(config.Host != "localhost" && !strings.HasPrefix(config.Host, "/tmp/") && !strings.HasPrefix(config.Host, "/private/tmp/")) {
			t.Fatal("use isolated loopback database named nu54_p09_*")
		}
	}
	ctx := context.Background()
	reader, err := Connect(ctx, readURL, false)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	writer, err := Connect(ctx, writeURL, true)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()
	key, signer, _ := ed25519.GenerateKey(rand.Reader)
	base := `{"version":REV,"provider":"lab","environment":"synthetic","updatedAt":"2026-11-20T00:00:00Z","places":[{"id":"park","region":"jongno","name":"Example park","address":"Trial corner","source":"Synthetic source","updatedAt":"2026-11-20T00:00:00Z","opening":[{"weekday":1,"timezone":"Asia/Seoul","opens":"09:00","closes":"10:00"}]}]}`
	for _, revision := range []string{"1", "2"} {
		doc := []byte(strings.Replace(base, "REV", revision, 1))
		sig := ed25519.Sign(signer, doc)
		if err = reader.Publish(ctx, key, doc, sig, time.Now()); err == nil {
			t.Fatal("read role publishes")
		}
		if err = writer.Publish(ctx, key, doc, sig, time.Now()); err != nil {
			t.Fatal(err)
		}
		if err = writer.Publish(ctx, key, doc, sig, time.Now()); err == nil {
			t.Fatal("replayed revision published")
		}
	}
	reader.Close()
	reader, err = Connect(ctx, readURL, false)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	published, err := reader.Published(ctx, key)
	if err != nil {
		t.Fatal(err)
	}
	if published.Version != 2 || len(published.Places) != 1 || len(published.Places[0].Opening) != 1 ||
		published.Places[0].BranchRef != "" || published.Environment != "synthetic" {
		t.Fatal("invalid published snapshot")
	}
	if _, err = reader.pool.Exec(ctx, `UPDATE travel.place SET name='tampered'`); err == nil {
		t.Fatal("read role mutated rows")
	}
}
