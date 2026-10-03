package core

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"

	"github.com/0xmhha/nu-54v-dk-toy/packages/contracts-abi/go/registry"
	"github.com/0xmhha/nu-54v-dk-toy/packages/contracts-abi/go/registryext"
	"github.com/0xmhha/nu-54v-dk-toy/packages/contracts-abi/go/settlement"
	"github.com/0xmhha/nu-54v-dk-toy/packages/contracts-abi/go/settlementext"
	"github.com/0xmhha/nu-54v-dk-toy/packages/contracts-abi/go/testusdc"
)

// Chain wraps the RPC client and the deployment's registry.
type Chain struct {
	client *ethclient.Client
	reg    *registryext.Registryext
	dep    Deployment
	read   *big.Int // block tag for reads and the anchor time: finalized, or latest on a sandbox
}

// ReadLatest makes reads use the latest block. Only for a local sandbox, whose finalized tag
// trails latest; on StableNet finalized equals latest and operations decide on finalized.
func (c *Chain) ReadLatest() { c.read = nil }

func Dial(ctx context.Context, rpcURL string, dep Deployment) (*Chain, error) {
	c, err := ethclient.DialContext(ctx, rpcURL)
	if err != nil {
		return nil, err
	}
	id, err := c.ChainID(ctx)
	if err != nil {
		return nil, err
	}
	if id.Uint64() != dep.ChainID {
		return nil, fmt.Errorf("RPC chain %d, deployment chain %d", id, dep.ChainID)
	}
	reg, err := registryext.NewRegistryext(dep.Registry, c)
	if err != nil {
		return nil, err
	}
	return &Chain{client: c, reg: reg, dep: dep, read: big.NewInt(int64(rpc.FinalizedBlockNumber))}, nil
}

func (c *Chain) MerchantStatus(merchant common.Address) (bool, common.Address, error) {
	s, err := c.reg.MerchantStatus(&bind.CallOpts{BlockNumber: c.read}, merchant)
	return s.Active, s.Payout, err
}

func (c *Chain) PendingPayoutOf(merchant common.Address) (common.Address, uint64, error) {
	p, err := c.reg.PendingPayoutOf(&bind.CallOpts{BlockNumber: c.read}, merchant)
	if err != nil {
		return common.Address{}, 0, err
	}
	return p.Payout, p.EffectiveAt.Uint64(), nil
}

// FinalizedTime is the timestamp of the latest finalized block (TimeAnchor source, [N06]).
func (c *Chain) FinalizedTime(ctx context.Context) (uint64, error) {
	h, err := c.client.HeaderByNumber(ctx, c.read)
	if err != nil {
		return 0, err
	}
	return h.Time, nil
}

// Transactor returns options that pay the node's suggested priority fee and a fee cap of
// 2 x base fee + tip; StableNet refuses lower values (2026-10-01 observation).
func (c *Chain) Transactor(ctx context.Context, key *ecdsa.PrivateKey) (*bind.TransactOpts, error) {
	opts, err := bind.NewKeyedTransactorWithChainID(key, new(big.Int).SetUint64(c.dep.ChainID))
	if err != nil {
		return nil, err
	}
	tip, err := c.client.SuggestGasTipCap(ctx)
	if err != nil {
		return nil, err
	}
	head, err := c.client.HeaderByNumber(ctx, nil)
	if err != nil {
		return nil, err
	}
	opts.Context = ctx
	opts.GasTipCap = tip
	opts.GasFeeCap = new(big.Int).Add(new(big.Int).Mul(head.BaseFee, big.NewInt(2)), tip)
	return opts, nil
}

// RegisterMerchant registers (merchant, payout) unless the registry already has exactly that
// (P05-FR-01). Returns the transaction hash, or the zero hash when nothing was sent.
func (c *Chain) RegisterMerchant(ctx context.Context, admin *ecdsa.PrivateKey, merchant, payout common.Address) (common.Hash, error) {
	active, current, err := c.MerchantStatus(merchant)
	if err != nil {
		return common.Hash{}, err
	}
	if active && current == payout {
		return common.Hash{}, nil
	}
	reg, err := registry.NewRegistry(c.dep.Registry, c.client)
	if err != nil {
		return common.Hash{}, err
	}
	opts, err := c.Transactor(ctx, admin)
	if err != nil {
		return common.Hash{}, err
	}
	tx, err := reg.RegisterMerchant(opts, merchant, payout)
	return c.send(ctx, "registerMerchant", tx, err)
}

