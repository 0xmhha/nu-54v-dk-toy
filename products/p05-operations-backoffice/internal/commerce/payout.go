package commerce

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strings"
	"time"

	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/service"
	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

type payoutProposal struct {
	Request string `json:"requestKey"`
	Address string `json:"address,omitempty"`
	Reason  string `json:"reason,omitempty"`
	Expires int64  `json:"expiresAt,omitempty"`
	Proof   string `json:"proof,omitempty"`
}

func payoutMessage(branch string, profile Profile, proposal payoutProposal, now time.Time) (string, error) {
	if !operatorRef.MatchString(branch) || !operatorRef.MatchString(proposal.Request) || profile.Chain != 8283 ||
		profile.Contract == (common.Address{}) || profile.MerchantID == ([32]byte{}) ||
		!catalogAsset.MatchString(proposal.Address) || common.HexToAddress(proposal.Address) == (common.Address{}) ||
		common.HexToAddress(proposal.Address) == profile.Beneficiary || proposal.Expires <= now.Unix() ||
		proposal.Expires > now.Add(15*time.Minute).Unix() || profile.BeneficiaryVersion == 0 ||
		(proposal.Reason != "custody_rotation" && proposal.Reason != "governance") {
		return "", service.BadRequest
	}
	return fmt.Sprintf("NU54 payout-address ownership v1\nchain %d\nsettlement %s\nmerchant %x\nbranch %s\nrequest %s\nregistered version %d\nold address %s\nnew address %s\nreason %s\nexpires %d",
		profile.Chain, strings.ToLower(profile.Contract.Hex()), profile.MerchantID,
		branch, proposal.Request, profile.BeneficiaryVersion, strings.ToLower(profile.Beneficiary.Hex()),
		proposal.Address, proposal.Reason, proposal.Expires), nil
}

func payoutOwnership(branch string, profile Profile, proposal payoutProposal, now time.Time) error {
	message, err := payoutMessage(branch, profile, proposal, now)
	if err != nil {
		return err
	}
	proof, err := hex.DecodeString(strings.TrimPrefix(proposal.Proof, "0x"))
	if err != nil || len(proof) != 65 || (proof[64] != 27 && proof[64] != 28) {
		return service.BadRequest
	}
	proof[64] -= 27
	if !crypto.ValidateSignatureValues(proof[64], new(big.Int).SetBytes(proof[:32]), new(big.Int).SetBytes(proof[32:64]), true) {
		return service.BadRequest
	}
	owner, err := crypto.SigToPub(accounts.TextHash([]byte(message)), proof)
	if err != nil || crypto.PubkeyToAddress(*owner) != common.HexToAddress(proposal.Address) {
		return service.Forbidden
	}
	return nil
}

func (h HTTP) registerPayout(mux *http.ServeMux) {
	mux.HandleFunc("GET /v2/operations/payout/{branch}", h.operatePayout)
	mux.HandleFunc("POST /v2/operations/payout/{branch}/{operation}", h.operatePayout)
}

