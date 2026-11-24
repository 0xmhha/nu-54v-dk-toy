package settlement

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math/big"
	"regexp"
	"time"

	"github.com/0xmhha/nu-54v-dk-toy/packages/contracts-abi/go/testusdc3009"
	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/service"
	"github.com/0xmhha/nu-54v-dk-toy/products/p05-operations-backoffice/internal/commerce"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
)

// faucetGas covers one ERC-20 transfer.
const faucetGas = 120_000

// FaucetToken is one test token a store's customers may ask for. Nothing has a default.
type FaucetToken struct {
	Address  common.Address `json:"address"`
	Symbol   string         `json:"symbol"`
	Decimals uint8          `json:"decimals"`
	// Amount is one claim in base units.
	Amount string `json:"amount"`
	// PerDay is how many claims one account or customer may make in any 24 hours.
	PerDay int32 `json:"perDay"`
	// DailyTotal bounds all claims of this token in any 24 hours, in base units.
	DailyTotal string `json:"dailyTotal"`
}

// Faucet sends pre-minted tokens from an account of its own, which also pays the gas.
type Faucet struct {
	Account common.Address
	Signer  Signer
	Tokens  map[string][]FaucetToken // by branch
	// Funds reads what the account can send of one token now. Zero without gas money.
	Funds func(context.Context, common.Address) (*big.Int, error)
}

var faucetSymbol = regexp.MustCompile(`^[A-Za-z0-9.-]{1,12}$`)

func units(value string) (*big.Int, bool) {
	amount, ok := new(big.Int).SetString(value, 10)
	return amount, ok && amount.Sign() > 0 && amount.BitLen() <= 256 && amount.String() == value
}

// CheckFaucet refuses a list that leaves a limit open or names an account with another job.
func CheckFaucet(faucet Faucet, targets map[string]Target, other ...common.Address) error {
	if faucet.Account == (common.Address{}) || faucet.Signer == nil || len(faucet.Tokens) == 0 {
		return errors.New("enroll the faucet account, its signer and its tokens together")
	}
	for _, account := range other {
		if account == faucet.Account {
			return errors.New("the faucet account must not have another job")
		}
	}
	totals := map[common.Address]string{}
	for branch, tokens := range faucet.Tokens {
		target, ok := targets[branch]
		if !ok || !target.Escrow() || len(tokens) < 1 || len(tokens) > 16 {
			return errors.New("faucet tokens need an active order escrow branch")
		}
		seen := map[common.Address]bool{}
		for _, token := range tokens {
			amount, valid := units(token.Amount)
			total, bounded := units(token.DailyTotal)
			if token.Address == (common.Address{}) || token.Address == target.Contract || seen[token.Address] || !faucetSymbol.MatchString(token.Symbol) ||
				token.Decimals > 24 || !valid || !bounded || total.Cmp(amount) < 0 || token.PerDay < 1 || token.PerDay > 100 {
				return errors.New("invalid faucet token")
			}
			seen[token.Address] = true
			if known, again := totals[token.Address]; again && known != token.DailyTotal {
				return errors.New("a faucet token has one daily total")
			}
			totals[token.Address] = token.DailyTotal
		}
	}
	return nil
}

// CheckFaucetTokens compares the list with the chain. The account must not be able to mint.
func CheckFaucetTokens(ctx context.Context, client *ethclient.Client, faucet Faucet) error {
	for _, tokens := range faucet.Tokens {
		for _, token := range tokens {
			caller, err := testusdc3009.NewTestusdc3009Caller(token.Address, client)
			if err != nil {
				return err
			}
			opts := &bind.CallOpts{Context: ctx}
			symbol, err := caller.Symbol(opts)
			if err != nil || symbol != token.Symbol {
				return errors.New("faucet token has another symbol on chain")
			}
			if decimals, err := caller.Decimals(opts); err != nil || decimals != token.Decimals {
				return errors.New("faucet token has other decimals on chain")
			}
			if owner, err := caller.Owner(opts); err != nil || owner == faucet.Account {
				return errors.New("the faucet account must not own its token")
			}
		}
	}
	return nil
}

