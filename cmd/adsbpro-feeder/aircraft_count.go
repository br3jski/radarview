package main

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	aircraftRetention       = 30 * time.Second
	aircraftMatchTolerance  = 5 * time.Second
	aircraftPollInterval    = 2 * time.Second
	aircraftReadTimeout     = 4 * time.Second
	aircraftRateWindow      = 10 * time.Second
	maxAircraftDocumentSize = 8 * 1024 * 1024
)

var commonAircraftFiles = []string{
	"/run/readsb/aircraft.json",
	"/var/run/readsb/aircraft.json",
	"/run/dump1090-fa/aircraft.json",
	"/var/run/dump1090-fa/aircraft.json",
	"/run/dump1090-mutability/aircraft.json",
	"/var/run/dump1090-mutability/aircraft.json",
	"/dev/shm/readsb/aircraft.json",
	"/dev/shm/aircraft.json",
}

type aircraftDocument struct {
	Aircraft []aircraftJSON `json:"aircraft"`
}

type aircraftJSON struct {
	Hex      string          `json:"hex"`
	Flight   string          `json:"flight"`
	Altitude json.RawMessage `json:"alt_baro"`
	Speed    *float64        `json:"gs"`
	Track    *float64        `json:"track"`
	Distance *float64        `json:"r_dst"`
	RSSI     *float64        `json:"rssi"`
	Messages *uint64         `json:"messages"`
	Seen     *float64        `json:"seen"`
}

type aircraftMetadata struct {
	ICAO         string
	Supported    bool
	Callsign     string
	AltitudeFeet *float64
	OnGround     bool
	SpeedKnots   *float64
	TrackDegrees *float64
	DistanceNM   *float64
	RSSIDBFS     *float64
	Messages     *uint64
	SeenSeconds  float64
}

type aircraftRatePoint struct {
	at       time.Time
	messages uint64
}

type aircraftRateBucket struct {
	second int64
	frames uint64
}

type trackedAircraft struct {
	icao         string
	supported    bool
	hasMetadata  bool
	callsign     string
	altitudeFeet *float64
	onGround     bool
	speedKnots   *float64
	trackDegrees *float64
	distanceNM   *float64
	rssiDBFS     *float64
	lastLocalAt  time.Time
	lastSentAt   time.Time
	localRates   []aircraftRatePoint
	sentRates    [10]aircraftRateBucket
}

type aircraftTracker struct {
	mu                sync.Mutex
	aircraft          map[string]*trackedAircraft
	metadataAvailable bool
	metadataSource    string
	sampledAt         time.Time
	parserFormat      string
	beastParser       beastAircraftParser
	sbsBuffer         []byte
}

type aircraftStatusRow struct {
	ICAO                   string   `json:"icao"`
	Callsign               string   `json:"callsign,omitempty"`
	AltitudeFeet           *float64 `json:"altitudeFeet,omitempty"`
	OnGround               bool     `json:"onGround,omitempty"`
	SpeedKnots             *float64 `json:"speedKnots,omitempty"`
	TrackDegrees           *float64 `json:"trackDegrees,omitempty"`
	DistanceNM             *float64 `json:"distanceNm,omitempty"`
	RSSIDBFS               *float64 `json:"rssiDbfs,omitempty"`
	LocalMessagesPerSecond float64  `json:"localMessagesPerSecond"`
	SentMessagesPerSecond  float64  `json:"sentMessagesPerSecond"`
	LastSeenAt             string   `json:"lastSeenAt,omitempty"`
	LastSentAt             string   `json:"lastSentAt,omitempty"`
	ReasonCode             string   `json:"reasonCode,omitempty"`
	Reason                 string   `json:"reason,omitempty"`
}

type aircraftTrafficSnapshot struct {
	MetadataAvailable bool                `json:"metadataAvailable"`
	Source            string              `json:"source,omitempty"`
	SampledAt         string              `json:"sampledAt,omitempty"`
	Sent              []aircraftStatusRow `json:"sent"`
	NotSent           []aircraftStatusRow `json:"notSent"`
	Count             *int                `json:"-"`
}

