package main

import (
 "context"
 "errors"
 "log"
 "net/http"
 "os"
 "os/signal"
 "syscall"
 "time"

 "github.com/vietkeynet/ZalGPS-server/v2/internal/ingest"
)
func main(){
 h,err:=ingest.New(os.Getenv("ZALGPS_TENANT_ID"),os.Getenv("ZALGPS_DEVICE_ID"),os.Getenv("ZALGPS_DEVICE_TOKEN"))
 if err!=nil {log.Fatal(err)}
 s:=&http.Server{Addr:"127.0.0.1:18082",Handler:h,ReadHeaderTimeout:3*time.Second,ReadTimeout:10*time.Second,WriteTimeout:10*time.Second,IdleTimeout:30*time.Second,MaxHeaderBytes:8192}
 ctx,stop:=signal.NotifyContext(context.Background(),os.Interrupt,syscall.SIGTERM)
 defer stop()
 errs:=make(chan error,1)
 go func(){errs<-s.ListenAndServe()}()
 log.Print("ZalGPS V2 development only: loopback :18082; volatile memory")
 select{
 case err:=<-errs:
  if !errors.Is(err,http.ErrServerClosed){log.Fatal(err)}
 case <-ctx.Done():
  shutdown,cancel:=context.WithTimeout(context.Background(),5*time.Second)
  defer cancel()
  if err:=s.Shutdown(shutdown);err!=nil{log.Print(err);_ = s.Close()}
 }
}
