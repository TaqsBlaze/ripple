# Desktop development

## Architecture boundary

`rippled` owns identity, pairing, trusted-peer authorization, HTTPS transfer handling, and storage. Qt uses the user-only Unix-domain socket for settings, transfer approvals, and transfer status. The UI does not read identity keys or proxy file bytes.

IPC uses a four-byte big-endian length followed by UTF-8 JSON. Frames are limited to 1 MiB. The service checks `SO_PEERCRED` and accepts only the current user. The socket and its directory are user-only.

## Current integration

The service provides TLS 1.3 QR enrollment, signed peer status, authenticated Android-to-PC transfers, desktop approval or a persisted auto-accept policy, and verified received-file storage. Linux publishes `_ripple._tcp` with protocol version and an opaque identifier derived from its pinned identity. Android can use that identifier to locate a previously trusted peer at a new address, then verifies the saved TLS pin and signed status before updating its endpoint. Discovery does not authorize a new peer; new devices still require QR approval.

The desktop Transfers view exposes incoming offers, approval controls, progress, completed receipts, and opening completed files with the system's registered application. Linux-to-Android transfers are offered from a local file drop on the Ripple window right edge; Android polls the paired host, requires explicit acceptance, writes to the selected SAF folder, verifies the digest, and returns a receipt. Durable resume remains unavailable.

## Operational notes

- mDNS uses UDP port 5353 on the local network; the Ripple transfer/enrollment listener uses TCP port 43821.
- Network isolation, guest Wi-Fi, or multicast filtering can prevent discovery. QR/manual pairing remains available.
- Discovery data is not identity proof; Android authenticates all reconnections using the existing certificate pin and device signature.

## Verification still required

- Qt UI checks on supported Wayland and X11 sessions
- Android/Linux cancellation and address-change rediscovery on physical devices
- restart, force-stop, invalid fingerprint, unapproved peer, and changed-key scenarios
- physical-device tests for bidirectional transfer, SAF provider behavior, source changes, and receipts