func (c *Chain) send(ctx context.Context, what string, tx *types.Transaction, err error) (common.Hash, error) {
	if err != nil {
		return common.Hash{}, fmt.Errorf("%s: %w", what, err)
	}
	rc, err := bind.WaitMined(ctx, c.client, tx)
	if err != nil {
		return tx.Hash(), err
	}
	if rc.Status != 1 {
		return tx.Hash(), fmt.Errorf("%s reverted in %s", what, tx.Hash().Hex())
	}
	return tx.Hash(), nil
}

// MintTestToken mints test tokens with the token owner key. The recipient must not be in
// refusal mode (the token refuses minting with RecipientRefusing).
func (c *Chain) MintTestToken(ctx context.Context, owner *ecdsa.PrivateKey, to common.Address, amount *big.Int) (common.Hash, error) {
	tok, err := testusdc.NewTestusdc(c.dep.Token, c.client)
	if err != nil {
		return common.Hash{}, err
	}
	opts, err := c.Transactor(ctx, owner)
	if err != nil {
		return common.Hash{}, err
	}
	tx, err := tok.Mint(opts, to, amount)
	return c.send(ctx, "mint", tx, err)
}

// Deposit funds a device account with the operator key (depositFor). It approves the
// settlement contract first when the allowance is short. Returns the deposit transaction.
func (c *Chain) Deposit(ctx context.Context, operator *ecdsa.PrivateKey, device common.Address, amount *big.Int, withdraw common.Address) (common.Hash, error) {
	tok, err := testusdc.NewTestusdc(c.dep.Token, c.client)
	if err != nil {
		return common.Hash{}, err
	}
	from := crypto.PubkeyToAddress(operator.PublicKey)
	bal, err := tok.BalanceOf(&bind.CallOpts{Context: ctx}, from)
	if err != nil {
		return common.Hash{}, err
	}
	if bal.Cmp(amount) < 0 {
		return common.Hash{}, fmt.Errorf("operator holds %s test token base units, deposit needs %s; mint first", bal, amount)
	}
	allowance, err := tok.Allowance(&bind.CallOpts{Context: ctx}, from, c.dep.Settlement)
	if err != nil {
		return common.Hash{}, err
	}
	if allowance.Cmp(amount) < 0 {
		opts, err := c.Transactor(ctx, operator)
		if err != nil {
			return common.Hash{}, err
		}
		tx, err := tok.Approve(opts, c.dep.Settlement, amount)
		if _, err := c.send(ctx, "approve", tx, err); err != nil {
			return common.Hash{}, err
		}
	}
	st, err := settlement.NewSettlement(c.dep.Settlement, c.client)
	if err != nil {
		return common.Hash{}, err
	}
	opts, err := c.Transactor(ctx, operator)
	if err != nil {
		return common.Hash{}, err
	}
	tx, err := st.DepositFor(opts, device, amount, withdraw)
	return c.send(ctx, "depositFor", tx, err)
}

// Account is a device account as the finalized (or sandbox latest) state shows it.
type Account struct {
	Exists        bool // a deposit created it (it has a withdraw address)
	ClosedAt      uint64
	Balance       *big.Int
	WithdrawAfter uint64
}

// AccountOf reads a device account (accountOf in the settlement extensions).
func (c *Chain) AccountOf(ctx context.Context, device common.Address) (Account, error) {
	ext, err := settlementext.NewSettlementext(c.dep.Settlement, c.client)
	if err != nil {
		return Account{}, err
	}
	v, err := ext.AccountOf(&bind.CallOpts{Context: ctx, BlockNumber: c.read}, device)
	if err != nil {
		return Account{}, err
	}
	return Account{Exists: v.WithdrawAddress != (common.Address{}), ClosedAt: v.ClosedAt.Uint64(), Balance: v.Balance, WithdrawAfter: v.WithdrawAfter.Uint64()}, nil
}

