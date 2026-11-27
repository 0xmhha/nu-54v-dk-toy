//go:build cgo && (android || ios)

package main

/*
#include <stdint.h>
*/
import "C"
import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
	"unsafe"

	"github.com/0xmhha/nu-54v-dk-toy/products/p03-cloud-mpc-wallet/internal/ceremony"
	"github.com/0xmhha/nu-54v-dk-toy/products/p03-cloud-mpc-wallet/internal/mobileconfig"
)

var runtimeState struct {
	sync.Mutex
	cancel context.CancelFunc
	done   chan struct{}
	root   string
	digest [32]byte
	lease  int64
}

//export nu54_p03_start
func nu54_p03_start(directory *C.char, key unsafe.Pointer, size C.int, subject, authority *C.char, expires C.longlong) C.int {
	if directory == nil || key == nil || size != 32 {
		return -1
	}
	root := C.GoString(directory)
	if !filepath.IsAbs(root) {
		return -1
	}
	runtimeState.Lock()
	defer runtimeState.Unlock()
	owner := C.GoString(subject)
	var digest [32]byte
	lease := int64(expires)
	if owner != "" {
		now := time.Now().Unix()
		if lease <= now || lease > now+330 {
			return -1
		}
		raw, err := os.ReadFile(filepath.Join(root, "roster.json"))
		if err != nil {
			return -1
		}
		digest = sha256.Sum256(raw)
	}
	if runtimeState.cancel != nil {
		select {
		case <-runtimeState.done:
			stopLocked()
		default:
			if runtimeState.root != root+"|"+owner {
				return -2
			}
			if runtimeState.digest == digest && runtimeState.lease == lease {
				return 0
			}
			stopLocked()
		}
	}
	localKey := C.GoBytes(key, size)
	defer clear(localKey)
	var cfg ceremony.Config
	var cert tls.Certificate
	var ca []byte
	if owner != "" {
		public, err := base64.StdEncoding.Strict().DecodeString(C.GoString(authority))
		if err != nil {
			return -1
		}
		cfg, cert, ca, err = mobileconfig.Open(root, localKey, owner, public)
		if err != nil {
			return -1
		}
	} else {
		raw, err := os.ReadFile(filepath.Join(root, "participant.json"))
		if err != nil || len(raw) > 65536 || json.Unmarshal(raw, &cfg) != nil {
			return -1
		}
		cfg.Certificate = filepath.Join(root, "tls.crt")
		cfg.CertificateKey = filepath.Join(root, "tls.key")
		cfg.ClientCA = filepath.Join(root, "root.pem")
		cfg.Vault = filepath.Join(root, "vault")
		cfg.VaultKey = ""
	}
	if err := os.Mkdir(cfg.Vault, 0700); err != nil && !os.IsExist(err) {
		return -1
	}
	var ctx context.Context
	var cancel context.CancelFunc
	if owner != "" {
		ctx, cancel = context.WithDeadline(context.Background(), minDeadline(cert.Leaf.NotAfter, time.Unix(lease, 0)))
	} else {
		ctx, cancel = context.WithCancel(context.Background())
	}
	done := make(chan struct{})
	ready := make(chan struct{})
	go func() {
		defer close(done)
		notify := func() { close(ready) }
		if owner != "" {
			_ = ceremony.ServeWithCredentials(ctx, cfg, localKey, cert, ca, notify)
		} else {
			_ = ceremony.ServeWithVaultKey(ctx, cfg, localKey, notify)
		}
	}()
	select {
	case <-ready:
		runtimeState.cancel, runtimeState.done, runtimeState.root = cancel, done, root+"|"+owner
		runtimeState.digest, runtimeState.lease = digest, lease
		return 0
	case <-done:
		cancel()
		return -1
	case <-time.After(5 * time.Second):
		cancel()
		<-done
		return -1
	}
}

//export nu54_p03_stop
func nu54_p03_stop() {
	runtimeState.Lock()
	defer runtimeState.Unlock()
	stopLocked()
}

func minDeadline(one, two time.Time) time.Time {
	if one.Before(two) {
		return one
	}
	return two
}

func stopLocked() {
	if runtimeState.cancel != nil {
		runtimeState.cancel()
		<-runtimeState.done
		runtimeState.cancel = nil
		runtimeState.root = ""
	}
}

//export nu54_p03_prepare
func nu54_p03_prepare(directory *C.char, key unsafe.Pointer, size C.int, out *C.char, capacity C.int) C.int {
	if directory == nil || key == nil || size != 32 || out == nil || capacity < 512 || capacity > 8192 {
		return -1
	}
	runtimeState.Lock()
	defer runtimeState.Unlock()
	root := C.GoString(directory)
	if !filepath.IsAbs(root) {
		return -1
	}
	local := C.GoBytes(key, size)
	defer clear(local)
	csr, err := mobileconfig.Prepare(root, local)
	if err != nil || len(csr) >= int(capacity) {
		return -1
	}
	dst := unsafe.Slice((*byte)(unsafe.Pointer(out)), int(capacity))
	copy(dst, csr)
	dst[len(csr)] = 0
	return C.int(len(csr))
}

//export nu54_p03_install
func nu54_p03_install(directory *C.char, key unsafe.Pointer, size C.int, subject, authority, roster *C.char) C.int {
	if directory == nil || key == nil || size != 32 || subject == nil || authority == nil || roster == nil {
		return -1
	}
	runtimeState.Lock()
	defer runtimeState.Unlock()
	root := C.GoString(directory)
	if !filepath.IsAbs(root) {
		return -1
	}
	local := C.GoBytes(key, size)
	defer clear(local)
	pub, err := base64.StdEncoding.Strict().DecodeString(C.GoString(authority))
	if err != nil || mobileconfig.Install(root, local, C.GoString(subject), []byte(C.GoString(roster)), pub) != nil {
		return -1
	}
	if runtimeState.root == root+"|"+C.GoString(subject) {
		installed, err := os.ReadFile(filepath.Join(root, "roster.json"))
		if err != nil || sha256.Sum256(installed) != runtimeState.digest {
			stopLocked()
		}
	}
	return 0
}

func main() {}
