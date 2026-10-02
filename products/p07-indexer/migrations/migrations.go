// Package migrations holds the P07 schema; the indexer applies it at start.
package migrations

import _ "embed"

//go:embed 001_receipts.sql
var Receipts string
