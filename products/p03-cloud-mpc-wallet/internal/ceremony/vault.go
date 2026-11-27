package ceremony

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/json"
	"errors"
	"io"
	"os"

	"github.com/bnb-chain/tss-lib/v4/ecdsa/keygen"
	"golang.org/x/sys/unix"
)

var errInterrupted = errors.New("unfinished ceremony cannot be restarted")

// Result is safe to transmit to the coordinator. Share material stays in record.
type Result struct {
	Session    [32]byte
	Peer       string
	State      string
	PublicKey  []byte `json:",omitempty"`
	Signature  []byte `json:",omitempty"`
	RecoveryID []byte `json:",omitempty"`
}

type record struct {
	Session Session
	Result  Result
	Share   *keygen.LocalPartySaveData
}

type vault struct {
	root   *os.Root
	lease  *os.File
	cipher cipher.AEAD
}

func openVault(dir string, key []byte) (*vault, error) {
	if len(key) != 32 {
		return nil, errors.New("vault key must be 32 bytes")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCMWithRandomNonce(block)
	if err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, err
	}
	fail := func(err error) (*vault, error) { root.Close(); return nil, err }
	info, err := root.Stat(".")
	if err != nil || info.Mode().Perm() != 0700 {
		return fail(errors.New("participant vault directory must have mode 0700"))
	}
	lease, err := root.OpenFile(".lease", os.O_RDWR|os.O_CREATE, 0600)
	if err != nil {
		return fail(err)
	}
	if err := unix.Flock(int(lease.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		lease.Close()
		return fail(errors.New("participant vault is already leased"))
	}
	return &vault{root: root, lease: lease, cipher: aead}, nil
}

func (v *vault) close() error { return errors.Join(v.lease.Close(), v.root.Close()) }

func (v *vault) load(name string, value any) error {
	f, err := v.root.Open(name)
	if err != nil {
		return err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, 1<<20))
	if err != nil {
		return err
	}
	clear, err := v.cipher.Open(nil, nil, b, []byte(name))
	if err != nil {
		return err
	}
	defer wipe(clear)
	return json.Unmarshal(clear, value)
}

func (v *vault) save(name string, value any) error {
	b, err := json.Marshal(value)
	if err != nil {
		return err
	}
	defer wipe(b)
	ciphertext := v.cipher.Seal(nil, nil, b, []byte(name))
	// The vault lease serializes writers between processes. Node.mu serializes
	// write operations inside the process, so one temporary name is sufficient.
	f, err := v.root.OpenFile(".pending", os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer v.root.Remove(".pending")
	_, writeErr := f.Write(ciphertext)
	if err := errors.Join(writeErr, f.Sync(), f.Close()); err != nil {
		return err
	}
	if err := v.root.Rename(".pending", name); err != nil {
		return err
	}
	dir, err := v.root.Open(".")
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}

func wipe(b []byte) { clear(b) }

func recordName(id string) string { return "session-" + id }

func (v *vault) record(id string) (record, error) {
	var row record
	if !uuid.MatchString(id) {
		return row, errors.New("invalid session ID")
	}
	return row, v.load(recordName(id), &row)
}

func (v *vault) putRecord(row record) error { return v.save(recordName(row.Session.ID), row) }

func (v *vault) fence(wallet string) (uint64, error) {
	var epoch uint64
	err := v.load("fence-"+wallet, &epoch)
	if errors.Is(err, os.ErrNotExist) {
		return 0, nil
	}
	return epoch, err
}

func (v *vault) putFence(wallet string, epoch uint64) error {
	return v.save("fence-"+wallet, epoch)
}
