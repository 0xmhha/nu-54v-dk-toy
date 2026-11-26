package commerce

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/jackc/pgx/v5"

	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/service"
)

// A picture with detail everywhere, so its JPEG is not trivially small.
func testPicture(w, h int, alpha uint8) *image.NRGBA {
	out := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			out.SetNRGBA(x, y, color.NRGBA{uint8(x * 255 / w), uint8(y * 255 / h), uint8((x ^ y) * 7), alpha})
		}
	}
	return out
}

func encoded(t *testing.T, picture image.Image, kind string) []byte {
	t.Helper()
	var out bytes.Buffer
	var err error
	if kind == "png" {
		err = png.Encode(&out, picture)
	} else {
		err = jpeg.Encode(&out, picture, &jpeg.Options{Quality: 95})
	}
	if err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func TestMenuPhotosAreMadeAgain(t *testing.T) {
	for name, upload := range map[string][]byte{
		"wide jpeg": encoded(t, testPicture(1600, 900, 255), "jpeg"), "tall png": encoded(t, testPicture(600, 1400, 255), "png"),
		"small jpeg": encoded(t, testPicture(200, 150, 255), "jpeg"), "4096 square": encoded(t, image.NewGray(image.Rect(0, 0, 4096, 4096)), "png"),
	} {
		photo, thumb, err := makeMenuPhotos(upload)
		if err != nil {
			t.Fatal(name, err)
		}
		for want, file := range map[[3]int][]byte{{800, 600, photoBytes}: photo, {320, 240, thumbBytes}: thumb} {
			config, format, err := image.DecodeConfig(bytes.NewReader(file))
			if err != nil || format != "jpeg" || config.Width != want[0] || config.Height != want[1] || len(file) > want[2] {
				t.Fatal(name, format, config, len(file), err)
			}
		}
		t.Logf("%s: %d bytes in, %d and %d bytes out", name, len(upload), len(photo), len(thumb))
	}
	// Only the pixels come across: a marker after the image's start is not in the photo the server keeps.
	marked := encoded(t, testPicture(900, 700, 255), "jpeg")
	exif := append([]byte{0xff, 0xe1, 0, 0x16}, []byte("Exif\x00\x00GPS 37.55 126.97")...)
	marked = append(append(append([]byte{}, marked[:2]...), exif...), marked[2:]...)
	if photo, thumb, err := makeMenuPhotos(marked); err != nil || bytes.Contains(photo, []byte("GPS")) || bytes.Contains(thumb, []byte("Exif")) {
		t.Fatal("camera data kept", err)
	}
	// A transparent picture is shown on white.
	photo, _, err := makeMenuPhotos(encoded(t, testPicture(800, 600, 0), "png"))
	if err != nil {
		t.Fatal(err)
	}
	if shown, _ := jpeg.Decode(bytes.NewReader(photo)); shown != nil {
		if r, g, b, _ := shown.At(400, 300).RGBA(); r < 0xf000 || g < 0xf000 || b < 0xf000 {
			t.Fatal("not white", r, g, b)
		}
	}
	gif := []byte("GIF89a\x01\x00\x01\x00\x00\x00\x00;")
	heic := append([]byte{0, 0, 0, 0x18}, []byte("ftypheic")...)
	wide := encoded(t, image.NewGray(image.Rect(0, 0, 4097, 10)), "png")
	for name, upload := range map[string][]byte{"gif": gif, "heic": heic, "empty": nil, "too wide": wide,
		"broken jpeg": []byte{0xff, 0xd8, 0xff, 0xe0, 1, 2}, "above 5 MB": append([]byte{0xff, 0xd8, 0xff}, make([]byte, 5<<20)...)} {
		if _, _, err := makeMenuPhotos(upload); err == nil {
			t.Error(name, "was taken")
		}
	}
}

func TestPostgresMenuPhotos(t *testing.T) {
	config, err := pgx.ParseConfig(os.Getenv("P05_COMMERCE_TEST_DATABASE_URL"))
	if os.Getenv("P05_COMMERCE_TEST_DATABASE_URL") == "" {
		t.Skip("requires disposable migrated PostgreSQL")
	}
	if err != nil || !strings.HasPrefix(config.Database, "nu54_commerce_") {
		t.Fatal("use disposable commerce database")
	}
	ctx, stop := context.WithTimeout(context.Background(), time.Minute)
	defer stop()
	admin, err := pgx.ConnectConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close(context.Background())
	execute := func(query string, args ...any) {
		t.Helper()
		if _, err := admin.Exec(ctx, query, args...); err != nil {
			t.Fatal(err)
		}
	}
	role := "nu54_photos_" + strings.TrimPrefix(config.Database, "nu54_commerce_")
	execute("CREATE ROLE " + pgx.Identifier{role}.Sanitize() + " LOGIN IN ROLE nu54_p05_commerce")
	defer admin.Exec(context.Background(), "DROP ROLE "+pgx.Identifier{role}.Sanitize())
	store, err := Open(ctx, config.ConnString()+" user="+role)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	fresh := func() string { return uuidForCatalogFixture(t, ctx, admin) }
	branch, operator, customer, stranger := fresh(), fresh(), fresh(), fresh()
	execute("INSERT INTO ledger.branch VALUES ($1,$2,'Asia/Seoul','04:00')", branch, tenant)
	execute("INSERT INTO ledger.login_subject VALUES ($2,$1)", operator, "photos_"+operator)
	execute("INSERT INTO ledger.staff_grant VALUES ($1,$2,'operator',now()-interval '1 minute',null)", branch, operator)
	execute("INSERT INTO ledger.command_rule VALUES ($1,'photos test','fixture','dispose',interval '2 minutes')", branch)
	execute("INSERT INTO ledger.commerce_grant(branch_id,actor_id,role_code,valid_until) VALUES($1,$2,'customer',now()+interval '5 minutes')", branch, customer)

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{SerialNumber: big.NewInt(1), IsCA: true, BasicConstraintsValid: true,
		NotBefore: time.Now().Add(-time.Minute), NotAfter: time.Now().Add(5 * time.Minute),
		KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}}
	der, err := x509.CreateCertificate(rand.Reader, template, template, key.Public(), key)
	if err != nil {
		t.Fatal(err)
	}
	authority, _ := x509.ParseCertificate(der)
	pool := x509.NewCertPool()
	pool.AddCert(authority)
	digest := sha256.Sum256(der)
	asset := "0x" + strings.Repeat("2", 40)
	mux := http.NewServeMux()
	server := httptest.NewUnstartedServer(mux)
	defer server.Close()
	origin := "https://" + server.Listener.Addr().String()
	HTTP{Store: store, OperatorOrigin: origin, Profiles: map[string]Profile{branch: {Chain: 8283, Asset: common.HexToAddress(asset), Token: wonForOptions}},
		Credentials: map[string]service.Principal{hex.EncodeToString(digest[:]): {Tenant: tenant, Subject: operator, Role: "operator",
			ExpiresAt: time.Now().Add(5 * time.Minute).Unix()}}}.Register(mux)
	server.TLS = &tls.Config{MinVersion: tls.VersionTLS13, ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: pool}
	server.StartTLS()
	client := server.Client()
	client.Transport.(*http.Transport).TLSClientConfig.Certificates = []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}}
	call := func(method, path string, body any, want int) []byte {
		t.Helper()
		raw, _ := json.Marshal(body)
		req, _ := http.NewRequestWithContext(ctx, method, server.URL+"/v2/operations/catalog/"+branch+path, bytes.NewReader(raw))
		req.Header.Set("Origin", origin)
		req.Header.Set("Content-Type", "application/json")
		reply, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer reply.Body.Close()
		out, _ := io.ReadAll(reply.Body)
		if reply.StatusCode != want {
			t.Fatalf("%s: got %d, want %d: %.300s", path, reply.StatusCode, want, out)
		}
		return out
	}
	upload := func(picture []byte, rights bool, want int) map[string]any {
		var out map[string]any
		_ = json.Unmarshal(call("POST", "/photo", map[string]any{"requestKey": fresh(), "image": base64.StdEncoding.EncodeToString(picture), "rightsConfirmed": rights}, want), &out)
		return out
	}

	latte := upload(encoded(t, testPicture(1200, 900, 255), "jpeg"), true, 200)
	again := upload(encoded(t, testPicture(1200, 900, 255), "jpeg"), true, 200)
	if latte["photo"] != again["photo"] || !photoDigest.MatchString(fmt.Sprint(latte["thumb"])) || latte["bytes"].(float64) > photoBytes {
		t.Fatal(latte, again)
	}
	upload(encoded(t, testPicture(10, 10, 255), "jpeg"), false, 400)
	upload([]byte("GIF89a"), true, 422)
	photo, thumb := fmt.Sprint(latte["photo"]), fmt.Sprint(latte["thumb"])
	if shown := call("GET", "/photo/"+photo, nil, 200); fmt.Sprintf("%x", sha256.Sum256(shown)) != photo {
		t.Fatal("the operator sees another photo")
	}
	call("GET", "/photo/"+strings.Repeat("a", 64), nil, 404)

	read := func(who string, page int32) ([]Price, []CatalogPhoto, error) {
		rows, _, _, photos, err := store.catalogMenuPhotoWindow(ctx, authorize(t, actor{who, "customer"}, "catalog.read", branch, branch, "",
			map[string]string{"page": fmt.Sprint(page)}), page)
		return rows, photos, err
	}
	part := func(who, name string, offset int64) (*PhotoPart, error) {
		return store.photoPart(ctx, authorize(t, actor{who, "customer"}, "catalog.photo", branch, branch, "",
			map[string]string{"photo": name, "offset": fmt.Sprint(offset)}), name, offset)
	}
	// Not published yet: nobody but the operator reads it.
	if _, err := part(customer, photo, 0); err == nil {
		t.Fatal("an unpublished photo was read")
	}
	row := func(sku string) operatorRow {
		return operatorRow{SKU: sku, Option: "standard", Label: sku, Asset: asset, DisplayPrice: "4500"}
	}
	publication := func(version int64, photos any) map[string]any {
		return map[string]any{"requestKey": fresh(), "version": version, "expiresAt": time.Now().Add(time.Hour).UTC(),
			"rows": []operatorRow{row("latte"), row("cookie")}, "photos": photos}
	}
	// Uploading the same picture again starts its day again, so it is not removed under an open draft.
	execute("UPDATE ledger.menu_photo SET created_at=now()-interval '23 hours 59 minutes' WHERE branch_id=$1", branch)
	upload(encoded(t, testPicture(1200, 900, 255), "jpeg"), true, 200)
	var fresher bool
	if err := admin.QueryRow(ctx, "SELECT bool_and(created_at > now()-interval '1 minute') FROM ledger.menu_photo WHERE branch_id=$1", branch).Scan(&fresher); err != nil || !fresher {
		t.Fatal("uploading again did not start the photo's day again", err)
	}
	first := publication(0, []menuPhoto{{"latte", photo}})
	call("POST", "/publish", first, 200)
	call("POST", "/publish", first, 200) // the same request again
	first["photos"] = []menuPhoto{}
	call("POST", "/publish", first, 409) // the same key, other photos
	for name, photos := range map[string]any{"a product that is not on the menu": []menuPhoto{{"mocha", photo}},
		"a photo that was never uploaded": []menuPhoto{{"latte", strings.Repeat("b", 64)}},
		"one product twice":               []menuPhoto{{"latte", photo}, {"latte", photo}}} {
		call("POST", "/publish", publication(1, photos), 400)
		_ = name
	}
	rows, photos, err := read(customer, 0)
	if err != nil || len(rows) != 2 || len(photos) != 1 || photos[0] != (CatalogPhoto{"latte", photo, thumb}) {
		t.Fatal(rows, photos, err)
	}
	var home struct {
		Photos []map[string]any `json:"photos"`
	}
	if json.Unmarshal(call("GET", "", nil, 200), &home) != nil || len(home.Photos) != 1 || home.Photos[0]["thumb"] != thumb {
		t.Fatal(home)
	}
	// The app joins the parts, each under the reply limit, and checks the name.
	for name, size := range map[string]float64{photo: latte["bytes"].(float64), thumb: latte["thumbBytes"].(float64)} {
		var joined []byte
		for offset := int64(0); ; {
			piece, err := part(customer, name, offset)
			if err != nil {
				t.Fatal(err)
			}
			raw, _ := json.Marshal(piece)
			data, _ := base64.StdEncoding.DecodeString(piece.Data)
			if len(raw) > 24600 || piece.Size != int(size) {
				t.Fatal(len(raw), piece.Size)
			}
			joined, offset = append(joined, data...), offset+int64(len(data))
			if offset >= int64(piece.Size) {
				break
			}
		}
		if fmt.Sprintf("%x", sha256.Sum256(joined)) != name {
			t.Fatal("joined parts differ")
		}
	}
	if _, err := part(customer, thumb, int64(latte["thumbBytes"].(float64))); err == nil {
		t.Fatal("read past the end")
	}
	if _, err := part(stranger, photo, 0); err == nil {
		t.Fatal("a stranger read a photo")
	}
	// A publication from an older page has no photos; the photo stays for the next one.
	call("POST", "/publish", map[string]any{"requestKey": fresh(), "version": 1, "expiresAt": time.Now().Add(time.Hour).UTC(),
		"rows": []operatorRow{row("latte")}}, 200)
	if _, photos, err := read(customer, 0); err != nil || len(photos) != 0 {
		t.Fatal(photos, err)
	}
	call("POST", "/publish", publication(2, []menuPhoto{{"cookie", photo}, {"latte", photo}}), 200)
	if _, photos, _ := read(customer, 0); len(photos) != 2 {
		t.Fatal(photos)
	}
	// A price that includes its tax takes no tax amount on top; one that says nothing keeps the old way.
	taxed := func(tax string) map[string]any {
		r := row("latte")
		r.Metadata, r.Tax = &menuMetadata{Tax: "vat10"}, tax
		return map[string]any{"requestKey": fresh(), "version": 3, "expiresAt": time.Now().Add(time.Hour).UTC(), "rows": []operatorRow{r}}
	}
	call("POST", "/publish", taxed("100"), 400)
	call("POST", "/publish", taxed("0"), 200)
	if rows, _, _ := read(customer, 0); len(rows) != 1 || rows[0].Metadata == nil || rows[0].Metadata.Tax != "vat10" || rows[0].UnitPrice != "450000" {
		t.Fatalf("%+v", rows)
	}
	call("POST", "/publish", publication(4, []menuPhoto{{"cookie", photo}, {"latte", photo}}), 200)
	// A publication without photos is recorded as none: the same request with photos is refused, not added to it.
	none := publication(5, nil)
	delete(none, "photos")
	call("POST", "/publish", none, 200)
	none["photos"] = []menuPhoto{{"latte", photo}}
	call("POST", "/publish", none, 409)

	// At most 50 photos a day that no menu uses.
	execute("INSERT INTO ledger.menu_photo SELECT branch_id, encode(sha256(photo||n::text::bytea),'hex'), encode(sha256(thumb||n::text::bytea),'hex'), photo||n::text::bytea, thumb||n::text::bytea, uploaded_by, now(), now() FROM ledger.menu_photo, generate_series(1,50) n WHERE branch_id=$1", branch)
	upload(encoded(t, testPicture(640, 480, 255), "png"), true, 400)
	upload(encoded(t, testPicture(1200, 900, 255), "jpeg"), true, 200) // one it has already

	// A day later the unpublished uploads go; the one a published version names stays.
	execute("UPDATE ledger.menu_photo SET created_at=now()-interval '2 days' WHERE branch_id=$1", branch)
	for gone := 1; gone > 0; {
		if gone, err = store.PruneMenuPhotos(ctx); err != nil {
			t.Fatal(err)
		}
	}
	var left []string
	if err := admin.QueryRow(ctx, "SELECT array_agg(digest) FROM ledger.menu_photo WHERE branch_id=$1", branch).Scan(&left); err != nil || len(left) != 1 || left[0] != photo {
		t.Fatal(left, err)
	}
	if _, err := part(customer, photo, 0); err != nil {
		t.Fatal("the published photo went", err)
	}
}
