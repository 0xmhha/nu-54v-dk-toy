// Command opsctl is the P05 operations CLI ([N20]). Each command group maps to
// a future opsd API resource. Commands are implemented per WBS2-P05-01..04.
package main

import (
	"context"
	"crypto/ecdsa"
	"crypto/rand"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"

	"github.com/0xmhha/nu-54v-dk-toy/products/p05-operations-backoffice/internal/ble"
	"github.com/0xmhha/nu-54v-dk-toy/products/p05-operations-backoffice/internal/core"
	"github.com/0xmhha/nu-54v-dk-toy/products/p05-operations-backoffice/internal/core/ops"
)

// commands lists the groups and the WBS task that implements them; "done" marks the
// commands that run now.
var commands = map[string]string{
	"merchant":    "register (done) | revoke | payout-change | payout-cancel   (WBS2-P05-01, P05-04)",
	"attestation": "issue (done)                                              (WBS2-P05-01)",
	"anchor":      "sign (done)   (TimeAnchor for the development setup)     (WBS2-P05-02)",
	"order":       "sign (done)   (test merchant)                             (WBS2-P05-01)",
	"rental":      "deposit (done) | provision (done) | re-anchor (done) | return   (WBS2-P05-02, P05-03)",
	"token":       "mint (done)   (test token, token-owner key)                (development setup)",
	"withdraw":    "request | cancel | execute                                (WBS2-P05-03)",
	"refusal":     "host   (UNSUPPORTED_TYPE demo)                            (WBS2-P05-04)",
	"key":         "handover   (test merchant key to kiosk secretRef)         (WBS2-P05-01)",
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: opsctl <group> <command> [flags]   (opsctl <group> <command> -h for flags)")
	names := make([]string, 0, len(commands))
	for n := range commands {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		fmt.Fprintf(os.Stderr, "  %-12s %s\n", n, commands[n])
	}
}

func main() {
	if len(os.Args) < 3 || commands[os.Args[1]] == "" {
		usage()
		os.Exit(2)
	}
	run := map[string]func([]string) error{
		"merchant register": merchantRegister,
		"attestation issue": attestationIssue,
		"anchor sign":       anchorSign,
		"order sign":        orderSign,
		"rental deposit":    rentalDeposit,
		"rental provision":  rentalProvision,
		"rental re-anchor":  rentalReanchor,
		"token mint":        tokenMint,
	}[os.Args[1]+" "+os.Args[2]]
	if run == nil {
		fmt.Fprintf(os.Stderr, "opsctl %s %s: not implemented yet (%s)\n", os.Args[1], os.Args[2], strings.TrimSpace(commands[os.Args[1]]))
		os.Exit(1)
	}
	if err := run(os.Args[3:]); err != nil {
		fmt.Fprintf(os.Stderr, "opsctl %s %s: %v\n", os.Args[1], os.Args[2], err)
		os.Exit(1)
	}
}

// ---------------------------------------------------------------- shared flags

type env struct {
	repo, rpc, deployment, keystores, read string
	dep                                    core.Deployment
}

func commonFlags(fs *flag.FlagSet) *env {
	e := &env{}
	home, _ := os.UserHomeDir()
	fs.StringVar(&e.repo, "repo", findRepo(), "repository root (register and deployment files)")
	fs.StringVar(&e.rpc, "rpc", "https://api.test.stablenet.network/", "RPC endpoint")
	fs.StringVar(&e.deployment, "deployment", "", "deployment record (default <repo>/products/p06-stablenet-contracts/deployments/8283.json)")
	fs.StringVar(&e.keystores, "keystores", filepath.Join(home, ".nu54", "keystores"), "directory of nu54-<role> keystores")
	fs.StringVar(&e.read, "read", "finalized", "block for reads and the anchor time: finalized, or latest (local sandbox only)")
	return e
}

func (e *env) load() error {
	if e.deployment == "" {
		e.deployment = filepath.Join(e.repo, "products/p06-stablenet-contracts/deployments/8283.json")
	}
	if e.read != "finalized" && e.read != "latest" {
		return errors.New("--read must be finalized or latest")
	}
	if err := checkRoles(filepath.Join(filepath.Dir(e.keystores), "testnet-accounts.env")); err != nil {
		return err
	}
	var err error
	e.dep, err = core.LoadDeployment(e.deployment)
	return err
}

// checkRoles stops when two role keys share an address (P05 design 1). The public role
// addresses come from the account list script/testnet-accounts.sh writes.
func checkRoles(path string) error {
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil // no role accounts on this machine yet; each command still needs its keystore
	}
	if err != nil {
		return err
	}
	roles := map[string]common.Address{}
	for _, line := range strings.Split(string(b), "\n") {
		k, v, ok := strings.Cut(strings.TrimSpace(line), "=")
		if ok && strings.HasPrefix(k, "NU54_ADDR_") && common.IsHexAddress(v) {
			roles[k] = common.HexToAddress(v)
		}
	}
	return core.DistinctRoles(roles)
}

