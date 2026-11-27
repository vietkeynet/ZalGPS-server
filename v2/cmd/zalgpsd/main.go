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
	command := os.Args[1]
	dsnName := map[string]string{"migrate": "ZALGPS_MIGRATOR_DATABASE_URL", "serve": "ZALGPS_RUNTIME_DATABASE_URL", "enroll": "ZALGPS_PROVISIONER_DATABASE_URL", "rotate": "ZALGPS_PROVISIONER_DATABASE_URL", "revoke": "ZALGPS_PROVISIONER_DATABASE_URL"}[command]
	if dsnName == "" {
		log.Fatal("unknown command")
	}
	db, e := storage.Open(ctx, os.Getenv(dsnName))
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
		if e = db.CheckRuntimeSchema(ctx); e != nil {
			log.Fatal("database schema not ready")
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
