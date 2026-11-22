package ops

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"regexp"
	"strconv"

	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/rentalv1"
	"github.com/0xmhha/nu-54v-dk-toy/products/p05-operations-backoffice/internal/ble"
	"github.com/0xmhha/nu-54v-dk-toy/products/p05-operations-backoffice/internal/core"
)

type RentalCollectorClient struct {
	client         *http.Client
	endpoint       string
	recordEndpoint string
}

var collectorUUID = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

func ValidRentalCollectionRequestKey(key string) bool { return collectorUUID.MatchString(key) }

func NewRentalCollectorClient(origin string, roots *x509.CertPool, identity tls.Certificate, pin [32]byte,
	branch, device string, generation uint64) (*RentalCollectorClient, error) {
	if !collectorUUID.MatchString(branch) || !collectorUUID.MatchString(device) || generation == 0 {
		return nil, ErrRentalCollection
	}
	endpoint, client, err := core.NewPinnedRentalHTTP(origin, roots, identity, pin)
	if err != nil {
		return nil, ErrRentalCollection
	}
	basePath := "/v2/private/rental-collector/" + branch + "/" + device + "/" + strconv.FormatUint(generation, 10)
	endpoint.Path = basePath + "/freeze-challenge"
	issue := endpoint.String()
	endpoint.Path = basePath + "/record"
	return &RentalCollectorClient{client: client, endpoint: issue, recordEndpoint: endpoint.String()}, nil
}

func (c *RentalCollectorClient) Close() { c.client.CloseIdleConnections() }

func (c *RentalCollectorClient) Issue(ctx context.Context, channel ble.ProtectedRentalChallenge) (RentalCollectorGrant, error) {
	var empty RentalCollectorGrant
	if c == nil || c.client == nil || channel.Nonce == [32]byte{} || channel.ExpiresAt == 0 {
		return empty, ErrRentalCollection
	}
	body, err := json.Marshal(struct {
		ChannelNonce string `json:"channelNonce"`
		ExpiresAt    uint64 `json:"expiresAt"`
	}{hex.EncodeToString(channel.Nonce[:]), channel.ExpiresAt})
	if err != nil {
		return empty, ErrRentalCollection
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		return empty, ErrRentalCollection
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Cache-Control", "no-store")
	response, err := c.client.Do(req)
	if err != nil {
		return empty, ErrRentalCollection
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK || response.TLS == nil || len(response.TLS.VerifiedChains) == 0 {
		return empty, ErrRentalCollection
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, 1025))
	if err != nil || len(raw) > 1024 {
		return empty, ErrRentalCollection
	}
	var reply struct {
		Binding, Challenge, Signature string
		ExpiresAt                     uint64
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var grant RentalCollectorGrant
	signature := make([]byte, 65)
	if decoder.Decode(&reply) != nil || decoder.Decode(new(any)) != io.EOF ||
		!fixedHex("0x"+reply.Binding, grant.Binding[:]) ||
		!fixedHex("0x"+reply.Challenge, grant.Challenge[:]) ||
		!fixedHex("0x"+reply.Signature, signature) || reply.ExpiresAt == 0 || reply.ExpiresAt > channel.ExpiresAt {
		return empty, ErrRentalCollection
	}
	grant.ExpiresAt, grant.Signature = reply.ExpiresAt, signature
	return grant, nil
}

// Record sends only authenticated board input; the writer independently gets
// and validates the assessment and never accepts counts from the station.
func (c *RentalCollectorClient) Record(ctx context.Context, key string, proof rentalv1.Request) error {
	if c == nil || c.client == nil || !ValidRentalCollectionRequestKey(key) || len(proof.Projection) < 277 ||
		len(proof.Projection) > 1285 || len(proof.Receipt) != rentalv1.FreezeReceiptSize {
		return ErrRentalCollection
	}
	body, err := json.Marshal(struct {
		Key        string `json:"requestKey"`
		Projection []byte `json:"projection"`
		Receipt    []byte `json:"freezeReceipt"`
	}{key, proof.Projection, proof.Receipt})
	if err != nil {
		return ErrRentalCollection
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.recordEndpoint, bytes.NewReader(body))
	if err != nil {
		return ErrRentalCollection
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Cache-Control", "no-store")
	response, err := c.client.Do(req)
	if err != nil {
		return ErrRentalCollection
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK || response.TLS == nil || len(response.TLS.VerifiedChains) == 0 {
		return ErrRentalCollection
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, 129))
	if err != nil || len(raw) > 128 {
		return ErrRentalCollection
	}
	var result struct {
		Recorded bool `json:"recorded"`
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&result) != nil || decoder.Decode(new(any)) != io.EOF || !result.Recorded {
		return ErrRentalCollection
	}
	return nil
}
