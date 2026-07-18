package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/br3jski/radarview/internal/source"
)

func testMonitor(t *testing.T) *runtimeMonitor {
	t.Helper()
	return newRuntimeMonitor(config{
		DataDir: t.TempDir(), Label: "Roof feeder",
		Source: source.Config{Host: "127.0.0.1", Mode: "auto", BeastPort: 30005, SBSPort: 30003},
	}, "12345678-1234-4678-9abc-123456789def")
}

func TestStatusAPIAndPage(t *testing.T) {
	monitor := testMonitor(t)
	monitor.setState("active", "sbs", "", "t***t@e***.com")
	monitor.recordPayload([]byte("MSG,1\nMSG,2\n"), "sbs")
	count := 12
	monitor.setAircraft(&count, "/run/readsb/aircraft.json")
	monitor.setLatest("9.0.0", time.Now())

	server := httptest.NewServer(statusHandler(monitor))
	defer server.Close()
	response, err := http.Get(server.URL + "/api/status")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK || response.Header.Get("Cache-Control") != "no-store" {
		t.Fatalf("unexpected API response: %d %#v", response.StatusCode, response.Header)
	}
	var value apiStatus
	if err := json.NewDecoder(response.Body).Decode(&value); err != nil {
		t.Fatal(err)
	}
	if value.State != "active" || value.AccountDisplay != "t***t@e***.com" || value.Source.Aircraft == nil || *value.Source.Aircraft != 12 {
		t.Fatalf("unexpected snapshot: %#v", value)
	}
	if value.Traffic.Frames != 2 || value.Traffic.Bytes != 12 || !value.Update.Available {
		t.Fatalf("unexpected counters or update: %#v", value)
	}

	page, err := http.Get(server.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer page.Body.Close()
	body, _ := io.ReadAll(page.Body)
	if !strings.Contains(string(body), "UPDATE AVAILABLE") || !strings.Contains(string(body), "No feeder token is stored") {
		t.Fatalf("status page is missing required content")
	}
	if !strings.Contains(page.Header.Get("Content-Security-Policy"), "default-src 'self'") {
		t.Fatalf("status page is missing CSP")
	}
}

func TestFrameCountersHandleChunkBoundariesAndEscapes(t *testing.T) {
	monitor := testMonitor(t)
	monitor.recordPayload([]byte{0x1a}, "beast")
	monitor.recordPayload([]byte{0x31, 0x01, 0x1a, 0x1a, 0x31, 0x1a, 0x32}, "beast")
	snapshot := monitor.snapshot()
	if snapshot.Traffic.Frames != 2 {
		t.Fatalf("expected 2 Beast frames, got %d", snapshot.Traffic.Frames)
	}
}

func TestLatestVersionAndFetch(t *testing.T) {
	for _, test := range []struct {
		candidate string
		current   string
		want      bool
	}{{"2.2.0", "2.1.0", true}, {"2.1.0", "2.1.0", false}, {"2.0.9", "2.1.0", false}, {"bad", "2.1.0", false}} {
		if got := newerVersion(test.candidate, test.current); got != test.want {
			t.Fatalf("newerVersion(%q, %q) = %v", test.candidate, test.current, got)
		}
	}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(writer, `{"version":"3.4.5"}`)
	}))
	defer server.Close()
	got, err := fetchLatestVersion(context.Background(), server.URL)
	if err != nil || got != "3.4.5" {
		t.Fatalf("fetchLatestVersion = %q, %v", got, err)
	}
}

func TestAircraftCount(t *testing.T) {
	path := filepath.Join(t.TempDir(), "aircraft.json")
	if err := os.WriteFile(path, []byte(`{"aircraft":[{"seen":0.2},{"seen":59},{"seen":61},{}]}`), 0600); err != nil {
		t.Fatal(err)
	}
	count, sourceName, ok := readAircraftCount(context.Background(), path)
	if !ok || count != 3 || sourceName != path {
		t.Fatalf("readAircraftCount = %d, %q, %v", count, sourceName, ok)
	}
	if err := os.WriteFile(path, []byte(`not json`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, ok := readAircraftCount(context.Background(), path); ok {
		t.Fatal("invalid aircraft.json must not be reported as zero aircraft")
	}
}

func TestStatusHandlerRejectsWrites(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/api/status", strings.NewReader("ignored"))
	response := httptest.NewRecorder()
	statusHandler(testMonitor(t)).ServeHTTP(response, request)
	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", response.Code)
	}
}
