package chainview_test

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/subtle"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/0xmhha/nu-54v-dk-toy/products/p07-indexer/internal/chainview"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

func installedW12Credential() bool {
	return os.Getenv("P05_O09_W12_CREDENTIAL") == "1" && (os.Getenv("P05_O09_W12_INSTALLED_RUNNER") != "" || os.Getenv("P05_O09_W12_INSTALL_TIMING") == "1")
}

func w12OrderExpiry(t0 uint64) uint64 {
	if installedW12Credential() {
		return t0 + 175
	}
	return t0 + 120
}

func w12SlotOpenFixture() bool {
	return os.Getenv("P05_O09_W12_SLOT_OPEN") == "1"
}

type w12PublicCredentialTerm struct {
	CredentialExpiresAt, CredentialEvidenceUntil string
	SlotCheckedAt, SlotEvidenceUntil             string
}

type w12CredentialFixture struct {
	endpoint, token, audienceCert, audienceKey string
	frozen                                     atomic.Bool
	slotIssued, slotRefused                    atomic.Uint32
	calls, issued, refused                     atomic.Uint32
	observed                                   atomic.Pointer[w12PublicCredentialTerm]
}

func startW12CredentialFixture(t *testing.T, projection []byte, registeredAt uint64,
	operator, board *ecdsa.PrivateKey, audienceCert, audienceKey string) *w12CredentialFixture {
	t.Helper()
	program := os.Getenv("P05_O09_W12_CREDENTIAL_BOARD")
	if program == "" || len(projection) < 244 {
		t.Fatal("W12 credential mode requires the compiled P01 C host")
	}
	var token [32]byte
	if _, err := rand.Read(token[:]); err != nil {
		t.Fatal(err)
	}
	f := &w12CredentialFixture{token: hex.EncodeToString(token[:]), audienceCert: audienceCert, audienceKey: audienceKey}
	binding := [208]byte(projection[4:212])
	first := [32]byte(projection[212:244])
	ends := registeredAt + 153
	var issueOnce sync.Once
	var issuedAt uint64
	var signature [65]byte
	var issueErr error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost ||
			subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte("Bearer "+f.token)) != 1 {
			http.NotFound(w, r)
			return
		}
		if r.URL.Path == "/observed" {
			var term w12PublicCredentialTerm
			decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 512))
			decoder.DisallowUnknownFields()
			if decoder.Decode(&term) != nil || term.CredentialExpiresAt == "" || term.CredentialEvidenceUntil == "" {
				http.NotFound(w, r)
				return
			}
			f.observed.Store(&term)
			w.WriteHeader(http.StatusNoContent)
			return
		}
		slot := r.URL.Path == "/slot-open" && w12SlotOpenFixture()
		if !slot && r.URL.Path != "/credential" {
			http.NotFound(w, r)
			return
		}
		var request struct {
			Binding   [208]byte
			Challenge [32]byte
		}
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
		decoder.DisallowUnknownFields()
		if decoder.Decode(&request) != nil || request.Binding != binding || request.Challenge == ([32]byte{}) {
			http.NotFound(w, r)
			return
		}
		if !slot {
			f.calls.Add(1)
		}
		frozen := f.frozen.Load()
		// Only the request from a registered authority reaches this fixture.
		issueOnce.Do(func() {
			issuedAt = uint64(time.Now().Unix())
			signature, issueErr = signW12CredentialGrant(binding, issuedAt, ends, operator)
		})
		if issueErr != nil {
			http.NotFound(w, r)
			return
		}
		proof, err := runW12CredentialBoard(r.Context(), program, binding, first, operator, board,
			signature, request.Challenge, issuedAt, ends, frozen, slot)
		if err != nil {
			var returned *exec.ExitError
			if frozen && errors.As(err, &returned) && returned.ExitCode() == 2 {
				if slot {
					f.slotRefused.Add(1)
				} else {
					f.refused.Add(1)
				}
			}
			http.NotFound(w, r)
			return
		}
		if slot {
			f.slotIssued.Add(1)
		} else {
			f.issued.Add(1)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		_ = json.NewEncoder(w).Encode(proof)
	}))
	t.Cleanup(server.Close)
	f.endpoint = server.URL
	return f
}

