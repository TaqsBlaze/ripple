# Native capabilities

The Android `Ripple` Capacitor plugin keeps document URIs and permissions in native code. JavaScript receives opaque UUID handles, names, sizes (nullable when a provider does not report size), MIME types, and persisted-grant status. The plugin does not read file bodies into JavaScript and does not stage whole files.

Implemented locally:

- An in-app folder browser over user-authorized Android document trees. A system folder grant is requested once per root; after that, native queries return bounded 100-item pages and opaque file/folder identifiers. Up to 16 roots, 4096 cached browser nodes and offsets through 10000 keep metadata bounded. Only granted trees are visible. Android 11+ disallows some roots (including the Download root, storage root and Android/data or Android/obb); the Android picker enforces those limits. File selection from a tree is persisted against its root grant, so it restores after process restart without exposing a URI. Provider ordering is used for pagination; changing a folder while paging may require reloading it.
- Android Storage Access Framework multi-file selection (up to 32 per selection and 128 retained references), explicit cancellation, open/share via Android intents, and removal of a selection without deleting the original.
- Read grants are persisted only when the provider offers persistable access; otherwise a selection is temporary. Restored selections are checked against persisted read grants. A provider can revoke access or remove a file later; open/share checks report that error and request reselection.
- Receive folder selection with persisted read/write tree grants, replacement of the previous folder grant, and display-name metadata only across the bridge. This is a saved preference for future transfers; it does not claim a download has occurred or that the provider supports atomic rename/fsync.
- Private preferences for haptics and appearance; auto-accept remains off without a trusted peer.
- `_ripple._tcp` discovery using Android NSD, capped at 64 results. Discovery is stopped on activity pause/destroy and starts only from a user action; results are untrusted until enrollment is implemented. No service is advertised by Android.
- An installation identity generated with Android Keystore ECDSA P-256/SHA-256. Only a SHA-256 public-key fingerprint crosses the bridge. Backup is disabled. Uninstall destroys the private identity.
- QR scanning through the installed Capacitor barcode scanner, using ZXing to avoid a model-download dependency.
- Android hardware/predictive back is bridged to the UI navigation stack, with native exit fallback if there is no listener.
- Native WebView pinch/built-in zoom disabled; dark system bars and resize behavior for the keyboard.

Android now implements the enrollment client described in [protocol-v1.md](protocol-v1.md): it checks the QR expiry and shape, pins TLS to the scanned SPKI fingerprint, submits the Keystore public key, waits for explicit Linux approval, signs the returned challenge with the Keystore private key, and persists a peer only after a `paired` response. The Linux sibling service still needs the matching endpoints, explicit approval UI, and durable peer storage. Until that work lands, enrollment cannot complete end to end. Connect and transfer operations remain unavailable; this flow does not synthesize a peer or transfer receipt.

The plugin is not a transfer engine and does not run a foreground service. No ongoing transfer survives backgrounding because no transfer is started. Persisted file references/settings and the Keystore identity survive process death/reboot; temporary grants may not. File access after force-stop/reboot depends on the provider's persisted grants. Browser fallback reports an empty non-native state, rejects native actions and never stores file contents or keys.

## Invitation validation input (not an implemented wire protocol)

A JSON object no larger than 4096 UTF-8 bytes with these fields:

- `version`: integer `1`.
- `endpoint`: HTTPS URL with a host, optional valid port, root or empty path, and no credentials, query or fragment.
- `fingerprint`: exactly 64 hexadecimal characters representing SHA-256 of the server SPKI public key.
- `expiresAt`: RFC3339 instant strictly after the device time and no more than 120 seconds ahead.
- `sessionId`: canonical UUID string.
- `secret`: canonical unpadded base64url for exactly 32 bytes.

Validation creates no state and performs no network request. Final interoperable enrollment endpoints, proof-of-possession transitions, fixtures and protocol tests must be agreed in both repositories before enrollment is implemented. A valid shape is not proof that an invitation was issued by a real host.

## References checked during implementation

- [Android document access and persistable grants](https://developer.android.com/training/data-storage/shared/documents-files).
- [Android network service discovery](https://developer.android.com/develop/connectivity/wifi/use-nsd).
- [Local network permission](https://developer.android.com/privacy-and-security/local-network-permission): target SDK 36 retains normal local network behavior; API 37 targeting requires revisiting permission/picker behavior. Security exceptions and discovery failures are exposed for recovery.
- [Android Keystore](https://developer.android.com/privacy-and-security/keystore).

Hardware checks remain necessary for document-provider grants, denial/reselection, process restart, QR camera permissions, NSD networks/isolation, haptics, keyboard insets and actual Keystore support. Native unit tests cover invitation rejection and acceptance of shape only; they cannot verify trust or interoperability.