// FaucetFunds reads the account's balance of a token, or zero when it cannot pay for a transfer.
func FaucetFunds(client *ethclient.Client, account common.Address, maxGasPrice *big.Int) func(context.Context, common.Address) (*big.Int, error) {
	return func(ctx context.Context, token common.Address) (*big.Int, error) {
		price, err := client.SuggestGasPrice(ctx)
		if err != nil {
			return nil, err
		}
		price.Lsh(price, 1)
		if price.Cmp(maxGasPrice) > 0 {
			price.Set(maxGasPrice)
		}
		gas, err := client.BalanceAt(ctx, account, nil)
		if err != nil {
			return nil, err
		}
		if gas.Cmp(price.Mul(price, big.NewInt(faucetGas))) < 0 {
			return new(big.Int), nil
		}
		caller, err := testusdc3009.NewTestusdc3009Caller(token, client)
		if err != nil {
			return nil, err
		}
		return caller.BalanceOf(&bind.CallOpts{Context: ctx}, account)
	}
}

// faucetTokens is the store's list. A store without one has no faucet.
func (f Forwarder) faucetTokens(req service.Request, subject string) ([]FaucetToken, error) {
	if _, err := f.escrowTarget(req, subject); err != nil {
		return nil, err
	}
	if f.Faucet == nil || len(f.Faucet.Tokens[req.Domain]) == 0 {
		return nil, service.Capability
	}
	return f.Faucet.Tokens[req.Domain], nil
}

// ClaimFaucet stores one claim for the relay worker. The receiver is the enrolled account, never the request's.
func (f Forwarder) ClaimFaucet(ctx context.Context, bound service.Bound) error {
	req := bound.Request
	tokens, err := f.faucetTokens(req, bound.Subject)
	if err != nil {
		return err
	}
	wanted := common.HexToAddress(req.Body["token"])
	for _, token := range tokens {
		if token.Address != wanted {
			continue
		}
		intent, err := hex.DecodeString(bound.IntentDigest)
		if err != nil || len(intent) != 32 {
			return service.BadRequest
		}
		var available *string // unknown refuses a new claim and still answers a repeated one
		if funds, err := f.Faucet.Funds(ctx, token.Address); err == nil && funds != nil {
			value := funds.String()
			available = &value
		}
		_, err = f.Store.db.Exec(ctx, `SELECT ledger.store_faucet_claim($1,$2,$3,$4,$5,$6,$7::text::numeric,$8,$9::text::numeric,$10::text::numeric,$11)`,
			bound.Tenant, req.Domain, bound.Subject, req.IdempotencyKey, intent, lower(token.Address), token.Amount, token.PerDay, token.DailyTotal,
			available, lower(f.Faucet.Account))
		return sqlError(err)
	}
	return service.Capability
}

