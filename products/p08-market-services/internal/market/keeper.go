package market

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"errors"
	"fmt"
	"math/big"

	"github.com/0xmhha/nu-54v-dk-toy/products/p07-indexer/pkg/finality"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Keeper struct {
	Chain *Chain
	DB    *pgxpool.Pool
	Key   *ecdsa.PrivateKey
	next  uint64
}

type job struct {
	Position     string
	Kind         string
	Period       string
	Revision     string
	PriceVersion string
}

type attempt struct {
	job
	Hash      []byte
	Raw       []byte
	State     string
	Height    *int64
	BlockHash []byte
}

func (k *Keeper) venueKey() []any {
	p := k.Chain.Profile
	return []any{p.Genesis.Bytes(), p.Venue.Address.Bytes(), p.BlockHash.Bytes()}
}

const venueFilter = "genesis=$1 AND contract_address=$2 AND deployment_hash=$3"
const attemptFields = "position::text, kind, period::text, revision::text, price_version::text, tx_hash, raw_tx, state, block_height, block_hash"

// Tick processes one durable chain action. afterCommit is used by the process-loss integration test.
func (k *Keeper) Tick(ctx context.Context, afterCommit func() error) error {
	head, err := k.Chain.Head(ctx)
	if err != nil {
		return err
	}
	from := crypto.PubkeyToAddress(k.Key.PublicKey)
	roles, err := k.Chain.Venue(ctx, head.Hash(), "keeper")
	if err != nil || roles[0].(common.Address) != from {
		return errors.New("signing key does not match isolated keeper role")
	}
	conn, err := k.DB.Acquire(ctx)
	if err != nil {
		return err
	}
	defer conn.Release()
	tag := k.Chain.Profile.BlockHash.Hex()
	var locked bool
	if err := conn.QueryRow(ctx, "SELECT pg_try_advisory_lock(hashtextextended($1,0))", tag).Scan(&locked); err != nil || !locked {
		return err
	}
	defer func() {
		// Closing the session releases its lock if cancellation prevents an explicit unlock.
		if _, err := conn.Exec(ctx, "SELECT pg_advisory_unlock(hashtextextended($1,0))", tag); err != nil {
			conn.Hijack().Close(context.Background())
		}
	}()
	if _, err := conn.Exec(ctx, `INSERT INTO market.venue (genesis,contract_address,deployment_hash,code_hash,chain_id,profile)
		VALUES ($1,$2,$3,$4,$5,$6) ON CONFLICT DO NOTHING`,
		append(k.venueKey(), k.Chain.Profile.Venue.CodeHash.Bytes(), ChainID, k.Chain.Profile.Environment)...); err != nil {
		return err
	}
	pending, found, err := k.lookup(ctx, conn, " AND state IN ('prepared','submitted','unknown','reorged') LIMIT 1")
	if err != nil {
		return err
	}
	if found {
		return k.reconcile(ctx, conn, pending, from, k.Chain.headClock(head.Time) == "current")
	}
	last, found, err := k.lookup(ctx, conn, " AND block_height IS NOT NULL ORDER BY block_height DESC LIMIT 1")
	if err != nil {
		return err
	}
	if found {
		if canonical, ok := k.isCanonical(ctx, last); !ok {
			return errors.New("could not recheck keeper finality")
		} else if !canonical {
			return k.update(ctx, conn, last, "reorged", nil, nil)
		}
	}
	if k.Chain.headClock(head.Time) != "current" {
		return nil
	}
	next, err := k.candidate(ctx, head)
	if err != nil || next == (job{}) {
		return err
	}
	prepared, err := k.prepare(ctx, conn, from, next)
	if err != nil || prepared.State == "" {
		return err
	}
	if afterCommit != nil {
		if err := afterCommit(); err != nil {
			return err
		}
	}
	return k.reconcile(ctx, conn, prepared, from, true)
}

func (k *Keeper) lookup(ctx context.Context, conn *pgxpool.Conn, clause string) (attempt, bool, error) {
	item := attempt{}
	err := conn.QueryRow(ctx, "SELECT "+attemptFields+" FROM market.keeper_attempt WHERE "+venueFilter+clause, k.venueKey()...).
		Scan(&item.Position, &item.Kind, &item.Period, &item.Revision, &item.PriceVersion, &item.Hash, &item.Raw, &item.State, &item.Height, &item.BlockHash)
	if errors.Is(err, pgx.ErrNoRows) {
		return attempt{}, false, nil
	}
	return item, err == nil, err
}

