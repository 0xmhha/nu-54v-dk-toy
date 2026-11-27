package main

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/0xmhha/nu-54v-dk-toy/products/p05-operations-backoffice/internal/commerce"
	"github.com/0xmhha/nu-54v-dk-toy/products/p05-operations-backoffice/internal/settlement"
	"github.com/ethereum/go-ethereum/common"
)

const merchantGasPublication = "P05_MERCHANT_GAS_SPONSORS"

func readMerchantGasSponsors(path string) (map[string]settlement.GasSponsorGrant, error) {
	file, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	const maximum = 128 << 10
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > maximum {
		return nil, errors.New("merchant gas sponsorship publication must be a bounded regular file")
	}
	raw, err := io.ReadAll(io.LimitReader(file, maximum+1))
	if err != nil || len(raw) > maximum {
		return nil, errors.New("merchant gas sponsorship publication exceeds its read limit")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var entries []settlement.GasSponsorGrant
	if decoder.Decode(&entries) != nil || entries == nil || len(entries) > 128 || decoder.Decode(new(any)) != io.EOF {
		return nil, errors.New("invalid merchant gas sponsorship publication")
	}
	result := make(map[string]settlement.GasSponsorGrant, len(entries))
	for _, entry := range entries {
		if !targetUUID.MatchString(entry.Branch) {
			return nil, errors.New("invalid merchant sponsorship branch")
		}
		if _, duplicate := result[entry.Branch]; duplicate {
			return nil, errors.New("duplicate merchant sponsorship branch")
		}
		result[entry.Branch] = entry
	}
	return result, nil
}

func merchantGasSponsors(targets map[string]settlement.Target, reviews map[string]settlement.DeploymentReview,
	current func(context.Context, settlement.Target, settlement.DeploymentReview) bool,
) (commerce.GasSponsorLookup, error) {
	path := os.Getenv(merchantGasPublication)
	if path == "" {
		return nil, nil
	}
	authority, err := hex.DecodeString(os.Getenv("P05_SETTLEMENT_DEPLOYMENT_APPROVAL_KEY"))
	if !filepath.IsAbs(path) || err != nil || len(authority) != ed25519.PublicKeySize ||
		os.Getenv("P05_PRESENTATION_KEY_FILE") == "" {
		return nil, errors.New("merchant sponsorship requires the absolute grant publication, deployment approval key, and private presentation authority")
	}
	if _, err := readMerchantGasSponsors(path); err != nil {
		return nil, errors.New("cannot load merchant gas sponsorship publication")
	}
	return func(ctx context.Context, branch string, profile commerce.Profile, payer common.Address, now time.Time) (commerce.GasSponsorship, bool) {
		started := time.Now()
		target, active := targets[branch]
		if !active {
			return commerce.GasSponsorship{}, false
		}
		// Read the current operator publication. An atomic replacement or a read
		// failure stops issuing witnesses without restarting opsd.
		grants, err := readMerchantGasSponsors(path)
		grant, ok := grants[branch]
		if err != nil || !ok {
			return commerce.GasSponsorship{}, false
		}
		deployment := reviews[branch]
		witness, usable := settlement.VerifyMerchantGasSponsor(grant, branch, profile, target, deployment, payer,
			ed25519.PublicKey(authority), now)
		if !usable {
			return commerce.GasSponsorship{}, false
		}
		check, cancel := context.WithTimeout(ctx, time.Second)
		defer cancel()
		if !current(check, target, deployment) {
			return commerce.GasSponsorship{}, false
		}
		witness.CurrentPublication = func(at time.Time) bool {
			start := time.Now()
			latest, err := readMerchantGasSponsors(path)
			return err == nil && latest[branch] == grant && at.Add(time.Since(start)).Unix() < grant.ExpiresAt
		}
		if !witness.CurrentPublication(now.Add(time.Since(started))) {
			return commerce.GasSponsorship{}, false
		}
		return witness, true
	}, nil
}
