# AGENTS.md — Ripple Android

## Mission and priorities

Build the Android companion for Ripple using React, TypeScript, Tailwind CSS, Capacitor and Kotlin. First make encrypted file transfers reliable, then deliver useful LLM assistance through the paired Linux host. Keep this file at the root of the Android repository.

## Required architecture

- React + TypeScript own screens, view state and in-app drag feedback. Compile Tailwind through the build pipeline; bundle all app assets for offline use. Do not use runtime CDN dependencies.
- Capacitor bridges small typed commands and events to Kotlin. Kotlin owns discovery, pairing, TLS, file access, hashing, transfer streaming and lifecycle management.
- File selection uses Ripple’s in-app file manager. Use the Android system folder picker only to obtain a scoped Storage Access Framework tree grant, then browse files/folders in Ripple through Kotlin-owned queries and opaque handles. Persist grants only where supported and needed. Keep content URIs in native code; do not assume every URI maps to a filesystem path. Use app-private staging when necessary and account for its storage cost. Long-press dragging into the in-app drop area may show a rainbow ripple as selection feedback; it must not imply successful delivery.
- Store identity private keys in Android Keystore. Store approved public identities and resumable transfer metadata in app-private persistent storage. Never put keys or file contents in browser localStorage.
- Use the currently appropriate foreground/background execution mechanism for active user transfers, with visible progress and cancellation. Verify target-SDK permission and notification requirements. Survive WebView recreation; document behavior after process death, force-stop and reboot rather than promising unrestricted background execution.
- Use Android NSD for discovery; implement QR scanning with a maintained compatible component. Request permissions in context, explain denial and provide recovery.
- No direct cloud AI credentials in the Android MVP: the paired Linux host owns adapters and secrets. Display host unavailable when it cannot be reached.

Suggested layout (adapt to the existing repository):

```text
src/features/{devices,pairing,transfers,files,assistant,settings}/
src/native/ripple.ts
src/components/
android/app/src/main/java/<package>/ripple/{discovery,pairing,security,transfer,storage}/
docs/protocol-v1.md
tests/fixtures/protocol-v1/
```

Native bridge commands should include discovery start/stop, pairFromQr, listPeers, connectPeer, forgetPeer, pickFiles, offerTransfer, acceptTransfer, rejectTransfer, cancelTransfer, resumeTransfer, queryTransfer, submitAiRequest, approveAiAction and cancelAiRequest. Keep commands asynchronous and events typed. Pass opaque native file handles rather than base64 bytes. Throttle progress events to avoid saturating the WebView.

## Screens to implement

1. **Devices/Home:** connected PC, status, Send files, Ask a file, active transfers and recent receipts.
2. **Pair device:** same-network guidance, QR scan, verification, PC approval waiting state and failure recovery.
3. **Transfer activity:** direction, peer, file, progress, speed, pause/reconnect, cancellation and verified receipt.
4. **Files/Received:** received items, open/share actions, selected-file AI action and safe destination controls.
5. **Assistant:** streamed responses, selected attachments, cited sources, search result cards and explicit Send confirmation.
6. **Settings:** trusted peers, forget device, receive policy, storage destination, appearance and Linux-managed AI provider status.

Use tabs for Devices, Transfers and Assistant; secondary screens can be sheets or routes. Support the Android share sheet as a later v1 enhancement after in-app browser transfers work. In-app edge dragging is optional polish; do not claim global dragging from arbitrary Android apps.

## Delivery order and acceptance

- M1: offline app shell, native bridge, persistent identity and pairing against Linux; reject wrong fingerprints and unapproved peers.
- M2: real bidirectional single-file transfers with verification and receipts. Both in-app browser sending and receiving/opening work on physical hardware.
- M3: persistent activity, cancellation, network-loss recovery, duplicate-safe resume, storage failures and lifecycle handling.
- M4: Ask this file through Linux, provider availability, streamed/cancellable replies, cloud consent and source references.
- M5: Find and send with approved Linux search roots, candidate selection, exact action confirmation and normal delivery receipts.
- M6: accessibility, icon integration, multi-file queue and optional in-app edge animation. Screen sharing stays deferred.

## Android verification

Run repository-defined TypeScript checking, lint, unit tests and production web build; sync Capacitor and run Kotlin/Gradle checks and a debug build. Establish scripts if missing and document their exact usage. Never claim APK success without a completed build. Test real devices for URI grants, large files, backgrounding, permission denial, rotation, network switching and process restart. Verify memory remains bounded as file size grows and file bytes never cross the JS bridge.


## Shared product and interoperability contract

These rules apply equally to Ripple Android and Ripple Linux. Keep a versioned copy of the wire contract and identical JSON fixtures in both repositories. Changes to messages, trust, transfer state, or AI tools require compatible updates and cross-platform tests. This document proposes the v1 design; it does not claim the implementation already exists.

