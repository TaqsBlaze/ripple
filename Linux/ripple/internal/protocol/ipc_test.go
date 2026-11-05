package protocol

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func TestFrameRoundTrip(t *testing.T) {
	want := Request{Version: 1, Type: "service.status", RequestID: "request-1"}
	var buffer bytes.Buffer
	if err := WriteFrame(&buffer, want); err != nil {
		t.Fatal(err)
	}
	var got Request
	if err := ReadFrame(&buffer, &got); err != nil {
		t.Fatal(err)
	}
	if got.Version != want.Version || got.Type != want.Type || got.RequestID != want.RequestID {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

func TestRejectsOversizedFrame(t *testing.T) {
	var buffer bytes.Buffer
	if err := binary.Write(&buffer, binary.BigEndian, uint32(MaxIPCFrame+1)); err != nil {
		t.Fatal(err)
	}
	var request Request
	if err := ReadFrame(&buffer, &request); err == nil {
		t.Fatal("expected oversized frame to be rejected")
	}
}
