// SPDX-License-Identifier: Apache-2.0
package ingest

import (
 "crypto/sha256"
 "crypto/subtle"
 "encoding/json"
 "errors"
 "io"
 "math"
 "mime"
 "net/http"
 "strings"
 "sync"
 "time"
)

// Position is a normalized WGS84 fix. Units are explicit.
// Optional fields are pointers so absent is not confused with zero.
type Position struct {
 FixTime time.Time `json:"fixTime"`
 Latitude *float64 `json:"latitude"`
 Longitude *float64 `json:"longitude"`
 SpeedKPH *float64 `json:"speedKph,omitempty"`
}
type Record struct {
 TenantID string `json:"tenantId"`
 DeviceID string `json:"deviceId"`
 ReceivedAt time.Time `json:"receivedAt"`
 Position
}

// Handler is a single enrolled device, in-memory development vertical slice.
// It must not be exposed to public networks or used as durable storage.
type Handler struct {
 tenant, device string
 key [32]byte
 mu sync.RWMutex
 latest *Record
 now func() time.Time
}
func New(tenant, device, token string) (*Handler, error) {
 if strings.TrimSpace(tenant)=="" || strings.TrimSpace(device)=="" || len(token)<32 {
  return nil, errors.New("tenant, device and >=32-character token required")
 }
 return &Handler{tenant:tenant,device:device,key:sha256.Sum256([]byte(token)),now:time.Now},nil
}
func (h *Handler) authorized(r *http.Request) bool {
 value, ok := strings.CutPrefix(r.Header.Get("Authorization"),"Bearer ")
 if !ok {return false}
 got:=sha256.Sum256([]byte(value))
 return subtle.ConstantTimeCompare(got[:],h.key[:])==1
}
func valid(p Position, now time.Time) bool {
 if p.FixTime.IsZero() || p.FixTime.Before(time.Date(2000,1,1,0,0,0,0,time.UTC)) || p.FixTime.After(now.Add(5*time.Minute)) {return false}
 if p.Latitude==nil || p.Longitude==nil {return false}
 lat,lon:=*p.Latitude,*p.Longitude
 if math.IsNaN(lat)||math.IsNaN(lon)||math.IsInf(lat,0)||math.IsInf(lon,0)||lat < -90||lat > 90||lon < -180||lon > 180 {return false}
 if p.SpeedKPH!=nil && (math.IsNaN(*p.SpeedKPH)||math.IsInf(*p.SpeedKPH,0)||*p.SpeedKPH < 0||*p.SpeedKPH > 1000) {return false}
 return true
}
func write(w http.ResponseWriter, status int, payload any) {
 w.Header().Set("Content-Type","application/json")
 w.Header().Set("Cache-Control","no-store")
 w.WriteHeader(status)
 _=json.NewEncoder(w).Encode(payload)
}
func (h *Handler) ServeHTTP(w http.ResponseWriter,r *http.Request) {
 if r.URL.Path=="/healthz" && r.Method==http.MethodGet {
  write(w,200,map[string]string{"status":"ok","mode":"development-memory"});return
 }
 if r.URL.Path!="/v2/positions" && r.URL.Path!="/v2/positions/latest" {http.NotFound(w,r);return}
 if !h.authorized(r) {write(w,401,map[string]string{"error":"unauthorized"});return}
 if r.URL.Path=="/v2/positions/latest" {
  if r.Method!=http.MethodGet {w.Header().Set("Allow","GET");w.WriteHeader(405);return}
  h.mu.RLock();record:=h.latest;h.mu.RUnlock()
  if record==nil {write(w,404,map[string]string{"error":"no position"});return}
  write(w,200,record);return
 }
 if r.Method!=http.MethodPost {w.Header().Set("Allow","POST");w.WriteHeader(405);return}
 media,_,err:=mime.ParseMediaType(r.Header.Get("Content-Type"))
 if err!=nil || media!="application/json" {w.WriteHeader(415);return}
 r.Body=http.MaxBytesReader(w,r.Body,4096)
 defer r.Body.Close()
 decoder:=json.NewDecoder(r.Body)
 decoder.DisallowUnknownFields()
 var p Position
 if err:=decoder.Decode(&p);err!=nil {
  var tooLarge *http.MaxBytesError
  if errors.As(err,&tooLarge) {w.WriteHeader(413)} else {w.WriteHeader(400)}
  return
 }
 if err:=decoder.Decode(new(any));err!=io.EOF {
  var tooLarge *http.MaxBytesError
  if errors.As(err,&tooLarge) {w.WriteHeader(413)} else {w.WriteHeader(400)}
  return
 }
 now:=h.now().UTC()
 if !valid(p,now) {write(w,422,map[string]string{"error":"invalid fix"});return}
 p.FixTime=p.FixTime.UTC()
 record:=&Record{TenantID:h.tenant,DeviceID:h.device,ReceivedAt:now,Position:p}
 h.mu.Lock()
 if h.latest!=nil && !p.FixTime.After(h.latest.FixTime) {
  h.mu.Unlock();write(w,409,map[string]string{"error":"fix is not newer"});return
 }
 h.latest=record
 h.mu.Unlock()
 write(w,202,map[string]string{"status":"accepted-volatile"})
}
