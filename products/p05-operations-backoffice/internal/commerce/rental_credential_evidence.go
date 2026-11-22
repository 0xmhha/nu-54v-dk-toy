package commerce

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/binary"
	"math/big"
	"strconv"
	"time"

	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/rentalv1"
	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/service"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

const (
	rentalCredentialChallengeLifetime = 30 * time.Second
	credentialReceiptType             = "RentalSlotCredentialReceipt(bytes32 credential,bytes32 challenge,uint64 checkedAt)"
	slotOpenReceiptType               = "RentalSlotOpenReceipt(bytes32 credential,bytes32 challenge,uint64 checkedAt)"
)

// Internal to the private collector exchange. Neither signature is persisted.
type rentalCredentialEvidence struct {
	revision, policy, issuedAt, endsAt, checkedAt uint64
	challenge                                     [32]byte
	issuerSignature, boardSignature               [65]byte
}

type rentalCredentialTerm struct {
	endsAt, validUntil int64
}

type rentalCredentialSource func(context.Context, [208]byte, [32]byte) (rentalCredentialEvidence, error)
type rentalCredentialAuthority func(context.Context, service.VerifiedClient, rentalv1.AuthorityRequest) (rentalv1.AuthorityResponse, error)

// The deployment pin identifies firmware enforcing the protected credential
// policy. Authority is derived from the current verified TLS client chain.
func (r *RentalAuthorityReader) verifyCredential(ctx context.Context, tlsState *tls.ConnectionState,
	request rentalv1.AuthorityRequest, profile [32]byte, source rentalCredentialSource) (rentalCredentialTerm, bool) {
	client, verified := service.CurrentVerifiedClient(tlsState, time.Now())
	if r == nil || !verified {
		return rentalCredentialTerm{}, false
	}
	return r.verifyCredentialUsing(ctx, client, request, profile, source, r.read, time.Now)
}

func (r *RentalAuthorityReader) verifyCredentialUsing(ctx context.Context, client service.VerifiedClient,
	request rentalv1.AuthorityRequest, profile [32]byte, source rentalCredentialSource,
	read rentalCredentialAuthority, now func() time.Time) (rentalCredentialTerm, bool) {
	observation, verified := r.verifyRentalObservation(ctx, client, request, profile, source, read, now, credentialReceiptType)
	return observation.rentalCredentialTerm, verified
}

type rentalObservation struct {
	rentalCredentialTerm
	checkedAt int64
}

func (r *RentalAuthorityReader) verifyRentalObservation(ctx context.Context, client service.VerifiedClient,
	request rentalv1.AuthorityRequest, profile [32]byte, source rentalCredentialSource,
	read rentalCredentialAuthority, now func() time.Time, receiptType string) (rentalObservation, bool) {
	var unknown rentalObservation
	if source == nil || profile == ([32]byte{}) || client.Fingerprint == ([32]byte{}) {
		return unknown, false
	}
	started := now()
	deadline := started.Add(rentalCredentialChallengeLifetime)
	if !client.ExpiresAt.After(started) {
		return unknown, false
	}
	ctx, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()

	current, err := read(ctx, client, request)
	if err != nil || current.AuthorityRequest != request || common.HexToHash(current.Measurement) != profile ||
		current.RegisteredAt <= 0 || current.RegisteredAt > started.Unix() ||
		current.ValidUntil <= started.Unix() || current.ChallengeUntil <= started.Unix() ||
		current.ChainID != r.deployment.ChainID || common.HexToAddress(current.Settlement) != r.deployment.Settlement || ctx.Err() != nil {
		return unknown, false
	}
	generation, err := strconv.ParseInt(current.Generation, 10, 64)
	if err != nil || generation < 1 {
		return unknown, false
	}
	registration, deployment := common.HexToHash(current.Registration), common.HexToHash(current.Deployment)
	if registration == (common.Hash{}) || deployment == (common.Hash{}) {
		return unknown, false
	}
	binding := rentalCollectorBinding(current.Branch, current.Board, generation, r.deployment,
		current.Account, registration[:], profile[:], deployment[:])

	var challenge [32]byte
	if _, err := rand.Read(challenge[:]); err != nil || challenge == ([32]byte{}) ||
		challenge == [32]byte(common.HexToHash(current.Challenge)) {
		return unknown, false
	}
	evidence, err := source(ctx, binding, challenge)
	if err != nil || evidence.challenge != challenge {
		return unknown, false
	}
	verifiedAt := now()
	if !evidence.validAt(current, started.Unix(), verifiedAt.Unix()) || !evidence.signaturesValid(binding, current, receiptType) {
		return unknown, false
	}

	// Another registration, stream, or audience during collection invalidates
	// this result, even if the original receipt still verifies cryptographically.
	latest, err := read(ctx, client, request)
	finished := now()
	validUntil := min(deadline.Unix(), current.ValidUntil, current.ChallengeUntil,
		client.ExpiresAt.Unix(), int64(evidence.endsAt))
	if err != nil || latest != current || ctx.Err() != nil || finished.Before(verifiedAt) || finished.Unix() >= validUntil {
		return unknown, false
	}
	return rentalObservation{rentalCredentialTerm{endsAt: int64(evidence.endsAt), validUntil: validUntil}, int64(evidence.checkedAt)}, true
}

