package market

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"encoding/json"
	"errors"
	"io"
	"math/big"
	"net/http"
	"time"

	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/marketlab"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
)

type Handler struct {
	Chain  *Chain
	Signer *ecdsa.PrivateKey
}

func (h Handler) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/profile", h.profile)
	mux.HandleFunc("GET /v1/wallet/{address}", h.wallet)
	mux.HandleFunc("GET /v1/positions/{id}", h.position)
	mux.HandleFunc("GET /v1/positions/{id}/history", h.history)
	mux.HandleFunc("POST /v1/quotes", h.quote)
	mux.HandleFunc("POST /v1/transactions", h.relay)
	mux.HandleFunc("GET /v1/executions/{hash}/{digest}", h.result)
	mux.HandleFunc("GET /v1/approvals/{hash}/{user}", h.approval)
	return http.TimeoutHandler(mux, 15*time.Second, "Market request timed out")
}

func readJSON(w http.ResponseWriter, r *http.Request, into any) bool {
	body := http.MaxBytesReader(w, r.Body, 8192)
	defer body.Close()
	decoder := json.NewDecoder(body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(into); err != nil || decoder.Decode(new(any)) != io.EOF {
		reply(w, http.StatusBadRequest, map[string]string{"error": "invalid market request"})
		return false
	}
	return true
}

func reply(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(payload)
}

func fail(w http.ResponseWriter, status int, message string) {
	reply(w, status, map[string]string{"error": message})
}

func (h Handler) profile(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	head, err := h.Chain.Head(ctx)
	if err != nil {
		fail(w, 503, "market profile unavailable")
		return
	}
	paused, err := h.Chain.Venue(ctx, head.Hash(), "paused")
	if err != nil {
		fail(w, 503, "market profile unavailable")
		return
	}
	response := map[string]any{"profile": h.Chain.Profile, "time": head.Time, "state": "ready"}
	price, err := h.Chain.Venue(ctx, head.Hash(), "validPrice")
	if err != nil {
		response["state"] = "price-unavailable"
	} else {
		response["price"] = price[1].(*big.Int).String()
		response["priceVersion"] = price[0].(*big.Int).String()
	}
	response["headClock"] = h.Chain.headClock(head.Time)
	if response["headClock"] != "current" && response["state"] == "ready" {
		response["state"] = "head-clock-unavailable"
	}
	if paused[0].(bool) {
		response["state"] = "paused"
	}
	observations := make([]map[string]string, 0, 2)
	for i := int64(0); i < 2; i++ {
		item, err := h.Chain.Venue(ctx, head.Hash(), "observations", big.NewInt(i))
		if err != nil {
			fail(w, 503, "market observations unavailable")
			return
		}
		observations = append(observations, map[string]string{"price": item[0].(*big.Int).String(), "observedAt": item[1].(*big.Int).String()})
	}
	response["observations"] = observations
	reply(w, 200, response)
}

func (h Handler) position(w http.ResponseWriter, r *http.Request) {
	id, err := units(r.PathValue("id"))
	if err != nil || id.Sign() == 0 {
		fail(w, 400, "invalid position")
		return
	}
	head, err := h.Chain.Head(r.Context())
	if err != nil {
		fail(w, 503, "position unavailable")
		return
	}
	pos, err := h.Chain.Position(r.Context(), head.Hash(), id)
	if err != nil || pos.Owner == (common.Address{}) {
		fail(w, 404, "position unavailable")
		return
	}
	response := map[string]any{"position": pos}
	if !pos.Closed {
		equity, err := h.Chain.Venue(r.Context(), head.Hash(), "equity", id)
		if err == nil {
			response["equity"] = equity[0].(*big.Int).String()
			response["maintenance"] = equity[1].(*big.Int).String()
		}
	}
	reply(w, 200, response)
}

func (h Handler) quote(w http.ResponseWriter, r *http.Request) {
	var request QuoteRequest
	if !readJSON(w, r, &request) {
		return
	}
	approval, err := h.Chain.Quote(r.Context(), h.Signer, request)
	if err != nil {
		fail(w, 409, "quote unavailable at current price or requested bounds")
		return
	}
	reply(w, 200, approval)
}

type WalletState struct {
	Nonce          uint64 `json:"nonce"`
	GasPrice       string `json:"gasPrice"`
	GasBalance     string `json:"gasBalance"`
	BaseBalance    string `json:"baseBalance"`
	QuoteBalance   string `json:"quoteBalance"`
	BaseAllowance  string `json:"baseAllowance"`
	QuoteAllowance string `json:"quoteAllowance"`
	Shares         string `json:"shares"`
}

func (c *Chain) Wallet(ctx context.Context, user common.Address) (WalletState, error) {
	head, err := c.Head(ctx)
	if err != nil {
		return WalletState{}, err
	}
	out := WalletState{}
	out.Nonce, err = c.RPC.PendingNonceAt(ctx, user)
	if err != nil {
		return WalletState{}, err
	}
	price, err := c.RPC.SuggestGasPrice(ctx)
	if err != nil {
		return WalletState{}, err
	}
	out.GasPrice = price.String()
	balance, err := c.RPC.BalanceAtHash(ctx, user, head.Hash())
	if err != nil {
		return WalletState{}, err
	}
	out.GasBalance = balance.String()
	for _, token := range []struct {
		address            common.Address
		balance, allowance *string
	}{
		{c.Profile.Base.Address, &out.BaseBalance, &out.BaseAllowance},
		{c.Profile.Quote.Address, &out.QuoteBalance, &out.QuoteAllowance},
	} {
		values, err := c.call(ctx, head.Hash(), token.address, tokenABI, "balanceOf", user)
		if err != nil {
			return WalletState{}, err
		}
		*token.balance = values[0].(*big.Int).String()
		values, err = c.call(ctx, head.Hash(), token.address, tokenABI, "allowance", user, c.Profile.Venue.Address)
		if err != nil {
			return WalletState{}, err
		}
		*token.allowance = values[0].(*big.Int).String()
	}
	values, err := c.Venue(ctx, head.Hash(), "shares", user)
	if err != nil {
		return WalletState{}, err
	}
	out.Shares = values[0].(*big.Int).String()
	return out, nil
}

func (h Handler) wallet(w http.ResponseWriter, r *http.Request) {
	user := r.PathValue("address")
	if !common.IsHexAddress(user) || common.HexToAddress(user) == (common.Address{}) {
		fail(w, 400, "invalid isolated wallet")
		return
	}
	status, err := h.Chain.Wallet(r.Context(), common.HexToAddress(user))
	if err != nil {
		fail(w, 503, "wallet state unavailable")
		return
	}
	reply(w, 200, status)
}

// authorizeRelay distinguishes an existing node-known transaction (readback)
// from a new student broadcast at the current verified oracle.
func (c *Chain) authorizeRelay(ctx context.Context, tx *types.Transaction, offer []byte) (bool, error) {
	to := tx.To()
	if tx.Type() != types.LegacyTxType || tx.ChainId().Cmp(big.NewInt(ChainID)) != 0 || to == nil ||
		tx.Value().Sign() != 0 || tx.Gas() > 1_000_000 || tx.GasPrice().Cmp(big.NewInt(100_000_000_000)) > 0 {
		return false, errors.New("outside isolated transaction profile")
	}
	from, err := types.Sender(types.LatestSignerForChainID(big.NewInt(ChainID)), tx)
	if err != nil {
		return false, err
	}
	data := tx.Data()
	switch *to {
	case c.Profile.Venue.Address:
		method := venueABI.Methods["execute"]
		if len(data) != marketlab.QuoteLength || !bytes.Equal(data[:4], method.ID) ||
			(len(offer) != 0 && !bytes.Equal(data, offer)) {
			return false, errors.New("relay only accepts exact quote execution calldata")
		}
		offer = data
	case c.Profile.Base.Address, c.Profile.Quote.Address:
		method := tokenABI.Methods["approve"]
		if (len(data) != 68 && len(data) != marketlab.BoundAllowanceLength) ||
			!bytes.Equal(data[:4], method.ID) || !bytes.Equal(data[4:16], make([]byte, 12)) ||
			!bytes.Equal(data[16:36], c.Profile.Venue.Address[:]) ||
			new(big.Int).SetBytes(data[36:68]).Cmp(new(big.Int).Exp(big.NewInt(10), big.NewInt(24), nil)) > 0 {
			return false, errors.New("relay only accepts allowances for the pinned market")
		}
	default:
		return false, errors.New("transaction target is outside the market")
	}
	head, err := c.Head(ctx)
	if err != nil {
		return false, err
	}
	for _, role := range []string{"admin", "quoteSigner", "publisherA", "publisherB", "keeper"} {
		values, err := c.Venue(ctx, head.Hash(), role)
		if err != nil || values[0].(common.Address) == from {
			return false, errors.New("operator keys cannot use the student relay")
		}
	}
	known, _, err := c.RPC.TransactionByHash(ctx, tx.Hash())
	if err == nil && known != nil && known.Hash() == tx.Hash() {
		return true, nil
	}
	if err != nil && !errors.Is(err, ethereum.NotFound) {
		return false, err
	}
	if c.headClock(head.Time) != "current" {
		return false, errors.New("isolated chain head is not current")
	}
	if *to != c.Profile.Venue.Address && len(data) != marketlab.BoundAllowanceLength {
		return false, errors.New("unknown legacy allowance cannot be broadcast")
	}
	policy := marketlab.Policy{
		Venue: c.Profile.Venue.Address, Base: c.Profile.Base.Address, Quote: c.Profile.Quote.Address,
		Signer: c.Profile.QuoteSigner, CodeHash: c.Profile.Venue.CodeHash,
		Deployment: c.Profile.BlockHash, Genesis: c.Profile.Genesis,
	}
	terms, err := marketlab.Review(policy, marketlab.Request{
		Deployment: policy.Deployment, User: from, To: *to, Nonce: tx.Nonce(), GasPrice: tx.GasPrice().Uint64(),
		Calldata: data, Offer: offer,
	}, time.Unix(max(int64(head.Time), c.wallTime().Unix()), 0))
	if err != nil || tx.Gas() != terms.Gas {
		return false, errors.New("signature and original terms must match the current pinned market")
	}
	values, err := venueABI.Methods["execute"].Inputs.Unpack(offer[4:])
	if err != nil {
		return false, err
	}
	order, ok := abiOrder(values[0])
	if !ok {
		return false, errors.New("invalid quoted tuple")
	}
	if terms.Allowance != "" || order.Action == 6 || order.Action == 7 {
		used, err := c.Venue(ctx, head.Hash(), "used", order.User, order.Nonce)
		if err != nil || used[0].(bool) {
			return false, errors.New("quoted order already consumed")
		}
	}
	if _, err := c.Venue(ctx, head.Hash(), "preview", order); err != nil {
		return false, errors.New("current isolated market will not accept this quote")
	}
	// Recheck the observed canonical head immediately before dispatch. Once
	// sent, the original transaction remains independently reconcilable.
	latest, err := c.Head(ctx)
	if err != nil || latest.Hash() != head.Hash() || c.headClock(head.Time) != "current" ||
		c.wallTime().Unix() >= int64(terms.Deadline) {
		return false, errors.New("isolated chain head moved before dispatch")
	}
	return false, nil
}

func (h Handler) relay(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		Raw   hexutil.Bytes `json:"raw"`
		Offer hexutil.Bytes `json:"offer,omitempty"`
	}
	if !readJSON(w, r, &payload) {
		return
	}
	var tx types.Transaction
	if len(payload.Raw) > 2048 || len(payload.Offer) > marketlab.QuoteLength || tx.UnmarshalBinary(payload.Raw) != nil {
		fail(w, 400, "transaction is outside the isolated market")
		return
	}
	known, err := h.Chain.authorizeRelay(r.Context(), &tx, payload.Offer)
	if err != nil {
		fail(w, 400, "transaction is outside the isolated market")
		return
	}
	if known {
		reply(w, 202, map[string]any{"txHash": tx.Hash(), "state": "submitted"})
		return
	}
	if err := h.Chain.RPC.SendTransaction(r.Context(), &tx); err != nil {
		reply(w, 202, map[string]any{"txHash": tx.Hash(), "state": "unknown"})
		return
	}
	reply(w, 202, map[string]any{"txHash": tx.Hash(), "state": "submitted"})
}

func (h Handler) result(w http.ResponseWriter, r *http.Request) {
	var hash, digest common.Hash
	if hash.UnmarshalText([]byte(r.PathValue("hash"))) != nil || digest.UnmarshalText([]byte(r.PathValue("digest"))) != nil {
		fail(w, 400, "invalid transaction or quote hash")
		return
	}
	result, err := h.Chain.Result(r.Context(), hash, digest)
	if err != nil {
		fail(w, 409, "could not reconcile the original transaction")
		return
	}
	reply(w, 200, result)
}
