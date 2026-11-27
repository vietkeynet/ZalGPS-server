package storage

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"github.com/vietkeynet/ZalGPS-server/v2/internal/ingest"
	"os"
	"testing"
	"time"
)

func randomKey(t *testing.T) [32]byte {
	t.Helper()
	var b [32]byte
	if _, e := rand.Read(b[:]); e != nil {
		t.Fatal(e)
	}
	k, e := ingest.HashToken(hex.EncodeToString(b[:]))
	if e != nil {
		t.Fatal(e)
	}
	return k
}
func TestPostgresIntegration(t *testing.T) {
	dsn := os.Getenv("ZALGPS_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("disposable integration database not configured")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	p, e := Open(ctx, dsn)
	if e != nil {
		t.Fatal(e)
	}
	defer p.Close()
	if e = p.Migrate(ctx); e != nil {
		t.Fatal(e)
	}
	var suffix [8]byte
	if _, e = rand.Read(suffix[:]); e != nil {
		t.Fatal(e)
	}
	a, b := "test-a-"+hex.EncodeToString(suffix[:]), "test-b-"+hex.EncodeToString(suffix[:])
	ka, kb, kc := randomKey(t), randomKey(t), randomKey(t)
	for _, x := range []struct {
		tenant string
		k      [32]byte
	}{{a, ka}, {b, kb}} {
		if e = p.Enroll(ctx, x.tenant, "same-device", x.k); e != nil {
			t.Fatal(e)
		}
	}
	if e = p.Enroll(ctx, b, "other-device", ka); e == nil {
		t.Fatal("duplicate token allowed")
	}
	lat, lon := 0.0, 0.0
	now := time.Now().UTC().Truncate(time.Microsecond)
	pos := ingest.Position{FixTime: now, Latitude: &lat, Longitude: &lon}
	if ok, e := p.Accept(ctx, ka, pos, now); e != nil || !ok {
		t.Fatalf("insert: %v %v", ok, e)
	}
	if ok, e := p.Accept(ctx, ka, pos, now); e != nil || ok {
		t.Fatalf("duplicate: %v %v", ok, e)
	}
	late := pos
	late.FixTime = now.Add(-time.Hour)
	if ok, e := p.Accept(ctx, ka, late, now); e != nil || !ok {
		t.Fatalf("late: %v %v", ok, e)
	}
	if _, e = p.Latest(ctx, kb); !errors.Is(e, ingest.ErrNotFound) {
		t.Fatalf("cross tenant read: %v", e)
	}
	if ok, e := p.Accept(ctx, kb, pos, now); e != nil || !ok {
		t.Fatalf("tenant scoped dedup: %v %v", ok, e)
	}
	p.Close()
	p, e = Open(ctx, dsn)
	if e != nil {
		t.Fatal(e)
	}
	defer p.Close()
	r, e := p.Latest(ctx, ka)
	if e != nil || r.TenantID != a || !r.FixTime.Equal(now) {
		t.Fatalf("latest after reconnect: %v", e)
	}
	if e = p.Rotate(ctx, a, "same-device", kc); e != nil {
		t.Fatal(e)
	}
	if _, e = p.Latest(ctx, ka); !errors.Is(e, ingest.ErrUnauthorized) {
		t.Fatal("old token still active")
	}
	if _, e = p.Latest(ctx, kc); e != nil {
		t.Fatal(e)
	}
	if e = p.Revoke(ctx, a, "same-device"); e != nil {
		t.Fatal(e)
	}
	if _, e = p.Accept(ctx, kc, pos, now); !errors.Is(e, ingest.ErrUnauthorized) {
		t.Fatal("revoked token accepted")
	}
	if _, e = p.Latest(ctx, kb); e != nil {
		t.Fatal("other tenant affected")
	}
}
