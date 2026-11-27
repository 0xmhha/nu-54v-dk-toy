package commerce

import (
	"context"
	"encoding/hex"
	"fmt"
	"math/big"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/service"
	"github.com/0xmhha/nu-54v-dk-toy/products/p05-operations-backoffice/internal/core"
	"github.com/ethereum/go-ethereum/common"
)

type RenterV1Issuer struct {
	Tenant, Branch, Terminal, Merchant, Payout string
	ExpiresAt                                  int64
}

type RenterV1Origins struct {
	Deployment     core.Deployment
	Manifest       [32]byte
	Issuers        map[string]RenterV1Issuer
	MerchantStatus func(context.Context, common.Address) (bool, common.Address, error)
	TokenMetadata  func(context.Context, common.Hash) (uint8, string, error)
}

func (v RenterV1Origins) Validate() error {
	if v.Deployment.ChainID != 8283 || v.Deployment.Settlement == (common.Address{}) ||
		v.Deployment.Token == (common.Address{}) || v.Manifest == [32]byte{} || len(v.Issuers) == 0 || v.MerchantStatus == nil {
		return fmt.Errorf("invalid v1 renter origin deployment or terminal authorities")
	}
	for fingerprint, authority := range v.Issuers {
		if !renterV1Hex(fingerprint, 32) || !operatorRef.MatchString(authority.Tenant) ||
			!operatorRef.MatchString(authority.Branch) || !operatorRef.MatchString(authority.Terminal) ||
			!renterAccount.MatchString(authority.Merchant) || !renterAccount.MatchString(authority.Payout) ||
			common.HexToAddress(authority.Merchant) == (common.Address{}) ||
			common.HexToAddress(authority.Payout) == (common.Address{}) || authority.ExpiresAt <= 0 {
			return fmt.Errorf("invalid v1 renter origin terminal authority")
		}
	}
	return nil
}

func renterV1Hex(text string, size int) bool {
	if len(text) != size*2 || strings.ToLower(text) != text {
		return false
	}
	_, err := hex.DecodeString(text)
	return err == nil
}

type renterV1Order struct {
	Account           string `json:"account"`
	OrderKey          string `json:"orderKey"`
	Token             string `json:"token"`
	Amount            string `json:"amount"`
	Expiry            string `json:"expiry"`
	MerchantSignature string `json:"merchantSignature"`
	Origin            string `json:"origin,omitempty"`
	Nonce             string `json:"nonce,omitempty"`
	ApprovalSignature string `json:"approvalSignature,omitempty"`
}

func (order renterV1Order) verify(v *RenterV1Origins, issuer RenterV1Issuer, confirm bool) (merchantDigest, approval, nonce []byte, expiry time.Time, ok bool) {
	amount, validAmount := new(big.Int).SetString(order.Amount, 10)
	seconds, expiryError := strconv.ParseInt(order.Expiry, 10, 64)
	if !renterAccount.MatchString(order.Account) || common.HexToAddress(order.Account) == (common.Address{}) ||
		!strings.HasPrefix(order.OrderKey, "0x") || !renterV1Hex(order.OrderKey[2:], 32) ||
		common.HexToHash(order.OrderKey) == (common.Hash{}) || order.Token != strings.ToLower(v.Deployment.Token.Hex()) ||
		!validAmount || amount.Sign() <= 0 || amount.BitLen() > 256 || amount.String() != order.Amount ||
		expiryError != nil || seconds <= 0 || strconv.FormatInt(seconds, 10) != order.Expiry ||
		!strings.HasPrefix(order.MerchantSignature, "0x") || !renterV1Hex(order.MerchantSignature[2:], 65) {
		return
	}
	expiry = time.Unix(seconds, 0).UTC()
	domain := v.Deployment.Domain()
	fields := map[string]any{"orderId": order.OrderKey, "token": order.Token, "amount": order.Amount, "payout": issuer.Payout, "expiry": order.Expiry}
	merchantDigest, err := core.Digest(domain, "MerchantOrder", fields)
	if err != nil {
		return merchantDigest, nil, nil, expiry, false
	}
	sig, _ := hex.DecodeString(order.MerchantSignature[2:])
	signer, err := core.RecoverSigner(merchantDigest, sig)
	if err != nil || signer != common.HexToAddress(issuer.Merchant) {
		return merchantDigest, nil, nil, expiry, false
	}
	if !confirm {
		return merchantDigest, nil, nil, expiry, order.Origin == "" && order.Nonce == "" && order.ApprovalSignature == ""
	}
	n, validNonce := new(big.Int).SetString(order.Nonce, 10)
	if !operatorRef.MatchString(order.Origin) || !validNonce || n.Sign() < 0 || n.BitLen() > 256 || n.String() != order.Nonce ||
		!strings.HasPrefix(order.ApprovalSignature, "0x") || !renterV1Hex(order.ApprovalSignature[2:], 65) {
		return merchantDigest, nil, nil, expiry, false
	}
	fields["chainId"], fields["contract"], fields["merchant"], fields["nonce"] = strconv.FormatUint(v.Deployment.ChainID, 10), v.Deployment.Settlement.Hex(), issuer.Merchant, order.Nonce
	approval, err = core.Digest(domain, "PaymentAuthorization", fields)
	if err != nil {
		return merchantDigest, nil, nil, expiry, false
	}
	sig, _ = hex.DecodeString(order.ApprovalSignature[2:])
	signer, err = core.RecoverSigner(approval, sig)
	return merchantDigest, approval, n.FillBytes(make([]byte, 32)), expiry, err == nil && signer == common.HexToAddress(order.Account)
}

