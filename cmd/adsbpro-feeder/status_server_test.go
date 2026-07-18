package main

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
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
	if value.Traffic.Frames != 2 || value.Traffic.Bytes != 12 || value.Traffic.PayloadBytesPerSecond != 1.2 || !value.Update.Available {
		t.Fatalf("unexpected counters or update: %#v", value)
	}

	page, err := http.Get(server.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer page.Body.Close()
	body, _ := io.ReadAll(page.Body)
	if !strings.Contains(string(body), "UPDATE AVAILABLE") || !strings.Contains(string(body), "FEEDER STATUS") {
		t.Fatalf("status page is missing required content")
	}
	if strings.Contains(string(body), "No feeder token is stored") || strings.Contains(string(body), "frames forwarded") || strings.Contains(string(body), "ADS-B payload") {
		t.Fatalf("status page contains removed cumulative text")
	}
	if !strings.Contains(string(body), `id="upload-unit"`) {
		t.Fatalf("status page does not separate the upload rate unit")
	}
	if !strings.Contains(page.Header.Get("Content-Security-Policy"), "default-src 'self'") {
		t.Fatalf("status page is missing CSP")
	}

	style, err := http.Get(server.URL + "/style.css")
	if err != nil {
		t.Fatal(err)
	}
	defer style.Body.Close()
	stylesheet, _ := io.ReadAll(style.Body)
	if !strings.Contains(string(stylesheet), ".metric{height:190px") || !strings.Contains(string(stylesheet), ".metric{height:145px") || !strings.Contains(string(stylesheet), "white-space:nowrap") {
		t.Fatalf("status cards do not have fixed, single-line layout rules")
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

func TestPrivateStatusAddressClassification(t *testing.T) {
	tests := map[string]bool{
		"127.0.0.1": true, "10.12.1.4": true, "172.20.1.4": true,
		"192.168.50.2": true, "100.64.12.3": true, "fd7a:115c:a1e0::1": true,
		"8.8.8.8": false, "1.1.1.1": false, "2001:4860:4860::8888": false,
		"169.254.2.3": true,
	}
	for rawAddress, expected := range tests {
		if actual := statusAddressIsPrivate(netip.MustParseAddr(rawAddress)); actual != expected {
			t.Fatalf("statusAddressIsPrivate(%s) = %v, want %v", rawAddress, actual, expected)
		}
	}
}

func TestExplicitStatusAddressIsNotExpanded(t *testing.T) {
	addresses, err := resolveStatusListenAddresses("0.0.0.0:54321")
	if err != nil || len(addresses) != 1 || addresses[0] != "0.0.0.0:54321" {
		t.Fatalf("unexpected explicit addresses: %#v, %v", addresses, err)
	}
}

func TestPrivateStatusResolutionNeverReturnsPublicAddress(t *testing.T) {
	addresses, err := resolveStatusListenAddresses("private:54321")
	if err != nil {
		t.Fatal(err)
	}
	foundLoopback := false
	for _, listenAddress := range addresses {
		host, _, splitErr := net.SplitHostPort(listenAddress)
		if splitErr != nil {
			t.Fatal(splitErr)
		}
		address := netip.MustParseAddr(host)
		if address.String() == "127.0.0.1" {
			foundLoopback = true
		}
		if !statusAddressIsPrivate(address) {
			t.Fatalf("public address %s was selected", address)
		}
	}
	if !foundLoopback {
		t.Fatal("private status resolution omitted IPv4 loopback")
	}
}
