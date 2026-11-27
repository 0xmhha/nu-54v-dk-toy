package commerce

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/sha512"
	"crypto/x509"
	"encoding/binary"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFirmwareProofRejectsOtherCampaignOrBoard(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	body := make([]byte, 180)
	copy(body, "N54F")
	for i := 4; i < 116; i++ {
		body[i] = byte(i)
	}
	hash := sha256.Sum256(body[:116])
	r, s, err := ecdsa.Sign(rand.Reader, key, hash[:])
	if err != nil {
		t.Fatal(err)
	}
	r.FillBytes(body[116:148])
	s.FillBytes(body[148:])
	public := elliptic.Marshal(elliptic.P256(), key.X, key.Y)
	if err := verifyFirmwareACK(body, body[4:36], body[36:52], body[52:84], body[84:116], public); err != nil {
		t.Fatal(err)
	}
	if err := verifyFirmwareACK(body, body[4:36], make([]byte, 16), body[52:84], body[84:116], public); err == nil {
		t.Fatal("accepted proof across campaigns")
	}
	mutation := append([]byte(nil), body...)
	mutation[111] ^= 1
	if err := verifyFirmwareACK(mutation, body[4:36], body[36:52], body[52:84], mutation[84:116], public); err == nil {
		t.Fatal("accepted a forged boot measurement")
	}
}

// syntheticCountedImage exercises the actual verifier with a reproducible
// test-only signing key and a non-executable payload. The NCS CI job supplies
// its independently built candidate through NU54_FOTA_TEST_BUILD.
func syntheticCountedImage(t *testing.T) ([]byte, ed25519.PublicKey) {
	t.Helper()
	seed := sha256.Sum256([]byte("nu54 counted firmware synthetic test key v1"))
	key := ed25519.NewKeyFromSeed(seed[:])
	public := key.Public().(ed25519.PublicKey)
	spki, err := x509.MarshalPKIXPublicKey(public)
	if err != nil {
		t.Fatal(err)
	}
	tlv := func(kind byte, value []byte) []byte {
		header := []byte{kind, 0, 0, 0}
		binary.LittleEndian.PutUint16(header[2:], uint16(len(value)))
		return append(header, value...)
	}
	area := func(magic uint16, records ...[]byte) []byte {
		wire := make([]byte, 4)
		for _, record := range records {
			wire = append(wire, record...)
		}
		binary.LittleEndian.PutUint16(wire, magic)
		binary.LittleEndian.PutUint16(wire[2:], uint16(len(wire)))
		return wire
	}
	protected := area(0x6908, tlv(0x50, []byte{2, 0, 0, 0}))
	payload := []byte("synthetic counted MCUboot payload, never flash")
	wire := make([]byte, 32)
	binary.LittleEndian.PutUint32(wire, 0x96f3b83d)
	binary.LittleEndian.PutUint16(wire[8:], uint16(len(wire)))
	binary.LittleEndian.PutUint16(wire[10:], uint16(len(protected)))
	binary.LittleEndian.PutUint32(wire[12:], uint32(len(payload)))
	wire[21] = 3
	wire = append(wire, payload...)
	wire = append(wire, protected...)
	digest := sha512.Sum512(wire)
	keyDigest := sha512.Sum512(spki)
	signature := ed25519.Sign(key, digest[:])
	return append(wire, area(0x6907, tlv(0x12, digest[:]), tlv(0x01, keyDigest[:]), tlv(0x24, signature))...), public
}

func countedImageFixture(t *testing.T) FirmwareRelease {
	t.Helper()
	var wire []byte
	var public ed25519.PublicKey
	if root := os.Getenv("NU54_FOTA_TEST_BUILD"); root != "" {
		var err error
		wire, err = os.ReadFile(filepath.Join(root, "app/zephyr/zephyr.signed.bin"))
		if err != nil {
			t.Fatal(err)
		}
		pemFile, err := os.ReadFile(filepath.Join(root, "release-public.pem"))
		if err != nil {
			t.Fatal(err)
		}
		block, _ := pem.Decode(pemFile)
		if block == nil {
			t.Fatal("invalid release public key PEM")
		}
		pub, err := x509.ParsePKIXPublicKey(block.Bytes)
		if err != nil {
			t.Fatal(err)
		}
		var ok bool
		public, ok = pub.(ed25519.PublicKey)
		if !ok {
			t.Fatal("expected Ed25519 release public key")
		}
	} else {
		wire, public = syntheticCountedImage(t)
	}
	if len(wire) < 32 {
		t.Fatal("truncated image header")
	}
	header := int(binary.LittleEndian.Uint16(wire[8:]))
	protected := header + int(binary.LittleEndian.Uint16(wire[10:])) + int(binary.LittleEndian.Uint32(wire[12:]))
	if header < 32 || header >= protected || protected > len(wire) {
		t.Fatal("invalid signed image layout")
	}
	release := FirmwareRelease{Version: "0.3.0", Revision: "nu54_rev1", Counter: 2,
		Measurement: fmt.Sprintf("%x", sha256.Sum256(wire[:protected])), ImageSHA256: fmt.Sprintf("%x", sha256.Sum256(wire)), Image: wire}
	if err := VerifyFirmwareImage(release, public); err != nil {
		t.Fatal(err)
	}
	mutated := release
	mutated.Image = append([]byte(nil), release.Image...)
	mutated.Image[header] ^= 1
	mutated.Measurement = fmt.Sprintf("%x", sha256.Sum256(mutated.Image[:protected]))
	mutated.ImageSHA256 = fmt.Sprintf("%x", sha256.Sum256(mutated.Image))
	if VerifyFirmwareImage(mutated, public) == nil {
		t.Fatal("accepted a rehashed image with its old signature")
	}
	mutated = release
	mutated.Counter++
	if VerifyFirmwareImage(mutated, public) == nil {
		t.Fatal("accepted a release with a mismatched security counter")
	}
	return release
}

