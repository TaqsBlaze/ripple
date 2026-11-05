# Ripple protocol v1: enrollment slice

This file defines the shared Android/Linux enrollment and bidirectional transfer messages. Both peers must keep routes and JSON fields compatible. All remote traffic uses TLS 1.3; Android validates the server's SHA-256 SPKI fingerprint against the QR invitation before sending any request.

The QR is UTF-8 JSON, at most 4096 bytes:

```json
{"version":1,"endpoint":"https://192.168.1.20:43821","fingerprint":"<64 lowercase hex characters>","expiresAt":"<RFC3339 UTC instant, at most 120 seconds ahead>","sessionId":"<UUID>","secret":"<32 random bytes, unpadded base64url>"}
```

Linux advertises `_ripple._tcp` with DNS-SD. The SRV record carries the port, and the TXT record contains only `version=1` and an opaque `deviceId` derived from the pinned server identity. Android may use this ID to find a new address for an already trusted PC, but it saves that address only after the usual SPKI pin check and authenticated signed status request. Discovery names, IDs and addresses are never proof of identity. A newly discovered PC still requires QR enrollment.

Enrollment routes are `POST /v1/enrollment/start`, `/status`, and `/proof`. Requests and responses are UTF-8 JSON, capped at 64 KiB. Unknown versions fail with HTTP 400; invalid or expired invitations fail with HTTP 401; pending status is HTTP 200; rejected/consumed invitation attempts fail with HTTP 409. The enrollment listener exposes no peer, file, or AI operations.

`start` includes `version`, `sessionId`, `secret`, `deviceId`, `deviceName`, and `publicKey` (DER SubjectPublicKeyInfo encoded with standard base64). The secret is compared in constant time. The server binds the invitation to this public key and returns `{"enrollmentId":"<UUID>"}`. Repeated starts for the same session/device are idempotent; a different key cannot replace the first candidate.

`status` includes `sessionId` and `enrollmentId`. It returns `{"state":"pending"}`, `{"state":"rejected"}`, `{"state":"expired"}`, or `{"state":"approved","challenge":"<base64 32 random bytes>"}`. Approval requires an explicit local action on Linux. The challenge is single-use and expires after 60 seconds.

`proof` includes `sessionId`, `enrollmentId`, `deviceId`, and `signature`. The signature is SHA256withECDSA over the exact challenge bytes using the Android Keystore P-256 key matching `publicKey`. On successful verification, Linux consumes the invitation and returns `{"state":"paired","peerId":"<stable opaque id>","deviceName":"<display name>"}`. The peer pin and public identity are persisted before this response. A failed proof does not create a peer.

`GET /v1/peers/status` is an authenticated post-pairing availability check. It has an empty body and the standard signed-request headers described below. Linux verifies the request against the enrolled device public key and returns `{"version":1,"state":"online","peerId":"<stable opaque id>"}`. An untrusted device receives HTTP 401. Android marks the PC online only after validating the TLS SPKI pin and this signed response; this status check does not imply that transfer routes are available.

## Transfer API draft

| Route | Purpose |
| --- | --- |
| `GET /v1/transfers/inbox` | Android polls peer-bound desktop offers |
| `POST /v1/transfers/{id}/decision` | Android accepts or rejects an offer |
| `GET /v1/transfers/{id}/content?offset=N` | Android downloads one bounded chunk |
| `POST /v1/transfers/{id}/receipt` | Android reports a verified, saved file |
| `POST /v1/transfers/{id}/cancel` | Cancel an authorized transfer |


Each trusted HTTPS request uses the paired endpoint and exact SPKI pin. Requests include `X-Ripple-Device-ID` and `X-Ripple-Signature`; the signature is Android Keystore SHA256withECDSA over UTF-8 `METHOD + "\n" + PATH_AND_QUERY + "\n" + lowercase-hex(SHA256(body))`. Empty bodies hash as SHA-256 of zero bytes. Linux checks the signature against the public key saved at enrollment. Maximum JSON body is 64 KiB. File bytes never cross JavaScript.

`POST /v1/transfers` offers `{version, transferId, name, size, mimeType, sha256}` and returns the current state. The desktop receiver asks for approval by default; a local desktop setting can auto-accept files from trusted Android peers. `GET /v1/transfers/{id}` returns `{state, offset, size, sha256}` for the authenticated peer. The desktop user accepts or rejects pending offers through the user-only IPC channel.

After acceptance, Android sends at most 1 MiB per `PUT /v1/transfers/{id}/chunks/{offset}`. The receiver accepts only the next contiguous offset, returns `{offset}`, and treats an exact retry of the immediately previous chunk as idempotent. Gaps and byte mismatches fail. `POST /v1/transfers/{id}/complete` with `{size,sha256}` asks the receiver to verify length and digest, finalize a collision-safe destination, persist a receipt, then return `{state:"completed",receiptId,sha256}`. Repeating completion returns the existing receipt.

Android computes the source digest in a bounded streaming pass before offering, then reopens and revalidates the source while uploading. A changed or unavailable source fails the transfer. `POST /v1/transfers/{id}/cancel` cancels an authorized transfer and removes its temporary partial file; a completed receipt cannot be cancelled. Durable resume is not enabled until the server can verify retained prefixes. For Linux-to-Android transfers, the desktop creates an authenticated peer-bound outgoing offer after the user drops a local file on the Ripple window right edge. `GET /v1/transfers/inbox` lets the paired Android poll offers; `POST /v1/transfers/{id}/decision` records the Android user's accept or reject choice. After acceptance, Android pulls sequential binary chunks from `GET /v1/transfers/{id}/content?offset=N`, writes them to a temporary document in the user-selected SAF receive folder, verifies SHA-256, finalizes the document, then posts `POST /v1/transfers/{id}/receipt`. The desktop marks Delivered only after validating that receipt and rechecking the unchanged source. The transfer is a copy and never removes the desktop original. Android requires explicit acceptance for each offer. Transfers larger than 1 MiB remain bounded to one chunk in memory; the current size cap is 10 GiB. Durable resume is not enabled yet.


Android may send a signed `POST /v1/drop-state` body `{"active":true}` while a dragged file is over the in-app drop target and `{"active":false}` when it leaves or is released. This authenticated signal contains no file metadata or bytes. Linux holds it for at most 1.5 seconds and the desktop may show a brief right-edge ripple; it is visual feedback only and does not accept or deliver a transfer. Android refreshes the active state every 500 ms while hovering.
