package mapterm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// OpenFreeMap is the default tile source's TileJSON.
const OpenFreeMap = "https://tiles.openfreemap.org/planet"

// Attribution is the credit OpenFreeMap asks clients other than MapLibre to
// show, and that the OpenStreetMap licence requires.
const Attribution = "© OpenMapTiles © OpenStreetMap"

// CopyrightURL is where the OpenStreetMap licence is explained.
const CopyrightURL = "https://www.openstreetmap.org/copyright"

// Source supplies vector tiles.
type Source interface {
	// Tile returns a tile's raw bytes. An empty slice is a tile with no data.
	Tile(ctx context.Context, id TileID) ([]byte, error)
	// MaxZoom is the deepest zoom with tiles.
	MaxZoom() int
}

// maxParallel is OpenFreeMap's limit on parallel requests per client.
const maxParallel = 4

// tileJSONLife is how long a TileJSON is trusted. OpenFreeMap serves it with
// max-age=3600, and its tile URLs name a dated build of the data.
const tileJSONLife = time.Hour

// maxTileDownload bounds one response.
const maxTileDownload = 8 << 20

// HTTPSource fetches tiles over HTTP from a TileJSON endpoint. It sends a
// User-Agent naming the application and makes at most four requests at once,
// as OpenFreeMap's terms ask.
type HTTPSource struct {
	URL       string
	UserAgent string
	Client    *http.Client
	// MetaPath, when set, keeps the last good TileJSON on disk, so a map
	// can start offline and draw from its disk cache.
	MetaPath string

	slots chan struct{}

	mu       sync.Mutex
	template string
	maxZoom  int
	loadedAt time.Time
}

// NewHTTPSource builds a source for a TileJSON URL.
func NewHTTPSource(tileJSON, userAgent string) *HTTPSource {
	return &HTTPSource{
		URL:       tileJSON,
		UserAgent: userAgent,
		Client:    &http.Client{Timeout: 20 * time.Second},
		slots:     make(chan struct{}, maxParallel),
		maxZoom:   14,
	}
}

// MaxZoom is the source's deepest zoom, from its TileJSON once loaded.
func (s *HTTPSource) MaxZoom() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.maxZoom
}

// Tile fetches one tile.
func (s *HTTPSource) Tile(ctx context.Context, id TileID) ([]byte, error) {
	if !id.Valid() {
		return nil, fmt.Errorf("tile %s does not exist", id)
	}
	tmpl, err := s.tileTemplate(ctx)
	if err != nil {
		return nil, err
	}
	u := strings.NewReplacer("{z}", fmt.Sprint(id.Z), "{x}", fmt.Sprint(id.X), "{y}", fmt.Sprint(id.Y)).Replace(tmpl)
	body, status, err := s.get(ctx, u)
	if err != nil {
		return nil, err
	}
	switch status {
	case http.StatusOK:
		return body, nil
	case http.StatusNoContent, http.StatusNotFound:
		// Sources leave out tiles with nothing in them, such as open sea.
		return []byte{}, nil
	}
	return nil, fmt.Errorf("tile %s: HTTP %d", id, status)
}

// tileTemplate loads the TileJSON when it is missing or older than an hour. A
// failed reload keeps the previous template, so a map keeps working offline
// from its cache.
func (s *HTTPSource) tileTemplate(ctx context.Context) (string, error) {
	s.mu.Lock()
	tmpl, fresh := s.template, time.Since(s.loadedAt) < tileJSONLife
	s.mu.Unlock()
	if tmpl != "" && fresh {
		return tmpl, nil
	}
	body, status, err := s.get(ctx, s.URL)
	if err == nil && status != http.StatusOK {
		err = fmt.Errorf("tile metadata: HTTP %d", status)
	}
	var tj struct {
		Tiles   []string `json:"tiles"`
		MaxZoom *int     `json:"maxzoom"`
	}
	if err == nil {
		err = json.Unmarshal(body, &tj)
	}
	if err == nil && (len(tj.Tiles) == 0 || !strings.Contains(tj.Tiles[0], "{z}")) {
		err = errors.New("tile metadata lists no tile URL")
	}
	if err != nil {
		if tmpl != "" {
			return tmpl, nil
		}
		if saved, serr := s.loadMeta(); serr == nil {
			return saved, nil
		}
		return "", err
	}
	if s.MetaPath != "" && os.MkdirAll(filepath.Dir(s.MetaPath), 0o700) == nil {
		_ = os.WriteFile(s.MetaPath, body, 0o600)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.template, s.loadedAt = tj.Tiles[0], time.Now()
	if tj.MaxZoom != nil && *tj.MaxZoom > 0 && *tj.MaxZoom <= 22 {
		s.maxZoom = *tj.MaxZoom
	}
	return s.template, nil
}

// loadMeta reads the saved TileJSON. It is used only while the server cannot
// be reached, and does not count as fresh, so the next request tries again.
func (s *HTTPSource) loadMeta() (string, error) {
	if s.MetaPath == "" {
		return "", errors.New("no saved metadata")
	}
	b, err := os.ReadFile(s.MetaPath)
	if err != nil {
		return "", err
	}
	var tj struct {
		Tiles []string `json:"tiles"`
	}
	if err := json.Unmarshal(b, &tj); err != nil || len(tj.Tiles) == 0 {
		return "", errors.New("saved metadata unreadable")
	}
	return tj.Tiles[0], nil
}

func (s *HTTPSource) get(ctx context.Context, url string) ([]byte, int, error) {
	select {
	case s.slots <- struct{}{}:
	case <-ctx.Done():
		return nil, 0, ctx.Err()
	}
	defer func() { <-s.slots }()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("User-Agent", s.UserAgent)
	resp, err := s.Client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxTileDownload+1))
	if err != nil {
		return nil, 0, err
	}
	if len(body) > maxTileDownload {
		return nil, 0, errors.New("response too large")
	}
	return body, resp.StatusCode, nil
}