### Scope

Release 1 delivers local bidirectional file copying plus optional LLM assistance: Ask this file and Find and send. Support LAN Wi-Fi, Ethernet on the PC, and a user-established hotspot. Internet is unnecessary for transfers and entirely local inference; cloud models require internet. No account or cloud relay is required.

Defer screen sharing, remote input, true extended displays, Bluetooth, Wi-Fi Direct automation, unattended destructive file actions, and multi-device routing. Implement an ordinary reliable file picker/drop zone before optional desktop edge interactions. Do not make AI a dependency of pairing or transfers.

### Connection and trust

- Linux is the v1 TLS server; Android initiates the connection. Both peers can send and receive files over this relationship. Linux-to-Android transfers must work without exposing an Android listening port.
- Discover `_ripple._tcp` through mDNS/DNS-SD. Announce only protocol version, port and an opaque device identifier; do not advertise file lists or secrets. Discovery names and IP addresses are not proof of identity.
- Provide QR/manual connection details if discovery is blocked. Explain network isolation/firewall failures accurately; never disable a firewall automatically.
- Require TLS 1.3. Use maintained platform libraries and negotiated AEAD cipher suites; never invent encryption or disable certificate validation to make pairing work.
- Each installation has a persistent device identity key and certificate. Use an interoperable signing algorithm supported by Android Keystore and Go, initially ECDSA P-256, and test actual device support.
- Pairing QR contains schema version, reachable endpoint, SHA-256 fingerprint of the server public key, expiry, session ID and a cryptographically random 256-bit single-use secret. Never include private keys. Expire invitations after two minutes and rate-limit attempts.
- Use a restricted enrollment connection: Android validates the scanned fingerprint before submitting its secret and device public identity. Linux requires explicit local approval and binds that invitation to the presented identity. Android must prove possession of its private key in the subsequent mutually authenticated connection before the enrollment becomes usable.
- Enrollment connections cannot list files, transfer data or execute AI tools. Successful enrollment consumes the invitation. Persist peer public-key pins and an allowlist; subsequent connections require mutual TLS and exact peer authorization, not merely any self-signed certificate.
- A changed identity requires explicit re-pairing. Forget device removes authorization, closes active sessions and invalidates that peer's resumable authentication state. Do not trust a replacement key solely because an address or device name matches.

### Proposed v1 transport

Use HTTPS for bounded metadata and binary streaming and an authenticated WebSocket for control/events. Kotlin owns Android networking and Go owns Linux networking. Qt and React never proxy file bytes.

| Route | Purpose |
| --- | --- |
| `GET /v1/events` | Authenticated WebSocket for offers, decisions, progress and terminal events |
| `POST /v1/transfers` | Android offers an upload to Linux |
| `GET /v1/transfers/{id}` | Query authorized transfer status and durable offset |
| `POST /v1/transfers/{id}/decision` | Android accepts or rejects a Linux offer |
| `PUT /v1/transfers/{id}/chunks/{offset}` | Upload bounded binary chunk from Android |
| `GET /v1/transfers/{id}/content?offset=N` | Android pulls an accepted Linux file from a verified offset |
| `POST /v1/transfers/{id}/receipt` | Android reports a verified, saved download |
| `POST /v1/transfers/{id}/cancel` | Cancel an authorized transfer |
| `POST /v1/ai/requests` | Submit a bounded AI request to the Linux host |
| `POST /v1/ai/requests/{id}/cancel` | Cancel an AI request |

Enrollment endpoints are separate and accessible only while pairing mode is active. Specify their exact schemas and proof/approval transitions in `docs/protocol-v1.md` before implementing them. Specify response codes, range rules, upload completion, WebSocket reconnect behavior and maximum sizes there as well; this table is the starting interface, not a complete protocol specification.

Use typed envelopes containing `version`, `type`, `requestId`, `transferId` where relevant, and `payload`. Unknown major versions fail clearly. Events include `transfer.offer`, `transfer.accepted`, `transfer.rejected`, `transfer.progress`, `transfer.paused`, `transfer.received`, `transfer.failed`, `transfer.cancelled`, `ai.delta`, `ai.completed`, `ai.failed`, and `ai.action_proposed`. Reconnect queries persistent status; events alone are not durable truth.

### Transfer correctness

1. Sender offers a UUID transfer ID, display filename, byte length, MIME hint, and SHA-256 digest. For v1, hash through native streaming before offering and show a Preparing state for large files.
2. Receiver checks authorization, destination permission and available capacity, and applies its acceptance policy. Default to asking; allow an explicit per-peer auto-accept setting.
3. Stream with bounded buffers into temporary storage. Start with a documented 1 MiB chunk size; use backpressure and cancellation. Never load a whole file into memory.
4. Track a durable contiguous byte offset. A retry at an already accepted offset must be idempotent. Reject gaps and mismatched content. Resume only after authenticating the same peer, validating the source identity/size/digest and verifying the retained prefix or chunk hashes.
5. Receiver verifies final length and digest, closes and flushes storage, then finalizes the destination. Use atomic rename where supported; otherwise implement and document safe provider-specific finalization. Do not claim arbitrary Android document providers offer atomic rename or fsync semantics.
6. Persist a completed receipt before emitting `transfer.received`. Sender shows Delivered only after this receipt or an equivalent status query. Duplicate completion requests return the existing receipt, without creating another file.

