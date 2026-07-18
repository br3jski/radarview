package protocol

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"testing"
)

func TestCanonicalProofFieldOrder(t *testing.T) {
	hello := Hello{Operation: "authenticate", InstallationID: "12345678-1234-4678-9abc-123456789def", KeyFingerprint: "fingerprint", InputFormat: "sbs", ClientVersion: "2.0.0"}
	challenge := Challenge{ChallengeID: "challenge", Nonce: "nonce", ServerName: "feed.ads-b.pro", IssuedAt: "2026-07-18T00:00:00.000Z", ExpiresAt: "2026-07-18T00:00:30.000Z"}
	expected := "ADSBPRO-FEEDER-V2\noperation=authenticate\nchallengeId=challenge\nnonce=nonce\ninstallationId=12345678-1234-4678-9abc-123456789def\nkeyFingerprint=fingerprint\ninputFormat=sbs\nclientVersion=2.0.0\nserverName=feed.ads-b.pro\nissuedAt=2026-07-18T00:00:00.000Z\nexpiresAt=2026-07-18T00:00:30.000Z"
	if string(CanonicalProof(hello, challenge)) != expected {
		t.Fatalf("canonical payload changed")
	}
}

func TestP1363RoundTrip(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	hello := Hello{Operation: "pair", InstallationID: "12345678-1234-4678-9abc-123456789def", KeyFingerprint: "abc", InputFormat: "beast", ClientVersion: "test"}
	challenge := Challenge{ChallengeID: "id", Nonce: "nonce", ServerName: "feed.ads-b.pro", IssuedAt: "2026-07-18T00:00:00.000Z", ExpiresAt: "2026-07-18T00:00:30.000Z"}
	signature, err := Sign(key, CanonicalProof(hello, challenge))
	if err != nil {
		t.Fatal(err)
	}
	if !Verify(&key.PublicKey, CanonicalProof(hello, challenge), signature) {
		t.Fatal("signature rejected")
	}
	hello.InputFormat = "sbs"
	if Verify(&key.PublicKey, CanonicalProof(hello, challenge), signature) {
		t.Fatal("modified payload accepted")
	}
}
