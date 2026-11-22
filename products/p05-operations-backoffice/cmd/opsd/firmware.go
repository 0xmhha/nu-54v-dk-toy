package main

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"

	"github.com/0xmhha/nu-54v-dk-toy/products/p05-operations-backoffice/internal/commerce"
)

var firmwareSettings = []string{
	"P05_FOTA_CHANNEL", "P05_FOTA_RELEASES", "P05_FOTA_RELEASE_SIGNER", "P05_FOTA_MCUBOOT_PUBLIC_KEY",
}

var firmwareDigest = regexp.MustCompile(`^[a-f0-9]{64}$`)

type firmwareEnrollment struct {
	Descriptor string `json:"descriptor"`
	Signature  string `json:"signature"`
	Image      string `json:"image"`
}

func readFirmwarePublic(env string) (ed25519.PublicKey, error) {
	path := os.Getenv(env)
	if !filepath.IsAbs(path) {
		return nil, errors.New("firmware trust key path must be absolute")
	}
	material, err := os.ReadFile(path)
	if err != nil {
		return nil, errors.New("firmware trust key unavailable")
	}
	block, remaining := pem.Decode(material)
	if block == nil || block.Type != "PUBLIC KEY" || len(bytes.TrimSpace(remaining)) != 0 {
		return nil, errors.New("invalid firmware trust key")
	}
	parsed, err := x509.ParsePKIXPublicKey(block.Bytes)
	key, ok := parsed.(ed25519.PublicKey)
	if err != nil || !ok {
		return nil, errors.New("firmware trust key is not Ed25519")
	}
	return key, nil
}

func loadFirmwareReleases() (map[string]commerce.FirmwareRelease, error) {
	if !anySetting(firmwareSettings) {
		return nil, nil
	}
	channel := os.Getenv("P05_FOTA_CHANNEL")
	if channel != "official" && channel != "custom" {
		return nil, errors.New("explicit firmware deployment channel is required")
	}
	publisher, err := readFirmwarePublic("P05_FOTA_RELEASE_SIGNER")
	if err != nil {
		return nil, err
	}
	boot, err := readFirmwarePublic("P05_FOTA_MCUBOOT_PUBLIC_KEY")
	if err != nil || bytes.Equal(publisher, boot) {
		return nil, errors.New("firmware release and MCUboot keys must be independently pinned")
	}
	var enrolled []firmwareEnrollment
	if err := configJSON("P05_FOTA_RELEASES", &enrolled); err != nil || len(enrolled) == 0 || len(enrolled) > 25 {
		return nil, errors.New("missing enrolled firmware releases")
	}
	releases := make(map[string]commerce.FirmwareRelease, len(enrolled))
	for _, entry := range enrolled {
		if !filepath.IsAbs(entry.Descriptor) || !filepath.IsAbs(entry.Signature) || !filepath.IsAbs(entry.Image) {
			return nil, errors.New("firmware release inputs require absolute paths")
		}
		statement, err := os.ReadFile(entry.Descriptor)
		if err != nil || len(statement) > 2048 {
			return nil, errors.New("firmware release descriptor unavailable")
		}
		signature, err := os.ReadFile(entry.Signature)
		if err != nil || !ed25519.Verify(publisher, statement, signature) {
			return nil, errors.New("release descriptor not signed by the enrolled publisher")
		}
		decoder := json.NewDecoder(bytes.NewReader(statement))
		decoder.DisallowUnknownFields()
		var release commerce.FirmwareRelease
		if decoder.Decode(&release) != nil || decoder.Decode(new(any)) != io.EOF || release.Channel != channel ||
			(channel == "official" && !firmwareDigest.MatchString(release.ReleaseEvidence)) {
			return nil, errors.New("invalid signed firmware release descriptor")
		}
		image, err := os.Open(entry.Image)
		if err != nil {
			return nil, errors.New("signed firmware image unavailable")
		}
		release.Image, err = io.ReadAll(io.LimitReader(image, 1+1024*1024))
		_ = image.Close()
		if err != nil || commerce.VerifyFirmwareImage(release, boot) != nil {
			return nil, errors.New("release image differs from enrolled signed firmware")
		}
		hash := sha256.Sum256(statement)
		releases[hex.EncodeToString(hash[:])] = release
	}
	return releases, nil
}