func (e *env) dial(ctx context.Context) (*core.Chain, error) {
	c, err := core.Dial(ctx, e.rpc, e.dep)
	if err == nil && e.read == "latest" {
		c.ReadLatest()
	}
	return c, err
}

// roleKey opens nu54-<role> with its password from the Keychain (secretRef keychain:nu54-<role>).
func (e *env) roleKey(role string) (*ecdsa.PrivateKey, error) {
	return core.LoadKey(filepath.Join(e.keystores, "nu54-"+role), "keychain:nu54-"+role)
}

func findRepo() string {
	dir, _ := os.Getwd()
	for d := dir; d != filepath.Dir(d); d = filepath.Dir(d) {
		if _, err := os.Stat(filepath.Join(d, "go.work")); err == nil {
			return d
		}
	}
	return dir
}

func address(s, name string) (common.Address, error) {
	if !common.IsHexAddress(s) {
		return common.Address{}, fmt.Errorf("--%s: not an address", name)
	}
	return common.HexToAddress(s), nil
}

// emit prints the JSON result and appends it to evidence/p05/<date>-<command>.log (git-ignored).
func emit(e *env, command string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(b))
	dir := filepath.Join(e.repo, "evidence", "p05")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	f, err := os.OpenFile(filepath.Join(dir, time.Now().UTC().Format("2006-01-02")+"-"+command+".log"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = fmt.Fprintf(f, "%s %s\n", time.Now().UTC().Format(time.RFC3339), b)
	return err
}

// ---------------------------------------------------------------- commands

func merchantRegister(args []string) error {
	fs := flag.NewFlagSet("merchant register", flag.ExitOnError)
	e := commonFlags(fs)
	merchant := fs.String("merchant", "", "merchant signing address (the kiosk's merchant key)")
	payout := fs.String("payout", "", "payout address")
	_ = fs.Parse(args)
	if err := e.load(); err != nil {
		return err
	}
	m, err := address(*merchant, "merchant")
	if err != nil {
		return err
	}
	p, err := address(*payout, "payout")
	if err != nil {
		return err
	}
	admin, err := e.roleKey("registry-admin")
	if err != nil {
		return err
	}
	ctx := context.Background()
	chain, err := e.dial(ctx)
	if err != nil {
		return err
	}
	tx, err := chain.RegisterMerchant(ctx, admin, m, p)
	if err != nil {
		return err
	}
	result := map[string]any{"merchant": m, "payout": p, "registry": e.dep.Registry, "tx": nil}
	if tx != (common.Hash{}) {
		result["tx"] = tx
	}
	return emit(e, "merchant-register", result)
}

func attestationIssue(args []string) error {
	fs := flag.NewFlagSet("attestation issue", flag.ExitOnError)
	e := commonFlags(fs)
	merchant := fs.String("merchant", "", "merchant signing address")
	payout := fs.String("payout", "", "payout address (must equal the registry payout)")
	name := fs.String("name", "", "merchant name shown on the phone app")
	_ = fs.Parse(args)
	if err := e.load(); err != nil {
		return err
	}
	m, err := address(*merchant, "merchant")
	if err != nil {
		return err
	}
	p, err := address(*payout, "payout")
	if err != nil {
		return err
	}
	if *name == "" {
		return errors.New("--name is required")
	}
	validity, err := core.RegisterParameter(e.repo, "attestationValidity")
	if err != nil {
		return err
	}
	operator, err := e.roleKey("operator")
	if err != nil {
		return err
	}
	ctx := context.Background()
	chain, err := e.dial(ctx)
	if err != nil {
		return err
	}
	now, err := chain.FinalizedTime(ctx)
	if err != nil {
		return err
	}
	a, err := ops.IssueAttestation(chain, e.dep.Domain(), operator, m, p, *name, now, validity)
	if err != nil {
		return err
	}
	return emit(e, "attestation-issue", a)
}

func anchorSign(args []string) error {
	fs := flag.NewFlagSet("anchor sign", flag.ExitOnError)
	e := commonFlags(fs)
	device := fs.String("device", "", "device address (from the keygen ack)")
	last := fs.Uint64("last-anchor", 0, "lastAnchor the device reported; the new timestamp must be later")
	_ = fs.Parse(args)
	if err := e.load(); err != nil {
		return err
	}
	d, err := address(*device, "device")
	if err != nil {
		return err
	}
	operator, err := e.roleKey("operator")
	if err != nil {
		return err
	}
	ctx := context.Background()
	chain, err := e.dial(ctx)
	if err != nil {
		return err
	}
	ts, err := chain.FinalizedTime(ctx) // finalized block time, not the host clock (P05-FR-06)
	if err != nil {
		return err
	}
	a, err := ops.SignTimeAnchor(e.dep.Domain(), operator, d, ts, *last)
	if err != nil {
		return err
	}
	return emit(e, "anchor-sign", a)
}

func orderSign(args []string) error {
	fs := flag.NewFlagSet("order sign", flag.ExitOnError)
	e := commonFlags(fs)
	role := fs.String("merchant-role", "kiosk", "keystore role that holds the test merchant key")
	orderID := fs.String("order-id", "", "32-byte order id (0x-hex)")
	amount := fs.String("amount", "", "amount in token base units")
	payout := fs.String("payout", "", "payout address")
	expiry := fs.Uint64("expiry", 0, "expiry (unix seconds)")
	_ = fs.Parse(args)
	if err := e.load(); err != nil {
		return err
	}
	p, err := address(*payout, "payout")
	if err != nil {
		return err
	}
	amt, ok := new(big.Int).SetString(*amount, 10)
	id, idErr := hexutil.Decode(*orderID)
	if !ok || idErr != nil || len(id) != 32 || *expiry == 0 {
		return errors.New("--order-id (32 bytes), --amount and --expiry are required")
	}
	key, err := e.roleKey(*role)
	if err != nil {
		return err
	}
	o := ops.MerchantOrder{OrderID: common.BytesToHash(id), Token: e.dep.Token, Amount: amt, Payout: p, Expiry: *expiry}
	sig, err := ops.SignMerchantOrder(e.dep.Domain(), key, o)
	if err != nil {
		return err
	}
	return emit(e, "order-sign", map[string]any{"order": o, "merchant": core.Address(key), "merchantSignature": hexutil.Bytes(sig)})
}

func amountFlag(fs *flag.FlagSet) *string {
	return fs.String("amount", "", "amount in test token base units (6 decimals: 50000000 = 50)")
}

func parseAmount(s string) (*big.Int, error) {
	n, ok := new(big.Int).SetString(s, 10)
	if !ok || n.Sign() <= 0 {
		return nil, errors.New("--amount must be a positive integer of base units")
	}
	return n, nil
}

// rentalDeposit funds a device account (the depositFor step of rental provision, used alone
// for the development setup until the BLE setup session exists).
func rentalDeposit(args []string) error {
	fs := flag.NewFlagSet("rental deposit", flag.ExitOnError)
	e := commonFlags(fs)
	device := fs.String("device", "", "device address")
	withdraw := fs.String("withdraw", "", "renter's withdraw address (fixed on the first deposit)")
	amount := amountFlag(fs)
	_ = fs.Parse(args)
	if err := e.load(); err != nil {
		return err
	}
	d, err := address(*device, "device")
	if err != nil {
		return err
	}
	w, err := address(*withdraw, "withdraw")
	if err != nil {
		return err
	}
	amt, err := parseAmount(*amount)
	if err != nil {
		return err
	}
	operator, err := e.roleKey("operator")
	if err != nil {
		return err
	}
	ctx := context.Background()
	chain, err := e.dial(ctx)
	if err != nil {
		return err
	}
	tx, err := chain.Deposit(ctx, operator, d, amt, w)
	if err != nil {
		return err
	}
	return emit(e, "rental-deposit", map[string]any{"device": d, "amount": amt.String(), "withdraw": w, "tx": tx})
}

// openSetup finds the nearest device in setup reach over BLE and opens a setup session.
func openSetup(ctx context.Context, scan time.Duration) (*ble.Session, ble.Found, error) {
	fmt.Fprintln(os.Stderr, "scanning for the device...")
	found, err := ble.Scan(ctx, scan)
	if err != nil {
		return nil, found, err
	}
	fmt.Fprintf(os.Stderr, "connecting to %s (%s, RSSI %d)\n", found.Name, found.Address.String(), found.RSSI)
	link, err := ble.Connect(found)
	if err != nil {
		return nil, found, err
	}
	s, err := ble.Open(ctx, link, ble.Options{})
	if err != nil {
		_ = link.Close()
		return nil, found, err
	}
	return s, found, nil
}

// rentalProvision runs the rental setup over BLE (payment-protocol.md 5): the renter confirms the
// operator values and sets the PIN on the device; opsctl anchors it and deposits for it.
func rentalProvision(args []string) error {
	fs := flag.NewFlagSet("rental provision", flag.ExitOnError)
	e := commonFlags(fs)
	withdraw := fs.String("withdraw", "", "renter's withdraw address (fixed on the first deposit)")
	amount := amountFlag(fs)
	passkey := fs.Int("passkey", -1, "pairing passkey 0-999999 for the label (default: random)")
	scan := fs.Duration("scan", 10*time.Second, "how long to scan for the device")
	_ = fs.Parse(args)
	if err := e.load(); err != nil {
		return err
	}
	w, err := address(*withdraw, "withdraw")
	if err != nil {
		return err
	}
	amt, err := parseAmount(*amount)
	if err != nil {
		return err
	}
	pk := *passkey
	if pk < 0 {
		n, err := rand.Int(rand.Reader, big.NewInt(1_000_000))
		if err != nil {
			return err
		}
		pk = int(n.Int64())
	}
	if pk > 999999 {
		return errors.New("--passkey has at most six digits")
	}
	operator, err := e.roleKey("operator")
	if err != nil {
		return err
	}
	ctx := context.Background()
	chain, err := e.dial(ctx)
	if err != nil {
		return err
	}
	s, found, err := openSetup(ctx, *scan)
	if err != nil {
		return err
	}
	defer s.Close()
	if s.State != "UNPROVISIONED" {
		return fmt.Errorf("the device is %s, not UNPROVISIONED: use rental re-anchor, or return it first", s.State)
	}
	res, err := ops.Provision(ctx, s, ops.ProvisionParams{
		Domain: e.dep.Domain(), Operator: operator, Passkey: uint32(pk), Amount: amt, Withdraw: w,
		Progress: func(step string) { fmt.Fprintln(os.Stderr, "renter:", step) },
	}, chain.FinalizedTime, func(ctx context.Context, d common.Address, a *big.Int, w common.Address) (common.Hash, error) {
		return chain.Deposit(ctx, operator, d, a, w)
	})
	if err != nil {
		return err
	}
	// The label QR carries the BLE address and the passkey for later bonding (payment-protocol.md 3).
	return emit(e, "rental-provision", map[string]any{"result": res, "bleAddress": found.Address.String(),
		"label": fmt.Sprintf("NU54:%s:%s", found.Address.String(), res.Passkey), "withdraw": w, "amount": amt.String()})
}

// rentalReanchor gives a PROVISIONED_NO_ANCHOR device a fresh TimeAnchor after a reset.
func rentalReanchor(args []string) error {
	fs := flag.NewFlagSet("rental re-anchor", flag.ExitOnError)
	e := commonFlags(fs)
	scan := fs.Duration("scan", 10*time.Second, "how long to scan for the device")
	_ = fs.Parse(args)
	if err := e.load(); err != nil {
		return err
	}
	operator, err := e.roleKey("operator")
	if err != nil {
		return err
	}
	ctx := context.Background()
	chain, err := e.dial(ctx)
	if err != nil {
		return err
	}
	s, _, err := openSetup(ctx, *scan)
	if err != nil {
		return err
	}
	defer s.Close()
	if s.State != "PROVISIONED_NO_ANCHOR" || !common.IsHexAddress(s.Device) {
		return fmt.Errorf("the device is %s: re-anchor is for PROVISIONED_NO_ANCHOR", s.State)
	}
	ts, err := chain.FinalizedTime(ctx)
	if err != nil {
		return err
	}
	device := common.HexToAddress(s.Device)
	a, err := ops.SignTimeAnchor(e.dep.Domain(), operator, device, ts, s.LastAnchor)
	if err != nil {
		return err
	}
	last, err := s.SendTimeAnchor(ctx, strings.ToLower(device.Hex()), ts, a.OperatorSignature)
	if err != nil {
		return err
	}
	return emit(e, "rental-re-anchor", map[string]any{"device": device, "anchor": ts, "lastAnchor": last})
}

func tokenMint(args []string) error {
	fs := flag.NewFlagSet("token mint", flag.ExitOnError)
	e := commonFlags(fs)
	to := fs.String("to", "", "recipient (refusal mode must be off)")
	amount := amountFlag(fs)
	_ = fs.Parse(args)
	if err := e.load(); err != nil {
		return err
	}
	t, err := address(*to, "to")
	if err != nil {
		return err
	}
	amt, err := parseAmount(*amount)
	if err != nil {
		return err
	}
	owner, err := e.roleKey("token-owner")
	if err != nil {
		return err
	}
	ctx := context.Background()
	chain, err := e.dial(ctx)
	if err != nil {
		return err
	}
	tx, err := chain.MintTestToken(ctx, owner, t, amt)
	if err != nil {
		return err
	}
	return emit(e, "token-mint", map[string]any{"to": t, "amount": amt.String(), "tx": tx})
}