// CloseAccount stops the device account at once with the operator key (closeAccount):
// settle refuses it from then on and the balance goes to the withdraw address after the delay.
func (c *Chain) CloseAccount(ctx context.Context, operator *ecdsa.PrivateKey, device common.Address) (common.Hash, error) {
	ext, err := settlementext.NewSettlementext(c.dep.Settlement, c.client)
	if err != nil {
		return common.Hash{}, err
	}
	opts, err := c.Transactor(ctx, operator)
	if err != nil {
		return common.Hash{}, err
	}
	tx, err := ext.CloseAccount(opts, device)
	return c.send(ctx, "closeAccount", tx, err)
}

// AccountClosedBlock finds the device's AccountClosed event up to the read block (finalized on
// the testnet). ok is false while no such event is final yet.
func (c *Chain) AccountClosedBlock(ctx context.Context, device common.Address) (block uint64, tx common.Hash, ok bool, err error) {
	ext, err := settlementext.NewSettlementext(c.dep.Settlement, c.client)
	if err != nil {
		return 0, common.Hash{}, false, err
	}
	head, err := c.client.HeaderByNumber(ctx, c.read)
	if err != nil {
		return 0, common.Hash{}, false, err
	}
	end := head.Number.Uint64()
	it, err := ext.FilterAccountClosed(&bind.FilterOpts{Context: ctx, Start: c.dep.SettlementBlock, End: &end}, []common.Address{device})
	if err != nil {
		return 0, common.Hash{}, false, err
	}
	defer it.Close()
	for it.Next() {
		return it.Event.Raw.BlockNumber, it.Event.Raw.TxHash, true, nil
	}
	return 0, common.Hash{}, false, it.Error()
}

// RevokeMerchant stops a merchant at once with the registry admin key (P05-FR-03). Settlement
// refuses its orders as MerchantRevoked. Returns the zero hash when it was already inactive.
func (c *Chain) RevokeMerchant(ctx context.Context, admin *ecdsa.PrivateKey, merchant common.Address) (common.Hash, error) {
	active, _, err := c.MerchantStatus(merchant)
	if err != nil {
		return common.Hash{}, err
	}
	if !active {
		return common.Hash{}, nil
	}
	reg, err := registry.NewRegistry(c.dep.Registry, c.client)
	if err != nil {
		return common.Hash{}, err
	}
	opts, err := c.Transactor(ctx, admin)
	if err != nil {
		return common.Hash{}, err
	}
	tx, err := reg.RevokeMerchant(opts, merchant)
	return c.send(ctx, "revokeMerchant", tx, err)
}

// RequestPayoutChange queues a new payout that takes effect after payoutChangeDelay; the
// merchant can refuse it until then. Returns the zero hash when the same change is already queued.
func (c *Chain) RequestPayoutChange(ctx context.Context, admin *ecdsa.PrivateKey, merchant, payout common.Address) (common.Hash, error) {
	pending, effectiveAt, err := c.PendingPayoutOf(merchant)
	if err != nil {
		return common.Hash{}, err
	}
	if pending == payout && effectiveAt != 0 {
		return common.Hash{}, nil
	}
	opts, err := c.Transactor(ctx, admin)
	if err != nil {
		return common.Hash{}, err
	}
	tx, err := c.reg.RequestPayoutChange(opts, merchant, payout)
	return c.send(ctx, "requestPayoutChange", tx, err)
}

// CancelPayoutChange drops a queued payout change with the admin or the merchant key. Returns
// the zero hash when nothing is pending.
func (c *Chain) CancelPayoutChange(ctx context.Context, key *ecdsa.PrivateKey, merchant common.Address) (common.Hash, error) {
	_, effectiveAt, err := c.PendingPayoutOf(merchant)
	if err != nil {
		return common.Hash{}, err
	}
	if effectiveAt == 0 {
		return common.Hash{}, nil
	}
	opts, err := c.Transactor(ctx, key)
	if err != nil {
		return common.Hash{}, err
	}
	tx, err := c.reg.CancelPayoutChange(opts, merchant)
	return c.send(ctx, "cancelPayoutChange", tx, err)
}