func newAircraftTracker() *aircraftTracker {
	return &aircraftTracker{aircraft: make(map[string]*trackedAircraft)}
}

func monitorAircraft(ctx context.Context, configuredSource string, monitor *runtimeMonitor) {
	check := func() {
		readCtx, cancel := context.WithTimeout(ctx, aircraftReadTimeout)
		defer cancel()
		metadata, sourceName, err := readAircraftMetadata(readCtx, configuredSource)
		if err != nil {
			monitor.aircraftTracker.setMetadataUnavailable(time.Now())
			return
		}
		monitor.aircraftTracker.updateMetadata(metadata, sourceName, time.Now())
	}
	check()
	ticker := time.NewTicker(aircraftPollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			check()
		}
	}
}

func readAircraftMetadata(ctx context.Context, configuredSource string) ([]aircraftMetadata, string, error) {
	if configuredSource != "" {
		body, err := readAircraftDocument(ctx, configuredSource)
		if err != nil {
			return nil, "", err
		}
		metadata, err := parseAircraftMetadata(body)
		return metadata, configuredSource, err
	}
	for _, path := range commonAircraftFiles {
		body, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		metadata, parseErr := parseAircraftMetadata(body)
		if parseErr == nil {
			return metadata, path, nil
		}
	}
	return nil, "", errors.New("aircraft.json not found")
}

func readAircraftDocument(ctx context.Context, location string) ([]byte, error) {
	if !strings.HasPrefix(location, "http://") && !strings.HasPrefix(location, "https://") {
		return os.ReadFile(location)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, location, nil)
	if err != nil {
		return nil, err
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, io.ErrUnexpectedEOF
	}
	return io.ReadAll(io.LimitReader(response.Body, maxAircraftDocumentSize))
}

func parseAircraftMetadata(body []byte) ([]aircraftMetadata, error) {
	var document aircraftDocument
	if err := json.Unmarshal(body, &document); err != nil {
		return nil, err
	}
	if document.Aircraft == nil {
		return nil, errors.New("aircraft.json has no aircraft array")
	}
	metadata := make([]aircraftMetadata, 0, len(document.Aircraft))
	for _, value := range document.Aircraft {
		icao, supported, ok := normalizeICAO(value.Hex)
		if !ok {
			continue
		}
		altitude, onGround := parseAltitude(value.Altitude)
		seen := 0.0
		if value.Seen != nil && *value.Seen > 0 {
			seen = *value.Seen
		}
		metadata = append(metadata, aircraftMetadata{
			ICAO: icao, Supported: supported, Callsign: strings.TrimSpace(value.Flight),
			AltitudeFeet: altitude, OnGround: onGround, SpeedKnots: value.Speed,
			TrackDegrees: value.Track, DistanceNM: value.Distance, RSSIDBFS: value.RSSI,
			Messages: value.Messages, SeenSeconds: seen,
		})
	}
	return metadata, nil
}

func parseAltitude(raw json.RawMessage) (*float64, bool) {
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return nil, false
	}
	var number float64
	if json.Unmarshal(raw, &number) == nil {
		return &number, false
	}
	var text string
	if json.Unmarshal(raw, &text) == nil && strings.EqualFold(text, "ground") {
		return nil, true
	}
	return nil, false
}

func normalizeICAO(value string) (string, bool, bool) {
	value = strings.ToUpper(strings.TrimSpace(value))
	supported := true
	raw := value
	if strings.HasPrefix(raw, "~") {
		supported = false
		raw = strings.TrimPrefix(raw, "~")
	}
	if len(raw) != 6 {
		return "", false, false
	}
	if _, err := hex.DecodeString(raw); err != nil || raw == "000000" {
		return "", false, false
	}
	if !supported {
		return "~" + raw, false, true
	}
	return raw, true, true
}

