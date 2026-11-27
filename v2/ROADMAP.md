# ZalGPS V2 — master product and engineering roadmap

Planning baseline: 2026-11-27. Go modular monolith, native binary and PostgreSQL/PostGIS. Java remains the reference and fallback until protocol/API parity and migration gates pass. Rust is optional only with measured need. The TODO is the execution queue; milestones below define the product boundary and exit gates. No calendar estimates or capacity promises until measured.

## Product strategy
One platform, separate Device, Asset, time-bound Assignment, Tenant, User/Membership, Dealer, SIM and Subscription. Shared data and authorization; separate user surfaces. Initial customers: owners of vehicles/assets and small fleets, then logistics, construction, cold chain, organizations and device resellers. Self-host/community, SaaS personal/fleet, white-label and enterprise on-prem/API. OEM provisioning and recurring service are separate from tracking credentials. Commercial implementations, real customer records and private infrastructure remain private. Review Apache-2.0 notices and any third-party app/codec licenses; no relicense assumptions.

| Milestone | Scope | Exit gate | Status |
|---|---|---|---|
| M0 | Legacy audit, Go pilot, planning and public repo safety | baseline CI, threat model, security controls verified | IN_PROGRESS |
| M1 | Production-grade data foundation: versioned migrations, DB roles/RLS, PostGIS, durable inbox/outbox, retention, backup | isolation, crash/retry/restore and migrations proven | TODO |
| M2 | Protocol SDK and simulator; native HTTP, OsmAnd, GT06, Teltonika | differential fixtures, ACK/framing/fuzz and real hardware pilot | TODO |
| M3 | Management API and identity | OpenAPI, OIDC, RBAC, tenant/asset/device assignment, audit | TODO |
| M4 | Fleet web MVP | realtime map, history, geofence, alerts, reports, accessibility, E2E | TODO |
| M5 | Mobile and desktop | Manager, consent-based Client, offline sync, PWA; desktop agent only if needed | TODO |
| M6 | Hardware lifecycle and dealer portal | inventory, one-time claim, SIM/APN, bench test, warranty, white-label | TODO |
| M7 | Fleet/enterprise operations | drivers, jobs, trips, fuel, maintenance, ERP/TMS/WMS integrations, tenancy | TODO |
| M8 | Migration, pilot operations and release | shadow comparison, rollback, signed releases, SLO, restore, approvals | TODO |
| M9 | Scale and extension | profile-driven split gateway/worker/API, optional messaging, SDK marketplace, video separate | TODO |

## Product acceptance by surface
| Surface | MVP | Later |
|---|---|---|
| Core | authenticated ingest, latest/history, events, geofence, audit | scalable workers, optional message bus |
| Fleet | map, vehicle list, trips/stops, reports, alert rules | dispatch, route/ETA, driver score, fuel, maintenance |
| Assets | device assignment, battery/last seen, geofence | sensor rules, container and cold chain |
| Manager | authorized map, alerts, history | fleet actions, reports, delegated roles |
| Client | explicit opt-in location, visible tracking, stop/pause, encrypted local queue | battery profiles, remote config with consent |
| Desktop | PWA management | opt-in agent, GNSS source/accuracy |
| Partner | dealer/customer hierarchy, stock, activation, entitlement | white-label, billing, RMA and renewals |
| Configurator | verified model profiles, USB/vendor API where authorized, QR claim | batch provisioning and signed updates |
| Enterprise | on-prem deployment and API | OIDC/SSO, ERP/TMS/WMS, SLA and custom policies |
| Vision | out of MVP | separate media pipeline, consent, signed URLs, retention |

## Priorities and dependencies
M0 security gates -> M1 DB/identity/durability -> M2 codecs and simulator; M3 management API may overlap only after shared schema/identity contracts freeze. Web/mobile consume the versioned API, not tables. Dealer/billing depends on authorization and device lifecycle; no feature flag may grant cross-tenant data. M8 begins with synthetic staging after M1 and expands to real pilot only after M2–M4 gates. Production requires explicit approval. M9 scales only after profiling. Market/legal requirements are validated for actual device and jurisdiction before sale; compatibility is not certification.

## Non-goals for MVP
No all-model compatibility claim, no remote immobilization, no surveillance without consent, no video ingest in core, no Kubernetes requirement, no AI in critical ACK path, no production cutover or private topology in public repo.
