# Ripple Android

A mobile-first React, TypeScript and compiled Tailwind interface in a Capacitor Android application. Devices, Transfers and Assistant use sliding bottom-tab transitions. Active and History use an animated sliding selector and content track. Reduced-motion preferences are respected. Zoom is disabled by the viewport configuration and native WebView settings.

## Current functionality

- In-app file manager: grant access to a folder once through Android, then browse folders/files in Ripple’s grid with search and bounded pagination.
- Long-press a file to lift it, drag into the drop area for a rainbow ripple, and release to select it. Tap a file and use **Add selected file** as an alternative. Neither feedback nor selection claims delivery.
- Native open/share actions and removal of selected-file references without deleting originals.
- Native receive-folder picker and persistent app-private settings.
- Android NSD discovery of `_ripple._tcp` services. Nearby devices are untrusted until pairing is implemented.
- Persistent ECDSA P-256 identity in Android Keystore; only its public-key fingerprint crosses the bridge.
- QR scanning and bounded invitation validation; no peer authorization is granted by validation alone.
- Real empty states for transfer activity and received files; host-unavailable state for the assistant. No sample files, replies, connection success or progress are generated.

The sibling Linux repository currently contains only `AGENTS.md`, with no host service or enrollment API. Secure pairing, authenticated connections, encrypted transfers, verified receipts and host-mediated AI are **not implemented**. Their commands reject unavailable requests. This build must not be described as a working Android/Linux transfer integration.

Android restricts which folders can be granted (including storage roots and the root Downloads folder on newer versions). Use a permitted subfolder; Ripple does not request unrestricted storage access. File content and content URIs remain in Kotlin. React receives only opaque IDs and bounded metadata. Selected files are not received files. Removing a selection releases its retained grant where applicable and preserves the original.

## Build

Use Node 22.18+ (Node 24 recommended), Bun and the checked-in `bun.lock`:

```sh
bun install --frozen-lockfile
npm run check
npm run lint
npm test
npm run build
npm run android:sync
npm run android:check
npm run android:build
```

Android requires JDK 21 and a configured Android SDK. Compile/target SDK: 36. Minimum SDK: 26, required by the installed barcode scanner. Configure the SDK through `ANDROID_HOME` or a local `android/local.properties`. See [Capacitor environment requirements](https://capacitorjs.com/docs/getting-started/environment-setup).

Debug APK: `android/app/build/outputs/apk/debug/app-debug.apk`. Production web bundle: `dist/`. Run `android:sync` after each web build and before Gradle. Do not sync while Gradle is running.

## Testing

`npm test` checks that the web fallback cannot fabricate successful native operations or trust state. Kotlin invitation tests run through `npm run android:check`.

Additional DOM and browser checks use separately installed test tooling:

```sh
RIPPLE_JSDOM=/absolute/path/to/jsdom/lib/api.js npm run test:ui
RIPPLE_JSDOM=/absolute/path/to/jsdom/lib/api.js RIPPLE_NATIVE_FIXTURE=1 npm run test:ui
npm run dev
RIPPLE_PLAYWRIGHT=/absolute/path/to/playwright/index.mjs \
RIPPLE_CHROMIUM=/absolute/path/to/chromium npm run test:browser
```

The DOM test evaluates the production bundle and checks interaction/state. The native-fixture variant tests folder navigation, scrolling versus long-press, drag cancellation, drop selection, ripple presence and the tap alternative against a test bridge. Its fixture file is test data only and is never bundled in the app. Tested tooling: jsdom 26.1.0 and Playwright 1.56.1, installable outside the app with `npm install --prefix /tmp/ripple-test-tools jsdom@26.1.0 playwright@1.56.1`. It does not verify layout, animation smoothness or Android native code. Browser tests require a browser-capable environment. Native picker grants, QR permissions, NSD discovery, open/share intents, lifecycle and physical-device behavior require Android hardware verification.

Persistent file grants and settings survive WebView recreation/process restart when supported by the provider. Temporary grants can expire; choose the file again when necessary. Discovery stops when the app backgrounds. No background transfer/reboot recovery is claimed because there is no transfer engine yet.

The supplied `src/assets/icon.png` is used by the app interface. The Android launcher/splash assets still need a dedicated branding pass. See `docs/verification.md` for actual check results and remaining limits.
