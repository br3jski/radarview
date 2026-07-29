package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/br3jski/radarview/internal/source"
)

type monitoredWriter struct {
	Writer  interface{ Write([]byte) (int, error) }
	monitor *runtimeMonitor
	format  string
}

func (writer *monitoredWriter) Write(buffer []byte) (int, error) {
	count, err := writer.Writer.Write(buffer)
	if count > 0 {
		writer.monitor.recordPayload(buffer[:count], writer.format)
	}
	return count, err
}

type rateBucket struct {
	second int64
	frames uint64
	bytes  uint64
}

type runtimeMonitor struct {
	mu sync.Mutex

	dataDir         string
	installationID  string
	label           string
	sourceConfig    source.Config
	startedAt       time.Time
	state           string
	stateUpdatedAt  time.Time
	errorMessage    string
	inputFormat     string
	connectedAt     time.Time
	sessionActive   bool
	lastFrameAt     time.Time
	accountDisplay  string
	bytesForwarded  uint64
	framesForwarded uint64
	reconnects      uint64
	rateBuckets     [10]rateBucket
	beastEscape     bool
	aircraftTracker *aircraftTracker
	latestVersion   string
	updateCheckedAt time.Time
}

type sourceSnapshot struct {
	Host       string `json:"host"`
	Mode       string `json:"mode"`
	Format     string `json:"format,omitempty"`
	Port       int    `json:"port"`
	LastFrame  string `json:"lastFrameAt,omitempty"`
	Aircraft   *int   `json:"aircraft,omitempty"`
	AircraftBy string `json:"aircraftSource,omitempty"`
}

type trafficSnapshot struct {
	Frames                uint64  `json:"frames"`
	Bytes                 uint64  `json:"bytes"`
	MessagesPerSecond     float64 `json:"messagesPerSecond"`
	PayloadBytesPerSecond float64 `json:"payloadBytesPerSecond"`
	Reconnects            uint64  `json:"reconnects"`
}

type updateSnapshot struct {
	Available    bool   `json:"available"`
	Latest       string `json:"latestVersion,omitempty"`
	LastChecked  string `json:"lastCheckedAt,omitempty"`
	LinuxCommand string `json:"linuxCommand"`
	WinCommand   string `json:"windowsCommand"`
}

type apiStatus struct {
	Version        string                  `json:"version"`
	State          string                  `json:"state"`
	StateUpdatedAt string                  `json:"stateUpdatedAt"`
	StartedAt      string                  `json:"startedAt"`
	ConnectedAt    string                  `json:"connectedAt,omitempty"`
	InstallationID string                  `json:"installationId"`
	Label          string                  `json:"label"`
	AccountDisplay string                  `json:"accountDisplay,omitempty"`
	Error          string                  `json:"error,omitempty"`
	Source         sourceSnapshot          `json:"source"`
	Traffic        trafficSnapshot         `json:"traffic"`
	Aircraft       aircraftTrafficSnapshot `json:"aircraftTraffic"`
	Update         updateSnapshot          `json:"update"`
}

func newRuntimeMonitor(configuration config, installationID string) *runtimeMonitor {
	now := time.Now()
	return &runtimeMonitor{
		dataDir: configuration.DataDir, installationID: installationID,
		label: configuration.Label, sourceConfig: configuration.Source,
		startedAt: now, state: "starting", stateUpdatedAt: now,
		aircraftTracker: newAircraftTracker(),
	}
}

func (monitor *runtimeMonitor) setState(stateValue, inputFormat, errorMessage, accountDisplay string) {
	now := time.Now()
	monitor.mu.Lock()
	monitor.state = stateValue
	monitor.stateUpdatedAt = now
	monitor.errorMessage = errorMessage
	if inputFormat != "" {
		monitor.inputFormat = inputFormat
	}
	if accountDisplay != "" {
		monitor.accountDisplay = accountDisplay
	}
	if stateValue == "ready" {
		monitor.sessionActive = true
	}
	if stateValue == "active" && monitor.connectedAt.IsZero() {
		monitor.connectedAt = now
	}
	if stateValue == "disconnected" {
		monitor.connectedAt = time.Time{}
		if monitor.sessionActive {
			monitor.reconnects++
			monitor.sessionActive = false
		}
	}
	diskStatus := status{
		State: stateValue, InstallationID: monitor.installationID,
		InputFormat: monitor.inputFormat, UpdatedAt: now, Error: errorMessage,
		AccountDisplay: monitor.accountDisplay,
	}
	monitor.mu.Unlock()
	writeStatusFile(monitor.dataDir, diskStatus)
}

