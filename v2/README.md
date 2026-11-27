# ZalGPS V2 — Go development baseline

Status: experimental, NOT production. Legacy Java server remains untouched.
Base reference: 83d7f889c2aec641d94fc6a40c8398995481f4fb (Traccar-derived, Apache-2.0).
Architecture and delivery gates: [docs/architecture.md](docs/architecture.md).

## What is implemented
Single enrolled development device, authenticated HTTP JSON ingest, WGS84 validation, bounded request body, normalized timestamps and units, latest-fix read, rejection of non-newer fixes, loopback-only service and graceful shutdown. No external Go dependencies. Tests cover authentication, tenant spoofing, invalid payloads, bounds, duplicates and late fixes.
This is NOT a Traccar or OsmAnd-compatible endpoint, durable ingest, production tenant isolation or a replacement for device protocols. All data is volatile. HTTP 202 explicitly means accepted in memory, not durable ACK. Use synthetic data only.

## Run locally
```sh
cd v2
go test -race ./...
go vet ./...
export ZALGPS_TENANT_ID=demo ZALGPS_DEVICE_ID=demo-device
export ZALGPS_DEVICE_TOKEN="$(openssl rand -hex 32)"
go run ./cmd/zalgpsd
```
In another shell with the same token (do not commit credentials):
```sh
curl -fsS http://127.0.0.1:18082/healthz
curl -fsS -X POST http://127.0.0.1:18082/v2/positions \
 -H "Authorization: Bearer $ZALGPS_DEVICE_TOKEN" -H 'Content-Type: application/json' \
 -d "{\"fixTime\":\"$(date -u +%Y-%m-%dT%H:%M:%SZ)\",\"latitude\":20.4,\"longitude\":106.2}"
curl -fsS -H "Authorization: Bearer $ZALGPS_DEVICE_TOKEN" http://127.0.0.1:18082/v2/positions/latest
```
Use a supported patched Go toolchain before production. Never expose this development server publicly.
