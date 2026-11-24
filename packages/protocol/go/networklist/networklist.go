// Package networklist checks the operator-signed list of network addresses. The TypeScript SDK
// (network-list.ts) reads the same text; both share network-list.vectors.
package networklist

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"

	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/orderescrow"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

// Limit bounds the signed file, which must fit one commerce reply.
const Limit = 24576

const (
	longestValidity = 400 * 24 * 3600
	clockAllowance  = 300
	maxSafeInteger  = 1<<53 - 1
)

type Token struct {
	Address  string `json:"address"`
	Symbol   string `json:"symbol"`
	Name     string `json:"name"`
	Decimals *int   `json:"decimals"`
}

// Row names a network by exactly one of ChainID (EVM) and ID, and carries no chain anchor.
type Row struct {
	ChainID    int64   `json:"chainId,omitempty"`
	ID         string  `json:"id,omitempty"`
	Name       string  `json:"name"`
	RPC        string  `json:"rpc"`
	Explorer   string  `json:"explorer,omitempty"`
	GasCoinURL string  `json:"gasCoinUrl,omitempty"`
	Tokens     []Token `json:"tokens,omitempty"`
}

type List struct {
	Version   int   `json:"v"`
	IssuedAt  int64 `json:"issuedAt"`
	ExpiresAt int64 `json:"expiresAt"`
	Sequence  int64 `json:"sequence"`
	Networks  []Row `json:"networks"`
}

// Envelope is what the operator publishes: the exact signed text and the signature over it.
type Envelope struct {
	Payload   string `json:"payload"`
	Signature string `json:"signature"`
}

var (
	errShape     = errors.New("invalid network list")
	errSignature = errors.New("network list has another signer")
	errTime      = errors.New("network list is not valid now")

	server    = regexp.MustCompile(`^https://[A-Za-z0-9.\[\]:_-]+(?:/[A-Za-z0-9/_.-]*)?$`)
	networkID = regexp.MustCompile(`^[a-z][a-z0-9-]{1,31}$`)
	tokenRef  = regexp.MustCompile(`^0x[0-9a-f]{40}$`)
	symbol    = regexp.MustCompile(`^[A-Za-z0-9.-]{1,12}$`)
	hidden    = regexp.MustCompile(`[\x{0}-\x{1f}\x{7f}-\x{9f}\x{200b}-\x{200f}\x{2028}-\x{202e}\x{2066}-\x{2069}\x{feff}]`)
)

func text(value string) []byte { return crypto.Keccak256([]byte(value)) }

// Digest is the EIP-712 digest of the exact payload text, in a domain of its own.
func Digest(payload string) common.Hash {
	domain := crypto.Keccak256(text("EIP712Domain(string name,string version)"), text("NU54 Network List"), text("1"))
	return crypto.Keccak256Hash([]byte{0x19, 0x01}, domain, crypto.Keccak256(text("NetworkList(string payload)"), text(payload)))
}

// Payload is the one text form of a valid list.
func (l List) Payload() (string, error) {
	var out bytes.Buffer
	encoder := json.NewEncoder(&out)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(l); err != nil {
		return "", err
	}
	payload := strings.TrimSuffix(out.String(), "\n")
	return payload, l.check()
}

func address(value string) bool { return len(value) <= 512 && server.MatchString(value) }

func label(value string) bool {
	return value != "" && value == strings.TrimSpace(value) && len(utf16.Encode([]rune(value))) <= 40 && !hidden.MatchString(value)
}

func (l List) check() error {
	if l.Version != 1 || l.IssuedAt <= 0 || l.ExpiresAt <= l.IssuedAt || l.ExpiresAt-l.IssuedAt > longestValidity ||
		l.ExpiresAt > maxSafeInteger || l.Sequence <= 0 || l.Sequence > maxSafeInteger || len(l.Networks) < 1 || len(l.Networks) > 32 {
		return errShape
	}
	seen := map[string]bool{}
	for _, row := range l.Networks {
		evm := row.ChainID != 0
		key := row.ID
		if evm {
			key = strconv.FormatInt(row.ChainID, 10)
		}
		if evm == (row.ID != "") || row.ChainID < 0 || row.ChainID > maxSafeInteger || (!evm && !networkID.MatchString(row.ID)) || seen[key] ||
			!label(row.Name) || !address(row.RPC) || (row.Explorer != "" && !address(row.Explorer)) ||
			(row.GasCoinURL != "" && !address(row.GasCoinURL)) || len(row.Tokens) > 8 || (!evm && len(row.Tokens) != 0) {
			return errShape
		}
		seen[key] = true
		tokens := map[string]bool{}
		for _, token := range row.Tokens {
			if !tokenRef.MatchString(token.Address) || common.HexToAddress(token.Address) == (common.Address{}) || tokens[token.Address] ||
				!symbol.MatchString(token.Symbol) || !label(token.Name) || token.Decimals == nil || *token.Decimals < 0 || *token.Decimals > 24 {
				return errShape
			}
			tokens[token.Address] = true
		}
	}
	return nil
}

// Parse reads a list in any JSON layout, as an operator writes it. Unknown fields are refused, so a
// list can never name a chain anchor.
func Parse(raw []byte) (List, error) {
	var list List
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if len(raw) > Limit || decoder.Decode(&list) != nil || decoder.Decode(new(any)) != io.EOF {
		return List{}, errShape
	}
	return list, list.check()
}

// Read checks a published envelope: its signer, its exact text and its time.
func Read(raw []byte, operator common.Address, now time.Time) (Envelope, List, error) {
	var envelope Envelope
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if len(raw) > Limit || decoder.Decode(&envelope) != nil || decoder.Decode(new(any)) != io.EOF {
		return Envelope{}, List{}, errShape
	}
	signature, err := hex.DecodeString(strings.TrimPrefix(envelope.Signature, "0x"))
	if err != nil || envelope.Signature != "0x"+hex.EncodeToString(signature) ||
		!orderescrow.SignedBy(Digest(envelope.Payload), signature, operator) {
		return Envelope{}, List{}, errSignature
	}
	list, err := Parse([]byte(envelope.Payload))
	if err != nil {
		return Envelope{}, List{}, err
	}
	if payload, err := list.Payload(); err != nil || payload != envelope.Payload {
		return Envelope{}, List{}, errShape
	}
	if list.IssuedAt > now.Unix()+clockAllowance || list.ExpiresAt <= now.Unix() {
		return Envelope{}, List{}, errTime
	}
	return envelope, list, nil
}