func (e rentalCredentialEvidence) validAt(scope rentalv1.AuthorityResponse, started, observed int64) bool {
	return started > 0 && observed >= started && e.revision == 1 && e.policy == 1 &&
		e.issuedAt >= uint64(scope.RegisteredAt) && e.issuedAt <= e.checkedAt &&
		e.checkedAt >= uint64(started) && e.checkedAt <= uint64(observed) &&
		e.endsAt <= uint64(1<<63-1) && e.endsAt > uint64(observed)
}

func (e rentalCredentialEvidence) digests(binding [208]byte) (common.Hash, common.Hash) {
	return e.receiptDigests(binding, credentialReceiptType)
}

func (e rentalCredentialEvidence) receiptDigests(binding [208]byte, receiptType string) (common.Hash, common.Hash) {
	domainType := crypto.Keccak256([]byte("EIP712Domain(string name,string version,uint256 chainId,address verifyingContract)"))
	domain := crypto.Keccak256(domainType, crypto.Keccak256([]byte("NU54 Payment Settlement")),
		crypto.Keccak256([]byte("1")), binding[136:168], common.LeftPadBytes(binding[168:188], 32))
	credentialType := crypto.Keccak256([]byte("RentalSlotCredential(bytes32 binding,uint64 revision,uint64 policy,uint64 issuedAt,uint64 expiresAt)"))
	var fields [128]byte
	for i, value := range []uint64{e.revision, e.policy, e.issuedAt, e.endsAt} {
		binary.BigEndian.PutUint64(fields[32*i+24:], value)
	}
	body := crypto.Keccak256(credentialType, crypto.Keccak256(binding[:]), fields[:])
	credential := crypto.Keccak256Hash([]byte{0x19, 1}, domain, body)
	receiptTypeHash := crypto.Keccak256([]byte(receiptType))
	var checked [32]byte
	binary.BigEndian.PutUint64(checked[24:], e.checkedAt)
	body = crypto.Keccak256(receiptTypeHash, credential[:], e.challenge[:], checked[:])
	return credential, crypto.Keccak256Hash([]byte{0x19, 1}, domain, body)
}

func (e rentalCredentialEvidence) signaturesValid(binding [208]byte, scope rentalv1.AuthorityResponse, receiptType string) bool {
	issuer, board, account := common.HexToAddress(scope.Operator), common.HexToAddress(scope.BoardSigner), common.HexToAddress(scope.Account)
	if issuer == (common.Address{}) || board == (common.Address{}) || account == (common.Address{}) ||
		issuer == board || board == account || issuer == account {
		return false
	}
	grantDigest, receiptDigest := e.receiptDigests(binding, receiptType)
	return rentalCredentialSignedBy(grantDigest, e.issuerSignature, issuer) &&
		rentalCredentialSignedBy(receiptDigest, e.boardSignature, board)
}

func rentalCredentialSignedBy(digest common.Hash, signature [65]byte, expected common.Address) bool {
	if signature[64] != 27 && signature[64] != 28 {
		return false
	}
	signature[64] -= 27
	if !crypto.ValidateSignatureValues(signature[64], new(big.Int).SetBytes(signature[:32]),
		new(big.Int).SetBytes(signature[32:64]), true) {
		return false
	}
	key, err := crypto.SigToPub(digest[:], signature[:])
	return err == nil && crypto.PubkeyToAddress(*key) == expected
}
