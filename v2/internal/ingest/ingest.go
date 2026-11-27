// SPDX-License-Identifier: Apache-2.0
package ingest

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"math"
	"mime"
	"net/http"
	"strings"
	"time"
)

// Position is a normalized WGS84 fix. Units are explicit.
// Optional fields are pointers so absent is not confused with zero.
type Position struct {
	FixTime   time.Time `json:"fixTime"`
	Latitude  *float64  `json:"latitude"`
	Longitude *float64  `json:"longitude"`
	SpeedKPH  *float64  `json:"speedKph,omitempty"`
}
type Record struct {
	TenantID   string    `json:"tenantId"`
	DeviceID   string    `json:"deviceId"`
	ReceivedAt time.Time `json:"receivedAt"`
	Position
}

var ErrUnauthorized = errors.New("unauthorized")
var ErrNotFound = errors.New("not found")

type Store interface {
	Accept(context.Context, [32]byte, Position, time.Time) (bool, error)
	Latest(context.Context, [32]byte) (Record, error)
}
type Handler struct {
	store Store
	now   func() time.Time
	slots chan struct{}
}

func New(store Store) *Handler {
	return &Handler{store: store, now: time.Now, slots: make(chan struct{}, 64)}
}
func HashToken(token string) ([32]byte, error) {
	if len(token) != 64 || strings.ToLower(token) != token {
		return [32]byte{}, ErrUnauthorized
	}
	b, e := hex.DecodeString(token)
	if e != nil {
		return [32]byte{}, ErrUnauthorized
	}
	return sha256.Sum256(b), nil
}
func storeError(w http.ResponseWriter, e error) {
	switch {
	case errors.Is(e, ErrUnauthorized):
		write(w, 401, map[string]string{"error": "unauthorized"})
	case errors.Is(e, ErrNotFound):
		write(w, 404, map[string]string{"error": "no position"})
	default:
		write(w, 503, map[string]string{"error": "temporarily unavailable"})
	}
}
func valid(p Position, now time.Time) bool {
	if p.FixTime.IsZero() || p.FixTime.Before(time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)) || p.FixTime.After(now.Add(5*time.Minute)) {
		return false
	}
	if p.Latitude == nil || p.Longitude == nil {
		return false
	}
	lat, lon := *p.Latitude, *p.Longitude
	if math.IsNaN(lat) || math.IsNaN(lon) || math.IsInf(lat, 0) || math.IsInf(lon, 0) || lat < -90 || lat > 90 || lon < -180 || lon > 180 {
		return false
	}
	if p.SpeedKPH != nil && (math.IsNaN(*p.SpeedKPH) || math.IsInf(*p.SpeedKPH, 0) || *p.SpeedKPH < 0 || *p.SpeedKPH > 1000) {
		return false
	}
	return true
}
func write(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if r.URL.Path == "/healthz" && r.Method == http.MethodGet {
		write(w, 200, map[string]string{"status": "ok", "check": "liveness"})
		return
	}
	if r.URL.Path != "/v2/positions" && r.URL.Path != "/v2/positions/latest" {
		http.NotFound(w, r)
		return
	}
	select {
	case h.slots <- struct{}{}:
		defer func() { <-h.slots }()
	default:
		w.Header().Set("Retry-After", "1")
		w.WriteHeader(503)
		return
	}
	token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	key, e := HashToken(token)
	if !ok || e != nil {
		storeError(w, ErrUnauthorized)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	if r.URL.Path == "/v2/positions/latest" {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", "GET")
			w.WriteHeader(405)
			return
		}
		record, e := h.store.Latest(ctx, key)
		if e != nil {
			storeError(w, e)
			return
		}
		write(w, 200, record)
		return
	}
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		w.WriteHeader(405)
		return
	}
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "application/json" {
		w.WriteHeader(415)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	defer r.Body.Close()
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	var p Position
	if err := decoder.Decode(&p); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			w.WriteHeader(413)
		} else {
			w.WriteHeader(400)
		}
		return
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			w.WriteHeader(413)
		} else {
			w.WriteHeader(400)
		}
		return
	}
	now := h.now().UTC()
	if !valid(p, now) {
		write(w, 422, map[string]string{"error": "invalid fix"})
		return
	}
	p.FixTime = p.FixTime.UTC().Truncate(time.Microsecond)
	if *p.Latitude == 0 {
		*p.Latitude = 0
	}
	if *p.Longitude == 0 {
		*p.Longitude = 0
	}
	if p.SpeedKPH != nil && *p.SpeedKPH == 0 {
		*p.SpeedKPH = 0
	}
	inserted, e := h.store.Accept(ctx, key, p, now)
	if e != nil {
		storeError(w, e)
		return
	}
	if !inserted {
		write(w, 200, map[string]string{"status": "duplicate"})
		return
	}
	write(w, 202, map[string]string{"status": "accepted-durable"})
}
