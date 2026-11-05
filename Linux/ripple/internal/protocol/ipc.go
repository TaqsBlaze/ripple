package protocol

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

const (
	IPCMajorVersion = 1
	MaxIPCFrame     = 1 << 20
)

type Request struct {
	Version   int             `json:"version"`
	Type      string          `json:"type"`
	RequestID string          `json:"requestId"`
	Payload   json.RawMessage `json:"payload,omitempty"`
}

type Response struct {
	Version   int        `json:"version"`
	Type      string     `json:"type"`
	RequestID string     `json:"requestId"`
	Payload   any        `json:"payload,omitempty"`
	Error     *ErrorBody `json:"error,omitempty"`
}

type ErrorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type ServiceStatus struct {
	ServiceVersion      string `json:"serviceVersion"`
	DeviceName          string `json:"deviceName"`
	IdentityFingerprint string `json:"identityFingerprint"`
	NetworkReady        bool   `json:"networkReady"`
	PairingReady        bool   `json:"pairingReady"`
	TransferReady       bool   `json:"transferReady"`
	PendingPairs        any    `json:"pendingPairs"`
	TrustedPeers        any    `json:"trustedPeers"`
	Transfers           any    `json:"transfers"`
	ReceiveDir          string `json:"receiveDir"`
	AutoAccept          bool   `json:"autoAccept"`
}

func ReadFrame(r io.Reader, value any) error {
	var size uint32
	if err := binary.Read(r, binary.BigEndian, &size); err != nil {
		return err
	}
	if size == 0 || size > MaxIPCFrame {
		return fmt.Errorf("invalid IPC frame size %d", size)
	}
	data := make([]byte, size)
	if _, err := io.ReadFull(r, data); err != nil {
		return err
	}
	if err := json.Unmarshal(data, value); err != nil {
		return fmt.Errorf("decode IPC frame: %w", err)
	}
	return nil
}

func WriteFrame(w io.Writer, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("encode IPC frame: %w", err)
	}
	if len(data) == 0 || len(data) > MaxIPCFrame {
		return errors.New("IPC frame is outside the permitted size")
	}
	if err := binary.Write(w, binary.BigEndian, uint32(len(data))); err != nil {
		return err
	}
	_, err = w.Write(data)
	return err
}
