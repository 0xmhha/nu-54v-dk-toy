package chainview_test

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/0xmhha/nu-54v-dk-toy/packages/contracts-abi/go/registry"
	v1 "github.com/0xmhha/nu-54v-dk-toy/packages/contracts-abi/go/settlement"
	extended "github.com/0xmhha/nu-54v-dk-toy/packages/contracts-abi/go/settlementext"
	"github.com/0xmhha/nu-54v-dk-toy/packages/contracts-abi/go/testusdc"
	wire "github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/protocol"
	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/rentalv1"
	"github.com/0xmhha/nu-54v-dk-toy/products/p07-indexer/internal/chainview"
	"github.com/0xmhha/nu-54v-dk-toy/products/p07-indexer/internal/evidencev2"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/jackc/pgx/v5"
)

const (
	stream = "098cdd27-c6d0-46c4-914e-27810d349f28"
	active = "c9c0dc8c-d40c-4f13-8c38-b6e1d1130d79"
	branch = "c0b6ebd9-c5c0-4bfa-8837-2e4c1492756c"
	board  = "b5eaa23d-6a8c-40e6-ad7d-d77412b83cb6"
)

type fixtureFreeze struct {
	authority chainview.GenerationReceiptAuthority
	revoked   *atomic.Bool
}

func (f fixtureFreeze) GenerationReceiptAuthority(_ context.Context, grant evidencev2.GenerationGrant, challenge common.Hash) (chainview.GenerationReceiptAuthority, error) {
	if f.revoked.Load() || grant.Branch != f.authority.Binding.Branch || grant.Board != f.authority.Binding.Board ||
		grant.Stream != stream || challenge != f.authority.Challenge {
		return chainview.GenerationReceiptAuthority{}, errors.New("fixture freeze unverified")
	}
	return f.authority, nil
}

type checkedReader func(context.Context, []byte, chainview.VerifiedGenerationFreeze, *rentalv1.Anchor, func() time.Time) chainview.GenerationAssessment

func (f checkedReader) ReconcileGeneration(ctx context.Context, data []byte, freeze chainview.VerifiedGenerationFreeze, previous *rentalv1.Anchor, clock func() time.Time) chainview.GenerationAssessment {
	return f(ctx, data, freeze, previous, clock)
}

func signedFreeze(t *testing.T, key *ecdsa.PrivateKey, projection []byte, challenge common.Hash, frozenAt, expiry uint64) []byte {
	t.Helper()
	binding, root, err := chainview.ParseGenerationProjection(projection)
	if err != nil {
		t.Fatal(err)
	}
	commitment, err := chainview.GenerationBindingDigest(binding)
	if err != nil {
		t.Fatal(err)
	}
	receipt := make([]byte, 188)
	copy(receipt[:4], "RFR1")
	copy(receipt[4:36], commitment[:])
	copy(receipt[36:68], challenge[:])
	copy(receipt[68:100], root[:])
	copy(receipt[100:164], projection[212:276])
	binary.BigEndian.PutUint64(receipt[164:172], uint64(projection[276]))
	binary.BigEndian.PutUint64(receipt[172:180], frozenAt)
	binary.BigEndian.PutUint64(receipt[180:188], expiry)
	separator := crypto.Keccak256(
		crypto.Keccak256([]byte("EIP712Domain(string name,string version,uint256 chainId,address verifyingContract)")),
		crypto.Keccak256([]byte("NU54 Payment Settlement")), crypto.Keccak256([]byte("1")),
		common.LeftPadBytes(big.NewInt(8283).Bytes(), 32), common.LeftPadBytes(binding.Settlement[:], 32))
	fields := append([]byte(nil), receipt[4:164]...)
	for _, field := range []uint64{uint64(projection[276]), frozenAt, expiry} {
		fields = append(fields, common.LeftPadBytes(new(big.Int).SetUint64(field).Bytes(), 32)...)
	}
	const encodeType = "RentalFreezeReceipt(bytes32 binding,bytes32 challenge,bytes32 projectionRoot,uint256 firstNonce,uint256 nextNonce,uint64 entryCount,uint64 frozenAt,uint64 expiresAt)"
	digest := crypto.Keccak256Hash([]byte{0x19, 0x01}, separator, crypto.Keccak256(crypto.Keccak256([]byte(encodeType)), fields))
	sig, err := crypto.Sign(digest[:], key)
	if err != nil {
		t.Fatal(err)
	}
	sig[64] += 27
	return append(receipt, sig...)
}