func (k *Keeper) isCanonical(ctx context.Context, last attempt) (canonical, known bool) {
	header, err := k.Chain.RPC.HeaderByNumber(ctx, big.NewInt(*last.Height))
	if errors.Is(err, ethereum.NotFound) {
		return false, true
	}
	if err != nil || header.Number.Int64() != *last.Height {
		return false, false
	}
	if header.Hash() != common.BytesToHash(last.BlockHash) {
		return false, true
	}
	finalized, err := k.Chain.RPC.HeaderByNumber(ctx, big.NewInt(int64(rpc.FinalizedBlockNumber)))
	return true, err == nil && finalized.Number.IsInt64() && finalized.Number.Int64() >= *last.Height
}

func (k *Keeper) candidate(ctx context.Context, head *types.Header) (job, error) {
	at := head.Hash()
	paused, err := k.Chain.Venue(ctx, at, "paused")
	if err != nil || paused[0].(bool) {
		return job{}, err
	}
	valid, err := k.Chain.Venue(ctx, at, "validPrice")
	if err != nil {
		return job{}, nil // Price unavailability produces no risk action.
	}
	last, err := k.Chain.Venue(ctx, at, "nextPosition")
	if err != nil {
		return job{}, err
	}
	end := last[0].(*big.Int)
	if !end.IsUint64() || end.Sign() == 0 {
		return job{}, errors.New("position id exceeds keeper scan range")
	}
	count := end.Uint64()
	// Rotate across positions to bound work per polling tick.
	for visits := uint64(0); visits < min(count-1, 32); visits++ {
		if k.next == 0 || k.next >= count {
			k.next = 1
		}
		id := new(big.Int).SetUint64(k.next)
		k.next++
		pos, err := k.Chain.Position(ctx, at, id)
		if err != nil {
			return job{}, err
		}
		if pos.Closed || pos.Owner == (common.Address{}) {
			continue
		}
		item := job{Position: id.String(), Revision: pos.Revision, PriceVersion: valid[0].(*big.Int).String()}
		equity, err := k.Chain.Venue(ctx, at, "equity", id)
		if err != nil {
			return job{}, err
		}
		if equity[0].(*big.Int).Cmp(equity[1].(*big.Int)) <= 0 {
			item.Kind, item.Period = "liquidation", "0"
			return item, nil
		}
		period := new(big.Int).SetUint64(head.Time / FundingInterval).String()
		if pos.NextPeriod == period && head.Time%FundingInterval < 120 {
			item.Kind, item.Period = "funding", period
			return item, nil
		}
	}
	return job{}, nil
}

func jobCalldata(item job) ([]byte, error) {
	position, err := units(item.Position)
	if err != nil {
		return nil, err
	}
	revision, err := units(item.Revision)
	if err != nil {
		return nil, err
	}
	version, err := units(item.PriceVersion)
	if err != nil {
		return nil, err
	}
	if item.Kind == "liquidation" {
		return venueABI.Pack("liquidate", position, revision, version)
	}
	if item.Kind != "funding" {
		return nil, errors.New("invalid keeper work kind")
	}
	period, err := units(item.Period)
	if err != nil {
		return nil, err
	}
	return venueABI.Pack("fund", position, revision, period, version)
}

