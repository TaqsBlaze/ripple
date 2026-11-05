//go:build linux

package ipc

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"syscall"

	qrcode "github.com/skip2/go-qrcode"
	"ripple/internal/pairing"
	"ripple/internal/protocol"
	"ripple/internal/settings"
	"ripple/internal/transfer"
)

type Server struct {
	DeviceName *settings.DeviceName
	SocketPath string
	Status     protocol.ServiceStatus
	Pairing    *pairing.Manager
	Transfers  *transfer.Manager
	DropState  *pairing.DropState
	Invitation func() (pairing.Invitation, error)
	Logger     *slog.Logger
	listener   *net.UnixListener
}

func (server *Server) ListenAndServe() error {
	if server.Logger == nil {
		server.Logger = slog.Default()
	}
	if err := os.MkdirAll(filepath.Dir(server.SocketPath), 0o700); err != nil {
		return fmt.Errorf("create runtime directory: %w", err)
	}
	if err := rejectUnsafeExistingPath(server.SocketPath); err != nil {
		return err
	}
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: server.SocketPath, Net: "unix"})
	if err != nil {
		return fmt.Errorf("listen on private IPC socket: %w", err)
	}
	server.listener = listener
	if err := os.Chmod(server.SocketPath, 0o600); err != nil {
		_ = listener.Close()
		return fmt.Errorf("secure IPC socket: %w", err)
	}
	defer os.Remove(server.SocketPath)
	for {
		connection, err := listener.AcceptUnix()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return nil
			}
			return err
		}
		go server.handle(connection)
	}
}

func (server *Server) Close() error {
	if server.listener == nil {
		return nil
	}
	return server.listener.Close()
}

func (server *Server) handle(connection *net.UnixConn) {
	defer connection.Close()
	if err := verifyPeer(connection); err != nil {
		server.Logger.Warn("rejected local IPC peer", "error", err)
		return
	}
	for {
		var request protocol.Request
		if err := protocol.ReadFrame(connection, &request); err != nil {
			if !errors.Is(err, io.EOF) {
				server.Logger.Debug("IPC client disconnected", "error", err)
			}
			return
		}
		if err := protocol.WriteFrame(connection, server.dispatch(request)); err != nil {
			return
		}
	}
}

