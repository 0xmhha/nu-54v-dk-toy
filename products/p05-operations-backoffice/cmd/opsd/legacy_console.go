package main

import (
	"context"
	"crypto/ecdsa"
	"errors"
	"os"
	"path/filepath"

	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/service"
	"github.com/0xmhha/nu-54v-dk-toy/products/p05-operations-backoffice/internal/core"
	"github.com/ethereum/go-ethereum/common"
)

var legacyConsoleSettings = []string{"P05_V1_LEGACY_MERCHANTS", "P05_V1_CASHOUT_KEYSTORE", "P05_V1_CASHOUT_SECRET_REF"}

type legacyConsole struct {
	chain     *core.Chain
	key       *ecdsa.PrivateKey
	merchants map[string]common.Address
}

func loadLegacyConsole(rental *rentalController) (*legacyConsole, error) {
	if !anySetting(legacyConsoleSettings) {
		return nil, nil
	}
	if rental == nil {
		return nil, errors.New("legacy console requires the pinned v1 deployment and RPC")
	}
	merchants := map[string]common.Address{}
	if err := configJSON("P05_V1_LEGACY_MERCHANTS", &merchants); err != nil || len(merchants) == 0 || len(merchants) > 128 {
		return nil, errors.New("enroll the v1 merchant for each branch")
	}
	for branch, merchant := range merchants {
		if !targetUUID.MatchString(branch) || merchant == (common.Address{}) {
			return nil, errors.New("invalid v1 branch merchant")
		}
	}
	keystore := os.Getenv("P05_V1_CASHOUT_KEYSTORE")
	if !filepath.IsAbs(keystore) {
		return nil, errors.New("cashout requires a dedicated encrypted keystore")
	}
	key, err := core.LoadKey(keystore, os.Getenv("P05_V1_CASHOUT_SECRET_REF"))
	if err != nil || core.Address(key) == core.Address(rental.operator) {
		return nil, errors.New("cashout gas signer must be separate from the rental key")
	}
	return &legacyConsole{chain: rental.chain, key: key, merchants: merchants}, nil
}

func (l *legacyConsole) View(ctx context.Context, branch string) (core.LegacyLedger, error) {
	merchant, ok := l.merchants[branch]
	if !ok {
		return core.LegacyLedger{}, service.Forbidden
	}
	return l.chain.LegacyLedger(ctx, merchant)
}

func (l *legacyConsole) Preflight(ctx context.Context, branch string) (core.LegacyLedger, error) {
	return l.chain.LegacyLedgerLatest(ctx, l.merchants[branch])
}

func (l *legacyConsole) Prepare(ctx context.Context, branch string) ([]byte, error) {
	return l.chain.PrepareLegacyCashOut(ctx, l.key, l.merchants[branch])
}

func (l *legacyConsole) Observe(ctx context.Context, branch string, bytes []byte, submit bool) (core.LegacyCashOutObservation, error) {
	return l.chain.ObserveLegacyCashOut(ctx, core.Address(l.key), l.merchants[branch], bytes, submit)
}