func (h HTTP) operatePayout(w http.ResponseWriter, r *http.Request) {
	actor, allowed := h.operatorCredential(r)
	branch := r.PathValue("branch")
	if !allowed || h.Store == nil || !operatorRef.MatchString(branch) {
		http.NotFound(w, r)
		return
	}
	ctx := r.Context()
	var result json.RawMessage
	if r.Method == http.MethodGet {
		key := r.URL.Query().Get("requestKey")
		if key != "" && !operatorRef.MatchString(key) {
			operatorResponse(w, nil, service.BadRequest)
			return
		}
		var ref *string
		if key != "" {
			ref = &key
		}
		err := h.Store.db.QueryRow(ctx, "SELECT ledger.payout_state($1,$2,$3,$4)", actor.Tenant, branch, actor.Subject, ref).Scan(&result)
		h.payoutResponse(w, branch, result, err)
		return
	}
	if h.OperatorOrigin == "" || r.Header.Get("Origin") != h.OperatorOrigin || r.Header.Get("Content-Type") != "application/json" {
		http.NotFound(w, r)
		return
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
	decoder.DisallowUnknownFields()
	var proposal payoutProposal
	if decoder.Decode(&proposal) != nil || decoder.Decode(new(any)) != io.EOF || !operatorRef.MatchString(proposal.Request) {
		operatorResponse(w, nil, service.BadRequest)
		return
	}
	var err error
	switch r.PathValue("operation") {
	case "challenge", "propose":
		if _, err = h.Store.db.Exec(ctx, "SELECT ledger.payout_scope($1,$2,$3,ARRAY['payout_propose'])", actor.Tenant, branch, actor.Subject); err != nil {
			operatorResponse(w, nil, err)
			return
		}
		profile, err := h.currentProfile(ctx, branch)
		if err == nil && profile.Escrow {
			err = service.Capability // the payout worker speaks only to SettlementV2
		}
		if err != nil {
			operatorResponse(w, nil, err)
			return
		}
		now := time.Now()
		if r.PathValue("operation") == "challenge" {
			if proposal.Proof != "" {
				operatorResponse(w, nil, service.BadRequest)
				return
			}
			message, err := payoutMessage(branch, profile, proposal, now)
			if err != nil {
				operatorResponse(w, nil, err)
				return
			}
			writeJSON(w, http.StatusOK, map[string]string{"message": message, "requestKey": proposal.Request})
			return
		}
		if err = payoutOwnership(branch, profile, proposal, now); err != nil {
			operatorResponse(w, nil, err)
			return
		}
		// Normalize proof spelling before binding the exact ownership evidence.
		decoded, _ := hex.DecodeString(strings.TrimPrefix(proposal.Proof, "0x"))
		evidence := sha256.Sum256(decoded)
		digest := profile.Digest()
		err = h.Store.db.QueryRow(ctx, "SELECT ledger.propose_payout($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)",
			actor.Tenant, branch, actor.Subject, proposal.Request, proposal.Reason, profile.Beneficiary[:],
			common.HexToAddress(proposal.Address).Bytes(), profile.BeneficiaryVersion, digest[:], evidence[:]).Scan(&result)
		h.payoutResponse(w, branch, result, err)
		return
	case "approve", "acknowledge":
		if proposal.Address != "" || proposal.Expires != 0 || proposal.Reason != "" || proposal.Proof != "" {
			operatorResponse(w, nil, service.BadRequest)
			return
		}
		statement := "SELECT ledger.approve_payout($1,$2,$3,$4)"
		if r.PathValue("operation") == "acknowledge" {
			statement = "SELECT ledger.acknowledge_payout_notice($1,$2,$3,$4)"
		}
		err = h.Store.db.QueryRow(ctx, statement, actor.Tenant, branch, actor.Subject, proposal.Request).Scan(&result)
	default:
		http.NotFound(w, r)
		return
	}
	h.payoutResponse(w, branch, result, err)
}

func (h HTTP) payoutResponse(w http.ResponseWriter, branch string, raw json.RawMessage, err error) {
	if err != nil {
		operatorResponse(w, nil, err)
		return
	}
	// The original enrolled version has no payout-change row. A finalized
	// rotation already supplied the registry values in the scoped SQL snapshot.
	var response map[string]json.RawMessage
	if err := json.Unmarshal(raw, &response); err != nil {
		operatorResponse(w, nil, err)
		return
	}
	var version uint64
	if encoded := response["currentVersion"]; len(encoded) != 0 {
		if err := json.Unmarshal(encoded, &version); err != nil {
			operatorResponse(w, nil, err)
			return
		}
	}
	if profile, enrolled := h.Profiles[branch]; enrolled && version == 0 {
		response["currentAddress"], _ = json.Marshal(strings.ToLower(profile.Beneficiary.Hex()))
		response["currentVersion"], _ = json.Marshal(profile.BeneficiaryVersion)
	}
	writeJSON(w, http.StatusOK, response)
}

func (h HTTP) currentProfile(ctx context.Context, branch string) (Profile, error) {
	profile, ok := h.Profiles[branch]
	if !ok {
		return Profile{}, service.Forbidden
	}
	var address []byte
	var version *uint64
	err := h.Store.db.QueryRow(ctx, "SELECT address,version FROM ledger.active_payout($1)", branch).Scan(&address, &version)
	if err != nil {
		return Profile{}, domainError(err)
	}
	if version != nil && *version >= profile.BeneficiaryVersion {
		profile.Beneficiary = common.BytesToAddress(address)
		profile.BeneficiaryVersion = *version
	}
	return profile, nil
}
