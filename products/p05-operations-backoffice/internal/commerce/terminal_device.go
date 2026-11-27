package commerce

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"strings"

	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/service"
	"github.com/ethereum/go-ethereum/common"
)

// TerminalDevice is one kiosk's own settings. The changes and the payment values are for an operator.
type TerminalDevice struct {
	Revision    int64                  `json:"revision"`
	Language    string                 `json:"language"`
	IdleSeconds int                    `json:"idleSeconds"`
	ReadServer  string                 `json:"readServer,omitempty"`
	Changes     []TerminalDeviceChange `json:"changes,omitempty"`
	Payment     *TerminalPayment       `json:"payment,omitempty"`
}

type TerminalDeviceChange struct {
	Name  string `json:"name"`
	From  string `json:"from"`
	To    string `json:"to"`
	Actor string `json:"actor"`
	At    int64  `json:"at"`
}

// TerminalPayment shows what this server was enrolled with. No kiosk can change it.
type TerminalPayment struct {
	ChainID               uint64 `json:"chainId"`
	Asset                 string `json:"asset"`
	Beneficiary           string `json:"beneficiary"`
	Contract              string `json:"contract"`
	Escrow                bool   `json:"escrow"`
	GasSponsor            string `json:"gasSponsor,omitempty"`
	GasAccount            string `json:"gasAccount,omitempty"`
	RegistrationExpiresAt int64  `json:"registrationExpiresAt"`
}

// GasAccounts names who pays a branch's gas. A PaymentForwarder may provide it.
type GasAccounts interface {
	BranchGas(branch string) (sponsor, account common.Address)
}

type terminalDeviceRow struct {
	TerminalDevice
	RegistrationExpiresAt int64 `json:"registrationExpiresAt"`
}

func (h HTTP) deviceView(ctx context.Context, bound service.Bound, row terminalDeviceRow) (*TerminalDevice, error) {
	view := row.TerminalDevice
	if bound.Role != "operator" {
		return &view, nil
	}
	if _, enrolled := h.Profiles[bound.Request.Domain]; !enrolled {
		return &view, nil
	}
	profile, err := h.currentProfile(ctx, bound.Request.Domain)
	if err != nil {
		return nil, err
	}
	lower := func(address common.Address) string {
		if address == (common.Address{}) {
			return ""
		}
		return strings.ToLower(address.Hex())
	}
	view.Payment = &TerminalPayment{ChainID: profile.Chain, Asset: lower(profile.Asset), Beneficiary: lower(profile.Beneficiary),
		Contract: lower(profile.Contract), Escrow: profile.Escrow, RegistrationExpiresAt: row.RegistrationExpiresAt}
	if gas, ok := h.Payment.(GasAccounts); ok {
		sponsor, account := gas.BranchGas(bound.Request.Domain)
		view.Payment.GasSponsor, view.Payment.GasAccount = lower(sponsor), lower(account)
	}
	return &view, nil
}

func (h HTTP) terminalDevice(ctx context.Context, bound service.Bound) (*TerminalDevice, error) {
	var row terminalDeviceRow
	err := h.Store.db.QueryRow(ctx, "SELECT ledger.terminal_device_state($1,$2,$3,$4,$5)",
		bound.Tenant, bound.Request.Domain, bound.Subject, bound.Request.Object, bound.Role).Scan(&row)
	if err != nil {
		return nil, domainError(err)
	}
	return h.deviceView(ctx, bound, row)
}

func (h HTTP) changeTerminalDevice(ctx context.Context, bound service.Bound, value string) (*TerminalDevice, error) {
	body := bound.Request.Body
	digest := sha256.Sum256([]byte(value))
	expected, err := strconv.ParseInt(body["expectedRevision"], 10, 64)
	if err != nil || len(value) > 512 || body["valueDigest"] != hex.EncodeToString(digest[:]) {
		return nil, service.BadRequest
	}
	intent, err := hex.DecodeString(bound.IntentDigest)
	if err != nil {
		return nil, service.BadRequest
	}
	var row terminalDeviceRow
	err = h.Store.db.QueryRow(ctx, "SELECT ledger.change_terminal_device($1,$2,$3,$4,$5,$6,$7,$8,$9)", bound.Tenant, bound.Request.Domain,
		bound.Subject, bound.Request.Object, bound.Request.IdempotencyKey, expected, body["name"], value, intent).Scan(&row)
	if err != nil {
		return nil, domainError(err)
	}
	return h.deviceView(ctx, bound, row)
}
