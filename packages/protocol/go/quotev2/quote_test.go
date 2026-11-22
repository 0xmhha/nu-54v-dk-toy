package quotev2

import (
	"bytes"
	"encoding/hex"
	"os"
	"strconv"
	"strings"
	"testing"
)

func TestMerchantQuoteVectors(t *testing.T) {
	raw, err := os.ReadFile("../../v2/testdata/quote.vectors")
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(string(raw), "\n") {
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fixture := strings.Split(line, "|")
		t.Run(fixture[0], func(t *testing.T) {
			decode := func(hexValue string) []byte {
				value, err := hex.DecodeString(hexValue)
				if err != nil {
					t.Fatal(err)
				}
				return value
			}
			unsigned := decode(fixture[3])
			got, err := Digest(unsigned)
			if err == nil {
				fields := make([][]byte, len(widths))
				for i, w := range widths {
					fields[i] = unsigned[(i+1)*32-w : (i+1)*32]
				}
				encoded, encodeError := Encode(fields)
				if encodeError != nil || !bytes.Equal(encoded, unsigned) {
					t.Fatalf("encode failed: %v", encodeError)
				}
			}
			if fixture[7] == "" {
				if err == nil {
					t.Fatal("expected malformed unsigned quote")
				}
			} else if err != nil || hex.EncodeToString(got[:]) != fixture[7] {
				t.Fatalf("digest = %x, %v", got, err)
			}
			now, err := strconv.ParseUint(fixture[5], 10, 64)
			if err != nil {
				t.Fatal(err)
			}
			want, err := strconv.Atoi(fixture[8])
			if err != nil {
				t.Fatal(err)
			}
			status := Verify(decode(fixture[1]), Policy{
				Expected: decode(fixture[2]), MerchantSigner: decode(fixture[4]),
				Now: now, Consumed: fixture[6] == "true",
			})
			if int(status) != want {
				t.Fatalf("status = %v, want %v", status, want)
			}
		})
	}
}
