# ZalGPS V2 — authoritative execution backlog

Baseline 2026-11-27: implementation commit 8c245ce2b47e05cf427e7c6e73de2277282766e1; [CI #1 SUCCESS](https://github.com/vietkeynet/ZalGPS-server/actions/runs/45943717066). Status reflects scoped evidence, not production approval. Private security/deployment evidence is recorded separately. Dependencies are hard gates unless coordinator records an approved change. One active task per agent, one owner per file; coordinator updates statuses. No speculative issues/PRs.

| ID | Priority | Status | Depends on | Owner role | Deliverable / acceptance |
|---|---|---|---|---|---|
| BASE-01 | P0 | DONE | — | Core | Java inventory + Go HTTP pilot; legacy remains intact |
| BASE-02 | P0 | DONE | BASE-01 | Core/QA | PostgreSQL native ingest, hashed tokens, scoped dedup, late fixes, revoke/rotate; CI #1 |
| SEC-01 | P0 | IN_PROGRESS | BASE-02 | Security | Private full-ref history scan triage; false positives vs real credentials; revoke/rotate real keys before any history decision; no public sensitive details |
| SEC-02 | P0 | READY | BASE-02 | Maintainer | Verify/enable supported secret scanning/push protection, private reporting, dependency alerts and suitable branch rules; record approval and safe negative test; no force push |
| SEC-03 | P0 | IN_PROGRESS | BASE-02 | Security/CI | Threat model, CI least privilege, dependency/vulnerability checks, staged secret guard, review legacy workflows and action pins; no private runner |
| DB-01 | P0 | DONE | BASE-02 | Core | Versioned immutable migrations with checksums, upgrade from pilot schema, fresh DB and migration concurrency tests; no destructive migration |
| DB-02 | P0 | TODO | DB-01, SEC-03 | Core/Security | Separate migrator/provisioner/runtime DB roles, RLS and constrained device auth; prove no cross-tenant direct SQL, cache or query leakage; connection pool context reset; no owner/BYPASSRLS runtime |
| DB-03 | P0 | READY | DB-01 | Core | PostGIS WGS84 and spatial index, verify coordinate order and units; dedicated extension test DB |
| DB-04 | P0 | TODO | DB-01, DB-02 | Core | Durable inbox + transactional outbox, bounded worker, per-device ordering, protocol-aware identity and replay; crash after commit and retry tests |
| DB-05 | P0 | TODO | DB-04 | Core/QA | Latest-state table, history partitions, retention and legal hold; dedup across partitions, explain plans, migration and late-fix tests |
| OPS-01 | P0 | READY | BASE-02 | Ops | Isolated dev/test environment recipe, resource quota and generic deployment profile; no private config in public repo |
| OPS-02 | P0 | TODO | DB-02, DB-04, OPS-01 | Ops/QA | Dedicated staging, non-root runtime, least privilege, TLS, no public DB, readiness/metrics; backup + restore, DB outage and restart; verify RPO/RTO |
| CORE-01 | P0 | TODO | DB-02, DB-04 | Core | Versioned normalized telemetry envelope (fix/server/device times, units, quality, optional sensors), registry contract and idempotency semantics |
| PROTO-01 | P1 | TODO | CORE-01 | Protocol | Transport/codec SDK, session handling, bounded framing, ACK contract, safe command deny-default, fixture harness and simulator |
| PROTO-02 | P1 | TODO | PROTO-01 | Protocol/QA | OsmAnd HTTP compatibility, auth limitations documented; parity against Java and mobile app |
| PROTO-03 | P1 | TODO | PROTO-01 | Protocol/QA | GT06 frame/CRC/login/heartbeat/GPS/ACK and variants; fuzz, fragmentation and hardware test |
| PROTO-04 | P1 | TODO | PROTO-01 | Protocol/QA | Teltonika Codec 8/8E TCP/UDP handshake/AVL/IO/ACK; differential tests and hardware |
| PROTO-05 | P1 | TODO | PROTO-02, PROTO-03, PROTO-04 | Protocol | GPS103/TK103, H02, JT/T 808, Queclink, MQTT then inventory-driven backlog; registry statuses per model/firmware |
| AUTH-01 | P1 | TODO | DB-02, SEC-03 | Security/Core | User identity/OIDC, MFA policy, membership, tenant/role scopes, audit, session revoke; negative API tests |
| API-01 | P1 | TODO | AUTH-01, CORE-01 | Core | OpenAPI v2, asset/device/assignment lifecycle, latest/history, filter/pagination, versioning; contract and isolation tests |
| API-02 | P1 | TODO | API-01, DB-04 | Core | Realtime websocket and signed webhook delivery; scoped subscribe, backpressure, replay, audit |
| FLEET-01 | P1 | TODO | API-01, DB-03 | Core | Geofence, trips/stops, ignition/motion and alert rules; deterministic synthetic route tests |
| WEB-01 | P1 | TODO | API-01, API-02 | Web | Next.js/TypeScript map dashboard, vehicle list, history, tenant switching; API mock first, E2E and accessibility |
| WEB-02 | P1 | TODO | WEB-01, FLEET-01 | Web | Geofence editor, alerts/reports/export, localization, responsive access control |
| MOB-01 | P1 | TODO | API-01, API-02 | Mobile | Flutter Manager: authorized map/history/alerts, session revocation; Android/iOS tests |
| MOB-02 | P1 | TODO | CORE-01, PROTO-02 | Mobile | Flutter/native Client: opt-in permissions, visible background tracking, stop/pause, encrypted offline queue, retries and battery tests |
| DESK-01 | P2 | TODO | WEB-01 | Web/Mobile | PWA management first; only opt-in desktop location with accuracy, if product requires |
| HW-01 | P2 | TODO | PROTO-03, PROTO-04, API-01 | Hardware | Physical-device lab matrix, model/firmware, SIM/APN, QR one-time claim, bench test and provisioning profile; private evidence |
| DEAL-01 | P2 | TODO | AUTH-01, API-01, HW-01 | Product/Core | Dealer hierarchy, inventory, orders, device activation, warranty/RMA; delegated access only |
| BILL-01 | P2 | TODO | DEAL-01 | Product/Core | Entitlement/subscription lifecycle, idempotent payment integration, white-label branding; no card data storage |
| ENT-01 | P2 | TODO | FLEET-01, WEB-02 | Product/Core | Driver, dispatch, fuel/OBD, maintenance, cold chain, ERP/TMS integration; calibrated sensors and audit |
| COMP-01 | P2 | TODO | AUTH-01, HW-01 | Product/Security | Applicable privacy, transport and device standards review, retention and consent, evidence before certification claims |
| MIG-01 | P1 | TODO | DB-05, API-01, PROTO-02 | Core/QA | Java forwarding and compatibility facade, shadow replay/backfill, count/checksum/latest reconciliation, single ACK/command authority |
| PERF-01 | P1 | TODO | DB-05, PROTO-01, OPS-02 | QA/Ops | Baseline vs Java on same hardware, 1k/15s, burst/reconnect/outage, latency/RSS/disk metrics and loss accounting |
| REL-01 | P0 | TODO | SEC-01, SEC-02, SEC-03, OPS-02, PROTO-03, PROTO-04, WEB-02, MIG-01, PERF-01, COMP-01 | Coordinator/Ops | Release approval, signed artifacts/SBOM, restore, rollback, pilot acceptance and no open critical findings; no automatic production cutover |
| SCALE-01 | P2 | TODO | PERF-01, REL-01 | Core/Ops | Profile-guided gateway/worker/API split, HA and optional Redis/NATS; 10k/100k goals only after benchmarks |
| VISION-01 | P2 | TODO | REL-01 | Product/Core | Separate media service and compliant video lifecycle; signed access and retention |
| SDK-01 | P2 | TODO | API-01, PROTO-01 | Core/Product | Public SDK/docs and integration examples, license and security review |

## Next execution order
1. SEC-01 (private triage) and SEC-02 (maintainer approval/settings) first; SEC-03 may run in parallel without touching infrastructure.
2. DB-01 and OPS-01 may be developed in independent worktrees; coordinate schema and deployment contract.
3. DB-02 then DB-04; DB-03 after migration contract. QA independently verifies database privileges, durable acceptance, restart and restore.
4. Only then advance CORE-01 and PROTO-01. No hardware compatibility marketing until verified.

## Evidence policy
Record task ID, SHA, CI URL, test command/result, risk, and rollback. Do not mark DONE because code exists. Security scans are not proof of absence of vulnerabilities. Keep internal findings, hardware identifiers, network addresses and real data outside Git. Update TODO rather than creating duplicate issues.

## Coordination checkpoint
DB-01 DONE: integrated implementation 69090e0b0; exact-SHA [hosted CI #3 PASS](https://github.com/vietkeynet/ZalGPS-server/actions/runs/45992057716) at 5a9bc6f01, plus local PostgreSQL 16 race/integration, vet/build and scoped secret scan. DB-03 is now READY. SEC-03 threat model drafted and reviewed; current advisory checks and repeatable staged guard remain outstanding. SEC-01 private triage remains open; do not disclose individual findings.
