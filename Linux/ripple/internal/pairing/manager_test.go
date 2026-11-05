package pairing

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"testing"
)

func TestEnrollmentRequiresApprovalAndProof(t *testing.T) {
	manager, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	invitation, err := manager.CreateInvitation("https://127.0.0.1:43821", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatal(err)
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	publicKey, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	enrollmentID, err := manager.Start(invitation.SessionID, invitation.Secret, "android-1", "Android", publicKey)
	if err != nil {
		t.Fatal(err)
	}
	state, err := manager.Status(invitation.SessionID, enrollmentID)
	if err != nil || state["state"] != "pending" {
		t.Fatalf("state=%v err=%v", state, err)
	}
	if _, err := manager.Prove(invitation.SessionID, enrollmentID, "android-1", ""); err == nil {
		t.Fatal("proof succeeded before approval")
	}
	if err := manager.Decide(enrollmentID, true); err != nil {
		t.Fatal(err)
	}
	state, err = manager.Status(invitation.SessionID, enrollmentID)
	if err != nil || state["state"] != "approved" {
		t.Fatalf("state=%v err=%v", state, err)
	}
	challenge, err := base64.StdEncoding.DecodeString(state["challenge"])
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(challenge)
	signature, err := ecdsa.SignASN1(rand.Reader, key, digest[:])
	if err != nil {
		t.Fatal(err)
	}
	peer, err := manager.Prove(invitation.SessionID, enrollmentID, "android-1", base64.StdEncoding.EncodeToString(signature))
	if err != nil {
		t.Fatal(err)
	}
	if peer.ID == "" || len(manager.Peers()) != 1 {
		t.Fatal("peer was not persisted")
	}
	if _, err := manager.Prove(invitation.SessionID, enrollmentID, "android-1", base64.StdEncoding.EncodeToString(signature)); err == nil {
		t.Fatal("challenge was reusable")
	}
}

func TestEnrollmentRejectsWrongSecretAndKeyReplacement(t *testing.T) {
	manager, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	invitation, err := manager.CreateInvitation("https://127.0.0.1:43821", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatal(err)
	}
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	publicKey, _ := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if _, err := manager.Start(invitation.SessionID, "wrong", "android-1", "Android", publicKey); err == nil {
		t.Fatal("wrong secret was accepted")
	}
	id, err := manager.Start(invitation.SessionID, invitation.Secret, "android-1", "Android", publicKey)
	if err != nil {
		t.Fatal(err)
	}
	other, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	otherPublic, _ := x509.MarshalPKIXPublicKey(&other.PublicKey)
	otherID, err := manager.Start(invitation.SessionID, invitation.Secret, "android-2", "Other", otherPublic)
	if err != nil {
		t.Fatal(err)
	}
	if id == otherID {
		t.Fatal("distinct candidates shared an enrollment ID")
	}
	if err := manager.Decide(id, false); err != nil {
		t.Fatal(err)
	}
	state, err := manager.Status(invitation.SessionID, id)
	if err != nil || state["state"] != "rejected" {
		t.Fatalf("state=%v err=%v", state, err)
	}
}