func TestFirmwareVerifiesCountedReleaseImage(t *testing.T) {
	countedImageFixture(t)
}

func exerciseFirmwareHTTP(t *testing.T, execute func(string, ...any),
	call func(string, string, string, map[string]any, int) map[string]any, branch, foreign, actor, origin string, release FirmwareRelease) {
	t.Helper()
	const device = "c01242c1-3a37-438b-ac38-ef9e3fd23193"
	const otherDevice = "939a8763-5729-4d9a-b2a9-e6c4defdaf00"
	const campaign = "f473f1c3-9da0-4b29-a8d6-8eb7b825601d"
	const second = "fa1c8baf-bc5e-4e1c-a254-32a06bd4cf5a"
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	public := elliptic.Marshal(elliptic.P256(), key.X, key.Y)
	id := sha256.Sum256(public)
	execute(`INSERT INTO ledger.finance_grant VALUES($1,$2,'firmware_manage',now()+interval '1 hour'),
        ($1,$2,'firmware_station',now()+interval '1 hour')`, branch, actor)
	execute(`INSERT INTO ledger.firmware_member VALUES($1,$2,'nu54_rev1',$3,$4,1,'0.2.0',false,now()+interval '1 hour')`,
		branch, device, id[:], public)
	otherBoard, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	otherKey := elliptic.Marshal(elliptic.P256(), otherBoard.X, otherBoard.Y)
	otherID := sha256.Sum256(otherKey)
	execute(`INSERT INTO ledger.firmware_member VALUES($1,$2,'other_board',$3,$4,1,'0.2.0',false,now()+interval '1 hour')`,
		branch, otherDevice, otherID[:], otherKey)
	base := "/v2/operations/fota/" + branch + "/" + device
	call("GET", "/v2/operations/fota/"+foreign+"/"+device, origin, nil, 404)
	start := map[string]any{"campaign": campaign, "release": strings.Repeat("a", 64)}
	call("POST", "/v2/operations/fota/"+branch+"/"+otherDevice+"/start", origin, start, 409)
	call("POST", base+"/start", origin, start, 200)
	call("POST", base+"/start", origin, start, 200)
	ref := map[string]any{"campaign": campaign}
	first := call("POST", base+"/challenge", origin, ref, 200)
	proof := func(command string) string {
		t.Helper()
		binding, err := hex.DecodeString(command)
		if err != nil || len(binding) != 49 || binding[0] != 16 {
			t.Fatal("bad C1 firmware challenge")
		}
		measurement, err := hex.DecodeString(release.Measurement)
		if err != nil {
			t.Fatal(err)
		}
		wire := make([]byte, 180)
		copy(wire, []byte("N54F"))
		copy(wire[4:52], binding[1:])
		copy(wire[52:84], id[:])
		copy(wire[84:116], measurement)
		digest := sha256.Sum256(wire[:116])
		r, s, err := ecdsa.Sign(rand.Reader, key, digest[:])
		if err != nil {
			t.Fatal(err)
		}
		r.FillBytes(wire[116:148])
		s.FillBytes(wire[148:])
		return hex.EncodeToString(wire)
	}
	before := map[string]any{"campaign": campaign, "proof": proof(first["command"].(string))}
	call("POST", base+"/pause", origin, ref, 200)
	call("POST", base+"/ack", origin, before, 409)
	call("POST", base+"/resume", origin, ref, 200)
	current := call("POST", base+"/challenge", origin, ref, 200)
	if current["command"] == first["command"] {
		t.Fatal("resume did not replace challenge")
	}
	call("POST", base+"/ack", origin, before, 404)
	after := map[string]any{"campaign": campaign, "proof": proof(current["command"].(string))}
	now := call("POST", base+"/ack", origin, after, 200)
	if now["installedVersion"] != "0.3.0" || now["state"] != "verified" {
		t.Fatal(now)
	}
	execute("UPDATE ledger.firmware_member SET revision='nu54_rev1' WHERE branch_id=$1 AND device_ref=$2", branch, otherDevice)
	cohort := "/v2/operations/fota/" + branch + "/" + otherDevice
	call("POST", cohort+"/start", origin, start, 200)
	call("POST", cohort+"/pause", origin, ref, 200)
	call("POST", cohort+"/resume", origin, ref, 200)
	if unchanged := call("GET", base, origin, nil, 200)["status"].(map[string]any); unchanged["state"] != "verified" {
		t.Fatal("resuming another cohort target changed the verified one")
	}
	call("POST", cohort+"/ack", origin, after, 404)
	call("POST", base+"/ack", origin, after, 200)
	call("POST", base+"/start", origin, map[string]any{"campaign": second, "release": start["release"]}, 409)
}
