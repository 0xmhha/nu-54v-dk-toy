package commerce

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/rentalv1"
	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/service"
	"github.com/0xmhha/nu-54v-dk-toy/products/p05-operations-backoffice/internal/core"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

func TestVerifyRentalCredentialCurrentBoardAndIssuer(t *testing.T) {
	testVerifyRentalObservation(t, credentialReceiptType)
}

func TestVerifyRentalSlotOpenCurrentBoardAndIssuer(t *testing.T) {
	testVerifyRentalObservation(t, slotOpenReceiptType)
}

func testVerifyRentalObservation(t *testing.T, receiptType string) {
	operator, err := crypto.ToECDSA(common.LeftPadBytes([]byte{9}, 32))
	if err != nil {
		t.Fatal(err)
	}
	board, err := crypto.ToECDSA(common.LeftPadBytes([]byte{13}, 32))
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now().Truncate(time.Second)
	deployment := core.Deployment{ChainID: 8283, Settlement: common.HexToAddress("0x51")}
	profile := common.HexToHash("0x52")
	reader := &RentalAuthorityReader{deployment: deployment}
	request := rentalv1.AuthorityRequest{
		Stream: "00000000-0000-0000-0000-000000000007", StreamGeneration: "00000000-0000-0000-0000-000000000008",
		Branch: "00000000-0000-0000-0000-000000000002", Board: "00000000-0000-0000-0000-000000000003",
		Generation: "1", Challenge: common.HexToHash("0x53").Hex(), CallerFingerprint: "fixture-writer",
	}
	scope := rentalv1.AuthorityResponse{
		AuthorityRequest: request, Registration: common.HexToHash("0x54").Hex(), Measurement: profile.Hex(),
		Deployment: common.HexToHash("0x55").Hex(), ChainID: deployment.ChainID, Settlement: deployment.Settlement.Hex(),
		Account: common.HexToAddress("0x56").Hex(), BoardSigner: crypto.PubkeyToAddress(board.PublicKey).Hex(),
		Operator: crypto.PubkeyToAddress(operator.PublicKey).Hex(), RegisteredAt: start.Unix() - 5,
		ChallengeUntil: start.Unix() + 12, ValidUntil: start.Unix() + 240,
	}
	client := service.VerifiedClient{Fingerprint: [32]byte{1}, ExpiresAt: start.Add(time.Minute)}
	var previous rentalCredentialEvidence
	for _, tc := range []struct {
		name                    string
		known                   bool
		authorityReads, sources int
	}{
		{"unconfigured_source", false, 0, 0},
		{"unqualified_profile", false, 1, 0},
		{"unconfigured_profile", false, 0, 0},
		{"unauthenticated_client", false, 0, 0},
		{"expired_certificate", false, 0, 0},
		{"current_authority", true, 2, 1},
		{"replayed_receipt", false, 1, 1},
		{"foreign_generation", false, 1, 1},
		{"foreign_issuer", false, 1, 1},
		{"issuer_not_board", false, 1, 1},
		{"modified_term", false, 1, 1},
		{"pre_registration_issuance", false, 1, 1},
		{"future_board_check", false, 1, 1},
		{"expired_credential", false, 1, 1},
		{"expired_after_recheck", false, 2, 1},
		{"changed_registration", false, 2, 1},
		{"changed_stream", false, 2, 1},
		{"revoked_authority", false, 2, 1},
		{"source_unavailable", false, 1, 1},
		{"other_receipt_type", false, 1, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			readCount, sourceCount := 0, 0
			currentProfile, currentClient := [32]byte(profile), client
			clock := start
			authority := func(_ context.Context, _ service.VerifiedClient, req rentalv1.AuthorityRequest) (rentalv1.AuthorityResponse, error) {
				if req != request {
					t.Fatal("lookup escaped requested authority")
				}
				readCount++
				current := scope
				if readCount == 2 {
					switch tc.name {
					case "expired_after_recheck":
						clock = start.Add(12 * time.Second)
					case "changed_registration":
						current.Registration = common.HexToHash("0x60").Hex()
					case "changed_stream":
						current.StreamGeneration = "different"
					case "revoked_authority":
						return current, errors.New("authority unavailable")
					}
				}
				return current, nil
			}
			var source rentalCredentialSource = func(_ context.Context, binding [208]byte, challenge [32]byte) (rentalCredentialEvidence, error) {
				sourceCount++
				if challenge == [32]byte(common.HexToHash(request.Challenge)) || challenge == ([32]byte{}) {
					t.Fatal("collector did not receive a separate fresh challenge")
				}
				grant := rentalCredentialEvidence{
					revision: 1, policy: 1, issuedAt: uint64(start.Unix() - 4),
					endsAt: uint64(start.Unix() + 90), checkedAt: uint64(start.Unix()), challenge: challenge,
				}
				issuerKey, boardKey := operator, board
				switch tc.name {
				case "replayed_receipt":
					return previous, nil
				case "source_unavailable":
					return grant, errors.New("source unavailable")
				case "foreign_generation":
					binding[7]++
				case "foreign_issuer":
					issuerKey = board
				case "issuer_not_board":
					boardKey = operator
				case "pre_registration_issuance":
					grant.issuedAt -= 30
				case "future_board_check":
					grant.checkedAt++
				case "expired_credential":
					grant.endsAt = uint64(start.Unix())
				}
				signedType := receiptType
				if tc.name == "other_receipt_type" {
					signedType = credentialReceiptType
					if receiptType == signedType {
						signedType = slotOpenReceiptType
					}
				}
				issuerDigest, boardDigest := grant.receiptDigests(binding, signedType)
				issuerSignature, err := crypto.Sign(issuerDigest[:], issuerKey)
				if err != nil {
					t.Fatal(err)
				}
				boardSignature, err := crypto.Sign(boardDigest[:], boardKey)
				if err != nil {
					t.Fatal(err)
				}
				issuerSignature[64] += 27
				boardSignature[64] += 27
				grant.issuerSignature, grant.boardSignature = [65]byte(issuerSignature), [65]byte(boardSignature)
				if tc.name == "modified_term" {
					grant.endsAt++
				}
				if tc.known {
					previous = grant
				}
				return grant, nil
			}
			switch tc.name {
			case "unconfigured_source":
				source = nil
			case "unqualified_profile":
				currentProfile[0] ^= 1
			case "unconfigured_profile":
				currentProfile = [32]byte{}
			case "unauthenticated_client":
				currentClient.Fingerprint = [32]byte{}
			case "expired_certificate":
				currentClient.ExpiresAt = start
			}
			result, known := reader.verifyRentalObservation(context.Background(), currentClient, request, currentProfile,
				source, authority, func() time.Time { return clock }, receiptType)
			if known != tc.known || readCount != tc.authorityReads || sourceCount != tc.sources {
				t.Fatalf("known=%v reads=%d sources=%d", known, readCount, sourceCount)
			}
			if !known && result != (rentalObservation{}) {
				t.Fatal("unknown result disclosed term")
			}
			if known && result != (rentalObservation{rentalCredentialTerm{endsAt: start.Unix() + 90, validUntil: start.Unix() + 12}, start.Unix()}) {
				t.Fatal("credential term and evidence lifetime were conflated")
			}
		})
	}
}

