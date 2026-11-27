package main

import (
	"context"
	"errors"
	"github.com/vietkeynet/ZalGPS-server/v2/internal/ingest"
	"github.com/vietkeynet/ZalGPS-server/v2/internal/storage"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	if len(os.Args) != 2 {
		log.Fatal("usage: zalgpsd migrate|enroll|rotate|revoke|serve")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	db, e := storage.Open(ctx, os.Getenv("ZALGPS_DATABASE_URL"))
	if e != nil {
		log.Fatal("database unavailable or invalid configuration")
	}
	defer db.Close()
	t, d := os.Getenv("ZALGPS_TENANT_ID"), os.Getenv("ZALGPS_DEVICE_ID")
	switch os.Args[1] {
	case "migrate":
		e = db.Migrate(ctx)
	case "enroll", "rotate":
		k, err := ingest.HashToken(os.Getenv("ZALGPS_DEVICE_TOKEN"))
		if err != nil {
			log.Fatal("32 random bytes encoded as lowercase hex required")
		}
		if os.Args[1] == "enroll" {
			e = db.Enroll(ctx, t, d, k)
		} else {
			e = db.Rotate(ctx, t, d, k)
		}
	case "revoke":
		e = db.Revoke(ctx, t, d)
	case "serve":
		if e = db.CheckSchema(ctx); e != nil {
			log.Fatal("database migration required")
		}
		cancel()
		serve(ingest.New(db))
		return
	default:
		log.Fatal("unknown command")
	}
	if e != nil {
		log.Fatal("operation failed; check configuration privately")
	}
	log.Print("operation completed")
}
func serve(h http.Handler) {
	s := &http.Server{Addr: "127.0.0.1:18082", Handler: h, ReadHeaderTimeout: 3 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 10 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 8192}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	errs := make(chan error, 1)
	go func() { errs <- s.ListenAndServe() }()
	log.Print("ZalGPS V2 pilot: loopback only")
	select {
	case e := <-errs:
		if !errors.Is(e, http.ErrServerClosed) {
			log.Fatal("listener failed")
		}
	case <-ctx.Done():
		c, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if e := s.Shutdown(c); e != nil {
			_ = s.Close()
		}
	}
}
