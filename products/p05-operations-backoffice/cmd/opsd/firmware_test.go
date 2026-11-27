package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestFirmwareLoadsVerifiedDescriptor(t *testing.T) {
	root := os.Getenv("NU54_FOTA_PACKAGE")
	if root == "" {
		t.Skip("requires independently signed local FOTA descriptor and counted build")
	}
	enrolled := []firmwareEnrollment{{
		Descriptor: filepath.Join(root, "descriptor.json"),
		Signature:  filepath.Join(root, "descriptor.sig"),
		Image:      filepath.Join(root, "build/app/zephyr/zephyr.signed.bin"),
	}}
	wire, err := json.Marshal(enrolled)
	if err != nil {
		t.Fatal(err)
	}
	settings := filepath.Join(t.TempDir(), "firmware.json")
	if err := os.WriteFile(settings, wire, 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("P05_FOTA_CHANNEL", "custom")
	t.Setenv("P05_FOTA_RELEASES", settings)
	t.Setenv("P05_FOTA_RELEASE_SIGNER", filepath.Join(root, "publisher-public.pem"))
	t.Setenv("P05_FOTA_MCUBOOT_PUBLIC_KEY", filepath.Join(root, "build/release-public.pem"))
	releases, err := loadFirmwareReleases()
	if err != nil || len(releases) != 1 {
		t.Fatalf("actual signed firmware package failed: %v", err)
	}
	t.Setenv("P05_FOTA_CHANNEL", "official")
	if _, err := loadFirmwareReleases(); err == nil {
		t.Fatal("accepted custom firmware in official channel")
	}
	t.Setenv("P05_FOTA_CHANNEL", "custom")
	t.Setenv("P05_FOTA_RELEASE_SIGNER", filepath.Join(root, "build/release-public.pem"))
	if _, err := loadFirmwareReleases(); err == nil {
		t.Fatal("allowed shared MCUboot and publisher key")
	}
}
