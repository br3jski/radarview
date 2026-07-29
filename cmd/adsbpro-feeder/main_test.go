package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRemovePairingTokenSkipsAuthenticatedSessions(t *testing.T) {
	t.Parallel()

	if err := removePairingToken("authenticate", filepath.Join(t.TempDir(), "missing", "pairing-token")); err != nil {
		t.Fatalf("authenticated reconnect must not depend on token file removal: %v", err)
	}
}

func TestRemovePairingTokenAfterPairing(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "pairing-token")
	if err := os.WriteFile(path, []byte("test-token"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := removePairingToken("pair", path); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("pairing token still exists or stat returned an unexpected error: %v", err)
	}
}
