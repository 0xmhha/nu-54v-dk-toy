package places

import (
	"encoding/json"
	"errors"
	"fmt"
	"image/png"
	"net/http"
	"os"
	"strconv"
	"strings"
)

// RegisterMap serves an operator-approved, pre-rendered Web Mercator raster archive.
func RegisterMap(mux *http.ServeMux, root *os.Root, attribution string, minZoom, maxZoom int) error {
	if root == nil || strings.TrimSpace(attribution) == "" || len(attribution) > 200 ||
		minZoom < 0 || maxZoom > 19 || maxZoom < minZoom {
		return errors.New("invalid public map configuration")
	}
	cache := func(w http.ResponseWriter) {
		w.Header().Set("Cache-Control", "public, max-age=3600")
	}
	mux.HandleFunc("GET /v1/map", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.RawQuery != "" {
			http.NotFound(w, r)
			return
		}
		cache(w)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"minZoom": minZoom, "maxZoom": maxZoom, "attribution": attribution})
	})
	mux.HandleFunc("GET /v1/map/{z}/{x}/{tile}", func(w http.ResponseWriter, r *http.Request) {
		z, errZ := strconv.Atoi(r.PathValue("z"))
		x, errX := strconv.Atoi(r.PathValue("x"))
		y, errY := strconv.Atoi(strings.TrimSuffix(r.PathValue("tile"), ".png"))
		if errZ != nil || errX != nil || errY != nil || z < minZoom || z > maxZoom ||
			x < 0 || y < 0 || x >= 1<<z || y >= 1<<z || r.URL.RawQuery != "" ||
			r.PathValue("tile") != strconv.Itoa(y)+".png" {
			http.NotFound(w, r)
			return
		}
		f, err := root.Open(fmt.Sprintf("%d/%d/%d.png", z, x, y))
		if err != nil {
			http.NotFound(w, r)
			return
		}
		defer f.Close()
		stat, err := f.Stat()
		if err != nil || !stat.Mode().IsRegular() || stat.Size() < 8 || stat.Size() > 1<<20 {
			http.NotFound(w, r)
			return
		}
		info, err := png.DecodeConfig(f)
		if err != nil || info.Width != 256 || info.Height != 256 {
			http.NotFound(w, r)
			return
		}
		if _, err := f.Seek(0, 0); err != nil {
			http.NotFound(w, r)
			return
		}
		cache(w)
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		http.ServeContent(w, r, stat.Name(), stat.ModTime(), f)
	})
	return nil
}