func (monitor *runtimeMonitor) recordPayload(payload []byte, format string) {
	if len(payload) == 0 {
		return
	}
	now := time.Now()
	monitor.mu.Lock()
	monitor.bytesForwarded += uint64(len(payload))
	frames := monitor.countFrames(payload, format)
	second := now.Unix()
	bucket := &monitor.rateBuckets[second%int64(len(monitor.rateBuckets))]
	if bucket.second != second {
		bucket.second = second
		bucket.frames = 0
		bucket.bytes = 0
	}
	bucket.bytes += uint64(len(payload))
	if frames > 0 {
		monitor.framesForwarded += frames
		monitor.lastFrameAt = now
		bucket.frames += frames
	}
	monitor.mu.Unlock()
	monitor.aircraftTracker.recordForwarded(payload, format, now)
}

func (monitor *runtimeMonitor) countFrames(payload []byte, format string) uint64 {
	if format == "sbs" {
		var count uint64
		for _, value := range payload {
			if value == '\n' {
				count++
			}
		}
		return count
	}
	var count uint64
	for _, value := range payload {
		if monitor.beastEscape {
			monitor.beastEscape = false
			if value >= 0x31 && value <= 0x34 {
				count++
			} else if value == 0x1a {
				continue
			}
			continue
		}
		if value == 0x1a {
			monitor.beastEscape = true
		}
	}
	return count
}

func (monitor *runtimeMonitor) setLatest(versionValue string, checkedAt time.Time) {
	monitor.mu.Lock()
	monitor.latestVersion = versionValue
	monitor.updateCheckedAt = checkedAt
	monitor.mu.Unlock()
}

func (monitor *runtimeMonitor) snapshot() apiStatus {
	monitor.mu.Lock()
	now := time.Now()
	var recentFrames uint64
	var recentBytes uint64
	for _, bucket := range monitor.rateBuckets {
		if now.Unix()-bucket.second >= 0 && now.Unix()-bucket.second < int64(len(monitor.rateBuckets)) {
			recentFrames += bucket.frames
			recentBytes += bucket.bytes
		}
	}
	port := monitor.sourceConfig.BeastPort
	if monitor.inputFormat == "sbs" {
		port = monitor.sourceConfig.SBSPort
	}
	state := monitor.state
	errorMessage := monitor.errorMessage
	stateUpdatedAt := monitor.stateUpdatedAt
	inputFormat := monitor.inputFormat
	accountDisplay := monitor.accountDisplay
	connectedAt := monitor.connectedAt
	lastFrameAt := monitor.lastFrameAt
	framesForwarded := monitor.framesForwarded
	bytesForwarded := monitor.bytesForwarded
	reconnects := monitor.reconnects
	latestVersion := monitor.latestVersion
	updateCheckedAt := monitor.updateCheckedAt
	monitor.mu.Unlock()
	aircraft := monitor.aircraftTracker.snapshot(now, state, errorMessage)
	snapshot := apiStatus{
		Version: version, State: state, StateUpdatedAt: stateUpdatedAt.Format(time.RFC3339Nano),
		StartedAt: monitor.startedAt.Format(time.RFC3339Nano), InstallationID: monitor.installationID,
		Label: monitor.label, AccountDisplay: accountDisplay, Error: errorMessage,
		Source: sourceSnapshot{
			Host: monitor.sourceConfig.Host, Mode: monitor.sourceConfig.Mode, Format: inputFormat,
			Port: port, Aircraft: aircraft.Count, AircraftBy: aircraft.Source,
		},
		Traffic: trafficSnapshot{
			Frames: framesForwarded, Bytes: bytesForwarded,
			MessagesPerSecond:     float64(recentFrames) / float64(len(monitor.rateBuckets)),
			PayloadBytesPerSecond: float64(recentBytes) / float64(len(monitor.rateBuckets)),
			Reconnects:            reconnects,
		},
		Aircraft: aircraft,
		Update: updateSnapshot{
			Available: newerVersion(latestVersion, version), Latest: latestVersion,
			LinuxCommand: "curl -fsSL https://raw.githubusercontent.com/br3jski/radarview/main/radarview_setup.sh | sudo bash",
			WinCommand:   "irm https://raw.githubusercontent.com/br3jski/radarview/main/radarview_setup.ps1 | iex",
		},
	}
	if !connectedAt.IsZero() {
		snapshot.ConnectedAt = connectedAt.Format(time.RFC3339Nano)
	}
	if !lastFrameAt.IsZero() {
		snapshot.Source.LastFrame = lastFrameAt.Format(time.RFC3339Nano)
	}
	if !updateCheckedAt.IsZero() {
		snapshot.Update.LastChecked = updateCheckedAt.Format(time.RFC3339Nano)
	}
	return snapshot
}

func writeStatusFile(dataDir string, value status) {
	body, _ := json.Marshal(value)
	temporary := filepath.Join(dataDir, "status.json.new")
	if os.WriteFile(temporary, body, 0600) == nil {
		_ = os.Rename(temporary, filepath.Join(dataDir, "status.json"))
	}
}