func (tracker *aircraftTracker) updateMetadata(values []aircraftMetadata, sourceName string, sampledAt time.Time) {
	tracker.mu.Lock()
	defer tracker.mu.Unlock()
	tracker.metadataAvailable = true
	tracker.metadataSource = sourceName
	tracker.sampledAt = sampledAt
	for _, value := range values {
		current := tracker.aircraft[value.ICAO]
		if current == nil {
			current = &trackedAircraft{icao: value.ICAO}
			tracker.aircraft[value.ICAO] = current
		}
		current.supported = value.Supported
		current.hasMetadata = true
		if value.Callsign != "" {
			current.callsign = value.Callsign
		}
		if value.AltitudeFeet != nil || value.OnGround {
			current.altitudeFeet = value.AltitudeFeet
			current.onGround = value.OnGround
		}
		if value.SpeedKnots != nil {
			current.speedKnots = value.SpeedKnots
		}
		if value.TrackDegrees != nil {
			current.trackDegrees = value.TrackDegrees
		}
		if value.DistanceNM != nil {
			current.distanceNM = value.DistanceNM
		}
		if value.RSSIDBFS != nil {
			current.rssiDBFS = value.RSSIDBFS
		}
		current.lastLocalAt = sampledAt.Add(-time.Duration(value.SeenSeconds * float64(time.Second)))
		if value.Messages != nil {
			current.localRates = appendLocalRate(current.localRates, sampledAt, *value.Messages)
		}
	}
}

func appendLocalRate(points []aircraftRatePoint, sampledAt time.Time, messages uint64) []aircraftRatePoint {
	if len(points) > 0 && messages < points[len(points)-1].messages {
		points = points[:0]
	}
	points = append(points, aircraftRatePoint{at: sampledAt, messages: messages})
	cutoff := sampledAt.Add(-aircraftRateWindow - 2*time.Second)
	first := 0
	for first < len(points)-1 && points[first].at.Before(cutoff) {
		first++
	}
	return append(points[:0], points[first:]...)
}

func (tracker *aircraftTracker) setMetadataUnavailable(sampledAt time.Time) {
	tracker.mu.Lock()
	tracker.metadataAvailable = false
	tracker.metadataSource = ""
	tracker.sampledAt = sampledAt
	tracker.mu.Unlock()
}

func (tracker *aircraftTracker) beginForwarding(format string) {
	tracker.mu.Lock()
	tracker.parserFormat = format
	tracker.beastParser.reset()
	tracker.sbsBuffer = nil
	tracker.mu.Unlock()
}

func (tracker *aircraftTracker) recordForwarded(payload []byte, format string, now time.Time) {
	if len(payload) == 0 {
		return
	}
	tracker.mu.Lock()
	defer tracker.mu.Unlock()
	if tracker.parserFormat != format {
		tracker.parserFormat = format
		tracker.beastParser.reset()
		tracker.sbsBuffer = nil
	}
	var addresses []string
	if format == "sbs" {
		addresses = tracker.parseSBS(payload)
	} else {
		addresses = tracker.beastParser.process(payload, func(address uint32) bool {
			_, exists := tracker.aircraft[fmtICAO(address)]
			return exists
		})
	}
	for _, icao := range addresses {
		current := tracker.aircraft[icao]
		if current == nil {
			current = &trackedAircraft{icao: icao, supported: true}
			tracker.aircraft[icao] = current
		}
		current.lastSentAt = now
		second := now.Unix()
		bucket := &current.sentRates[second%int64(len(current.sentRates))]
		if bucket.second != second {
			bucket.second = second
			bucket.frames = 0
		}
		bucket.frames++
	}
}

func fmtICAO(address uint32) string {
	const digits = "0123456789ABCDEF"
	result := []byte{'0', '0', '0', '0', '0', '0'}
	for index := len(result) - 1; index >= 0; index-- {
		result[index] = digits[address&0x0f]
		address >>= 4
	}
	return string(result)
}

