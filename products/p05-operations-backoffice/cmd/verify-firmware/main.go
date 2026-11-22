// Command verify-firmware verifies the exact counted image used by the fleet gate.
package main

import (
	"crypto/ed25519"
	"crypto/sha256"
	"crypto/x509"
	"encoding/binary"
	"encoding/pem"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/0xmhha/nu-54v-dk-toy/products/p05-operations-backoffice/internal/commerce"
)

func main() {
	var release commerce.FirmwareRelease
	var public, image string
	flag.StringVar(&public, "key", "", "independently trusted MCUboot public key")
	flag.StringVar(&image, "image", "", "counted signed image")
	flag.StringVar(&release.Version, "version", "", "expected version")
	flag.StringVar(&release.Revision, "board", "", "expected board revision")
	var counter int
	flag.IntVar(&counter, "counter", 0, "expected security counter")
	flag.Parse()
	keyFile, err := os.ReadFile(public)
	if err != nil {
		log.Fatal("MCUboot key unavailable")
	}
	block, _ := pem.Decode(keyFile)
	if block == nil {
		log.Fatal("invalid MCUboot key")
	}
	key, err := x509.ParsePKIXPublicKey(block.Bytes)
	trusted, ok := key.(ed25519.PublicKey)
	if err != nil || !ok {
		log.Fatal("MCUboot release requires Ed25519")
	}
	wire, err := os.ReadFile(image)
	if err != nil || len(wire) < 32 {
		log.Fatal("counted MCUboot image unavailable")
	}
	protected := int(binary.LittleEndian.Uint16(wire[8:])) + int(binary.LittleEndian.Uint16(wire[10:])) +
		int(binary.LittleEndian.Uint32(wire[12:]))
	if protected > len(wire) || protected < 32 || counter < 1 || counter > 65535 {
		log.Fatal("invalid counted image layout")
	}
	release.Counter = counter
	release.Image = wire
	release.Measurement = fmt.Sprintf("%x", sha256.Sum256(wire[:protected]))
	release.ImageSHA256 = fmt.Sprintf("%x", sha256.Sum256(wire))
	if err := commerce.VerifyFirmwareImage(release, trusted); err != nil {
		log.Fatal(err)
	}
	fmt.Println(release.Measurement)
}
