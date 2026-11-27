package commerce

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"sort"
	"strings"

	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/service"
	"github.com/jackc/pgx/v5"
)

func (h HTTP) registerFirmware(mux *http.ServeMux) {
	mux.HandleFunc("GET /v2/operations/fota/{branch}/{device}", h.operateFirmware)
	mux.HandleFunc("POST /v2/operations/fota/{branch}/{device}/{action}", h.operateFirmware)
}

func (h HTTP) operateFirmware(w http.ResponseWriter, r *http.Request) {
	actor, ok := h.operatorCredential(r)
	branch, device := r.PathValue("branch"), r.PathValue("device")
	if !ok || h.Store == nil || len(h.Firmware) == 0 || !operatorRef.MatchString(branch) || !operatorRef.MatchString(device) {
		http.NotFound(w, r)
		return
	}
	ctx := r.Context()
	var raw json.RawMessage
	if r.Method == http.MethodGet {
		if err := h.Store.db.QueryRow(ctx, "SELECT ledger.firmware_status($1,$2,$3,$4,NULL)",
			actor.Tenant, branch, actor.Subject, device).Scan(&raw); err != nil {
			operatorResponse(w, nil, err)
			return
		}
		var info struct {
			Revision string `json:"revision"`
		}
		if json.Unmarshal(raw, &info) != nil {
			operatorResponse(w, nil, service.Unknown)
			return
		}
		type choice struct {
			ID string `json:"id"`
			FirmwareRelease
		}
		available := []choice{}
		for id, release := range h.Firmware {
			if release.Revision == info.Revision {
				available = append(available, choice{ID: id, FirmwareRelease: release})
			}
		}
		sort.Slice(available, func(i, j int) bool { return available[i].Counter < available[j].Counter })
		writeJSON(w, http.StatusOK, map[string]any{"status": raw, "releases": available})
		return
	}
	if h.OperatorOrigin == "" || r.Header.Get("Origin") != h.OperatorOrigin || r.Header.Get("Content-Type") != "application/json" {
		http.NotFound(w, r)
		return
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
	decoder.DisallowUnknownFields()
	var input struct {
		Campaign string `json:"campaign"`
		Release  string `json:"release,omitempty"`
		Proof    string `json:"proof,omitempty"`
	}
	if decoder.Decode(&input) != nil || decoder.Decode(new(any)) != io.EOF || !operatorRef.MatchString(input.Campaign) {
		operatorResponse(w, nil, service.BadRequest)
		return
	}
	action := r.PathValue("action")
	if (input.Release != "" && action != "start") || (input.Proof != "" && action != "ack") {
		operatorResponse(w, nil, service.BadRequest)
		return
	}
	var err error
	switch action {
	case "start":
		release, found := h.Firmware[input.Release]
		if !found {
			operatorResponse(w, nil, service.BadRequest)
			return
		}
		measurement, _ := hex.DecodeString(release.Measurement)
		challenge, err := freshId()
		if err != nil {
			operatorResponse(w, nil, service.Unknown)
			return
		}
		err = h.Store.db.QueryRow(ctx, "SELECT ledger.begin_firmware($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)",
			actor.Tenant, branch, actor.Subject, device, input.Campaign, input.Release, release.Version,
			release.Counter, release.Revision, measurement, challenge[:]).Scan(&raw)
		operatorResponse(w, raw, err)
		return
	case "pause", "resume":
		next := "paused"
		if action == "resume" {
			next = "running"
		}
		challenge, err := freshId()
		if err != nil {
			operatorResponse(w, nil, service.Unknown)
			return
		}
		err = h.Store.db.QueryRow(ctx, "SELECT ledger.control_firmware($1,$2,$3,$4,$5,$6,$7)",
			actor.Tenant, branch, actor.Subject, device, input.Campaign, next, challenge[:]).Scan(&raw)
		operatorResponse(w, raw, err)
		return
	case "image", "challenge", "ack":
		var release string
		var challenge, board, public, measure []byte
		err = h.Store.db.QueryRow(ctx, "SELECT * FROM ledger.firmware_station($1,$2,$3,$4,$5)",
			actor.Tenant, branch, actor.Subject, device, input.Campaign).Scan(&release, &challenge, &board, &public, &measure)
		campaign, _ := hex.DecodeString(strings.ReplaceAll(input.Campaign, "-", ""))
		if action == "ack" {
			proof, decodeErr := hex.DecodeString(input.Proof)
			if decodeErr != nil || len(proof) != 180 {
				operatorResponse(w, nil, service.BadRequest)
				return
			}
			digest := sha256.Sum256(proof)
			switch {
			case err == nil:
				if h.Firmware[release].Image == nil || verifyFirmwareACK(proof, challenge, campaign, board, measure, public) != nil {
					operatorResponse(w, nil, service.Forbidden)
					return
				}
			case errors.Is(err, pgx.ErrNoRows):
				// Allow exact receipt retrieval after activation, without permitting
				// an unverified proof to update a newly eligible or paused campaign.
				challenge = nil
			default:
				operatorResponse(w, nil, err)
				return
			}
			err = h.Store.db.QueryRow(ctx, "SELECT ledger.firmware_ack($1,$2,$3,$4,$5,$6,$7)",
				actor.Tenant, branch, actor.Subject, device, input.Campaign, challenge, digest[:]).Scan(&raw)
			operatorResponse(w, raw, err)
			return
		}
		if err != nil {
			operatorResponse(w, nil, err)
			return
		}
		image, ok := h.Firmware[release]
		if !ok {
			operatorResponse(w, nil, service.Conflict)
			return
		}
		if action == "image" {
			w.Header().Set("Cache-Control", "no-store")
			w.Header().Set("Content-Type", "application/octet-stream")
			w.Header().Set("X-Content-Type-Options", "nosniff")
			_, _ = w.Write(image.Image)
			return
		}
		command := append(append([]byte{16}, challenge...), campaign...)
		writeJSON(w, http.StatusOK, map[string]string{"command": hex.EncodeToString(command), "imageSha256": image.ImageSHA256})
	default:
		http.NotFound(w, r)
	}
}