func generationTLS(t *testing.T, now time.Time, handler func(string) http.Handler) (*httptest.Server, *http.Client) {
	t.Helper()
	pub, caKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	ca := x509.Certificate{SerialNumber: big.NewInt(1), IsCA: true, BasicConstraintsValid: true,
		NotBefore: now.Add(-time.Hour), NotAfter: now.Add(time.Hour), KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature}
	root, err := x509.CreateCertificate(rand.Reader, &ca, &ca, pub, caKey)
	if err != nil {
		t.Fatal(err)
	}
	rootCA, err := x509.ParseCertificate(root)
	if err != nil {
		t.Fatal(err)
	}
	pub, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	leaf := x509.Certificate{SerialNumber: big.NewInt(2), NotBefore: ca.NotBefore, NotAfter: ca.NotAfter,
		KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}}
	raw, err := x509.CreateCertificate(rand.Reader, &leaf, rootCA, pub, caKey)
	if err != nil {
		t.Fatal(err)
	}
	fingerprint := sha256.Sum256(raw)
	server := httptest.NewUnstartedServer(handler(hex.EncodeToString(fingerprint[:])))
	roots := x509.NewCertPool()
	roots.AddCert(rootCA)
	server.TLS = evidencev2.TLSConfig(roots)
	server.StartTLS()
	t.Cleanup(server.Close)
	settings := server.Client().Transport.(*http.Transport).TLSClientConfig.Clone()
	settings.Certificates = []tls.Certificate{{Certificate: [][]byte{raw}, PrivateKey: key}}
	client := &http.Client{Transport: &http.Transport{TLSClientConfig: settings}, Timeout: 20 * time.Second}
	t.Cleanup(client.CloseIdleConnections)
	return server, client
}

