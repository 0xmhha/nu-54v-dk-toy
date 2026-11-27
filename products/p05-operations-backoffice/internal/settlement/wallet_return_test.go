package settlement

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/0xmhha/nu-54v-dk-toy/packages/contracts-abi/go/testusdc3009"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
)

// The node allows 100 blocks per log query; the search adapts and still finds the one that counts.
func TestAuthorizationUseWithinRPCLimit(t *testing.T) {
	ctx := context.Background()
	abi, err := testusdc3009.Testusdc3009MetaData.GetAbi()
	if err != nil {
		t.Fatal(err)
	}
	asset, payer, nonce, want := common.Address{1}, common.Address{2}, common.Hash{3}, common.Hash{4}
	const head, used = 100_000, 40_777 // one block a second from time zero
	queries := 0
	node := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var call struct {
			ID     json.RawMessage
			Method string
			Params []json.RawMessage
		}
		_ = json.NewDecoder(r.Body).Decode(&call)
		reply := map[string]any{"jsonrpc": "2.0", "id": call.ID}
		switch call.Method {
		case "eth_getBlockByNumber":
			var name string
			_ = json.Unmarshal(call.Params[0], &name)
			height, err := hexutil.DecodeUint64(name)
			if err != nil {
				height = head
			}
			reply["result"] = &types.Header{Number: new(big.Int).SetUint64(height), Time: height, Difficulty: new(big.Int), Extra: []byte{}}
		case "eth_getLogs":
			var filter struct{ FromBlock, ToBlock hexutil.Uint64 }
			_ = json.Unmarshal(call.Params[0], &filter)
			queries++
			if filter.ToBlock-filter.FromBlock >= 100 {
				reply["error"] = map[string]any{"code": -32005, "message": "block range too large"}
				break
			}
			logs := []types.Log{}
			if filter.FromBlock <= used && used <= filter.ToBlock {
				logs = append(logs, types.Log{Address: asset, Topics: []common.Hash{abi.Events["AuthorizationUsed"].ID, common.BytesToHash(payer[:]), nonce},
					Data: []byte{}, BlockNumber: used, TxHash: want})
			}
			reply["result"] = logs
		default:
			reply["error"] = map[string]any{"code": -32601, "message": call.Method}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(reply)
	}))
	defer node.Close()
	client, err := ethclient.Dial(node.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	for _, c := range []struct {
		until int64
		found bool
	}{{41_000, true}, {used, true}, {used - 1, false}} {
		t.Run(fmt.Sprint(c.until), func(t *testing.T) {
			queries = 0
			got, err := LiveChain{client}.authorizationUse(ctx, asset, payer, nonce, c.until-3600, c.until)
			if err != nil || (got != nil) != c.found || (c.found && *got != want) || queries > 100 {
				t.Fatalf("found %v after %d queries: %v", got, queries, err)
			}
		})
	}
}
