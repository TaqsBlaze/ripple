# AGENTS.md — Ripple Linux

## Mission and priorities

Build Ripple's Linux companion with a Go service and Qt/QML desktop interface. Release 1 focuses on secure bidirectional file transfers and LLM integration. Keep this file at the root of the Linux repository.

## Required architecture

- Go owns network discovery, TLS, pairing, persistent trust, file transfer state, receipts, search, extraction orchestration and AI provider adapters.
- Qt/QML owns the desktop UI, accessible controls, file drops, pairing approval, provider setup and transfer presentation. Use a small C++ Qt bridge for native integration and IPC; keep business logic in Go. Avoid depending on an unmaintained Go-to-QML binding.
- Run the service as the logged-in user, never root. Use a Unix-domain socket beneath the user's runtime directory for UI-service IPC. Restrict directory/socket permissions to that user, verify peer credentials and use framed bounded messages. Do not expose a UI control API on unauthenticated TCP.
- Separate the user-only local IPC interface from the Android-facing TLS interface. Remote peers cannot change provider credentials, approved search roots or local administrative settings.
- Persist transfers, receipts and trust metadata in a transactional local store. Use platform-appropriate config/data/cache locations. Keep identity material access-restricted; store provider secrets through the desktop Secret Service where available. If unavailable, offer session-only credentials rather than silently saving plaintext secrets.
- Advertise `_ripple._tcp` with a maintained mDNS implementation compatible with Android NSD. Document the listening interface/port and give scoped firewall setup instructions if needed.
- Use a bounded worker pool for file preparation, hashing, transfers and AI work. User-driven transfers take priority over background indexing.

Suggested layout (adapt to existing conventions):

```text
cmd/rippled/
internal/{discovery,pairing,security,protocol,transfer,storage,ipc}/
internal/ai/{providers,tools,extract,retrieval,policy}/
desktop/{qml,src,assets}/
docs/{protocol-v1.md,development.md,security.md}/
tests/fixtures/protocol-v1/
```

Define provider interfaces for health, model capabilities, streamed responses, cancellation and tool calls. Implement local endpoint, OpenAI API and Gemini API adapters behind the same orchestration layer. Validate endpoint configuration and keep provider-specific payloads outside the UI. Use HTTPS for remote endpoints; permit HTTP only for explicitly configured loopback/local inference with a clear setting. Do not expose an unauthenticated model endpoint to the LAN merely to support Android.

## Desktop screens and behavior

1. **Overview:** paired Android devices, connection state, Send files, visible drop zone and current transfers.
2. **Pairing:** expiring QR invitation, candidate device identity, approve/reject and regenerate invitation.
3. **Transfers:** persistent queue/history with preparation, byte progress, verification, receipts, resume and failure reasons.
4. **Assistant:** local file selection, Ask this file, Linux file search, cited answers and send-to-phone confirmation.
5. **AI Connections:** provider, endpoint, model choice, test connection, secret entry, Local/Cloud labels and limits.
6. **Settings:** approved search folders, receive folder, trusted peers, acceptance policies, startup and network preferences.

Start with ordinary Qt file drag-and-drop into a visible window. Investigate screen-edge receiving areas only after the transfer engine works. Detect desktop/session capabilities and verify Wayland and X11 behavior using current official APIs. Do not promise a universally available global edge overlay; retain the visible drop zone when compositor integration is unavailable. Never require root or disable compositor security for this effect.

## AI implementation details

- Start search with allowed-root filename, extension, timestamp and size filters. Introduce semantic indexing only after the core flow is reliable and explicitly enabled.
- Canonicalize paths, reject symlink escapes and recheck permissions when opening files. File search responses should use opaque handles bound to the user/session, not grant arbitrary filesystem paths to remote callers.
- Extract selected files using maintained parsers with time, memory and page/byte limits. Prefer constrained subprocesses for untrusted document parsing. Do not execute embedded macros or follow document links automatically.
- Chunk long documents with source coordinates. Build answers from bounded retrieved passages and retain references; do not fabricate citations for unsupported claims.
- For an Android attachment, receive it through the verified transfer/staging path first, then give the AI job an authorized file handle. Define retention and provide deletion of temporary AI attachments and cached extracts.
- For Find and send, let the model create a structured proposal. A policy layer and explicit UI confirmation approve the exact source, destination peer and copy operation. Reuse the transfer engine, rather than letting a model send bytes.
- Limit cloud exposure to approved extracts or explicitly approved full attachments. Do not send the full search index, unrelated folder names or other files by default.

## Delivery order and acceptance

- M1: Go service, Qt shell, private IPC, persistent identity, QR enrollment and Android mutual TLS interoperability.
- M2: bidirectional single-file transfers through picker/drop zone; hash verification, safe finalization and persistent receipts.
- M3: bounded transfer queue, cancellation, resume, duplicate-safe status recovery and failures across service restarts.
- M4: AI Connections, local model adapter, then cloud adapters; Ask this file with consent, citations, cancellation and provider errors.
- M5: approved-folder search and Find and send with confirmation and normal verified delivery to Android.
- M6: packaging, accessibility, startup opt-in and capability-gated desktop edge integration. Screen sharing remains a later project phase.

## Linux verification and packaging

Run Go formatting, `go vet ./...`, `go test ./...` and appropriate race tests for concurrent transfer/AI code. Build the Qt desktop with its declared CMake/Qt toolchain and run relevant UI/IPC checks. Document exact commands and install prerequisites after confirming available versions. Test both a supported Wayland and X11 desktop before advertising support. Package the desktop and user service together with reproducible installation/uninstallation and opt-in autostart; do not install system-wide privileged services unnecessarily.

Integration checks must include real Android transfers in both directions, multi-gigabyte bounded-memory streaming, hash failures, full disk, duplicate filenames, permission loss, network interruption, service restart, invalid pins, unapproved clients, receipt recovery and malicious filenames. AI checks must cover offline local operation, cloud denial, invalid credentials, context limits, unsupported files, source citations, prompt injection and attempts to bypass user action approval.


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
