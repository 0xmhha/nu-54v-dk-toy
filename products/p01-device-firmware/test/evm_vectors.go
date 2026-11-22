// Run offline. Uses go-ethereum as an independent EIP-1559 encoding and hash oracle.
package main

import (
	"fmt"
	"log"
	"math/big"
	"os"
	"os/exec"

	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rlp"
)

func call(selector string, names []string, values ...any) []byte {
	var params abi.Arguments
	for _, name := range names {
		field, err := abi.NewType(name, "", nil)
		if err != nil {
			log.Fatal(err)
		}
		params = append(params, abi.Argument{Type: field})
	}
	packet, err := params.Pack(values...)
	if err != nil {
		log.Fatal(err)
	}
	return append(crypto.Keccak256([]byte(selector))[:4], packet...)
}

func main() {
	if len(os.Args) != 2 {
		log.Fatal("supply the path of the P01 test_evm binary")
	}
	message := "demo.example wants you to sign in with your Ethereum account:\n0x1111111111111111111111111111111111111111\n\n\nURI: https://demo.example/sign-in\nVersion: 1\nChain ID: 11155111\nNonce: 8abc0023\nIssued At: 1970-01-01T00:16:40.000Z\nExpiration Time: 1970-01-01T00:17:10.000Z"
	if output, err := exec.Command(os.Args[1], "siwe", fmt.Sprintf("%x", accounts.TextHash([]byte(message)))).CombinedOutput(); err != nil {
		log.Fatalf("SIWE: %v\n%s", err, output)
	}
	fmt.Println("SIWE digest matched go-ethereum EIP-191 and rejected alternate origin, chain, and expiry")
	key, err := crypto.GenerateKey()
	if err != nil {
		log.Fatal(err)
	}
	for _, chain := range []int64{11155111, 84532, 8283} {
		for _, operation := range []string{"native", "transfer", "revoke", "allowance"} {
			receiver := common.HexToAddress("0x1234567890123456789012345678901234567890")
			token := common.Address{}
			body := &types.DynamicFeeTx{
				ChainID: big.NewInt(chain), Nonce: 3, GasTipCap: big.NewInt(3), GasFeeCap: big.NewInt(12),
				Gas: 21000, To: &receiver, Value: big.NewInt(9),
			}
			if operation != "native" {
				token = common.HexToAddress("0x1111111111111111111111111111111111111111")
				body.To, body.Value, body.Gas = &token, big.NewInt(0), 43000
				body.Data = append(append([]byte{0xa9, 0x05, 0x9c, 0xbb}, common.LeftPadBytes(receiver[:], 32)...), common.LeftPadBytes([]byte{9}, 32)...)
			}
			if operation == "revoke" || operation == "allowance" {
				amount := byte(0)
				if operation == "allowance" {
					amount = 9
				}
				body.Data = append(append([]byte{0x09, 0x5e, 0xa7, 0xb3}, common.LeftPadBytes(receiver[:], 32)...), common.LeftPadBytes([]byte{amount}, 32)...)
			}
			unsigned := types.NewTx(body)
			signer := types.LatestSignerForChainID(body.ChainID)
			digest := signer.Hash(unsigned)
			sig, err := crypto.Sign(digest[:], key)
			if err != nil {
				log.Fatal(err)
			}
			signed, err := unsigned.WithSignature(signer, sig)
			if err != nil {
				log.Fatal(err)
			}
			signedBytes, err := signed.MarshalBinary()
			if err != nil {
				log.Fatal(err)
			}
			encoded, err := rlp.EncodeToBytes([]any{body.ChainID, body.Nonce, body.GasTipCap, body.GasFeeCap, body.Gas, body.To, body.Value, body.Data, body.AccessList})
			if err != nil {
				log.Fatal(err)
			}
			raw := append([]byte{2}, encoded...)
			sig[64] += 27
			l1Fee := uint64(0)
			if chain == 84532 {
				l1Fee = 1000
			}
			command := exec.Command(os.Args[1], fmt.Sprint(chain), fmt.Sprint(body.Gas*12+l1Fee), fmt.Sprint(l1Fee),
				fmt.Sprintf("%x", token.Bytes()), fmt.Sprintf("%x", raw), fmt.Sprintf("%x", digest.Bytes()),
				fmt.Sprintf("%x", sig), fmt.Sprintf("%x", signedBytes))
			if operation == "revoke" {
				command.Args = append(command.Args, "revoke")
			} else if operation == "allowance" {
				command.Args = append(command.Args, "allowance", fmt.Sprintf("%x", receiver.Bytes()), fmt.Sprint(body.Nonce))
			}
			if output, err := command.CombinedOutput(); err != nil {
				log.Fatalf("chain %d operation=%s: %v\n%s", chain, operation, err, output)
			}
			fmt.Printf("chain %d operation=%s matched go-ethereum hash and signed bytes\n", chain, operation)
		}
	}
	cases := []struct {
		chain int64
		name  string
		data  []byte
	}{
		{11155111, "ERC-721 safeTransferFrom", call("safeTransferFrom(address,address,uint256)", []string{"address", "address", "uint256"},
			common.HexToAddress("0x2222222222222222222222222222222222222222"), common.HexToAddress("0x3333333333333333333333333333333333333333"), big.NewInt(7))},
		{84532, "multicall", call("multicall(bytes[])", []string{"bytes[]"}, [][]byte{{0xa9, 5, 0x9c, 0xbb}})},
	}
	for _, test := range cases {
		target := common.HexToAddress("0x1111111111111111111111111111111111111111")
		encoded, err := rlp.EncodeToBytes([]any{big.NewInt(test.chain), uint64(3), big.NewInt(3), big.NewInt(12),
			uint64(90000), target, big.NewInt(0), test.data, types.AccessList{}})
		if err != nil {
			log.Fatal(err)
		}
		l1Fee := uint64(0)
		if test.chain == 84532 {
			l1Fee = 1000
		}
		if output, err := exec.Command(os.Args[1], "unsupported", fmt.Sprint(test.chain), fmt.Sprint(90000*12+l1Fee),
			fmt.Sprint(l1Fee), fmt.Sprintf("%x", target.Bytes()), fmt.Sprintf("%x", append([]byte{2}, encoded...))).CombinedOutput(); err != nil {
			log.Fatalf("%s refusal: %v\n%s", test.name, err, output)
		}
		fmt.Printf("chain %d refused %s before serialization\n", test.chain, test.name)
	}
}