type w12CredentialProof struct {
	Issued, Ends, Checked uint64
	Challenge             [32]byte
	Issuer, Board         [65]byte
}

func runW12CredentialBoard(ctx context.Context, program string, binding [208]byte, first [32]byte,
	operator, board *ecdsa.PrivateKey, signature [65]byte, challenge [32]byte, issued, ends uint64,
	frozen, slot bool) (w12CredentialProof, error) {
	var proof w12CredentialProof
	var input bytes.Buffer
	input.Write(binding[:])
	input.Write(first[:])
	input.Write(crypto.PubkeyToAddress(operator.PublicKey).Bytes())
	input.Write(signature[:])
	input.Write(crypto.FromECDSA(board))
	input.Write(challenge[:])
	input.Write(binary.BigEndian.AppendUint64(nil, issued))
	input.Write(binary.BigEndian.AppendUint64(nil, ends))
	if frozen {
		input.WriteByte(1)
	} else {
		input.WriteByte(0)
	}
	mode := "--w12-credential-fixture"
	if slot {
		mode = "--w12-slot-open-fixture"
	}
	command := exec.CommandContext(ctx, program, mode)
	command.Stdin = &input
	raw, err := command.Output()
	if err != nil {
		return proof, err
	}
	if len(raw) != 73 {
		return proof, errors.New("invalid P01 credential fixture receipt")
	}
	return w12CredentialProof{Issued: issued, Ends: ends, Checked: binary.BigEndian.Uint64(raw[:8]),
		Challenge: challenge, Issuer: signature, Board: [65]byte(raw[8:])}, nil
}

func signW12CredentialGrant(binding [208]byte, issued, ends uint64, operator *ecdsa.PrivateKey) ([65]byte, error) {
	tag := func(s string) []byte { return crypto.Keccak256([]byte(s)) }
	domain := crypto.Keccak256(tag("EIP712Domain(string name,string version,uint256 chainId,address verifyingContract)"),
		tag("NU54 Payment Settlement"), tag("1"), binding[136:168], common.LeftPadBytes(binding[168:188], 32))
	var fields [128]byte
	for i, value := range []uint64{1, 1, issued, ends} {
		binary.BigEndian.PutUint64(fields[i*32+24:], value)
	}
	data := crypto.Keccak256(tag("RentalSlotCredential(bytes32 binding,uint64 revision,uint64 policy,uint64 issuedAt,uint64 expiresAt)"),
		crypto.Keccak256(binding[:]), fields[:])
	signed, err := crypto.Sign(crypto.Keccak256([]byte{0x19, 0x01}, domain, data), operator)
	if err != nil {
		return [65]byte{}, err
	}
	signed[64] += 27
	return [65]byte(signed), nil
}

