package ingest

import (
 "net/http/httptest"
 "strings"
 "testing"
 "time"
)

const token="0123456789abcdef0123456789abcdef"
const good=`{"fixTime":"2026-11-27T00:00:00Z","latitude":20.4,"longitude":106.2,"speedKph":0}`
func TestIngest(t *testing.T) {
 h,err:=New("tenant-a","device-a",token)
 if err!=nil {t.Fatal(err)}
 h.now=func()time.Time{return time.Date(2026,11,27,1,0,0,0,time.UTC)}
 request:=func(method,path,key,body string)*httptest.ResponseRecorder{
  r:=httptest.NewRequest(method,path,strings.NewReader(body))
  r.Header.Set("Authorization","Bearer "+key)
  r.Header.Set("Content-Type","application/json")
  w:=httptest.NewRecorder();h.ServeHTTP(w,r);return w
 }
 cases:=[]struct{name,method,path,key,body string; status int}{
  {"no token","POST","/v2/positions","",good,401},
  {"empty latest","GET","/v2/positions/latest",token,"",404},
  {"invalid coordinate","POST","/v2/positions",token,strings.Replace(good,"20.4","91",1),422},
  {"missing coordinate","POST","/v2/positions",token,`{"fixTime":"2026-11-27T00:00:00Z","longitude":0}`,422},
  {"spoof tenant","POST","/v2/positions",token,strings.Replace(good,"{",`{"tenantId":"other",`,1),400},
  {"extra JSON","POST","/v2/positions",token,good+"{}",400},
  {"oversized","POST","/v2/positions",token,good+strings.Repeat(" ",5000),413},
  {"valid","POST","/v2/positions",token,good,202},
  {"duplicate","POST","/v2/positions",token,good,409},
  {"late","POST","/v2/positions",token,strings.Replace(good,"2026-11-27T00","2026-11-26T00",1),409},
  {"latest unauthorized","GET","/v2/positions/latest","wrong","",401},
  {"latest","GET","/v2/positions/latest",token,"",200},
 }
 for _,tc:=range cases {t.Run(tc.name,func(t *testing.T){
  w:=request(tc.method,tc.path,tc.key,tc.body)
  if w.Code!=tc.status {t.Fatalf("got %d want %d: %s",w.Code,tc.status,w.Body.String())}
  if tc.name=="latest" && (!strings.Contains(w.Body.String(),`"tenantId":"tenant-a"`)||!strings.Contains(w.Body.String(),`"deviceId":"device-a"`)){t.Fatal(w.Body.String())}
 })}
}
func TestEnrollment(t *testing.T) {
 if _,err:=New("t","d","short");err==nil {t.Fatal("accepted short token")}
 if _,err:=New("","d",token);err==nil {t.Fatal("accepted empty tenant")}
}
