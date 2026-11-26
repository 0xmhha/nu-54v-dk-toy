package commerce

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"net/http"
	"regexp"
	"strconv"

	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/service"
	"github.com/0xmhha/nu-54v-dk-toy/products/p05-operations-backoffice/internal/authority"
)

// A menu photo as the server makes it: the page photo, and a small one for lists that fits one reply.
const (
	photoUploadBytes = 5 << 20
	photoSourceSide  = 4096
	photoBytes       = 150 << 10
	thumbBytes       = 18000 // one part of catalog.photo
	photoPart        = 18000
)

var photoDigest = regexp.MustCompile(`^[0-9a-f]{64}$`)

type menuPhoto struct {
	SKU   string `json:"sku"`
	Photo string `json:"photo"`
}

// CatalogPhoto names the two sizes of a product's photo on a menu page.
type CatalogPhoto struct {
	SKU   string `json:"sku"`
	Photo string `json:"photo"`
	Thumb string `json:"thumb"`
}

// PhotoPart is one piece of a photo; the app joins the pieces and checks the name.
type PhotoPart struct {
	Size int    `json:"size"`
	Data string `json:"data"`
}

var errPhoto = errors.New("photo")

// makeMenuPhotos reads a JPEG or PNG and makes the two JPEGs the server keeps. Nothing of the upload is copied:
// the pixels are drawn again, so place, device and any other data written by the camera are left behind.
func makeMenuPhotos(upload []byte) (photo, thumb []byte, err error) {
	if len(upload) == 0 || len(upload) > photoUploadBytes {
		return nil, nil, errPhoto
	}
	var decode func(io.Reader) (image.Image, error)
	var config func(io.Reader) (image.Config, error)
	switch {
	case bytes.HasPrefix(upload, []byte{0xff, 0xd8, 0xff}):
		decode, config = jpeg.Decode, jpeg.DecodeConfig
	case bytes.HasPrefix(upload, []byte("\x89PNG\r\n\x1a\n")):
		decode, config = png.Decode, png.DecodeConfig
	default:
		return nil, nil, errPhoto
	}
	// The size is read before the pixels, so a small file that unpacks to a huge picture is refused first.
	size, err := config(bytes.NewReader(upload))
	if err != nil || size.Width < 1 || size.Height < 1 || size.Width > photoSourceSide || size.Height > photoSourceSide {
		return nil, nil, errPhoto
	}
	source, err := decode(bytes.NewReader(upload))
	if err != nil {
		return nil, nil, errPhoto
	}
	if photo, err = encodeWithin(cover(source, 800, 600), photoBytes, 82); err != nil {
		return nil, nil, err
	}
	thumb, err = encodeWithin(cover(source, 320, 240), thumbBytes, 75)
	return photo, thumb, err
}

// cover fills width×height with the middle of the picture, each target pixel the average of the source pixels under it.
func cover(source image.Image, width, height int) *image.RGBA {
	bounds := source.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	// The largest part of the source with the target's shape.
	cropW, cropH := w, w*height/width
	if cropH > h {
		cropW, cropH = h*width/height, h
	}
	left, top := bounds.Min.X+(w-cropW)/2, bounds.Min.Y+(h-cropH)/2
	out := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		y0, y1 := top+y*cropH/height, top+(y+1)*cropH/height
		if y1 <= y0 {
			y1 = y0 + 1
		}
		for x := 0; x < width; x++ {
			x0, x1 := left+x*cropW/width, left+(x+1)*cropW/width
			if x1 <= x0 {
				x1 = x0 + 1
			}
			var r, g, b, n uint64
			for sy := y0; sy < y1; sy++ {
				for sx := x0; sx < x1; sx++ {
					cr, cg, cb, ca := source.At(sx, sy).RGBA()
					// A transparent PNG is shown on white, as the screens are light.
					r += uint64(cr + (0xffff - ca))
					g += uint64(cg + (0xffff - ca))
					b += uint64(cb + (0xffff - ca))
					n++
				}
			}
			out.SetRGBA(x, y, color.RGBA{uint8(r / n >> 8), uint8(g / n >> 8), uint8(b / n >> 8), 0xff})
		}
	}
	return out
}

// encodeWithin lowers the JPEG quality until the file fits.
func encodeWithin(picture image.Image, limit, quality int) ([]byte, error) {
	for ; quality >= 30; quality -= 7 {
		var out bytes.Buffer
		if err := jpeg.Encode(&out, picture, &jpeg.Options{Quality: quality}); err != nil {
			return nil, errPhoto
		}
		if out.Len() <= limit {
			return out.Bytes(), nil
		}
	}
	return nil, errPhoto
}