func (s *combinedGenerationOpsd) verifyW12CredentialHost(t *testing.T, approval []byte, record func(*testing.T)) {
	t.Helper()
	if record == nil {
		t.Fatal("credential host requires the original mixed settlement")
	}
	if s.confirmInstalledCredentialRegistration != nil {
		if until := time.Until(time.Unix(int64(s.input.t0)+31, 0)); until > 0 {
			select {
			case <-time.After(until):
			case <-s.ctx.Done():
				t.Fatal(s.ctx.Err())
			}
		}
		s.confirmInstalledCredentialRegistration(t)
	}
	binding, _, err := chainview.ParseGenerationProjection(s.input.projection)
	if err != nil {
		t.Fatal(err)
	}
	account := strings.ToLower(binding.Account.Hex())
	offer, _ := json.Marshal(map[string]any{"customer": combinedRenter, "generation": s.input.expected.Generation,
		"account": account, "assignmentUntil": time.Unix(int64(s.input.t0)+155, 0)})
	res := s.call(t, s.operator, http.MethodPost, "/v2/operations/renter-handover/"+binding.Branch+"/"+binding.Board,
		"application/json", offer, http.StatusCreated)
	var assignment struct{ Assignment string }
	if json.Unmarshal(res, &assignment) != nil || assignment.Assignment == "" {
		t.Fatal("fixture handover was not issued")
	}
	path := "/v2/renter/returns/" + assignment.Assignment
	s.call(t, s.otherRenter, http.MethodGet, path+"?credential=1", "", nil, http.StatusNotFound)
	accept, _ := json.Marshal(map[string]string{"account": account})
	s.call(t, s.renter, http.MethodPost, path+"/accept", "application/json", accept, http.StatusOK)
	ordinary := s.call(t, s.renter, http.MethodGet, path, "", nil, http.StatusOK)
	if bytes.Contains(ordinary, []byte("credentialExpiresAt")) {
		t.Fatal("legacy read returned credential")
	}
	query := "?credential=1"
	if w12SlotOpenFixture() {
		query += "&slot=1"
	}
	raw := s.call(t, s.renter, http.MethodGet, path+query, "", nil, http.StatusOK)
	observed := s.input.credential.observed.Load()
	var status struct {
		CredentialExpiresAt, CredentialEvidenceUntil time.Time
		Settlement, Slot                             string
		SlotCheckedAt, SlotEvidenceUntil             time.Time
	}
	if json.Unmarshal(raw, &status) != nil || status.Settlement != "unknown" ||
		status.CredentialEvidenceUntil.IsZero() || time.Until(status.CredentialEvidenceUntil) <= 0 ||
		time.Until(status.CredentialEvidenceUntil) > 30*time.Second ||
		!status.CredentialEvidenceUntil.Before(status.CredentialExpiresAt) || s.input.credential.issued.Load() != 1 || observed == nil ||
		observed.CredentialExpiresAt != status.CredentialExpiresAt.Format(time.RFC3339) ||
		observed.CredentialEvidenceUntil != status.CredentialEvidenceUntil.Format(time.RFC3339) {
		t.Fatalf("live W12 credential projection: collection calls=%d C receipts=%d response pair=%t observed pair=%t",
			s.input.credential.calls.Load(), s.input.credential.issued.Load(),
			!status.CredentialEvidenceUntil.IsZero() && !status.CredentialExpiresAt.IsZero(), observed != nil)
	}
	if w12SlotOpenFixture() && (status.Slot != "open" || s.input.credential.slotIssued.Load() != 1 ||
		!status.SlotEvidenceUntil.After(time.Now()) || status.SlotCheckedAt.After(time.Now()) ||
		status.SlotEvidenceUntil.Sub(status.SlotCheckedAt) > 30*time.Second ||
		observed.SlotCheckedAt != status.SlotCheckedAt.Format(time.RFC3339) ||
		observed.SlotEvidenceUntil != status.SlotEvidenceUntil.Format(time.RFC3339)) {
		t.Fatal("live W12 slot evidence missing or invalid")
	}
	if installedW12Credential() {
		// The installed UI reaches the reservation after observing expiry.
		// The direct host runner also enters the original 120-second admission window.
		if until := time.Until(time.Unix(int64(w12OrderExpiry(s.input.t0))-119, 0)); until > 0 {
			select {
			case <-time.After(until):
			case <-s.ctx.Done():
				t.Fatal(s.ctx.Err())
			}
		}
	}
	s.input.credential.frozen.Store(true)
	origin := s.prepareInstalledRenterOriginal(t, approval, record)
	raw = s.call(t, s.renter, http.MethodGet, path+query, "", nil, http.StatusOK)
	if bytes.Contains(raw, []byte("credentialExpiresAt")) || !bytes.Contains(raw, []byte(`"pendingCount":2`)) ||
		s.input.credential.refused.Load() != 1 || !bytes.Contains(raw, []byte(origin)) {
		t.Fatal("frozen board did not preserve settlement separately")
	}
	if w12SlotOpenFixture() && (s.input.credential.slotRefused.Load() != 1 ||
		bytes.Contains(raw, []byte("slotCheckedAt")) || !bytes.Contains(raw, []byte(`"slot":"unconfirmed"`))) {
		t.Fatal("frozen board returned slot evidence")
	}
	s.call(t, s.operator, http.MethodPost, "/v2/operations/renter-handover/"+assignment.Assignment+"/revoke", "", nil, http.StatusNoContent)
	s.call(t, s.renter, http.MethodGet, path+"?credential=1", "", nil, http.StatusNotFound)
}