State machine: `offered -> accepted -> transferring -> verifying -> completed`, with `rejected`, `paused`, `failed`, `cancelled` as explicit states. A paused transfer can resume; terminal completion is immutable. Distinguish local bytes-sent progress from remote delivery.

Never trust filenames as paths. Prevent traversal, symlink escapes and overwrites. Choose a collision-safe name or ask. Bind transfer IDs and all endpoints to the authorized peer. Enforce byte counts, quotas, timeouts and bounded concurrency. Preserve originals: transfers are copies, not moves. Do not silently delete source files. Handle zero-byte files, Unicode names, low storage, source modification, process restarts and disconnects. Exclude directory recursion and archive extraction from the initial release.

### LLM design

Linux hosts the first AI orchestration service. Android sends requests through the paired connection and displays results. Offer local endpoints, Gemini API, OpenAI API and explicitly configured compatible endpoints through adapters. Do not imply a ChatGPT subscription includes API access. Verify current official provider documentation before choosing SDK versions, authentication, models or tool-call formats; do not hardcode invented model IDs.

- Providers expose capabilities, health, streaming, cancellation and bounded tool calling. File transfers still work with no provider configured or an unavailable provider.
- Keep API credentials on Linux in the desktop secret store; never send credentials to Android, the UI logs, prompts or analytics. Local endpoints should normally bind to loopback and remain behind the Ripple service.
- Default to local processing when chosen. Never silently fall back from local to cloud. Before a cloud request, show exactly which content will leave the paired devices and obtain approval.
- Ask this file: explicit selection -> bounded extraction -> retrieval of relevant passages if needed -> model response with file/page or line references. Start with UTF-8 text, Markdown and text-based PDFs. Clearly report unsupported, encrypted, scanned or oversized content. OCR and images are later capability-gated additions.
- Find and send: search user-approved Linux folders using deterministic filename/metadata search first; let the LLM interpret the request and present candidates. Show filename, location, destination and size; obtain explicit transfer confirmation. Use the normal transfer engine and receipt flow.
- AI can access only explicitly granted roots and selected attachments. Indexing is opt-in, cancellable and local by default. Scope cached extracts and search indexes to the user's permissions and remove them when access is revoked.
- Model output is untrusted. Use typed allowlisted tools such as `search_files`, `read_selected_file`, `propose_transfer` and `get_transfer_status`. Validate argument schemas and canonical paths outside the model. No arbitrary shell execution, unrestricted filesystem access or arbitrary URL fetching.
- Document text is data, never an instruction to call tools or bypass consent. A model proposal cannot approve itself. Bind user approval to exact files, peer and operation; revalidate if anything changes.
- Separate AI consent from trusted-peer auto-accept settings. Rate-limit requests, bound token/context/file sizes and tool iterations, support cancellation, and expose useful provider errors without secrets.

### Visual direction

Use the approved Ripple identity: three open concentric white circular rings on black, with aligned openings on the right and rounded ends. Use the symbol alone for the app icon and the lowercase wordmark where appropriate. Use an actual supplied asset; do not substitute a cryptocurrency logo or invent a new mark. If no asset is in the repository, use a temporary labeled placeholder and document the missing asset.

Create a clean black/charcoal interface, white primary text, restrained accents, clear progress, accessible contrast, keyboard/screen-reader labels and reduced-motion support. A subtle ripple can acknowledge a successful transfer; never use animation to imply delivery before confirmation. AI connection settings should clearly label Local and Cloud. Do not present screen sharing as available in v1.

### Development discipline

Inspect the repository and its existing instructions before editing. Pin compatible dependencies and record reproducible setup. Verify current official Android, Capacitor, Qt, Go and provider requirements during implementation; this plan intentionally does not pin unverified versions. Preserve user changes, keep secrets out of source control, and update protocol docs and fixtures alongside code. Do not introduce cloud infrastructure or paid dependencies without a concrete requirement.

Never report mocked behavior as working integration. Record commands actually run, results, limitations and remaining hardware checks. Include tests for trust failures, unauthorized transfer IDs, traversal, duplicate messages, interrupted/resumed transfers, source changes, checksum failure, lost receipts, provider failure, prompt injection and confirmation bypass. A release needs a real Android-to-Linux and Linux-to-Android interoperability run, not just mocked unit tests.