func (h HTTP) renterV1Origin(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	now := time.Now()
	actor, cert, active := service.CurrentClientPrincipal(r.TLS, h.Credentials, now)
	config := h.RenterV1Origins
	if !active || actor.Role != "terminal" || h.Store == nil || config == nil {
		http.NotFound(w, r)
		return
	}
	issuer, ok := config.Issuers[hex.EncodeToString(cert.Fingerprint[:])]
	if !ok || issuer.ExpiresAt <= now.Unix() || issuer.Tenant != actor.Tenant || issuer.Terminal != actor.Subject {
		http.NotFound(w, r)
		return
	}
	var input renterV1Order
	if !renterBody(w, r, &input) {
		operatorResponse(w, nil, service.BadRequest)
		return
	}
	confirm := r.URL.Path == "/v2/private/renter-v1-orders/confirm"
	digest, approval, nonce, expiry, ok := input.verify(config, issuer, confirm)
	if !ok {
		operatorResponse(w, nil, service.BadRequest)
		return
	}
	merchant, key := common.HexToAddress(issuer.Merchant), common.HexToHash(input.OrderKey)
	if !confirm {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		isMerchant, payout, err := config.MerchantStatus(ctx, merchant)
		cancel()
		if err != nil || !isMerchant || payout != common.HexToAddress(issuer.Payout) {
			http.NotFound(w, r)
			return
		}
	}
	if !cert.ExpiresAt.After(time.Now()) || issuer.ExpiresAt <= time.Now().Unix() ||
		actor.ExpiresAt <= time.Now().Unix() {
		http.NotFound(w, r)
		return
	}
	if confirm {
		var recorded bool
		err := h.Store.db.QueryRow(r.Context(), `SELECT ledger.confirm_renter_v1_order($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`,
			actor.Tenant, issuer.Branch, actor.Subject, input.Origin, input.Account, config.Manifest[:], merchant[:], key[:], digest, approval, nonce).Scan(&recorded)
		if err != nil {
			operatorResponse(w, nil, err)
			return
		}
		if !recorded {
			http.NotFound(w, r)
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"recorded": true})
		return
	}
	var origin *string
	err := h.Store.db.QueryRow(r.Context(), `SELECT ledger.reserve_renter_v1_order($1,$2,$3,$4,$5,$6,$7,$8,$9,$10::text::numeric,$11)::text`,
		actor.Tenant, issuer.Branch, actor.Subject, input.Account, config.Manifest[:], merchant[:], key[:], digest, input.Token, input.Amount, expiry).Scan(&origin)
	if err != nil {
		operatorResponse(w, nil, err)
		return
	}
	if origin == nil {
		http.NotFound(w, r)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"origin": *origin})
}