func (f Forwarder) FaucetStatus(ctx context.Context, bound service.Bound) (*commerce.FaucetStatus, error) {
	tokens, err := f.faucetTokens(bound.Request, bound.Subject)
	if err != nil {
		return nil, err
	}
	assets := make([]string, len(tokens))
	for i, token := range tokens {
		assets[i] = lower(token.Address)
	}
	var raw []byte
	err = f.Store.db.QueryRow(ctx, "SELECT ledger.faucet_status($1,$2,$3,$4)", bound.Tenant, bound.Request.Domain, bound.Subject, assets).Scan(&raw)
	if err != nil {
		return nil, sqlError(err)
	}
	var kept struct {
		Payer  string `json:"payer"`
		Tokens map[string]struct {
			Times       []int64               `json:"times"`
			Total, Held string                // base units
			Open        bool                  `json:"open"`
			Claim       *commerce.FaucetClaim `json:"claim"`
		} `json:"tokens"`
	}
	if err := json.Unmarshal(raw, &kept); err != nil {
		return nil, service.Unknown
	}
	view := commerce.FaucetStatus{Payer: kept.Payer}
	for i, token := range tokens {
		have := kept.Tokens[assets[i]]
		amount, _ := units(token.Amount)
		limit, _ := units(token.DailyTotal)
		total, counted := new(big.Int).SetString(have.Total, 10)
		held, known := new(big.Int).SetString(have.Held, 10)
		if !counted || !known {
			return nil, service.Unknown
		}
		row := commerce.FaucetToken{Token: assets[i], Symbol: token.Symbol, Decimals: token.Decimals, Amount: token.Amount, PerDay: token.PerDay,
			Remaining: max(0, token.PerDay-int32(len(have.Times))), State: "ready", Claim: have.Claim}
		funds, err := f.Faucet.Funds(ctx, token.Address)
		if err != nil {
			return nil, service.Busy
		}
		switch {
		case have.Open:
			row.State = "open"
		case row.Remaining == 0:
			row.State = "limit"
			row.AvailableAt = have.Times[token.PerDay-1] + 24*3600 + 1
		case total.Add(total, amount).Cmp(limit) > 0:
			row.State = "limit"
		case funds.Cmp(held.Add(held, amount)) < 0:
			row.State = "empty"
		}
		view.Tokens = append(view.Tokens, row)
	}
	return &view, nil
}

// faucetTransfer is one claim's token transfer. Only its nonce keeps it from running twice, so it is
// signed once and never signed for another nonce.
type faucetTransfer struct {
	token, sender, to common.Address
	amount            *big.Int
}

func (t faucetTransfer) call(id string, expires *time.Time, now time.Time) (relayCall, error) {
	token, err := testusdc3009.Testusdc3009MetaData.GetAbi()
	if err != nil {
		return relayCall{}, err
	}
	data, err := token.Pack("transfer", t.to, t.amount)
	// A transfer has no deadline of its own. The same bytes are sent until the chain takes the nonce.
	expiry := now.Truncate(time.Second).AddDate(10, 0, 0).UTC()
	if expires != nil {
		expiry = *expires
	}
	return relayCall{expiry, sha256.Sum256([]byte("nu54-faucet-v1:" + id)), data}, err
}

// moved reports whether a successful transaction shows exactly this transfer. A token may answer
// false, or hold the money back, without reverting.
func (t faucetTransfer) moved(receipt *types.Receipt) bool {
	token, err := testusdc3009.Testusdc3009MetaData.GetAbi()
	if err != nil {
		return false
	}
	for _, log := range receipt.Logs {
		if log.Address == t.token && len(log.Topics) == 3 && log.Topics[0] == token.Events["Transfer"].ID &&
			log.Topics[1] == common.BytesToHash(t.sender[:]) && log.Topics[2] == common.BytesToHash(t.to[:]) &&
			new(big.Int).SetBytes(log.Data).Cmp(t.amount) == 0 {
			return true
		}
	}
	return false
}

var errFaucetSpent = errors.New("the nonce of a faucet claim was used by another transaction")

// faucetGiveUp is how long a claim may wait before it ends without a transaction.
const faucetGiveUp = time.Hour

