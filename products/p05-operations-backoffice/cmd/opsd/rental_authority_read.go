package main

import (
	"context"

	"github.com/0xmhha/nu-54v-dk-toy/products/p05-operations-backoffice/internal/commerce"
	"github.com/ethereum/go-ethereum/crypto"
)

func openRentalAuthorityReader(ctx context.Context, dsn string, rental *rentalController) (*commerce.RentalAuthorityReader, error) {
	return commerce.OpenRentalAuthorityReader(ctx, dsn, rental.deployment, rental.manifest, crypto.PubkeyToAddress(rental.operator.PublicKey))
}