type photoUpload struct {
	Key    string `json:"requestKey"`
	Image  string `json:"image"`
	Rights bool   `json:"rightsConfirmed"`
}

// uploadMenuPhoto takes an operator's picture, base64 in JSON like every other operations request.
func (h HTTP) uploadMenuPhoto(w http.ResponseWriter, r *http.Request) {
	branch := r.PathValue("branch")
	actor, ok := h.operatorCredential(r)
	if !operatorRef.MatchString(branch) || !ok || h.Store == nil {
		http.NotFound(w, r)
		return
	}
	if h.OperatorOrigin == "" || r.Header.Get("Origin") != h.OperatorOrigin || r.Header.Get("Content-Type") != "application/json" {
		http.NotFound(w, r)
		return
	}
	var upload photoUpload
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, int64(base64.StdEncoding.EncodedLen(photoUploadBytes)+1024)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&upload); err != nil || !operatorRef.MatchString(upload.Key) || !upload.Rights {
		operatorResponse(w, nil, service.BadRequest)
		return
	}
	raw, err := base64.StdEncoding.DecodeString(upload.Image)
	if err != nil {
		operatorResponse(w, nil, service.BadRequest)
		return
	}
	photo, thumb, err := makeMenuPhotos(raw)
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "PHOTO_UNREADABLE"})
		return
	}
	var reply json.RawMessage
	err = h.Store.db.QueryRow(authority.Scope(r.Context(), actor, branch), "SELECT ledger.store_menu_photo($1,$2,$3,$4,$5)",
		actor.Tenant, branch, actor.Subject, photo, thumb).Scan(&reply)
	operatorResponse(w, reply, domainError(err))
}

// operatorPhoto shows the operator one photo of the branch, published or still waiting.
func (h HTTP) operatorPhoto(w http.ResponseWriter, r *http.Request) {
	branch, digest := r.PathValue("branch"), r.PathValue("digest")
	actor, ok := h.operatorCredential(r)
	if !operatorRef.MatchString(branch) || !photoDigest.MatchString(digest) || !ok || h.Store == nil {
		http.NotFound(w, r)
		return
	}
	var photo []byte
	err := h.Store.db.QueryRow(authority.Scope(r.Context(), actor, branch), "SELECT ledger.operator_menu_photo($1,$2,$3,$4)",
		actor.Tenant, branch, actor.Subject, digest).Scan(&photo)
	if err != nil || photo == nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "image/jpeg")
	// The name is the content, so the browser may keep it.
	w.Header().Set("Cache-Control", "private, max-age=31536000, immutable")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = w.Write(photo)
}

func (s *Store) photoPart(ctx context.Context, auth service.Bound, digest string, offset int64) (*PhotoPart, error) {
	if !photoDigest.MatchString(digest) || offset < 0 || offset > photoBytes {
		return nil, service.BadRequest
	}
	var size int
	var part []byte
	err := s.db.QueryRow(ctx, "SELECT size,part FROM ledger.catalog_photo_part($1,$2,$3,$4,$5)",
		auth.Tenant, auth.Request.Domain, auth.Subject, digest, offset).Scan(&size, &part)
	if err != nil {
		return nil, domainError(err)
	}
	return &PhotoPart{Size: size, Data: base64.StdEncoding.EncodeToString(part)}, nil
}

func (s *Store) catalogMenuPhotoWindow(ctx context.Context, auth service.Bound, page int32) ([]Price, *int64, []CatalogOptions, []CatalogPhoto, error) {
	var version *int64
	var prices []Price
	var options []CatalogOptions
	var photos []CatalogPhoto
	err := s.db.QueryRow(ctx, "SELECT snapshot_version,items,options,photos FROM ledger.catalog_menu_photo_window($1,$2,$3,$4)",
		auth.Tenant, auth.Request.Domain, auth.Subject, page).Scan(&version, &prices, &options, &photos)
	return prices, version, options, photos, domainError(err)
}

func validPhotos(photos []menuPhoto) bool {
	seen := map[string]bool{}
	for _, photo := range photos {
		if !sku.MatchString(photo.SKU) || !photoDigest.MatchString(photo.Photo) || seen[photo.SKU] {
			return false
		}
		seen[photo.SKU] = true
	}
	return len(photos) <= 200
}

// withPhotos binds the photos in the same statement, and so the same transaction, as the publication.
func withPhotos(call string, args []any, photos []menuPhoto) (string, []any) {
	if photos == nil {
		return call, args
	}
	raw, _ := json.Marshal(photos)
	n := strconv.Itoa(len(args) + 1)
	return "SELECT r FROM (" + call + ") pub(r), LATERAL (SELECT ledger.bind_menu_photos($1,$2,$3,(r->>'version')::bigint,$" + n + ")) bound",
		append(args, raw)
}