// processFaucet is final only for a transaction that moved the tokens in a finalized block. It never
// signs a claim for a second nonce: an unreadable node must not pay twice.
func (w *Worker) processFaucet(ctx context.Context, job work, target Target, retry actionResult) (actionResult, error) {
	var to []byte
	var asset, amount, sender string
	err := w.Store.db.QueryRow(ctx, "SELECT * FROM ledger.faucet_job($1,$2)", job.id, job.fence).Scan(&to, &asset, &amount, &sender)
	if err != nil {
		return retry, err
	}
	claim := &faucetTransfer{token: common.HexToAddress(asset), sender: common.HexToAddress(sender), to: common.BytesToAddress(to)}
	claim.amount, _ = new(big.Int).SetString(amount, 10)
	job.action.faucet = claim
	end := func(outcome string, hash *string, counted, uncertain bool, cause error) (actionResult, error) {
		_, err := w.Store.db.Exec(ctx, "SELECT ledger.record_faucet($1,$2,$3,$4,$5)", job.id, job.fence, hash, counted, uncertain)
		if err != nil {
			return retry, errors.Join(cause, err)
		}
		// The nonce is used in a finalized block, or nothing was ever signed.
		return actionResult{state: text("final"), outcome: text(outcome), finished: true}, cause
	}
	if err := w.adoptIncluded(ctx, &job); err != nil {
		return retry, err
	}
	merchant, enrolled := w.MerchantIDs[job.branch]
	// The stream gate needs the store's merchant, not an order.
	gate := Quote{}
	gate.Args.MerchantId = merchant
	if job.hash != nil {
		if !enrolled || w.Faucet == nil || w.Faucet.Account != claim.sender {
			return actionResult{delay: time.Minute}, errors.New("the account of a signed faucet claim is not enrolled")
		}
		receipt, err := w.Chain.TransactionReceipt(ctx, common.HexToHash(*job.hash))
		if errors.Is(err, ethereum.NotFound) {
			state, _, err := w.resume(ctx, job, target, gate, true)
			if !errors.Is(err, errFaucetSpent) {
				return actionResult{state: state, delay: retry.delay}, err
			}
			if w.Now().Sub(job.action.created) < faucetGiveUp {
				return actionResult{delay: time.Minute}, err // the node may still show it
			}
			return end("failed", nil, true, true, err)
		}
		if err != nil {
			return retry, err
		}
		if source, ok := w.Chain.(finalChain); ok {
			final, err := source.finalReceipt(ctx, receipt)
			if err != nil {
				return retry, err
			}
			if !final {
				state := text("observed")
				if receipt.Status == types.ReceiptStatusFailed {
					state = text("reverted")
				}
				return actionResult{state: state, delay: retry.delay}, nil
			}
		}
		if receipt.Status == types.ReceiptStatusFailed {
			return end("failed", job.hash, false, false, errors.New("faucet transfer reverted"))
		}
		if !claim.moved(receipt) {
			// Executed without paying this account: no money back into the limits until someone looks.
			return end("failed", job.hash, true, false, errors.New("faucet transfer moved nothing"))
		}
		return end("final", job.hash, true, false, nil)
	}
	if !job.action.open {
		return actionResult{finished: true}, nil
	}
	waited := w.Now().Sub(job.action.created)
	if !enrolled || !w.faucetLists(job.branch, claim) {
		return end("failed", nil, false, false, errors.New("faucet claim is no longer offered"))
	}
	if waited >= faucetGiveUp {
		return end("failed", nil, false, false, errors.New("faucet claim could not be sent"))
	}
	prepared, err := w.prepare(ctx, job, target, gate)
	if errors.Is(err, errWouldRevert) {
		return actionResult{delay: min(max(waited/2, 5*time.Second), time.Minute)}, err
	}
	if err != nil || !prepared {
		return retry, err
	}
	return actionResult{state: text("prepared")}, nil
}

// faucetLists reports that the account and the token of an unsigned claim are still enrolled.
func (w *Worker) faucetLists(branch string, claim *faucetTransfer) bool {
	if w.Faucet == nil || w.Faucet.Account != claim.sender {
		return false
	}
	for _, token := range w.Faucet.Tokens[branch] {
		if token.Address == claim.token {
			return true
		}
	}
	return false
}

func (w *Worker) validateFaucet(job work, data []byte) error {
	expected, err := job.action.faucet.call(job.id, job.expires, w.Now())
	if err != nil || !bytes.Equal(expected.data, data) || !bytes.Equal(expected.intent[:], job.intent) {
		return errors.New("stored transaction does not match faucet claim")
	}
	return nil
}
