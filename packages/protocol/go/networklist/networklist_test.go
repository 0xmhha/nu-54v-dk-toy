package networklist

import (
	"encoding/hex"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

var now = time.Unix(1_800_000_000, 0)

func vectors(t *testing.T) map[string]string {
	t.Helper()
	raw, err := os.ReadFile("../../v2/testdata/network-list.vectors")
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		key, value, _ := strings.Cut(line, "=")
		out[key] = value
	}
	return out
}

// A synthetic key; it signs nothing outside these tests.
func seal(t *testing.T, payload string) []byte {
	t.Helper()
	key, err := crypto.ToECDSA(crypto.Keccak256([]byte("nu54 network list test key")))
	if err != nil {
		t.Fatal(err)
	}
	digest := Digest(payload)
	signature, err := crypto.Sign(digest[:], key)
	if err != nil {
		t.Fatal(err)
	}
	signature[64] += 27
	raw, err := json.Marshal(Envelope{payload, "0x" + hex.EncodeToString(signature)})
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestReadsTheSharedVector(t *testing.T) {
	v := vectors(t)
	operator := common.HexToAddress(v["operator"])
	raw, _ := json.Marshal(Envelope{v["payload"], v["signature"]})
	_, list, err := Read(raw, operator, now)
	if err != nil || list.Sequence != 7 || len(list.Networks) != 2 || len(list.Networks[0].Tokens) != 1 {
		t.Fatalf("%+v %v", list, err)
	}
	if string(seal(t, v["payload"])) != string(raw) {
		t.Fatalf("signature differs: %s", seal(t, v["payload"]))
	}
	if digest := Digest(v["payload"]); digest.Hex() != v["digest"] {
		t.Fatalf("digest differs: %s", digest.Hex())
	}
	if _, _, err := Read(raw, common.HexToAddress("0x1"), now); err != errSignature {
		t.Fatalf("another operator: %v", err)
	}
	if _, _, err := Read(raw, operator, time.Unix(list.ExpiresAt, 0)); err != errTime {
		t.Fatalf("expired: %v", err)
	}
	if _, _, err := Read(raw, operator, time.Unix(list.IssuedAt-301, 0)); err != errTime {
		t.Fatalf("not issued yet: %v", err)
	}
	tampered, _ := json.Marshal(Envelope{strings.Replace(v["payload"], "api.test", "evil.test", 1), v["signature"]})
	if _, _, err := Read(tampered, operator, now); err != errSignature {
		t.Fatalf("tampered: %v", err)
	}
}

func TestRefusesSignedButInvalidLists(t *testing.T) {
	v := vectors(t)
	operator := common.HexToAddress(v["operator"])
	for name, change := range map[string][2]string{
		"http":           {`"rpc":"https://`, `"rpc":"http://`},
		"query":          {`network/"`, `network/?key=1"`},
		"genesis":        {`"v":1`, `"v":1,"genesis":"0x00"`},
		"anchor in row":  {`"name":"StableNet Testnet"`, `"name":"StableNet Testnet","anchor":"0x00"`},
		"duplicate key":  {`"v":1`, `"v":1,"v":1`},
		"layout":         {`"v":1`, `"v": 1`},
		"two names":      {`"id":"bitcoin-signet"`, `"chainId":1,"id":"bitcoin-signet"`},
		"same network":   {`"id":"bitcoin-signet"`, `"chainId":8283`},
		"token decimals": {`"decimals":6`, `"decimals":25`},
		"token address":  {`0x00000000000000000000000000000000000000a1`, `0x00000000000000000000000000000000000000A1`},
		"sequence":       {`"sequence":7`, `"sequence":0`},
		"validity":       {`"expiresAt":1802592000`, `"expiresAt":2802592000`},
		"hidden name":    {`StableNet Testnet`, "Stable\u202eNet"},
	} {
		changed := strings.Replace(v["payload"], change[0], change[1], 1)
		if changed == v["payload"] {
			t.Fatalf("%s changes nothing", name)
		}
		if _, _, err := Read(seal(t, changed), operator, now); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
}