func (tracker *aircraftTracker) parseSBS(payload []byte) []string {
	tracker.sbsBuffer = append(tracker.sbsBuffer, payload...)
	if len(tracker.sbsBuffer) > 64*1024 {
		if newline := bytes.LastIndexByte(tracker.sbsBuffer, '\n'); newline >= 0 {
			tracker.sbsBuffer = append([]byte(nil), tracker.sbsBuffer[newline+1:]...)
		} else {
			tracker.sbsBuffer = nil
		}
	}
	var addresses []string
	for {
		newline := bytes.IndexByte(tracker.sbsBuffer, '\n')
		if newline < 0 {
			break
		}
		line := strings.TrimSuffix(string(tracker.sbsBuffer[:newline]), "\r")
		tracker.sbsBuffer = tracker.sbsBuffer[newline+1:]
		fields := strings.Split(line, ",")
		if len(fields) < 5 || fields[0] != "MSG" {
			continue
		}
		icao, supported, ok := normalizeICAO(fields[4])
		if ok && supported {
			addresses = append(addresses, icao)
		}
	}
	return addresses
}

func (tracker *aircraftTracker) snapshot(now time.Time, feederState, errorMessage string) aircraftTrafficSnapshot {
	tracker.mu.Lock()
	defer tracker.mu.Unlock()
	result := aircraftTrafficSnapshot{
		MetadataAvailable: tracker.metadataAvailable,
		Sent:              make([]aircraftStatusRow, 0),
		NotSent:           make([]aircraftStatusRow, 0),
	}
	if tracker.metadataAvailable {
		result.Source = tracker.metadataSource
	}
	if !tracker.sampledAt.IsZero() {
		result.SampledAt = tracker.sampledAt.Format(time.RFC3339Nano)
	}
	activeMetadata := 0
	activeSent := 0
	for key, current := range tracker.aircraft {
		localActive := tracker.metadataAvailable && current.hasMetadata && now.Sub(current.lastLocalAt) <= aircraftRetention
		sentActive := !current.lastSentAt.IsZero() && now.Sub(current.lastSentAt) <= aircraftRetention
		if !localActive && !sentActive {
			delete(tracker.aircraft, key)
			continue
		}
		if localActive {
			activeMetadata++
		}
		matchesLatest := sentActive && !current.lastSentAt.Add(aircraftMatchTolerance).Before(current.lastLocalAt)
		row := current.snapshot(now)
		if localActive && !matchesLatest {
			row.ReasonCode, row.Reason = aircraftNotSentReason(current, feederState, errorMessage)
			result.NotSent = append(result.NotSent, row)
			continue
		}
		if sentActive {
			activeSent++
			result.Sent = append(result.Sent, row)
		}
	}
	sort.Slice(result.Sent, func(left, right int) bool {
		if result.Sent[left].SentMessagesPerSecond == result.Sent[right].SentMessagesPerSecond {
			return result.Sent[left].ICAO < result.Sent[right].ICAO
		}
		return result.Sent[left].SentMessagesPerSecond > result.Sent[right].SentMessagesPerSecond
	})
	sort.Slice(result.NotSent, func(left, right int) bool {
		return result.NotSent[left].LastSeenAt > result.NotSent[right].LastSeenAt
	})
	if tracker.metadataAvailable {
		count := activeMetadata
		result.Count = &count
	} else if activeSent > 0 {
		count := activeSent
		result.Count = &count
	}
	return result
}

