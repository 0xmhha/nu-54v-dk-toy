package commerce

import (
	"context"
	"io"
	"os"
	"time"

	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/networklist"
	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/service"
	"github.com/ethereum/go-ethereum/common"
)

// NetworkList is the published file that the operator signed offline. Publish with an atomic rename.
type NetworkList struct {
	Path     string
	Operator common.Address
}

// Read returns the current publication, or an error if it is missing, changed or no longer valid.
func (n NetworkList) Read(now time.Time) (networklist.Envelope, error) {
	file, err := os.Open(n.Path)
	if err != nil {
		return networklist.Envelope{}, err
	}
	defer file.Close()
	raw, err := io.ReadAll(io.LimitReader(file, networklist.Limit+1))
	if err != nil {
		return networklist.Envelope{}, err
	}
	envelope, _, err := networklist.Read(raw, n.Operator, now)
	return envelope, err
}

func (h HTTP) networkList(_ context.Context, bound service.Bound) (*networklist.Envelope, error) {
	if bound.Request.Object != bound.Subject {
		return nil, service.Forbidden
	}
	if h.NetworkList == nil {
		return nil, service.Capability
	}
	envelope, err := h.NetworkList.Read(time.Now())
	if err != nil {
		return nil, service.Capability
	}
	return &envelope, nil
}
