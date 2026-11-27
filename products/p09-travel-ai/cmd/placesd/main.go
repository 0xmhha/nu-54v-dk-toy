package main

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"errors"
	"flag"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
	_ "time/tzdata"

	"github.com/0xmhha/nu-54v-dk-toy/products/p09-travel-ai/internal/places"
	"github.com/0xmhha/nu-54v-dk-toy/products/p09-travel-ai/internal/records"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	database := flag.String("database", "", "database URL (defaults to P09_DATABASE_URL)")
	anchor := flag.String("trust-key", "", "approved signing key file, Ed25519 raw public key in hex")
	install := flag.String("install", "", "signed catalog JSON to publish")
	signature := flag.String("signature", "", "64-byte detached Ed25519 signature of the supplied JSON")
	mapDir := flag.String("map-dir", os.Getenv("P09_MAP_DIR"), "approved offline XYZ raster archive directory")
	mapAttribution := flag.String("map-attribution", os.Getenv("P09_MAP_ATTRIBUTION"), "visible source and license for map")
	mapMin := flag.Int("map-min-zoom", 0, "first zoom in map archive")
	mapMax := flag.Int("map-max-zoom", 17, "last zoom in map archive")
	addr := flag.String("listen", "127.0.0.1:8098", "HTTP address behind the public HTTPS ingress")
	flag.Parse()
	if *database == "" {
		*database = os.Getenv("P09_DATABASE_URL")
	}
	if *database == "" || *anchor == "" {
		return errors.New("use P09_DATABASE_URL and -trust-key")
	}
	raw, err := os.ReadFile(*anchor)
	if err != nil {
		return err
	}
	public, err := hex.DecodeString(strings.TrimSpace(string(raw)))
	if err != nil || len(public) != ed25519.PublicKeySize {
		return errors.New("invalid approved catalog key")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	store, err := places.Connect(ctx, *database, *install != "")
	if err != nil {
		return err
	}
	defer store.Close()
	if *install != "" {
		raw, err := os.ReadFile(*install)
		if err != nil {
			return err
		}
		sig, err := os.ReadFile(*signature)
		if err != nil {
			return err
		}
		return store.Publish(ctx, ed25519.PublicKey(public), raw, sig, time.Now())
	}
	catalog, err := store.Published(ctx, ed25519.PublicKey(public))
	if err != nil {
		return err
	}
	mux := http.NewServeMux()
	catalog.Register(mux)
	if *mapDir != "" {
		root, err := os.OpenRoot(*mapDir)
		if err != nil {
			return err
		}
		defer root.Close()
		if err := places.RegisterMap(mux, root, *mapAttribution, *mapMin, *mapMax); err != nil {
			return err
		}
	}
	if reviewDB := os.Getenv("P09_PUBLIC_REVIEW_DATABASE_URL"); reviewDB != "" {
		key, err := records.LoadKey(os.Getenv("P09_REVIEW_KEY"))
		if err != nil {
			return err
		}
		reader, err := records.OpenReviewReader(ctx, reviewDB, key)
		clear(key)
		if err != nil {
			return err
		}
		defer reader.Close()
		guard, err := reader.RestorePublication(ctx, records.RestorePublicationConfig{
			ControlDSN:      os.Getenv("P09_RECOVERY_DATABASE_URL"),
			ErasureGuardDSN: os.Getenv("P09_LIVE_ERASURE_GUARD_DATABASE_URL"),
			Scope:           os.Getenv("P09_RECOVERY_SCOPE"), Manifest: os.Getenv("P09_RECOVERY_MANIFEST"),
		})
		if err != nil {
			return err
		}
		defer guard.Close()
		reader.RegisterGuarded(mux, guard)
	}
	server := &http.Server{Addr: *addr, Handler: mux, ReadHeaderTimeout: 3 * time.Second, WriteTimeout: 5 * time.Second}
	return server.ListenAndServe()
}
