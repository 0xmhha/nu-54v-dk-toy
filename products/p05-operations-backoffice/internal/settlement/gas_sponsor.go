package settlement

import (
	"context"
	"errors"

	sponsorabi "github.com/0xmhha/nu-54v-dk-toy/packages/contracts-abi/go/gassponsor"
	escrowabi "github.com/0xmhha/nu-54v-dk-toy/packages/contracts-abi/go/orderescrow"
	"github.com/0xmhha/nu-54v-dk-toy/packages/contracts-abi/go/testusdc3009"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
)

// sponsorGas covers the extra call through GasSponsor and the copied calldata.
const sponsorGas = 100_000

// sponsored reports whether a job goes through the branch's GasSponsor. A dispute decision does not:
// the contract asks for the arbiter itself.
func (t Target) sponsored(job work) bool {
	return t.Sponsor != (common.Address{}) && t.Escrow() && (job.action == nil || job.action.kind != "resolve")
}

// relayTo is where the transaction of a job goes, and the calldata it carries.
func relayTo(job work, target Target, data []byte) (common.Address, []byte, error) {
	to := destination(job, target)
	if !target.sponsored(job) {
		return to, data, nil
	}
	contract, err := sponsorabi.GassponsorMetaData.GetAbi()
	if err != nil {
		return common.Address{}, nil, err
	}
	wrapped, err := contract.Pack("sponsor", to, data)
	return target.Sponsor, wrapped, err
}

// relayed returns the call inside a stored transaction. One signed before the branch had a sponsor is
// still a direct call.
func relayed(job work, target Target, to common.Address, data []byte) ([]byte, error) {
	inner := destination(job, target)
	if to == inner {
		return data, nil
	}
	if !target.sponsored(job) || to != target.Sponsor {
		return nil, errors.New("relay transaction has another destination")
	}
	contract, err := sponsorabi.GassponsorMetaData.GetAbi()
	if err != nil {
		return nil, err
	}
	method := contract.Methods["sponsor"]
	if len(data) < 4 || string(data[:4]) != string(method.ID) {
		return nil, errors.New("relay transaction is not a sponsored call")
	}
	args, err := method.Inputs.Unpack(data[4:])
	if err != nil || len(args) != 2 {
		return nil, errors.New("invalid sponsored call")
	}
	called, ok := args[0].(common.Address)
	call, valid := args[1].([]byte)
	if !ok || !valid || called != inner {
		return nil, errors.New("sponsored call names another contract")
	}
	// Only the one encoding this server makes.
	if _, again, err := relayTo(job, target, call); err != nil || string(again) != string(data) {
		return nil, errors.New("sponsored call is not canonical")
	}
	return call, nil
}

// SponsorReady reports whether the branch's GasSponsor lets this gas account relay a payment.
func SponsorReady(ctx context.Context, client *ethclient.Client, target Target, gas common.Address) bool {
	if target.Sponsor == (common.Address{}) {
		return true
	}
	caller, err := sponsorabi.NewGassponsorCaller(target.Sponsor, client)
	if err != nil || !target.Escrow() || target.Sponsor == target.Contract || target.Sponsor == target.Asset {
		return false
	}
	opts := &bind.CallOpts{Context: ctx}
	allowed, err := caller.Relays(opts, gas)
	if err != nil || !allowed {
		return false
	}
	selectors, err := sponsoredSelectors()
	if err != nil {
		return false
	}
	for to, list := range map[common.Address][][4]byte{target.Contract: selectors.escrow, target.Asset: selectors.token} {
		for _, selector := range list {
			if kind, err := caller.Actions(opts, to, selector); err != nil || kind != signedAction {
				return false
			}
		}
	}
	return true
}

// signedAction is GasSponsor.Kind.SIGNED: the target checks the signature itself.
const signedAction = 1

type selectorLists struct{ escrow, token [][4]byte }

// sponsoredSelectors lists every call this server relays.
func sponsoredSelectors() (selectorLists, error) {
	var out selectorLists
	escrow, err := escrowabi.OrderescrowMetaData.GetAbi()
	if err != nil {
		return out, err
	}
	token, err := testusdc3009.Testusdc3009MetaData.GetAbi()
	if err != nil {
		return out, err
	}
	names := []string{"lock", "claimBatch"}
	for kind, method := range escrowCalls {
		if kind != "resolve" {
			names = append(names, method)
		}
	}
	for _, name := range names {
		method, ok := escrow.Methods[name]
		if !ok {
			return out, errors.New("unknown escrow method " + name)
		}
		out.escrow = append(out.escrow, [4]byte(method.ID))
	}
	out.token = append(out.token, [4]byte(token.Methods["transferWithAuthorization"].ID))
	return out, nil
}
