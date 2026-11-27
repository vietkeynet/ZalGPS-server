# ZalGPS V2 — Architecture and product roadmap

## Baseline
Legacy source: 83d7f889c2aec641d94fc6a40c8398995481f4fb. Java 21, Netty, Jetty, Jersey, Guice, Liquibase. Tree audit found 269 *Protocol.java classes, 682 Java files in protocol and 268 *ProtocolDecoderTest.java classes (not verified hardware counts). The legacy server has configurable listeners, per-device queues, batch position writer, event handlers, geocoding, report and permission services. Preserve license and notices for ported code and fixtures. Verify web/mobile and third-party code licenses individually.

## Decision
Go modular monolith first (standard library networking, pgx, PostgreSQL/PostGIS, SQL migrations, OpenAPI, WebSocket). Measure against legacy using identical hardware/data. Rust only when profiling justifies a specialized edge or codec component; no mixed language requirement. Develop under v2/ on isolated branch; no master changes or production cutover.

## Core boundaries
Listener/transport TCP UDP HTTP MQTT -> bounded frame parser -> device enrollment/session and protocol codec -> normalized position/telemetry -> durable inbox + dedup -> per-device ordered processing -> position history/latest state + transactional outbox -> events, rules, webhooks, realtime API. Protocol registry declares model/firmware/transport, frame size, ACK and command capabilities, test status. ACK semantics are protocol-specific; acknowledge accepted records only after durable handoff when supported; handle legacy protocols that cannot retransmit. Track raw packets only with restricted retention and access; no default plaintext location logs.
Keep separate Tenant, User, Membership, Dealer, Asset, Device, DeviceAssignment (time-bound), SIM, Position, Telemetry, Event, Geofence, Trip, Driver, Maintenance, Command, Order, Inventory, Subscription, Audit and Consent domains. Tenant scope resolved from enrollment/membership; enforce in SQL and all APIs, cache, websocket, media, exports, jobs; cross-tenant tests required. PostGIS WGS84, UTC, explicit units, preserve legacy knots conversion at compat boundary. History time partitioning; latest state monotonic; dedup by protocol-aware event identity; bounded queues/backpressure, retry, quarantine and metrics.
Native HTTPS token/mTLS ingestion, per-device credential rotation; legacy IMEI-only protocols restricted to enrolled devices/network controls, not strong auth. Commands deny by default; no remote vehicle immobilization in MVP. Rate limits, audit, encrypted storage, privacy, account deletion and authorized tracking only. No stealth tracking. Android/iOS foreground disclosure and background permission; desktop only authorized location sources, indicate accuracy; BLE tags require authorized gateways. No claim of arbitrary tracker compatibility.

## Product surfaces
Core self-host; cloud fleet/asset; personal tracking; enterprise on-prem and API; Android/iOS tracker and manager; web admin (Next.js, TypeScript, MapLibre, charts); desktop PWA first; authorized desktop agent later; dealer portal (white-label, customer, subscription); provisioning utility for OEM model/firmware profiles; QR claim one-time secrets, SIM/APN configuration, bench test, assignment, activation, RMA/revocation. Video separate service with signed URLs; not in initial scope.

## Compatibility stages
1. Inventory legacy ports, device catalog, API, fixtures and unit conversions; replay fixtures against legacy; build Go durable inbox, registry and simulator. Native HTTPS, then OsmAnd compatibility.
2. GT06 variants, Teltonika Codec 8/8E, GPS103/TK103, H02; then Queclink, JT/T 808, Meitrack, Ruptela, Suntech, MQTT and vendor APIs. Select based on actual inventory. Separate TCP/UDP, frame/CRC/ACK, commands and IO support; unit, fuzz, fragmented/coalesced packets, replay and physical device matrix. No broad claims without evidence.
3. Legacy Java protocol sidecar/forwarding for unported protocols, preserve stable endpoints; golden fixtures and differential tests; use only one command authority and avoid double ACK. Legacy API compatibility facade and shadow replay, isolated database, backfill and reconcile counts/checksums/latest, staged per-device migration with rollback.
4. Fleet and mobile: auth, map, history, alarms, geofences, reports, maintenance; dealer billing and device lifecycle; enterprise integrations and compliance module.
5. Scale via separate gateway/worker/API processes from one binary; optional Redis and NATS JetStream only when measured demand warrants. No Kubernetes required. No cutover before restore tests and tenant isolation.

## Proposed engineering targets (not measured)
Pilot 1,000 connected trackers, 15 s interval (~67 records/s), 2 vCPU/4 GiB host including DB; core RSS <=256 MiB under specified load. Growth 10,000 trackers, 667 records/s; 100,000 trackers ~6,667/s (distributed sizing required). Test sustained traffic, 5x burst, reconnect storm, DB outage, duplicate/late packets, graceful shutdown, loss accounting and p95 latency. Target p95 ingest-to-durable <=500 ms at pilot load, with protocol ACK deadlines explicitly measured. Storage example 10,000 devices at 15 s and 500 bytes per record = 28.8 GB/day logical uncompressed before indexes/WAL/replica/backup. Define configurable retention/legal holds and verify disk usage empirically. No production promises until benchmarks.

## Research references
- Traccar: https://www.traccar.org/devices/ ; https://www.traccar.org/protocols/
- Wialon: https://wialon.com/ ; flespi: https://flespi.com/raw-data-ingestion ; Navixy: https://navixy.com/
- Samsara: https://www.samsara.com/products ; Geotab: https://www.geotab.com/geotab-marketplace/
- Vietnam: https://vietmap.vn/giai-phap-giam-sat ; https://www.vtracking.vn/
- Vietnam personal data law 91/2025/QH15 effective 2026-01-01, Decree 356/2025/ND-CP effective same date. Decree 319/2026/ND-CP effective 2027-07-01 with additional transition provisions. Review applicable current requirements and future technical standards with local compliance experts before claiming certification. No product certification implied.

## Delivery gates
P0: build/test/race/fuzz, durable DB/inbox and device auth, backup restore, legacy parity, no cross-tenant access. P1: management APIs and web/mobile, physical hardware pilot. P2: commercial inventory/subscription, enterprise integration and regulatory review. Do not create speculative PRs; one coherent change at a time. Current code is a volatile development slice only.