func TestRentalCredentialDigestMatchesP01(t *testing.T) {
	deployment := core.Deployment{ChainID: 8283, Settlement: common.HexToAddress("0x51")}
	registration, measurement, manifest := common.HexToHash("0x54"), common.HexToHash("0x52"), common.HexToHash("0x55")
	binding := rentalCollectorBinding("00000000-0000-0000-0000-000000000002", "00000000-0000-0000-0000-000000000003",
		1, deployment, common.HexToAddress("0x56").Hex(), registration[:], measurement[:], manifest[:])
	evidence := rentalCredentialEvidence{revision: 1, policy: 1, issuedAt: 1000, endsAt: 1100, checkedAt: 1001, challenge: [32]byte{0x72}}
	issuer, board := evidence.digests(binding)
	// Generated by the two digest functions in nu54_rental_credential.c.
	if issuer != common.HexToHash("0xcb09c907e9ee623fde99c3f4c449b88291c9a91edaf845560250b263103cd058") ||
		board != common.HexToHash("0x1619ead29a4ab4c7dece21216eb009cce1dca9559d7a53b674f6c3a7df544f1f") {
		t.Fatalf("issuer=%s board=%s", issuer, board)
	}
}

func TestRentalCredentialRejectsMissingTLSAuthority(t *testing.T) {
	reader := &RentalAuthorityReader{}
	source := func(context.Context, [208]byte, [32]byte) (rentalCredentialEvidence, error) {
		t.Fatal("requested board evidence without verified TLS authority")
		return rentalCredentialEvidence{}, nil
	}
	if result, known := reader.verifyCredential(context.Background(), nil, rentalv1.AuthorityRequest{}, [32]byte{1}, source); known || result != (rentalCredentialTerm{}) {
		t.Fatal("missing TLS authority returned a credential term")
	}
}
