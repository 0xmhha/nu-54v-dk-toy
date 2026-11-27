// Standalone Go SDK consumer used by the release smoke.
package main

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	legacy "github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/protocol"
	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/quotev2"
	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/service"
)

func main() {
	input, err := os.ReadFile(os.Args[1])
	must(err)
	// A typed legacy value also ensures the published module retains the v1 entry point.
	_ = legacy.PaymentAuthorization{}
	count := 0
	for _, line := range strings.Split(string(input), "\n") {
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		row := strings.Split(line, "|")
		digest, err := quotev2.Digest(decode(row[3]))
		if row[7] == "" {
			check(err != nil, "malformed unsigned quote accepted")
		} else {
			must(err)
			check(bytes.Equal(digest[:], decode(row[7])), "wrong quote digest")
		}
		now, err := strconv.ParseUint(row[5], 10, 64)
		must(err)
		want, err := strconv.Atoi(row[8])
		must(err)
		got := quotev2.Verify(decode(row[1]), quotev2.Policy{
			Expected: decode(row[2]), MerchantSigner: decode(row[4]), Now: now, Consumed: row[6] == "true",
		})
		check(got == want, "unexpected quote status: "+row[0])
		count++
	}
	check(count >= 31, "missing quote vectors")
	const domain, object = "987e50a6-a874-487c-8d85-1451edacfe99", "03f43389-3f6b-4bc4-a1d7-6cb11af65ccb"
	request := service.Request{Version: 2, Contract: "C7", Operation: "evidence.read",
		RequestID: "23a6f801-e925-42b9-b4fa-05f5d4f11cee", Domain: domain, Object: object, ExpiresAt: 1630,
		Body: map[string]string{"kind": "finalized"}}
	principal := service.Principal{Role: "evidence-reader", Tenant: "80d39491-fa9b-4f35-9c2a-f6a258dd45d6",
		Subject: "0fb9757b-d626-4fb8-b02e-68a9fc8930f0", ExpiresAt: 1800,
		Grants: []service.Grant{{Domain: domain, Object: object, Operations: []string{"C7/evidence.read"}}}}
	if _, err := service.Authorize(request, principal, time.Unix(1600, 0)); err != nil {
		panic(err)
	}
	principal.Grants = nil
	_, err = service.Authorize(request, principal, time.Unix(1600, 0))
	check(service.CodeOf(err) == service.Forbidden, "ungranted C7 request accepted")
	body := bytes.Repeat([]byte("protocol-frame"), 8)
	writer := legacy.FrameWriter{MTU: 23}
	fragments, err := writer.Write(body)
	must(err)
	check(len(fragments) > 1, "fragmented v1 test body required")
	var receiver legacy.Reassembler
	_, err = receiver.Feed(fragments[1])
	check(err != nil, "out-of-order fragment accepted")
	for i, fragment := range fragments {
		value, err := receiver.Feed(fragment)
		must(err)
		if i == len(fragments)-1 {
			check(bytes.Equal(value, body), "fragment restart changes payload")
		} else {
			check(value == nil, "incomplete body released")
		}
	}
	fmt.Printf("%d installed Go quote fixtures, v1 fragments/reset, C7 approval/denial passed\n", count)
}

func decode(value string) []byte {
	bytes, err := hex.DecodeString(value)
	must(err)
	return bytes
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}

func check(condition bool, message string) {
	if !condition {
		panic(message)
	}
}