func (aircraft *trackedAircraft) snapshot(now time.Time) aircraftStatusRow {
	row := aircraftStatusRow{
		ICAO: aircraft.icao, Callsign: aircraft.callsign, AltitudeFeet: aircraft.altitudeFeet,
		OnGround: aircraft.onGround, SpeedKnots: aircraft.speedKnots, TrackDegrees: aircraft.trackDegrees,
		DistanceNM: aircraft.distanceNM, RSSIDBFS: aircraft.rssiDBFS,
		LocalMessagesPerSecond: localMessageRate(aircraft.localRates),
		SentMessagesPerSecond:  sentMessageRate(aircraft.sentRates, now),
	}
	if !aircraft.lastLocalAt.IsZero() {
		row.LastSeenAt = aircraft.lastLocalAt.Format(time.RFC3339Nano)
	}
	if !aircraft.lastSentAt.IsZero() {
		row.LastSentAt = aircraft.lastSentAt.Format(time.RFC3339Nano)
	}
	return row
}

func localMessageRate(points []aircraftRatePoint) float64 {
	if len(points) < 2 {
		return 0
	}
	last := points[len(points)-1]
	cutoff := last.at.Add(-aircraftRateWindow)
	first := last
	for _, point := range points {
		if !point.at.Before(cutoff) {
			first = point
			break
		}
	}
	if last.messages < first.messages {
		return 0
	}
	seconds := last.at.Sub(first.at).Seconds()
	if seconds <= 0 {
		return 0
	}
	return float64(last.messages-first.messages) / seconds
}

func sentMessageRate(buckets [10]aircraftRateBucket, now time.Time) float64 {
	var frames uint64
	for _, bucket := range buckets {
		age := now.Unix() - bucket.second
		if age >= 0 && age < int64(len(buckets)) {
			frames += bucket.frames
		}
	}
	return float64(frames) / float64(len(buckets))
}

func aircraftNotSentReason(aircraft *trackedAircraft, feederState, errorMessage string) (string, string) {
	if !aircraft.supported {
		return "UNSUPPORTED_ADDRESS", "Address type cannot be matched safely"
	}
	if strings.Contains(errorMessage, "ADS-B source") {
		return "SOURCE_UNAVAILABLE", "Local ADS-B source is unavailable"
	}
	if feederState != "ready" && feederState != "active" {
		return "NO_ADSBPRO_CONNECTION", "Not connected to ADS-B.Pro"
	}
	return "NO_MATCHING_FRAME", "No matching frame was written to TLS"
}

func readAircraftCount(ctx context.Context, configuredSource string) (int, string, bool) {
	var body []byte
	var sourceName string
	var err error
	if configuredSource != "" {
		body, err = readAircraftDocument(ctx, configuredSource)
		sourceName = configuredSource
	} else {
		for _, path := range commonAircraftFiles {
			body, err = os.ReadFile(path)
			if err == nil {
				sourceName = path
				break
			}
		}
	}
	if err != nil || body == nil {
		return 0, "", false
	}
	count, err := countRecentAircraft(body)
	return count, sourceName, err == nil
}

func countRecentAircraft(body []byte) (int, error) {
	var document aircraftDocument
	if err := json.Unmarshal(body, &document); err != nil {
		return 0, err
	}
	if document.Aircraft == nil {
		return 0, errors.New("aircraft.json has no aircraft array")
	}
	count := 0
	for _, aircraft := range document.Aircraft {
		if aircraft.Seen == nil || *aircraft.Seen <= 60 {
			count++
		}
	}
	return count, nil
}

type beastAircraftParser struct {
	buffer []byte
}

func (parser *beastAircraftParser) reset() {
	parser.buffer = nil
}

