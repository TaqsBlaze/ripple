# Ripple for Linux

Ripple's PC companion is a Go user service with a Qt/QML desktop interface. It provides persistent P-256 identity, QR enrollment, trusted-peer authentication, bidirectional Android file transfers, and local UI/service IPC. AI is not implemented yet.

## Prerequisites

- Go 1.26 or a compatible version declared by `go.mod`
- Qt 6 with Quick, Quick Controls 2, Network, qmake, and a C++17 compiler
- GNU Make

This checkout was verified with Go 1.26.5 and Qt 6.8.2. CMake was not installed, so the initial Qt build uses qmake.

## Build and verify

```sh
make check
make build
```

Start the desktop app as your normal user:

```sh
./build/bin/ripple-desktop
```

The desktop starts the adjacent `rippled` executable and stops its child service when the app exits. If a service is already running, the app connects to it and leaves it running on exit. Keep both executables together. For service development, you can still run `./build/bin/rippled` separately.

The service uses `$XDG_RUNTIME_DIR/ripple/ripple.sock` for local IPC and `$XDG_DATA_HOME/ripple` (or `~/.local/share/ripple`) for persistent data. For an isolated development run, pass explicit `--runtime-dir` and `--data-dir` values to `rippled`.

## Current behavior

- The service creates a persistent ECDSA P-256 identity and self-signed certificate.
- The private key is written with user-only permissions and does not cross into QML.
- Local IPC is length-prefixed, JSON encoded, capped at 1 MiB, permission-restricted, and checked with Linux peer credentials.
- The Qt shell shows the real service/device status and identity fingerprint.
- QR enrollment uses TLS 1.3, explicit PC approval, and persisted peer keys.
- The service advertises `_ripple._tcp` with only protocol version and an opaque identity-derived device ID. Android verifies the existing pin before updating a trusted PC's address.
- Android-to-Linux and Linux-to-Android file transfers use signed HTTPS offers and bounded chunks. The desktop can require approval or auto-accept trusted Android uploads; Android accepts desktop offers manually or automatically from trusted PCs after choosing a SAF receive folder. Completed received files open through the platform's registered file-type application.
- On the desktop, choose or drop one file to offer it to the single paired Android device. Open Ripple on Android and accept the offer; the desktop Transfers page shows its state. Sending is disabled if more than one device is paired.
- Set the desktop receive folder in Settings; new incoming files use that folder and older files remain where they were received. Android has its own receive-folder setting.
- Android can cancel an active outgoing transfer; the PC removes the partial file and records the cancellation.
- Durable resume, AI, and packaging remain unavailable.

See [docs/development.md](docs/development.md) for development notes and the next implementation slice.
