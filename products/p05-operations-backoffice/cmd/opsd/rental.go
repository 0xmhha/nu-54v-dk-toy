package main

import (
	"context"
	"crypto/ecdsa"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"math/big"
	"os"
	"path/filepath"

	"github.com/0xmhha/nu-54v-dk-toy/products/p05-operations-backoffice/internal/commerce"
	"github.com/0xmhha/nu-54v-dk-toy/products/p05-operations-backoffice/internal/core"
	"github.com/0xmhha/nu-54v-dk-toy/products/p05-operations-backoffice/internal/core/ops"
	"github.com/ethereum/go-ethereum/common"
)

var rentalSettings = []string{
	"P05_V1_RENTAL_RPC", "P05_V1_RENTAL_MANIFEST", "P05_V1_RENTAL_MANIFEST_SHA256",
	"P05_V1_RENTAL_KEYSTORE", "P05_V1_RENTAL_SECRET_REF",
}

type rentalController struct {
	chain      *core.Chain
	deployment core.Deployment
	manifest   [32]byte
	operator   *ecdsa.PrivateKey
}

func loadRentalController(ctx context.Context) (*rentalController, error) {
	if !anySetting(rentalSettings) {
		return nil, nil
	}
	path := os.Getenv("P05_V1_RENTAL_MANIFEST")
	rpc := os.Getenv("P05_V1_RENTAL_RPC")
	keystore := os.Getenv("P05_V1_RENTAL_KEYSTORE")
	if !filepath.IsAbs(path) || !filepath.IsAbs(keystore) || rpc == "" {
		return nil, errors.New("v1 rental requires an enrolled RPC, absolute manifest and keystore")
	}
	material, err := os.ReadFile(path)
	if err != nil {
		return nil, errors.New("v1 rental deployment manifest unavailable")
	}
	hash := sha256.Sum256(material)
	if hex.EncodeToString(hash[:]) != os.Getenv("P05_V1_RENTAL_MANIFEST_SHA256") {
		return nil, errors.New("v1 rental manifest digest differs")
	}
	deployment, err := core.ParseDeployment(material)
	if err != nil || deployment.ChainID != 8283 || deployment.Token == (common.Address{}) || deployment.SettlementBlock == 0 {
		return nil, errors.New("pinned legacy rental deployment is invalid")
	}
	operator, err := core.LoadKey(keystore, os.Getenv("P05_V1_RENTAL_SECRET_REF"))
	if err != nil {
		return nil, errors.New("dedicated v1 rental operator key unavailable")
	}
	chain, err := core.Dial(ctx, rpc, deployment)
	if err != nil {
		return nil, errors.New("v1 rental network differs from enrollment or is unavailable")
	}
	return &rentalController{chain: chain, deployment: deployment, manifest: hash, operator: operator}, nil
}

func (c *rentalController) managed(member commerce.RentalMember) ops.ManagedRental {
	return ops.ManagedRental{Device: common.HexToAddress(member.Account), Destination: common.HexToAddress(member.Destination),
		Key: c.operator, Chain: c.chain}
}

func (c *rentalController) Observe(ctx context.Context, member commerce.RentalMember) (ops.RentalObservation, error) {
	managed := c.managed(member)
	view, err := managed.Observe(ctx, member.Hold)
	if err != nil {
		return view, err
	}
	if view.Status == "unopened" {
		perPayment, daily, err := c.chain.RentalOpenLimits(ctx)
		if err != nil {
			return view, err
		}
		view.PerPaymentLimit, view.DailyLimit = perPayment.String(), daily.String()
	}
	opening := member.Opening()
	if opening != nil {
		amount, ok := new(big.Int).SetString(opening.Amount, 10)
		if ok && amount.Sign() > 0 && opening.Device != (common.Address{}) && opening.Destination != (common.Address{}) {
			// A missing proof leaves the exact known transaction pending for re-query.
			opening.Finalized, _ = c.chain.RentalOpeningBlock(ctx, opening.TxHash, opening.Device, opening.Destination, amount)
		}
		view.Opening = opening
	}
	return view, nil
}

func (c *rentalController) Submit(ctx context.Context, member commerce.RentalMember, action, _ string, amount *big.Int) (ops.RentalObservation, error) {
	managed := c.managed(member)
	switch action {
	case "open":
		view, err := managed.Observe(ctx, member.Hold)
		if err != nil || view.Status != "unopened" {
			return view, errors.New("rental account already exists or evidence is unavailable")
		}
		// The attested setup station prepares board/PIN/anchor first. Opening is
		// one exact deposit to that already-enrolled v1 device and route.
		hash, err := c.chain.Deposit(ctx, c.operator, managed.Device, amount, managed.Destination)
		if hash != (common.Hash{}) {
			view.Submitted = &hash
		}
		return view, err
	case "cleanup":
		return ops.RentalObservation{}, errors.New("protected cleanup station required")
	default:
		return managed.Submit(ctx, action, member.Hold)
	}
}