func (parser *beastAircraftParser) process(payload []byte, knownAddress func(uint32) bool) []string {
	parser.buffer = append(parser.buffer, payload...)
	if len(parser.buffer) > 128*1024 {
		parser.buffer = append([]byte(nil), parser.buffer[len(parser.buffer)-64*1024:]...)
	}
	var addresses []string
	for {
		start := findBeastFrameStart(parser.buffer, 0)
		if start < 0 {
			if len(parser.buffer) > 0 && parser.buffer[len(parser.buffer)-1] == 0x1a {
				parser.buffer = append(parser.buffer[:0], 0x1a)
			} else {
				parser.buffer = nil
			}
			break
		}
		if start > 0 {
			parser.buffer = parser.buffer[start:]
		}
		if len(parser.buffer) < 2 {
			break
		}
		frameType := parser.buffer[1]
		if frameType == 0x34 {
			next := findNextBeastFrameStart(parser.buffer, 2)
			if next < 0 {
				if len(parser.buffer) > 256 {
					parser.buffer = parser.buffer[:2]
				}
				break
			}
			parser.buffer = parser.buffer[next:]
			continue
		}
		dataLength := 0
		switch frameType {
		case 0x31:
			dataLength = 2
		case 0x32:
			dataLength = 7
		case 0x33:
			dataLength = 14
		default:
			parser.buffer = parser.buffer[1:]
			continue
		}
		frame, consumed, complete := unescapeBeastFrame(parser.buffer, 7+dataLength)
		if !complete {
			break
		}
		if consumed <= 0 {
			parser.buffer = parser.buffer[1:]
			continue
		}
		parser.buffer = parser.buffer[consumed:]
		if frameType == 0x31 {
			continue
		}
		if address, ok := modeSAddress(frame[7:], knownAddress); ok {
			addresses = append(addresses, fmtICAO(address))
		}
	}
	return addresses
}

func findBeastFrameStart(buffer []byte, offset int) int {
	for index := offset; index+1 < len(buffer); index++ {
		if buffer[index] == 0x1a && buffer[index+1] >= 0x31 && buffer[index+1] <= 0x34 {
			return index
		}
	}
	return -1
}

func findNextBeastFrameStart(buffer []byte, offset int) int {
	for index := offset; index+1 < len(buffer); index++ {
		if buffer[index] != 0x1a {
			continue
		}
		if buffer[index+1] == 0x1a {
			index++
			continue
		}
		if buffer[index+1] >= 0x31 && buffer[index+1] <= 0x34 {
			return index
		}
	}
	return -1
}

func unescapeBeastFrame(buffer []byte, expected int) ([]byte, int, bool) {
	output := make([]byte, 0, expected)
	for index := 2; index < len(buffer) && len(output) < expected; {
		value := buffer[index]
		if value != 0x1a {
			output = append(output, value)
			index++
			if len(output) == expected {
				return output, index, true
			}
			continue
		}
		if index+1 >= len(buffer) {
			return nil, 0, false
		}
		if buffer[index+1] != 0x1a {
			return nil, -1, true
		}
		output = append(output, 0x1a)
		index += 2
		if len(output) == expected {
			return output, index, true
		}
	}
	return nil, 0, false
}

func modeSAddress(message []byte, knownAddress func(uint32) bool) (uint32, bool) {
	if len(message) != 7 && len(message) != 14 {
		return 0, false
	}
	df := message[0] >> 3
	checksum := modeSChecksum(message)
	switch df {
	case 17, 18:
		if checksum != 0 {
			return 0, false
		}
		address := uint32(message[1])<<16 | uint32(message[2])<<8 | uint32(message[3])
		return address, address != 0
	case 11:
		address := uint32(message[1])<<16 | uint32(message[2])<<8 | uint32(message[3])
		if address == 0 || checksum&0xffff80 != 0 {
			return 0, false
		}
		return address, true
	case 0, 4, 5, 16, 20, 21, 24, 25, 26, 27, 28, 29, 30, 31:
		if checksum != 0 && knownAddress(checksum) {
			return checksum, true
		}
	}
	return 0, false
}

func modeSChecksum(message []byte) uint32 {
	const polynomial uint32 = 0x1fff409
	var remainder uint32
	for _, value := range message {
		remainder ^= uint32(value) << 16
		for bit := 0; bit < 8; bit++ {
			remainder <<= 1
			if remainder&0x1000000 != 0 {
				remainder ^= polynomial
			}
		}
	}
	return remainder & 0xffffff
}