func (server *Server) dispatch(request protocol.Request) protocol.Response {
	response := protocol.Response{Version: protocol.IPCMajorVersion, Type: request.Type + ".result", RequestID: request.RequestID}
	if request.Version != protocol.IPCMajorVersion {
		response.Error = &protocol.ErrorBody{Code: "unsupported_version", Message: "This desktop app and service use incompatible IPC versions."}
		return response
	}
	switch request.Type {
	case "service.status":
		if server.Pairing != nil {
			server.Status.PendingPairs = server.Pairing.Pending()
			server.Status.TrustedPeers = server.Pairing.Peers()
		}
		if server.Transfers != nil {
			server.Status.Transfers = server.Transfers.List()
			server.Status.AutoAccept = server.Transfers.AutoAccept()
			server.Status.ReceiveDir = server.Transfers.ReceiveDir()
			items := server.Transfers.List()
			views := make([]map[string]any, 0, len(items))
			for _, item := range items {
				if item.Deleted {
					continue
				}
				view := map[string]any{"id": item.ID, "peerId": item.PeerID, "peerName": item.PeerName, "name": item.Name, "mimeType": item.MIME, "size": item.Size, "offset": item.Offset, "state": item.State, "direction": item.Direction, "receiptId": item.ReceiptID, "updatedAt": item.UpdatedAt, "deleted": item.Deleted}
				if item.Direction == "receive" && item.State == "completed" && !item.Deleted {
					if path, err := server.Transfers.FilePath(item.ID); err == nil {
						view["filePath"] = path
					}
				}
				views = append(views, view)
			}
			server.Status.Transfers = views
		}
		status := server.Status
		if server.DeviceName != nil {
			status.DeviceName = server.DeviceName.Get()
		}
		response.Payload = status
	case "device.rename":
		var payload struct {
			Name string `json:"name"`
		}
		if json.Unmarshal(request.Payload, &payload) != nil || server.DeviceName == nil {
			response.Error = &protocol.ErrorBody{Code: "invalid_request", Message: "Invalid device name."}
			break
		}
		if err := server.DeviceName.Set(payload.Name); err != nil {
			response.Error = &protocol.ErrorBody{Code: "rename_failed", Message: err.Error()}
		}
	case "drop.status":
		response.Payload = map[string]bool{"active": server.DropState != nil && server.DropState.Active()}
	case "drop.desktop":
		var payload struct {
			Active bool `json:"active"`
		}
		if json.Unmarshal(request.Payload, &payload) != nil || server.DropState == nil {
			response.Error = &protocol.ErrorBody{Code: "invalid_request", Message: "Invalid desktop drop state."}
			break
		}
		server.DropState.SetDesktop(payload.Active)
		response.Payload = map[string]bool{"active": payload.Active}
	case "pairing.invitation":
		if server.Invitation == nil {
			response.Error = &protocol.ErrorBody{Code: "unavailable", Message: "Pairing is unavailable."}
			break
		}
		invitation, err := server.Invitation()
		if err != nil {
			response.Error = &protocol.ErrorBody{Code: "invitation_failed", Message: err.Error()}
		} else {
			encoded, encodeErr := json.Marshal(invitation)
			if encodeErr != nil {
				response.Error = &protocol.ErrorBody{Code: "invitation_failed", Message: "Could not encode invitation."}
				break
			}
			png, qrErr := qrcode.Encode(string(encoded), qrcode.Medium, 384)
			if qrErr != nil {
				response.Error = &protocol.ErrorBody{Code: "qr_failed", Message: "Could not create pairing QR."}
				break
			}
			response.Payload = map[string]string{"invitation": string(encoded), "qrData": "data:image/png;base64," + base64.StdEncoding.EncodeToString(png)}
		}
	case "pairing.decide":
		var payload struct {
			ID      string `json:"id"`
			Approve bool   `json:"approve"`
		}
		if err := json.Unmarshal(request.Payload, &payload); err != nil || server.Pairing == nil {
			response.Error = &protocol.ErrorBody{Code: "invalid_request", Message: "Invalid pairing decision."}
			break
		}
		if err := server.Pairing.Decide(payload.ID, payload.Approve); err != nil {
			response.Error = &protocol.ErrorBody{Code: "decision_failed", Message: err.Error()}
		}
	case "transfer.decide":
		var payload struct {
			ID      string `json:"id"`
			Approve bool   `json:"approve"`
		}
		if err := json.Unmarshal(request.Payload, &payload); err != nil || server.Transfers == nil {
			response.Error = &protocol.ErrorBody{Code: "invalid_request", Message: "Invalid transfer decision."}
			break
		}
		if err := server.Transfers.Decide(payload.ID, payload.Approve); err != nil {
			response.Error = &protocol.ErrorBody{Code: "decision_failed", Message: err.Error()}
		}
	case "transfer.offer_local":
		var payload struct {
			PeerID string `json:"peerId"`
			Path   string `json:"path"`
		}
		if err := json.Unmarshal(request.Payload, &payload); err != nil || server.Transfers == nil || server.Pairing == nil {
			response.Error = &protocol.ErrorBody{Code: "invalid_request", Message: "Invalid desktop file offer."}
			break
		}
		var target *pairing.Peer
		for _, peer := range server.Pairing.Peers() {
			if peer.ID == payload.PeerID {
				selected := peer
				target = &selected
				break
			}
		}
		if target == nil {
			response.Error = &protocol.ErrorBody{Code: "untrusted_peer", Message: "Pair this Android device before sending files."}
			break
		}
		item, err := server.Transfers.OfferLocalFile(target.ID, target.Name, payload.Path)
		if err != nil {
			response.Error = &protocol.ErrorBody{Code: "offer_failed", Message: err.Error()}
		} else {
			response.Payload = map[string]any{"id": item.ID, "name": item.Name, "state": item.State, "peerName": item.PeerName}
		}
	case "transfer.receive_dir":
		var payload struct {
			Path string `json:"path"`
		}
		if json.Unmarshal(request.Payload, &payload) != nil || server.Transfers == nil {
			response.Error = &protocol.ErrorBody{Code: "invalid_request", Message: "Invalid receive folder."}
			break
		}
		if err := server.Transfers.SetReceiveDir(payload.Path); err != nil {
			response.Error = &protocol.ErrorBody{Code: "folder_failed", Message: err.Error()}
		}
	case "transfer.auto_accept":
		var payload struct {
			Enabled bool `json:"enabled"`
		}
		if err := json.Unmarshal(request.Payload, &payload); err != nil || server.Transfers == nil {
			response.Error = &protocol.ErrorBody{Code: "invalid_request", Message: "Invalid receive policy."}
			break
		}
		if err := server.Transfers.SetAutoAccept(payload.Enabled); err != nil {
			response.Error = &protocol.ErrorBody{Code: "policy_failed", Message: "Could not save receive policy."}
		}
	case "transfer.delete":
		var payload struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(request.Payload, &payload); err != nil || server.Transfers == nil {
			response.Error = &protocol.ErrorBody{Code: "invalid_request", Message: "Invalid received-file deletion request."}
			break
		}
		if err := server.Transfers.DeleteCompleted(payload.ID); err != nil {
			response.Error = &protocol.ErrorBody{Code: "delete_failed", Message: err.Error()}
		}
	default:
		response.Error = &protocol.ErrorBody{Code: "unknown_command", Message: "The requested local command is not supported."}
	}
	return response
}

func verifyPeer(connection *net.UnixConn) error {
	raw, err := connection.SyscallConn()
	if err != nil {
		return err
	}
	var credentials *syscall.Ucred
	var socketErr error
	if err := raw.Control(func(fd uintptr) {
		credentials, socketErr = syscall.GetsockoptUcred(int(fd), syscall.SOL_SOCKET, syscall.SO_PEERCRED)
	}); err != nil {
		return err
	}
	if socketErr != nil {
		return socketErr
	}
	if credentials == nil || credentials.Uid != uint32(os.Getuid()) {
		return errors.New("IPC peer does not belong to the current user")
	}
	return nil
}

func rejectUnsafeExistingPath(path string) error {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSocket == 0 || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("refusing to replace a non-socket IPC path")
	}
	connection, dialErr := net.Dial("unix", path)
	if dialErr == nil {
		_ = connection.Close()
		return errors.New("another Ripple service is already running")
	}
	return os.Remove(path)
}
