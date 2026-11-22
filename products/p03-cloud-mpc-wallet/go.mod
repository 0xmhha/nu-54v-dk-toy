module github.com/0xmhha/nu-54v-dk-toy/products/p03-cloud-mpc-wallet

go 1.26.10

require (
	github.com/bnb-chain/tss-lib/v4 v4.0.0-20260923032227-f204718ae595
	github.com/coreos/go-oidc/v3 v3.21.0
	github.com/ethereum/go-ethereum v1.17.10
	github.com/go-jose/go-jose/v4 v4.1.9
	golang.org/x/oauth2 v0.38.0
	golang.org/x/sys v0.50.0
)

require (
	github.com/ProjectZKM/Ziren/crates/go-runtime/zkvm_runtime v0.0.0-20251001021608-1fe7b43fc4d6 // indirect
	github.com/holiman/uint256 v1.3.2 // indirect
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/jackc/puddle/v2 v2.2.2 // indirect
	github.com/mattn/go-isatty v0.0.22 // indirect
	golang.org/x/sync v0.25.0 // indirect
	golang.org/x/text v0.44.0 // indirect
)

require (
	filippo.io/bigmod v0.1.0 // indirect
	github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go v0.2.0
	github.com/agl/ed25519 v0.0.0-20200225211852-fd4d107ace12 // indirect
	github.com/btcsuite/btcd v0.25.0 // indirect
	github.com/btcsuite/btcd/btcec/v2 v2.5.1 // indirect
	github.com/btcsuite/btcd/chaincfg/chainhash v1.2.0 // indirect
	github.com/btcsuite/btcutil v1.0.2 // indirect
	github.com/decred/dcrd/dcrec/edwards/v2 v2.0.3 // indirect
	github.com/decred/dcrd/dcrec/secp256k1/v4 v4.4.1
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/google/uuid v1.6.0
	github.com/hashicorp/errwrap v1.0.0 // indirect
	github.com/hashicorp/go-multierror v1.1.1 // indirect
	github.com/ipfs/go-log v1.0.5 // indirect
	github.com/ipfs/go-log/v2 v2.9.2 // indirect
	github.com/jackc/pgx/v5 v5.11.0
	github.com/opentracing/opentracing-go v1.2.0 // indirect
	github.com/otiai10/primes v0.0.0-20210501021515-f1b2be525a11 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	go.uber.org/zap v1.28.0 // indirect
	golang.org/x/crypto v0.59.0
	google.golang.org/protobuf v1.36.12 // indirect
)

replace github.com/agl/ed25519 => github.com/binance-chain/edwards25519 v0.0.0-20200305024217-f36fc4b53d43

replace github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go => ../../packages/protocol/go
