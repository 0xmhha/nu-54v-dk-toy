// signfixture implements the real pinned signer protocol with synthetic local keys.
package main

import (
	"bufio"
	"encoding/hex"
	"fmt"
	"io"
	"os"

	"github.com/ethereum/go-ethereum/crypto"
)

func main() {
	if len(os.Args) != 2 {
		os.Exit(1)
	}
	key, err := crypto.HexToECDSA(os.Args[1])
	if err != nil {
		os.Exit(1)
	}
	scan := bufio.NewScanner(io.LimitReader(os.Stdin, 66))
	if !scan.Scan() {
		os.Exit(1)
	}
	digest, err := hex.DecodeString(scan.Text())
	if err != nil || len(digest) != 32 || scan.Scan() {
		os.Exit(1)
	}
	signature, err := crypto.Sign(digest, key)
	if err != nil {
		os.Exit(1)
	}
	signature[64] += 27
	fmt.Printf("%x\n", signature)
}
