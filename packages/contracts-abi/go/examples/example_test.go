// Package examples shows how the operator tool and the indexer use the generated
// bindings. The examples compile in CI; they are not run, because they need a chain.
package examples

import (
	"context"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"

	"github.com/0xmhha/nu-54v-dk-toy/packages/contracts-abi/go/registry"
	"github.com/0xmhha/nu-54v-dk-toy/packages/contracts-abi/go/settlement"
)

// Example_registerMerchant registers a merchant as the registry admin (operator tool, week 6).
func Example_registerMerchant() {
	ctx := context.Background()
	client, err := ethclient.DialContext(ctx, "http://127.0.0.1:8545") // sandbox; the testnet RPC in production
	if err != nil {
		panic(err)
	}
	reg, err := registry.NewRegistry(common.HexToAddress("0x<registry>"), client)
	if err != nil {
		panic(err)
	}
	// admin comes from an encrypted keystore opened through its secretRef (N32).
	var admin *bind.TransactOpts
	tx, err := reg.RegisterMerchant(admin, common.HexToAddress("0x<merchant>"), common.HexToAddress("0x<payout>"))
	if err != nil {
		panic(err)
	}
	fmt.Println("registerMerchant tx", tx.Hash())
}

// Example_watchPaymentSettled reads finalized PaymentSettled events for one merchant and order
// (kiosk watcher and receipt indexer).
func Example_watchPaymentSettled() {
	ctx := context.Background()
	client, err := ethclient.DialContext(ctx, "http://127.0.0.1:8545")
	if err != nil {
		panic(err)
	}
	s, err := settlement.NewSettlement(common.HexToAddress("0x<settlement>"), client)
	if err != nil {
		panic(err)
	}
	finalized, err := client.HeaderByNumber(ctx, big.NewInt(-3)) // rpc.FinalizedBlockNumber
	if err != nil {
		panic(err)
	}
	end := finalized.Number.Uint64()
	it, err := s.FilterPaymentSettled(&bind.FilterOpts{Start: 0, End: &end, Context: ctx},
		[]common.Address{common.HexToAddress("0x<merchant>")}, [][32]byte{{0x01}}, nil)
	if err != nil {
		panic(err)
	}
	defer it.Close()
	for it.Next() {
		fmt.Println("paid", it.Event.Device, it.Event.Amount, it.Event.Nonce)
	}
}
