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
	aircraftCount   *int
	aircraftSource  string
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
	Version        string          `json:"version"`
	State          string          `json:"state"`
	StateUpdatedAt string          `json:"stateUpdatedAt"`
	StartedAt      string          `json:"startedAt"`
	ConnectedAt    string          `json:"connectedAt,omitempty"`
	InstallationID string          `json:"installationId"`
	Label          string          `json:"label"`
	AccountDisplay string          `json:"accountDisplay,omitempty"`
	Error          string          `json:"error,omitempty"`
	Source         sourceSnapshot  `json:"source"`
	Traffic        trafficSnapshot `json:"traffic"`
	Update         updateSnapshot  `json:"update"`
}

func newRuntimeMonitor(configuration config, installationID string) *runtimeMonitor {
	now := time.Now()
	return &runtimeMonitor{
		dataDir: configuration.DataDir, installationID: installationID,
		label: configuration.Label, sourceConfig: configuration.Source,
		startedAt: now, state: "starting", stateUpdatedAt: now,
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

func (monitor *runtimeMonitor) setAircraft(count *int, sourceName string) {
	monitor.mu.Lock()
	monitor.aircraftCount = count
	monitor.aircraftSource = sourceName
	monitor.mu.Unlock()
}

func (monitor *runtimeMonitor) setLatest(versionValue string, checkedAt time.Time) {
	monitor.mu.Lock()
	monitor.latestVersion = versionValue
	monitor.updateCheckedAt = checkedAt
	monitor.mu.Unlock()
}

func (monitor *runtimeMonitor) snapshot() apiStatus {
	monitor.mu.Lock()
	defer monitor.mu.Unlock()
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
	snapshot := apiStatus{
		Version: version, State: monitor.state, StateUpdatedAt: monitor.stateUpdatedAt.Format(time.RFC3339Nano),
		StartedAt: monitor.startedAt.Format(time.RFC3339Nano), InstallationID: monitor.installationID,
		Label: monitor.label, AccountDisplay: monitor.accountDisplay, Error: monitor.errorMessage,
		Source: sourceSnapshot{
			Host: monitor.sourceConfig.Host, Mode: monitor.sourceConfig.Mode, Format: monitor.inputFormat,
			Port: port, Aircraft: monitor.aircraftCount, AircraftBy: monitor.aircraftSource,
		},
		Traffic: trafficSnapshot{
			Frames: monitor.framesForwarded, Bytes: monitor.bytesForwarded,
			MessagesPerSecond:     float64(recentFrames) / float64(len(monitor.rateBuckets)),
			PayloadBytesPerSecond: float64(recentBytes) / float64(len(monitor.rateBuckets)),
			Reconnects:            monitor.reconnects,
		},
		Update: updateSnapshot{
			Available: newerVersion(monitor.latestVersion, version), Latest: monitor.latestVersion,
			LinuxCommand: "curl -fsSL https://raw.githubusercontent.com/br3jski/radarview/main/radarview_setup.sh | sudo bash",
			WinCommand:   "irm https://raw.githubusercontent.com/br3jski/radarview/main/radarview_setup.ps1 | iex",
		},
	}
	if !monitor.connectedAt.IsZero() {
		snapshot.ConnectedAt = monitor.connectedAt.Format(time.RFC3339Nano)
	}
	if !monitor.lastFrameAt.IsZero() {
		snapshot.Source.LastFrame = monitor.lastFrameAt.Format(time.RFC3339Nano)
	}
	if !monitor.updateCheckedAt.IsZero() {
		snapshot.Update.LastChecked = monitor.updateCheckedAt.Format(time.RFC3339Nano)
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
