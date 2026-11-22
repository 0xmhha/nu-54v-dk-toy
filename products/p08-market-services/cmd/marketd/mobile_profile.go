package main

import (
	"crypto/x509"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/0xmhha/nu-54v-dk-toy/products/p08-market-services/internal/market"
	"github.com/ethereum/go-ethereum/crypto"
)

var mobileOrigin = regexp.MustCompile(`^https://[a-z0-9][a-z0-9.-]*[.][a-z][a-z0-9-]*(?::([1-9][0-9]{0,4}))?$`)

func mobileDeploymentDigest(p market.Profile) string {
	fields := []string{p.Environment, strconv.FormatUint(p.Chain, 10), p.Genesis.Hex(), strconv.FormatUint(p.Height, 10),
		p.BlockHash.Hex(), p.QuoteSigner.Hex()}
	for _, contract := range []market.Contract{p.Venue, p.Base, p.Quote} {
		fields = append(fields, contract.Address.Hex(), contract.CodeHash.Hex())
	}
	return crypto.Keccak256Hash([]byte(strings.ToLower(strings.Join(fields, "|")))).Hex()
}

func validMobileOrigin(origin string) bool {
	parts := mobileOrigin.FindStringSubmatch(origin)
	if len(origin) > 260 || len(parts) == 0 {
		return false
	}
	port, _ := strconv.Atoi(parts[1])
	return port <= 65535
}

func writeMobileProfile(p market.Profile) error {
	origin := os.Getenv("P08_MOBILE_ORIGIN")
	if !validMobileOrigin(origin) {
		return errors.New("P08_MOBILE_ORIGIN requires an exact HTTPS DNS origin with a valid port")
	}
	operator := os.Getenv("P08_MOBILE_OPERATOR_ORIGIN")
	if operator != "" && (!validMobileOrigin(operator) || strings.TrimSuffix(operator, ":443") == strings.TrimSuffix(origin, ":443")) {
		return errors.New("P08_MOBILE_OPERATOR_ORIGIN requires a separate exact HTTPS DNS origin")
	}
	raw, err := os.ReadFile(os.Getenv("P08_MOBILE_CA_DER"))
	if err != nil || len(raw) > 4096 {
		return errors.New("P08_MOBILE_CA_DER requires an installed deployment CA in DER format")
	}
	cert, err := x509.ParseCertificate(raw)
	if err != nil || !cert.IsCA {
		return errors.New("P08_MOBILE_CA_DER is not a trusted CA configuration")
	}
	fields := []string{origin, mobileDeploymentDigest(p), base64.StdEncoding.EncodeToString(raw)}
	if operator != "" {
		fields = append(fields, operator)
	}
	_, err = fmt.Fprintln(os.Stdout, strings.Join(fields, "|"))
	return err
}
