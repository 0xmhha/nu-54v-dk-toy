package main

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/service"
	"github.com/0xmhha/nu-54v-dk-toy/products/p05-operations-backoffice/internal/settlement"
)

var printSettings = []string{
	"P05_PRINT_DATABASE_URL", "P05_PRINT_STAFF_DATABASE_URL", "P05_PRINT_STAFF_GRANTS", "P05_PRINT_DEVICE", "P05_PRINT_DEVICE_SHA256",
}

func installPrinter(mux *http.ServeMux, open func(string, string) (*settlement.Store, error), proof *settlement.EvidenceStore) (*settlement.Printer, error) {
	device, err := verifiedSigner("P05_PRINT_DEVICE")
	if err != nil {
		return nil, errors.New("pin printer executable path and SHA-256")
	}
	grants := map[string]service.Principal{}
	if err := configJSON("P05_PRINT_STAFF_GRANTS", &grants); err != nil {
		return nil, err
	}
	if len(grants) == 0 {
		return nil, errors.New("no printer staff identities enrolled")
	}
	for fingerprint, principal := range grants {
		decoded, err := hex.DecodeString(fingerprint)
		if err != nil || len(decoded) != 32 || principal.Role != "printer_staff" || principal.ExpiresAt <= time.Now().Unix() ||
			!targetUUID.MatchString(principal.Subject) || !targetUUID.MatchString(principal.Tenant) {
			return nil, errors.New("invalid enrolled printer staff identity")
		}
	}
	staff, err := open(os.Getenv("P05_PRINT_STAFF_DATABASE_URL"), "nu54_p05_print_staff")
	if err != nil {
		return nil, err
	}
	printer, err := open(os.Getenv("P05_PRINT_DATABASE_URL"), "nu54_p05_print")
	if err != nil {
		return nil, err
	}
	settlement.PrintStaffHTTP{Store: staff, Evidence: proof, Credentials: grants}.Register(mux)
	return &settlement.Printer{Store: printer, Evidence: proof, Device: printProgram{device}}, nil
}

type printProgram struct{ pin processSigner }

func (p printProgram) Print(ctx context.Context, frame settlement.PrintFrame) (settlement.PrintAck, error) {
	var ack settlement.PrintAck
	executable, err := p.pin.material()
	if err != nil {
		return ack, err
	}
	payload, err := json.Marshal(frame)
	if err != nil {
		return ack, err
	}
	output := signerOutput{}
	if err := runPinnedProgram(ctx, executable, bytes.NewReader(append(payload, '\n')), &output, []string{"PATH=/usr/bin:/bin"}); err != nil {
		return ack, err
	}
	parser := json.NewDecoder(&output)
	parser.DisallowUnknownFields()
	if err := parser.Decode(&ack); err != nil || parser.Decode(new(any)) != io.EOF {
		return settlement.PrintAck{}, errors.New("invalid printer acknowledgement")
	}
	return ack, nil
}
