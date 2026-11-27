package commerce

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/sha256"
	"crypto/sha512"
	"crypto/x509"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
)

type FirmwareRelease struct {
	Version         string `json:"version"`
	Channel         string `json:"channel"`
	ReleaseEvidence string `json:"releaseEvidence,omitempty"`
	Revision        string `json:"boardRevision"`
	Counter         int    `json:"counter"`
	Measurement     string `json:"measurement"`
	ImageSHA256     string `json:"imageSha256"`
	Image           []byte `json:"-"`
}

func VerifyFirmwareImage(release FirmwareRelease, trusted ed25519.PublicKey) error {
	wire := release.Image
	if len(wire) < 32 || len(wire) > 1024*1024 || binary.LittleEndian.Uint32(wire) != 0x96f3b83d ||
		!sku.MatchString(release.Revision) || len(trusted) != ed25519.PublicKeySize {
		return errors.New("invalid counted MCUboot image")
	}
	header, payload, protected := int(binary.LittleEndian.Uint16(wire[8:])),
		int(binary.LittleEndian.Uint32(wire[12:])), int(binary.LittleEndian.Uint16(wire[10:]))
	ext := header + payload
	end := ext + protected
	if header < 32 || payload == 0 || protected < 8 || end+4 > len(wire) {
		return errors.New("invalid signed image sizes")
	}
	version := fmt.Sprintf("%d.%d.%d", wire[20], wire[21], binary.LittleEndian.Uint16(wire[22:]))
	if build := binary.LittleEndian.Uint32(wire[24:]); build != 0 {
		version += fmt.Sprintf("+%d", build)
	}
	digest := sha256.Sum256(wire[:end])
	archive := sha256.Sum256(wire)
	if release.Version != version || release.Measurement != hex.EncodeToString(digest[:]) || release.ImageSHA256 != hex.EncodeToString(archive[:]) {
		return errors.New("image metadata differs from release")
	}
	verified := parseImageTLV(wire[ext:end], 0x6908)
	counter := verified[0x50]
	if len(counter) != 4 || release.Counter <= 0 || uint64(release.Counter) != uint64(binary.LittleEndian.Uint32(counter)) {
		return errors.New("MCUboot security counter differs")
	}
	tlvSize := int(binary.LittleEndian.Uint16(wire[end+2:]))
	if tlvSize < 4 || end+tlvSize != len(wire) {
		return errors.New("invalid image signature trailer")
	}
	signatures := parseImageTLV(wire[end:], 0x6907)
	signatureDigest := sha512.Sum512(wire[:end])
	spki, err := x509.MarshalPKIXPublicKey(trusted)
	if err != nil {
		return err
	}
	keyDigest := sha512.Sum512(spki)
	if !bytes.Equal(signatures[0x12], signatureDigest[:]) ||
		!bytes.Equal(signatures[0x01], keyDigest[:]) ||
		!ed25519.Verify(trusted, signatureDigest[:], signatures[0x24]) {
		return errors.New("MCUboot release signature differs")
	}
	return nil
}

func parseImageTLV(wire []byte, magic uint16) map[byte][]byte {
	if len(wire) < 4 || binary.LittleEndian.Uint16(wire) != magic || int(binary.LittleEndian.Uint16(wire[2:])) != len(wire) {
		return nil
	}
	result := map[byte][]byte{}
	for offset := 4; offset < len(wire); {
		if len(wire)-offset < 4 {
			return nil
		}
		kind, size := wire[offset], int(binary.LittleEndian.Uint16(wire[offset+2:]))
		if _, duplicate := result[kind]; duplicate || size == 0 || len(wire)-offset-4 < size {
			return nil
		}
		result[kind] = wire[offset+4 : offset+4+size]
		offset += 4 + size
	}
	return result
}

func verifyFirmwareACK(wire, challenge, campaign, board, image, public []byte) error {
	if len(wire) != 180 || len(challenge) != 32 || len(campaign) != 16 || len(board) != 32 || len(image) != 32 ||
		!bytes.Equal(wire[:4], []byte("N54F")) || !bytes.Equal(wire[4:36], challenge) ||
		!bytes.Equal(wire[36:52], campaign) || !bytes.Equal(wire[52:84], board) || !bytes.Equal(wire[84:116], image) {
		return errors.New("firmware acknowledgement scope differs")
	}
	x, y := elliptic.Unmarshal(elliptic.P256(), public)
	if x == nil {
		return errors.New("board identity key not enrolled")
	}
	digest := sha256.Sum256(wire[:116])
	key := ecdsa.PublicKey{Curve: elliptic.P256(), X: x, Y: y}
	if !ecdsa.Verify(&key, digest[:], new(big.Int).SetBytes(wire[116:148]), new(big.Int).SetBytes(wire[148:])) {
		return errors.New("firmware acknowledgement not signed by the enrolled board")
	}
	return nil
}