func (k *Keeper) prepare(ctx context.Context, conn *pgxpool.Conn, from common.Address, item job) (attempt, error) {
	key := append(k.venueKey(), item.Position, item.Kind, item.Period)
	input, err := jobCalldata(item)
	if err != nil {
		return attempt{}, err
	}
	to := k.Chain.Profile.Venue.Address
	gas, err := k.Chain.estimate(ctx, from, to, input)
	if err != nil {
		return attempt{}, nil // Latest contract preflight is definitive.
	}
	price, err := k.Chain.RPC.SuggestGasPrice(ctx)
	if err != nil {
		return attempt{}, err
	}
	nonce, err := k.Chain.RPC.PendingNonceAt(ctx, from)
	if err != nil {
		return attempt{}, err
	}
	transaction := types.NewTx(&types.LegacyTx{Nonce: nonce, GasPrice: price, Gas: gas + gas/5, To: &to, Data: input})
	signed, err := types.SignTx(transaction, types.LatestSignerForChainID(big.NewInt(ChainID)), k.Key)
	if err != nil {
		return attempt{}, err
	}
	raw, err := signed.MarshalBinary()
	if err != nil {
		return attempt{}, err
	}
	hash := signed.Hash().Bytes()
	tx, err := conn.Begin(ctx)
	if err != nil {
		return attempt{}, err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `INSERT INTO market.keeper_job (genesis,contract_address,deployment_hash,position,kind,period)
		VALUES ($1,$2,$3,$4::text::numeric,$5,$6::text::numeric) ON CONFLICT DO NOTHING`, key...); err != nil {
		return attempt{}, err
	}
	command, err := tx.Exec(ctx, `INSERT INTO market.keeper_attempt (genesis,contract_address,deployment_hash,position,kind,period,revision,price_version,tx_hash,raw_tx)
		VALUES ($1,$2,$3,$4::text::numeric,$5,$6::text::numeric,$7::text::numeric,$8::text::numeric,$9,$10)
		ON CONFLICT (genesis,contract_address,deployment_hash,position,kind,period,revision,price_version) DO NOTHING`,
		append(key, item.Revision, item.PriceVersion, hash, raw)...)
	if err != nil || command.RowsAffected() == 0 {
		return attempt{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return attempt{}, err
	}
	return attempt{job: item, Hash: hash, Raw: raw, State: "prepared"}, nil
}

func (k *Keeper) reconcile(ctx context.Context, conn *pgxpool.Conn, item attempt, from common.Address, canDispatch bool) error {
	hash := common.BytesToHash(item.Hash)
	var transaction types.Transaction
	if err := transaction.UnmarshalBinary(item.Raw); err != nil {
		return err
	}
	signer := types.LatestSignerForChainID(big.NewInt(ChainID))
	sender, err := types.Sender(signer, &transaction)
	want, calldataErr := jobCalldata(item.job)
	if err != nil || calldataErr != nil || transaction.Hash() != hash || sender != from ||
		transaction.ChainId().Cmp(big.NewInt(ChainID)) != 0 || transaction.Value().Sign() != 0 ||
		transaction.To() == nil || *transaction.To() != k.Chain.Profile.Venue.Address || !bytes.Equal(transaction.Data(), want) {
		return errors.New("stored keeper transaction does not match its work key")
	}
	proof, err := finality.CanonicalReceipt(ctx, k.Chain.RPC, hash)
	switch {
	case errors.Is(err, finality.ErrPending):
		return nil
	case errors.Is(err, ethereum.NotFound):
		if !canDispatch {
			return nil
		}
		sendErr := k.Chain.RPC.SendTransaction(ctx, &transaction)
		if sendErr != nil {
			return k.update(ctx, conn, item, "unknown", nil, nil)
		}
		return k.update(ctx, conn, item, "submitted", nil, nil)
	case err != nil:
		return err
	}
	receipt := proof.Receipt
	if !receipt.BlockNumber.IsInt64() {
		return errors.New("keeper receipt height out of range")
	}
	state := "rejected"
	if receipt.Status == types.ReceiptStatusSuccessful {
		if err := k.checkEffect(ctx, proof, item); err != nil {
			return err
		}
		state = "confirmed"
	}
	return k.update(ctx, conn, item, state, receipt.BlockNumber.Int64(), receipt.BlockHash.Bytes())
}

func (k *Keeper) checkEffect(ctx context.Context, proof finality.Evidence, item attempt) error {
	id, _ := units(item.Position)
	revision, _ := units(item.Revision)
	revision.Add(revision, big.NewInt(1))
	at := proof.Anchor.Hash()
	pos, err := k.Chain.Position(ctx, at, id)
	if err != nil {
		return err
	}
	current, err := units(pos.Revision)
	if err != nil || current.Cmp(revision) < 0 {
		return errors.New("finalized position revision does not cover keeper execution")
	}
	event := venueABI.Events["PositionFunded"]
	if item.Kind == "liquidation" {
		event = venueABI.Events["PositionClosed"]
	}
	count := 0
	for _, log := range proof.Receipt.Logs {
		if log.Address != k.Chain.Profile.Venue.Address || len(log.Topics) != 3 || log.Topics[0] != event.ID ||
			log.Topics[1].Big().Cmp(id) != 0 {
			continue
		}
		fields, err := event.Inputs.NonIndexed().Unpack(log.Data)
		if err != nil || fields[0].(*big.Int).Cmp(revision) != 0 || fields[1].(*big.Int).String() != item.PriceVersion {
			return errors.New("keeper event disagrees with expected revision or price")
		}
		if item.Kind == "funding" {
			period, _ := units(item.Period)
			cursor, _ := units(pos.NextPeriod)
			if log.Topics[2].Big().Cmp(period) != 0 || cursor.Cmp(period) <= 0 ||
				(current.Cmp(revision) == 0 && pos.Margin != fields[3].(*big.Int).String()) {
				return errors.New("funding event disagrees with finalized cursor or margin")
			}
		} else if log.Topics[2].Big().Cmp(big.NewInt(1)) != 0 || !pos.Closed || current.Cmp(revision) != 0 {
			return errors.New("liquidation event disagrees with finalized closure")
		}
		count++
	}
	if count != 1 {
		return fmt.Errorf("expected one finalized keeper effect, got %d", count)
	}
	return nil
}

func (k *Keeper) update(ctx context.Context, conn *pgxpool.Conn, item attempt, state string, height, blockHash any) error {
	_, err := conn.Exec(ctx, `UPDATE market.keeper_attempt SET state=$4,block_height=$5,block_hash=$6,reviewed_at=now()
		WHERE `+venueFilter+" AND tx_hash=$7", append(k.venueKey(), state, height, blockHash, item.Hash)...)
	return err
}