func TestGenerationReaderRealV1PostgresMutualTLS(t *testing.T) {
	address, dsn := os.Getenv("P07_GENERATION_TEST_RPC"), os.Getenv("P07_GENERATION_TEST_DATABASE_URL")
	artifacts := os.Getenv("P07_GENERATION_TEST_ARTIFACTS")
	if address == "" || dsn == "" || artifacts == "" {
		t.Skip("requires a disposable v1 Anvil, fresh contract artifacts and migrated PostgreSQL")
	}
	config, err := pgx.ParseConfig(dsn)
	u, parseErr := url.Parse(address)
	if err != nil || parseErr != nil || u.Scheme != "http" || u.Hostname() != "127.0.0.1" ||
		!strings.HasPrefix(config.Database, "nu54_generation_") ||
		!(strings.HasPrefix(config.Host, "/tmp/") || strings.HasPrefix(config.Host, "/private/tmp/")) {
		t.Fatal("fixture must use loopback Anvil and a named disposable PostgreSQL")
	}
	timeout := 60 * time.Second
	if os.Getenv("P05_O09_CHROME_DIST") != "" || os.Getenv("P05_O09_RENTER_V1_UPGRADE") == "1" {
		timeout = 150 * time.Second
	}
	if os.Getenv("P05_O09_W12_INSTALLED_RUNNER") != "" {
		timeout = 210 * time.Second
	} else if installedW12Credential() {
		timeout = 90 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	client, err := ethclient.DialContext(ctx, address)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	protectedClose := os.Getenv("P05_O09_PROTECTED_CLOSE_EXECUTOR") != "" || os.Getenv("P05_O09_SUPERVISED_CLOSE") != "" || os.Getenv("P05_O09_RENTER_ASSESSMENT") == "zero" || os.Getenv("P05_O09_RENTER_ASSESSMENT") == "limit"
	profile, projection, liveSignature, t0, manifest, fixtureOperator := deployGeneration(t, ctx, client, artifacts, protectedClose)
	db, err := pgx.ConnectConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close(context.Background())
	if _, err := db.Exec(ctx, "CREATE ROLE rental_o09_test_ingest LOGIN IN ROLE nu54_p07_ingest"); err != nil {
		t.Fatal(err)
	}
	// ConnString on a modified pgx config returns its original input.
	ingestDSN := dsn + " user=rental_o09_test_ingest"
	repo, err := chainview.Open(ctx, ingestDSN)
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	source, err := chainview.Dial(ctx, address, profile)
	if err != nil {
		t.Fatal(err)
	}
	defer source.Close()
	reader := &chainview.Reader{Profile: profile, Source: source, DB: repo}
	if err := repo.Register(ctx, profile); err != nil {
		t.Fatal(err)
	}
	poll := func() {
		t.Helper()
		if err := reader.Poll(ctx); err != nil {
			t.Fatal(err)
		}
	}
	poll()
	var switchID string
	err = db.QueryRow(ctx, `INSERT INTO chainview.switch_request
		(id,stream_id,target_id,expected_revision,operator_ref,compare_ref,requested_at)
		VALUES(gen_random_uuid(),$1,$2,0,gen_random_uuid(),chainview.record_comparison($1,$2,0),now()) RETURNING id::text`,
		stream, active).Scan(&switchID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, "SELECT chainview.apply_switch($1)", switchID); err != nil {
		t.Fatal(err)
	}

	now := time.Unix(int64(t0)+30, 0).UTC()
	clock := func() time.Time { return now }
	verified, _, err := chainview.ParseGenerationProjection(projection)
	if err != nil {
		t.Fatal(err)
	}
	boardKey, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	wrongKey, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	challenge := common.Hash{31: 77}
	t.Run("DedicatedDaemon", func(t *testing.T) {
		verifyGenerationDaemon(t, ctx, db, dsn, address, profile, projection, boardKey, challenge, t0, manifest, fixtureOperator, liveSignature)
	})
	if protectedClose || os.Getenv("P05_O09_RENTER_ASSESSMENT") != "" || os.Getenv("P05_O09_W12_INSTALLED_RUNNER") != "" {
		return
	}
	// The daemon case can advance the live final head. Keep the immediate
	// synthetic assessment clock after that checkpoint but before expiry.
	currentHead, err := client.HeaderByNumber(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if headClock := time.Unix(int64(currentHead.Time)+1, 0).UTC(); headClock.After(now) {
		now = headClock
	}
	var boardRevoked, revokeAfterRead atomic.Bool
	author := fixtureFreeze{revoked: &boardRevoked, authority: chainview.GenerationReceiptAuthority{
		Binding: verified, BoardSigner: crypto.PubkeyToAddress(boardKey.PublicKey),
		Operator: common.Address{19: 71}, Challenge: challenge, ChallengeUntil: time.Unix(int64(t0)+480, 0),
	}}
	verifier := evidencev2.RegisteredGenerationVerifier{Authority: author, Clock: clock}
	guarded := checkedReader(func(ctx context.Context, data []byte, freeze chainview.VerifiedGenerationFreeze, previous *rentalv1.Anchor, clock func() time.Time) chainview.GenerationAssessment {
		result := reader.ReconcileGeneration(ctx, data, freeze, previous, clock)
		if result.PendingCount != nil && revokeAfterRead.Load() {
			boardRevoked.Store(true)
		}
		return result
	})
	server, tlsClient := generationTLS(t, now, func(fingerprint string) http.Handler {
		return evidencev2.GenerationHandler(func(got string) (evidencev2.GenerationGrant, bool) {
			return evidencev2.GenerationGrant{Stream: stream, Branch: branch, Board: board, ExpiresAt: time.Unix(int64(t0)+480, 0)}, got == fingerprint
		}, verifier, func(got string) (evidencev2.GenerationReader, bool) { return guarded, got == stream }, clock)
	})
	var previous *rentalv1.Anchor
	post := func(data []byte, authenticate bool) chainview.GenerationAssessment {
		t.Helper()
		key := boardKey
		if !authenticate {
			key = wrongKey
		}
		receipt := signedFreeze(t, key, data, challenge, t0+15, uint64(now.Unix()+60))
		body, _ := json.Marshal(rentalv1.Request{Projection: data, Receipt: receipt, Previous: previous})
		resp, err := tlsClient.Post(server.URL+"/v2/private/rental-generation/reconcile", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		raw, err := io.ReadAll(resp.Body)
		var reply chainview.GenerationAssessment
		if resp.StatusCode != 200 || err != nil || json.Unmarshal(raw, &reply) != nil || bytes.Contains(raw, liveSignature) ||
			strings.Contains(string(raw), "merchant") || strings.Contains(string(raw), "projection") {
			t.Fatalf("unexpected generation reply: status %d", resp.StatusCode)
		}
		if reply.PendingCount != nil {
			freeze, err := chainview.VerifyGenerationReceipt(receipt, author.authority, now)
			expected := rentalv1.Binding{Stream: stream, StreamGeneration: active, Deployment: "0x" + profile.Manifest,
				FreezeReceipt: freeze.ReceiptDigest.Hex(), JournalRoot: freeze.Root.Hex()}
			if err != nil || reply.Validate(expected, int(freeze.EntryCount), freeze.FrozenAt, freeze.ValidUntil, now) != nil {
				t.Fatal("actual Reader reply would not pass the shared consumer binding")
			}
		}
		return reply
	}
	if bytes.Contains(projection, liveSignature) {
		t.Fatal("raw approval signature in projection")
	}
	if hidden := post(projection, false); hidden.Status != "unknown" || hidden.PendingCount != nil {
		t.Fatal("unverified freeze became evidence")
	}
	pending := post(projection, true)
	if pending.Status != "pending" || pending.PendingCount == nil || *pending.PendingCount != 1 || pending.AnchorHash == "" || pending.Digest == "" || pending.PendingPayments == nil || *pending.PendingPayments != 1 || pending.PendingLimits == nil || *pending.PendingLimits != 0 {
		t.Fatalf("expected one genuinely outstanding approval at the active finalized anchor, got %+v", pending)
	}
	previous = &rentalv1.Anchor{Height: *pending.AnchorHeight, Hash: pending.AnchorHash}
	wrongAncestor := *previous
	wrongAncestor.Hash = common.Hash{31: 1}.Hex()
	acceptedPrevious := previous
	previous = &wrongAncestor
	if missing := post(projection, true); missing.Status != "incomplete" || missing.PendingCount != nil {
		t.Fatalf("noncanonical ancestor was accepted: %+v", missing)
	}
	previous = acceptedPrevious
	revokeAfterRead.Store(true)
	if withheld := post(projection, true); withheld.Status != "unknown" || withheld.PendingCount != nil || !boardRevoked.Load() {
		t.Fatalf("revoked enrollment produced an in-flight count: %+v", withheld)
	}
	revokeAfterRead.Store(false)
	boardRevoked.Store(false)
	wrong := append([]byte(nil), projection...)
	wrong[277+126+94] ^= 1
	if withheld := post(wrong, true); withheld.Status != "incomplete" || withheld.PendingCount != nil {
		t.Fatalf("same value limit accepted a different approval: %+v", withheld)
	}
	burnConflict := append([]byte(nil), projection...)
	burnConflict[278] = 2
	if withheld := post(burnConflict, true); withheld.Status != "incomplete" || withheld.PendingCount != nil {
		t.Fatalf("burn claim contradicted used bit: %+v", withheld)
	}
	now = time.Unix(int64(t0)+120, 0)
	atExpiry := post(projection, true)
	if atExpiry.Status != "pending" || atExpiry.PendingCount == nil || *atExpiry.PendingCount != 1 ||
		atExpiry.ValidUntil != int64(t0)+121 {
		t.Fatalf("approval was not conservatively pending through its exact expiry second: %+v", atExpiry)
	}
	now = time.Unix(int64(t0)+121, 0)
	if unavailable := post(projection, true); unavailable.Status != "incomplete" || unavailable.PendingCount != nil {
		t.Fatalf("wall-clock expiry substituted for canonical final expiry: %+v", unavailable)
	}
	var snapshot string
	if err := client.Client().CallContext(ctx, &snapshot, "evm_snapshot"); err != nil {
		t.Fatal(err)
	}
	advanceAnvil(t, ctx, client, t0+122)
	poll()
	head, err := client.HeaderByNumber(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	now = time.Unix(int64(head.Time), 0)
	closed := post(projection, true)
	if closed.Status != "verified" || closed.PendingCount == nil || *closed.PendingCount != 0 || closed.Digest == pending.Digest || closed.Previous == nil || *closed.Previous != *previous {
		t.Fatalf("missing true unused expiry after finality: %+v", closed)
	}
	if _, err := db.Exec(ctx, `UPDATE chainview.block SET canonical=false
		WHERE generation_id=$1 AND height=$2`, active, profile.StartHeight+1); err != nil {
		t.Fatal(err)
	}
	if unavailable := post(projection, true); unavailable.Status != "incomplete" || unavailable.PendingCount != nil {
		t.Fatalf("missing contiguous coverage still returned a count: %+v", unavailable)
	}
	if _, err := db.Exec(ctx, `UPDATE chainview.block SET canonical=true
		WHERE generation_id=$1 AND height=$2`, active, profile.StartHeight+1); err != nil {
		t.Fatal(err)
	}
	var reverted bool
	if err := client.Client().CallContext(ctx, &reverted, "evm_revert", snapshot); err != nil || !reverted {
		t.Fatal("could not restore fixture chain")
	}
	if unavailable := post(projection, true); unavailable.Status != "incomplete" || unavailable.PendingCount != nil {
		t.Fatalf("retired final anchor still returned a count: %+v", unavailable)
	}
}

func advanceAnvil(t *testing.T, ctx context.Context, client *ethclient.Client, timestamp uint64) {
	t.Helper()
	if err := client.Client().CallContext(ctx, nil, "evm_setNextBlockTimestamp", timestamp); err != nil {
		t.Fatal(err)
	}
	if err := client.Client().CallContext(ctx, nil, "anvil_mine", 4); err != nil {
		t.Fatal(err)
	}
}

func deployGeneration(t *testing.T, ctx context.Context, client *ethclient.Client, artifacts string, allSettled bool) (chainview.Profile, []byte, []byte, uint64, []byte, *ecdsa.PrivateKey) {
	t.Helper()
	key, err := crypto.ToECDSA(append(make([]byte, 31), byte(100)))
	if err != nil {
		t.Fatal(err)
	}
	auth, err := bind.NewKeyedTransactorWithChainID(key, big.NewInt(8283))
	if err != nil {
		t.Fatal(err)
	}
	auth.Context, auth.GasLimit = ctx, 10_000_000
	deviceKey, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	device := crypto.PubkeyToAddress(deviceKey.PublicKey)
	mined := func(tx *types.Transaction, err error) *types.Receipt {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
		row, err := bind.WaitMined(ctx, client, tx)
		if err != nil || row.Status != types.ReceiptStatusSuccessful {
			t.Fatalf("actual fixture transaction reverted: %v", err)
		}
		return row
	}
	deploy := func(name string, args ...any) (common.Address, *types.Receipt) {
		t.Helper()
		data, err := os.ReadFile(filepath.Join(artifacts, name+".sol", name+".json"))
		if err != nil {
			t.Fatal(err)
		}
		var fixture struct {
			ABI      json.RawMessage
			Bytecode struct{ Object hexutil.Bytes }
		}
		if err := json.Unmarshal(data, &fixture); err != nil {
			t.Fatal(err)
		}
		spec, err := abi.JSON(bytes.NewReader(fixture.ABI))
		if err != nil {
			t.Fatal(err)
		}
		addr, tx, _, err := bind.DeployContract(auth, spec, fixture.Bytecode.Object, client, args...)
		return addr, mined(tx, err)
	}
	if err := client.Client().CallContext(ctx, nil, "anvil_setBalance", auth.From, "0x3635c9adc5dea00000"); err != nil {
		t.Fatal(err)
	}
	tokenAddress, _ := deploy("TestUSDC", auth.From)
	registryAddress, _ := deploy("MerchantRegistry", auth.From, big.NewInt(86400))
	settlement, creation := deploy("PaymentSettlement", tokenAddress, registryAddress, auth.From,
		big.NewInt(50), big.NewInt(200), big.NewInt(3600), big.NewInt(120))
	book, err := registry.NewRegistry(registryAddress, client)
	if err != nil {
		t.Fatal(err)
	}
	merchant := common.Address{19: 81}
	if os.Getenv("P05_O09_RENTER_V1_ORIGIN") == "1" {
		merchant = auth.From
	}
	mined(book.RegisterMerchant(auth, merchant, auth.From))
	token, err := testusdc.NewTestusdc(tokenAddress, client)
	if err != nil {
		t.Fatal(err)
	}
	legacy, err := v1.NewSettlement(settlement, client)
	if err != nil {
		t.Fatal(err)
	}
	limits, err := extended.NewSettlementext(settlement, client)
	if err != nil {
		t.Fatal(err)
	}
	deposit := big.NewInt(40)
	if mode := os.Getenv("P05_O09_PROTECTED_WITHDRAWAL"); mode == "positive" || mode == "recover" {
		deposit = big.NewInt(65)
	}
	mined(token.Mint(auth, auth.From, deposit))
	mined(token.Approve(auth, settlement, deposit))
	mined(legacy.DepositFor(auth, device, deposit, auth.From))
	latest, err := client.HeaderByNumber(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	t0 := latest.Time
	separator, err := legacy.DomainSeparator(nil)
	if err != nil {
		t.Fatal(err)
	}
	paymentABI, err := v1.SettlementMetaData.GetAbi()
	if err != nil {
		t.Fatal(err)
	}
	limitABI, err := extended.SettlementextMetaData.GetAbi()
	if err != nil {
		t.Fatal(err)
	}
	fixed, err := abi.NewType("bytes32", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	typedDigest := func(name string, method abi.Method, value any) common.Hash {
		t.Helper()
		packed, err := (abi.Arguments{{Type: fixed}, method.Inputs[0]}).Pack(crypto.Keccak256Hash([]byte(wire.EncodeType[name])), value)
		if err != nil {
			t.Fatal(err)
		}
		return crypto.Keccak256Hash([]byte{0x19, 0x01}, separator[:], crypto.Keccak256(packed))
	}
	sign := func(hash common.Hash) []byte {
		t.Helper()
		sig, err := crypto.Sign(hash[:], deviceKey)
		if err != nil {
			t.Fatal(err)
		}
		sig[64] += 27
		return sig
	}
	manifest, err := json.Marshal(map[string]any{"chainId": 8283, "contracts": map[string]any{
		"PaymentSettlement": map[string]any{"address": settlement, "block": creation.BlockNumber.Int64()},
		"MerchantRegistry":  map[string]any{"address": registryAddress},
		"TestUSDC":          map[string]any{"address": tokenAddress},
	}})
	if err != nil {
		t.Fatal(err)
	}
	deployment := common.Hash(sha256.Sum256(manifest))
	code, err := client.CodeAt(ctx, settlement, creation.BlockNumber)
	if err != nil || len(code) == 0 {
		t.Fatal("missing deployed v1 contract")
	}
	profile := chainview.Profile{StreamID: stream, GenerationID: active, Version: 1, ChainID: 8283, Contract: settlement, Asset: tokenAddress,
		ABIHash: "05bbfd86dc59243e5456ce494036f33870e319d66aa96a91ee82e7054670ca4d", Manifest: deployment.Hex()[2:],
		CodeHash: crypto.Keccak256Hash(code), StartHeight: creation.BlockNumber.Int64(), StartHash: creation.BlockHash}
	header := make([]byte, 277)
	copy(header, []byte("RJP1"))
	binary.BigEndian.PutUint64(header[4:12], 3)
	branchBytes, _ := hex.DecodeString(strings.ReplaceAll(branch, "-", ""))
	boardBytes, _ := hex.DecodeString(strings.ReplaceAll(board, "-", ""))
	copy(header[12:28], branchBytes)
	copy(header[28:44], boardBytes)
	header[75], header[107] = 1, 2
	copy(header[108:140], deployment[:])
	binary.BigEndian.PutUint64(header[164:172], 8283)
	copy(header[172:192], settlement[:])
	copy(header[192:212], device[:])
	header[275], header[276] = 6, 6
	projection := header
	add := func(kind, state byte, nonce int64, expiry uint64, merchant common.Address, order, digest common.Hash) {
		entry := make([]byte, 126)
		entry[0], entry[1] = kind, state
		binary.BigEndian.PutUint64(entry[26:34], uint64(nonce))
		binary.BigEndian.PutUint64(entry[34:42], expiry)
		copy(entry[42:62], merchant[:])
		copy(entry[62:94], order[:])
		copy(entry[94:], digest[:])
		projection = append(projection, entry...)
	}
	limit := func(nonce int64, expiry uint64, submit bool, state byte) {
		c := extended.PaymentTypesLimitChange{ChainId: big.NewInt(8283), ContractAddress: settlement,
			PerPaymentLimit: big.NewInt(50), DailyLimit: big.NewInt(200), Nonce: big.NewInt(nonce), Expiry: expiry}
		digest := typedDigest("LimitChange", limitABI.Methods["setLimits"], c)
		if state == 1 {
			sig := sign(digest)
			if submit {
				mined(limits.SetLimits(auth, c, sig))
			}
		}
		add(2, state, nonce, expiry, common.Address{}, common.Hash{}, digest)
	}
	payment := func(nonce int64, submit bool) []byte {
		order := common.Hash{31: byte(nonce + 10)}
		expiry := t0 + 120
		if !submit {
			expiry = w12OrderExpiry(t0)
		}
		a := v1.PaymentTypesPaymentAuthorization{ChainId: big.NewInt(8283), ContractAddress: settlement,
			Merchant: merchant, Payout: auth.From, Token: tokenAddress, Amount: big.NewInt(20), OrderId: order,
			Nonce: big.NewInt(nonce), Expiry: expiry}
		digest := typedDigest("PaymentAuthorization", paymentABI.Methods["settle"], a)
		sig := sign(digest)
		if submit {
			mined(legacy.Settle(auth, a, sig))
		}
		add(1, 1, nonce, a.Expiry, merchant, order, digest)
		return sig
	}
	limit(0, t0+100, true, 1)
	limit(1, t0+100, true, 1)
	payment(2, true)
	live := payment(3, allSettled)
	limitExpiry := t0 + 10
	if mode := os.Getenv("P05_O09_RENTER_ASSESSMENT"); mode == "limit" || mode == "mixed" {
		limitExpiry = w12OrderExpiry(t0)
	}
	limit(4, limitExpiry, false, 1)
	limit(5, t0+100, false, 2)
	advanceAnvil(t, ctx, client, t0+20)
	return profile, projection, live, t0, manifest, key
}
