module github.com/0xmhha/nu-54v-dk-toy/products/p09-travel-ai

go 1.26.0

require github.com/jackc/pgx/v5 v5.11.0

require (
	github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go v0.2.0
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/jackc/puddle/v2 v2.2.2 // indirect
	golang.org/x/sync v0.25.0 // indirect
	golang.org/x/text v0.44.0 // indirect
)

replace github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go => ../../packages/protocol/go
