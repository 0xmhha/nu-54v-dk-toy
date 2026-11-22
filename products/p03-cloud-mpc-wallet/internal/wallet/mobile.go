package wallet

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/0xmhha/nu-54v-dk-toy/products/p03-cloud-mpc-wallet/internal/ceremony"
	"github.com/0xmhha/nu-54v-dk-toy/products/p03-cloud-mpc-wallet/internal/mobileconfig"
)

type MobileRegistry struct {
	Requests string
	Approved string
}

func (r MobileRegistry) Handle(store Store, w http.ResponseWriter, req *http.Request, actor Claims) {
	if r.Requests == "" || r.Approved == "" {
		writeResponse(w, nil, ErrMissing)
		return
	}
	_, err := store.Subject(req.Context(), actor)
	linked := err == nil
	if errors.Is(err, ErrForbidden) {
		bootstrap, checkErr := store.ProfilePending(req.Context(), actor)
		if checkErr != nil {
			writeResponse(w, nil, checkErr)
			return
		}
		if bootstrap {
			err = nil
		}
	}
	if err != nil {
		writeResponse(w, nil, err)
		return
	}
	var input struct {
		CSR string `json:"csr"`
	}
	if !requestJSON(w, req, &input) {
		return
	}
	block, rest := pem.Decode([]byte(input.CSR))
	if block == nil || block.Type != "CERTIFICATE REQUEST" || len(bytes.TrimSpace(rest)) != 0 {
		http.Error(w, "invalid device identity", 400)
		return
	}
	csr, err := x509.ParseCertificateRequest(block.Bytes)
	if err != nil || csr.CheckSignature() != nil || csr.PublicKeyAlgorithm != x509.Ed25519 {
		http.Error(w, "invalid device identity", 400)
		return
	}
	ref := sha256.Sum256(csr.RawSubjectPublicKeyInfo)
	name := actor.Subject + "-" + hex.EncodeToString(ref[:])
	dir, err := os.OpenRoot(r.Approved)
	if err != nil {
		writeResponse(w, nil, err)
		return
	}
	defer dir.Close()
	file, err := dir.Open(name + ".json")
	if err == nil && !linked {
		file.Close()
		writeClaimsResponse(w, map[string]string{"state": "pending"}, nil, actor)
		return
	}
	if err == nil {
		defer file.Close()
		data, err := io.ReadAll(io.LimitReader(file, 6145))
		var env mobileconfig.Envelope
		var roster mobileconfig.Roster
		if err != nil || len(data) > 6144 || json.Unmarshal(data, &env) != nil || len(env.Signature) != ed25519.SignatureSize ||
			json.Unmarshal(env.Payload, &roster) != nil || roster.Subject != actor.Subject {
			writeResponse(w, nil, ErrConflict)
			return
		}
		certBlock, _ := pem.Decode([]byte(roster.Certificate))
		if certBlock == nil {
			writeResponse(w, nil, ErrConflict)
			return
		}
		cert, err := x509.ParseCertificate(certBlock.Bytes)
		if err != nil || !bytes.Equal(cert.RawSubjectPublicKeyInfo, csr.RawSubjectPublicKeyInfo) {
			writeResponse(w, nil, ErrConflict)
			return
		}
		if store.Bindings != nil && actor.Subject != store.Bindings.LegacySubject {
			p, err := readBinding(req.Context(), store.Pool, actor.Subject)
			if err != nil {
				writeResponse(w, nil, err)
				return
			}
			pin, keyPin := sha256.Sum256(cert.Raw), sha256.Sum256(cert.RawSubjectPublicKeyInfo)
			endpoint := p.Endpoints[roster.Config.ID]
			bound := false
			for _, peer := range p.profile().Committee(roster.Epoch) {
				bound = bound || peer.Role == ceremony.A && peer.ID == roster.Config.ID
			}
			if !bound || endpoint.CertificateSHA256 != hex.EncodeToString(pin[:]) || endpoint.SPKISHA256 != hex.EncodeToString(keyPin[:]) ||
				!p.ApprovalKey.Equal(roster.Config.ApprovalKey) || !p.RecoveryKey.Equal(roster.Config.RecoveryKey) ||
				!store.Bindings.CoordinatorKey.Equal(roster.Config.CoordinatorKey) {
				writeResponse(w, nil, ErrConflict)
				return
			}
			for id, peer := range roster.Config.Peers {
				if expected, ok := p.Endpoints[id]; !ok || expected != peer || id == roster.Config.ID {
					writeResponse(w, nil, ErrConflict)
					return
				}
			}
		}
		writeClaimsResponse(w, map[string]any{"state": "ready", "roster": env}, nil, actor)
		return
	}
	if !errors.Is(err, os.ErrNotExist) {
		writeResponse(w, nil, err)
		return
	}
	inbox, err := os.OpenRoot(r.Requests)
	if err != nil {
		writeResponse(w, nil, err)
		return
	}
	defer inbox.Close()
	if err := claimsCurrent(time.Now(), actor); err != nil {
		writeResponse(w, nil, err)
		return
	}
	reqFile, err := inbox.OpenFile(name+".csr", os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err == nil {
		_, writeErr := reqFile.Write(pem.EncodeToMemory(block))
		if err := errors.Join(writeErr, reqFile.Sync(), reqFile.Close()); err != nil {
			_ = inbox.Remove(name + ".csr")
			writeResponse(w, nil, err)
			return
		}
	} else if !errors.Is(err, os.ErrExist) {
		writeResponse(w, nil, err)
		return
	}
	writeClaimsResponse(w, map[string]string{"state": "pending"}, nil, actor)
}
