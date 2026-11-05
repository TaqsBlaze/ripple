# Ripple

![Ripple on desktop and Android](image/display.png)

**Bring your Android phone and Linux desktop closer together.**

Ripple connects your devices on the local network for trusted, bidirectional file transfers. A native Android experience pairs with a Linux service and Qt desktop interface, putting the connection and its controls in your hands.

## What Ripple does

- Pair Android and Linux using QR enrollment and explicit approval.
- Discover trusted devices on the local network and authenticate connections with pinned identities.
- Send files in both directions, with transfer approval and cancellation controls.
- Manage your connection from a Qt desktop interface backed by a local Go service.

Ripple is under active development. Durable transfer resume, AI features, and desktop packaging are not yet available. See the platform documentation for current behavior and limitations.

## Project layout

| Path | Purpose |
| --- | --- |
| `Android/ripple` | React, TypeScript, and Capacitor app |
| `native/android-template` | Android native project and Ripple-specific implementation |
| `Linux/ripple` | Go service and Qt/QML desktop application |
| `Linux/ripple/docs` | Protocol and development documentation |

## Get started

For Android, install Node.js 22 and Java 21, and configure the Android SDK. From the repository root:

```sh
cd Android/ripple
npm ci
cp -a ../../native/android-template android
npm run build
npx cap sync android
cd android
./gradlew :app:assembleDebug
```

The `android` working directory is ignored; make permanent native changes in `native/android-template` and copy them into your local Android project before syncing.

For Linux, install Go (the version in `Linux/ripple/go.mod`), Qt 6 development tools, qmake, a C++17 compiler, and Make:

```sh
cd Linux/ripple
make check
make build
./build/bin/ripple-desktop
```

The desktop starts and stops its own service automatically. Keep `rippled` next to `ripple-desktop`.

See the [Android guide](Android/ripple/README.md), [Linux guide](Linux/ripple/README.md), and [protocol documentation](Linux/ripple/docs/protocol-v1.md) for details.

## Releases

Publishing a GitHub release whose commit is reachable from `main` builds an Android release APK and a Linux x86-64 archive. The APK is unsigned and requires signing before installation or distribution. The Linux archive contains binaries and requires compatible Qt runtime libraries on the target system.

## Contributing

Contributions are welcome. Open an issue to discuss substantial changes, then submit a focused pull request describing the problem, your solution, and how you tested it. For Android changes, run `npm run check`, `npm test`, and the relevant Gradle checks. For Linux changes, run `make check`. Please avoid committing credentials, device data, generated builds, or signing keys.

## Security

Please report security vulnerabilities privately to the project maintainers rather than publishing exploit details in an issue. Do not include private keys or personal device data in reports.
