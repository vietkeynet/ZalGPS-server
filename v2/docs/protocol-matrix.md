# Protocol compatibility roadmap

Source of reference: Traccar legacy commit 83d7f889c2aec641d94fc6a40c8398995481f4fb; 269 protocol registration classes and 268 decoder test classes from source inventory, not verified device counts. See [device list](https://www.traccar.org/devices/) and [protocol documentation](https://www.traccar.org/protocols/). Verify license and attribution before porting fixtures/code.

| Protocol family | Target transport | Priority | V2 state | Acceptance focus |
|---|---|---|---|---|
| Native ZalGPS JSON | HTTP; HTTPS at deployment | P0 | TESTED (pilot only) | authenticated scoped ingest, durable commit, duplicate, late |
| OsmAnd | HTTP | P1 | PLANNED | mobile params/time/units, auth compatibility |
| GT06 | TCP, per variant | P1 | LEGACY_REFERENCE | login, frame/CRC, heartbeat, GPS, ACK |
| Teltonika 8/8E | TCP/UDP | P1 | LEGACY_REFERENCE | IMEI/session, AVL/IO, batch ACK |
| GPS103/TK103 | per variant | P1 | LEGACY_REFERENCE | text framing, time, status |
| H02 | per variant | P1 | LEGACY_REFERENCE | framing, ID, GPS, status |
| JT/T 808 | per variant | P1 | LEGACY_REFERENCE | message framing/escape, checksum, segmentation |
| Queclink | per variant | P1 | LEGACY_REFERENCE | model-specific text/binary, ACK |
| MQTT | TLS preferred | P1 | PLANNED | topic/device authorization and QoS |
| Meitrack, Ruptela, Suntech | per variant | P2 | LEGACY_REFERENCE | inventory-driven migration |
| Vendor API/Wialon IPS | per contract | P2 | PLANNED | scopes, rate limits, normalized telemetry |
| JT/T 1078/video | separate service | P2 | OUT_OF_MVP | media isolation and authorized access |

LEGACY_REFERENCE = Java source exists (not a verified running deployment); PORTED = Go code exists; TESTED = automated checks for specified scope; HARDWARE_VERIFIED = physical model+firmware+transport tested; CERTIFIED = applicable documented approval. A protocol family is not a hardware certification. No real IMEI/SIM/packet traces in public matrix. Maintain private serial/model test inventory and sanitized synthetic fixtures. Commands deny by default; no remote vehicle immobilization in MVP.

## Registry schema to implement (PROTO-01)
Protocol/version, transport, profile ID, model/firmware range, max frame, login/session, timestamp/units, ACK deadlines and durability policy, command capabilities, codec version, supported attributes, fixture IDs, test status, hardware evidence reference, migration/fallback state. Validate at startup; allowlist enabled listeners. Never auto-detect arbitrary binary protocols without bounds. Preserve separate legacy endpoint for unported devices.
