package wallet

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/marketlab"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

const MarketTestProfile = "isolated-market-8283-v1"

type MarketTransfer struct {
	Deployment common.Hash    `json:"deploymentHash"`
	User       common.Address `json:"user"`
	Calldata   hexutil.Bytes  `json:"calldata"`
	Offer      hexutil.Bytes  `json:"offer"`
}

func ReviewMarketTransfer(policy *marketlab.Policy, transfer Transfer, now time.Time) (marketlab.Terms, [32]byte, error) {
	bad := errors.New("invalid isolated market transfer")
	if policy == nil || transfer.Profile != MarketTestProfile || transfer.Chain != "8283" ||
		transfer.Market == nil || transfer.ValueWei != "0" || transfer.PriorityFeeWei != "0" ||
		!common.IsHexAddress(transfer.To) || strings.ToLower(transfer.To) != transfer.To || !strings.HasPrefix(transfer.To, "0x") {
		return marketlab.Terms{}, [32]byte{}, bad
	}
	nonce, err1 := strconv.ParseUint(transfer.Nonce, 10, 64)
	price, err2 := strconv.ParseUint(transfer.MaxFeeWei, 10, 64)
	if err1 != nil || err2 != nil || strconv.FormatUint(nonce, 10) != transfer.Nonce || strconv.FormatUint(price, 10) != transfer.MaxFeeWei {
		return marketlab.Terms{}, [32]byte{}, bad
	}
	input := transfer.Market
	bound, err := marketlab.Review(*policy, marketlab.Request{Deployment: input.Deployment, User: input.User, To: common.HexToAddress(transfer.To),
		Nonce: nonce, GasPrice: price, Calldata: input.Calldata, Offer: input.Offer}, now)
	if err != nil {
		return marketlab.Terms{}, [32]byte{}, err
	}
	intent, err := marketTransferIntent(transfer)
	return bound, intent, err
}

func marketTransferIntent(transfer Transfer) ([32]byte, error) {
	data, err := json.Marshal(transfer)
	if err != nil {
		return [32]byte{}, err
	}
	return sha256.Sum256(append([]byte("nu54/p03/market-transfer/v1\x00"), data...)), nil
}
