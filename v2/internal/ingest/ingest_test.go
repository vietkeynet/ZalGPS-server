package ingest

import (
	"context"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type fakeStore struct {
	err       error
	duplicate bool
	calls     int
}

func (f *fakeStore) Accept(_ context.Context, _ [32]byte, _ Position, _ time.Time) (bool, error) {
	f.calls++
	return !f.duplicate, f.err
}
func (f *fakeStore) Latest(context.Context, [32]byte) (Record, error) { return Record{}, f.err }
func TestIngest(t *testing.T) {
	token := strings.Repeat("ab", 32)
	good := `{"fixTime":"2026-11-27T00:00:00Z","latitude":0,"longitude":0,"speedKph":0}`
	cases := []struct {
		name, key, body string
		err             error
		duplicate       bool
		status, calls   int
	}{
		{"no token", "", good, nil, false, 401, 0},
		{"invalid coordinate", token, strings.Replace(good, "latitude\":0", "latitude\":91", 1), nil, false, 422, 0},
		{"missing coordinate", token, `{"fixTime":"2026-11-27T00:00:00Z","longitude":0}`, nil, false, 422, 0},
		{"spoof tenant", token, strings.Replace(good, "{", `{"tenantId":"other",`, 1), nil, false, 400, 0},
		{"extra JSON", token, good + "{}", nil, false, 400, 0},
		{"oversized", token, good + strings.Repeat(" ", 5000), nil, false, 413, 0},
		{"valid", token, good, nil, false, 202, 1},
		{"duplicate", token, good, nil, true, 200, 1},
		{"unauthorized", token, good, ErrUnauthorized, false, 401, 1},
		{"storage error", token, good, errors.New("internal diagnostic"), false, 503, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := &fakeStore{err: tc.err, duplicate: tc.duplicate}
			h := New(f)
			h.now = func() time.Time { return time.Date(2026, 11, 27, 1, 0, 0, 0, time.UTC) }
			r := httptest.NewRequest("POST", "/v2/positions", strings.NewReader(tc.body))
			r.Header.Set("Authorization", "Bearer "+tc.key)
			r.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != tc.status || f.calls != tc.calls {
				t.Fatalf("got %d calls %d want %d calls %d", w.Code, f.calls, tc.status, tc.calls)
			}
			if strings.Contains(w.Body.String(), "internal diagnostic") {
				t.Fatal("leaked error")
			}
		})
	}
}
