package marketlab

import (
	"crypto/ecdsa"
	"encoding/binary"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

func testKey(t *testing.T) *ecdsa.PrivateKey {
	t.Helper()
	k, e := crypto.GenerateKey()
	if e != nil {
		t.Fatal(e)
	}
	return k
}

func TestIndependentMarketReviewRebuildsSignedOrderAndRejectsBlindApproval(t *testing.T) {
	k := testKey(t)
	p := Policy{Venue: common.Address{1}, Base: common.Address{2}, Quote: common.Address{3}, Signer: crypto.PubkeyToAddress(k.PublicKey), CodeHash: common.Hash{4}, Deployment: common.Hash{5}, Genesis: common.Hash{6}}
	user := common.Address{7}
	now := time.Unix(1_800_000_005, 0)
	exec := make([]byte, 452)
	copy(exec, selector("execute((address,uint8,uint256,uint256,uint256,uint256,uint256,uint256,uint256),bytes)"))
	copy(exec[16:36], user[:])
	exec[67] = 0
	binary.BigEndian.PutUint64(exec[92:100], 100_000_000)
	binary.BigEndian.PutUint64(exec[156:164], 1_000_000)
	exec[227] = 1
	exec[259] = 9
	binary.BigEndian.PutUint64(exec[284:292], uint64(now.Unix()+40))
	exec[322] = 1
	exec[323] = 64
	exec[355] = 65
	hash := offerDigest(p, exec)
	sig, e := crypto.Sign(hash[:], k)
	if e != nil {
		t.Fatal(e)
	}
	sig[64] += 27
	copy(exec[356:421], sig)
	approve := make([]byte, 68)
	copy(approve, selector("approve(address,uint256)"))
	copy(approve[16:36], p.Venue[:])
	copy(approve[36:68], exec[68:100])
	valid := Request{Deployment: p.Deployment, User: user, To: p.Base, Nonce: 0, GasPrice: 1, Calldata: approve, Offer: exec}
	if v, e := Review(p, valid, now); e != nil || v.Action != 0 || v.Allowance != "100000000" || v.Chain != 8283 || v.Digest == (common.Hash{}) {
		t.Fatalf("%+v %v", v, e)
	}
	legacy, err := Review(p, valid, now)
	if err != nil {
		t.Fatal(err)
	}
	valid.Calldata = append(approve, hash[:]...)
	bound, err := Review(p, valid, now)
	if err != nil || bound.Digest == legacy.Digest || bound.OfferDigest != hash {
		t.Fatalf("allowance signer did not bind the original offer: %+v %v", bound, err)
	}
	for _, mutate := range []func(*Request){
		func(r *Request) { r.To = p.Quote },
		func(r *Request) { r.User[0]++ },
		func(r *Request) { r.Deployment[0]++ },
		func(r *Request) { r.Calldata[67]++ },
		func(r *Request) { r.Calldata[99] ^= 1 },
		func(r *Request) { r.Offer[99]-- },
		func(r *Request) { r.GasPrice = 100_000_000_001 },
	} {
		r := valid
		r.Calldata = append([]byte(nil), r.Calldata...)
		r.Offer = append([]byte(nil), r.Offer...)
		mutate(&r)
		if _, e := Review(p, r, now); e == nil {
			t.Fatal("accepted out-of-scope market request")
		}
	}
	if _, e := Review(p, valid, now.Add(41*time.Second)); e == nil {
		t.Fatal("accepted expired allowance")
	}
	if _, e := Review(p, valid, now.Add(-21*time.Second)); e == nil {
		t.Fatal("accepted future allowance")
	}
	valid.To = p.Venue
	valid.Calldata = exec
	if v, e := Review(p, valid, now); e != nil || v.Allowance != "" {
		t.Fatalf("execution %+v %v", v, e)
	}
}
