package main

import (
	"bufio"
	"encoding/hex"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/br3jski/radarview/internal/protocol"
)

func TestAircraftMovesFromSentToNotSentAndExpires(t *testing.T) {
	tracker := newAircraftTracker()
	start := time.Date(2026, 7, 29, 12, 0, 0, 0, time.UTC)
	firstMessages := uint64(100)
	tracker.updateMetadata([]aircraftMetadata{{
		ICAO: "40621D", Supported: true, Callsign: "TEST123",
		Messages: &firstMessages,
	}}, "/run/readsb/aircraft.json", start)
	frame := beastFrame(t, "8D40621D58C382D690C8AC2863A7")
	tracker.recordForwarded(frame[:7], "beast", start)
	tracker.recordForwarded(frame[7:], "beast", start)

	snapshot := tracker.snapshot(start, "active", "")
	if len(snapshot.Sent) != 1 || len(snapshot.NotSent) != 0 || snapshot.Sent[0].ICAO != "40621D" {
		t.Fatalf("unexpected initial classification: %#v", snapshot)
	}

	nextMessages := uint64(112)
	tracker.updateMetadata([]aircraftMetadata{{
		ICAO: "40621D", Supported: true, Callsign: "TEST123",
		Messages: &nextMessages,
	}}, "/run/readsb/aircraft.json", start.Add(6*time.Second))
	snapshot = tracker.snapshot(start.Add(6*time.Second), "active", "")
	if len(snapshot.Sent) != 0 || len(snapshot.NotSent) != 1 || snapshot.NotSent[0].ReasonCode != "NO_MATCHING_FRAME" {
		t.Fatalf("aircraft did not move to not-sent: %#v", snapshot)
	}
	if snapshot.NotSent[0].LocalMessagesPerSecond < 1.9 || snapshot.NotSent[0].LocalMessagesPerSecond > 2.1 {
		t.Fatalf("unexpected local message rate: %f", snapshot.NotSent[0].LocalMessagesPerSecond)
	}

	snapshot = tracker.snapshot(start.Add(37*time.Second), "active", "")
	if len(snapshot.Sent) != 0 || len(snapshot.NotSent) != 0 || snapshot.Count == nil || *snapshot.Count != 0 {
		t.Fatalf("aircraft was not pruned after 30 seconds: %#v", snapshot)
	}
}

func TestAircraftNotSentReasons(t *testing.T) {
	now := time.Now()
	for _, test := range []struct {
		name       string
		metadata   aircraftMetadata
		state      string
		err        string
		reasonCode string
	}{
		{"disconnected", aircraftMetadata{ICAO: "ABCDEF", Supported: true}, "disconnected", "server heartbeat timed out", "NO_ADSBPRO_CONNECTION"},
		{"source unavailable", aircraftMetadata{ICAO: "ABCDEF", Supported: true}, "disconnected", "ADS-B source: connection refused", "SOURCE_UNAVAILABLE"},
		{"unsupported", aircraftMetadata{ICAO: "~ABCDEF", Supported: false}, "active", "", "UNSUPPORTED_ADDRESS"},
	} {
		t.Run(test.name, func(t *testing.T) {
			tracker := newAircraftTracker()
			tracker.updateMetadata([]aircraftMetadata{test.metadata}, "/run/readsb/aircraft.json", now)
			snapshot := tracker.snapshot(now, test.state, test.err)
			if len(snapshot.NotSent) != 1 || snapshot.NotSent[0].ReasonCode != test.reasonCode {
				t.Fatalf("unexpected reason: %#v", snapshot.NotSent)
			}
		})
	}
}

func TestBeastParserHandlesEscapesChunksAndAddressParity(t *testing.T) {
	var parser beastAircraftParser
	explicitMessage := make([]byte, 14)
	copy(explicitMessage, []byte{0x8d, 0x40, 0x62, 0x1d, 0x1a, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06})
	setModeSParity(explicitMessage, 0)
	if checksum := modeSChecksum(explicitMessage); checksum != 0 {
		t.Fatalf("invalid explicit fixture checksum: %06X", checksum)
	}
	explicitFrame := encodeBeastFrame(explicitMessage)
	var got []string
	for _, chunk := range splitBytes(explicitFrame, 3) {
		got = append(got, parser.process(chunk, func(uint32) bool { return false })...)
	}
	if len(got) != 1 || got[0] != "40621D" {
		t.Fatalf("escaped explicit frame was not parsed: %#v", got)
	}

	address := uint32(0xABCDEF)
	addressParityMessage := []byte{0x20, 0x11, 0x22, 0x33, 0, 0, 0}
	setModeSParity(addressParityMessage, address)
	if checksum := modeSChecksum(addressParityMessage); checksum != address {
		t.Fatalf("invalid AP fixture checksum: %06X", checksum)
	}
	parser.reset()
	got = parser.process(encodeBeastFrame(addressParityMessage), func(candidate uint32) bool {
		return candidate == address
	})
	if len(got) != 1 || got[0] != "ABCDEF" {
		t.Fatalf("address/parity frame was not matched: %#v", got)
	}
	parser.reset()
	if unmatched := parser.process(encodeBeastFrame(addressParityMessage), func(uint32) bool { return false }); len(unmatched) != 0 {
		t.Fatalf("unknown AP address must be ignored: %#v", unmatched)
	}

	df11 := []byte{0x58, 0xAB, 0xCD, 0xEF, 0, 0, 0}
	setModeSParity(df11, 0x35)
	parser.reset()
	got = parser.process(encodeBeastFrame(df11), func(uint32) bool { return false })
	if len(got) != 1 || got[0] != "ABCDEF" {
		t.Fatalf("valid DF11 address was not parsed: %#v", got)
	}
	df11[len(df11)-1] ^= 0x80
	parser.reset()
	if corrupt := parser.process(encodeBeastFrame(df11), func(candidate uint32) bool {
		return candidate == 0xABCDEF
	}); len(corrupt) != 0 {
		t.Fatalf("corrupt DF11 must be ignored even when its address is known: %#v", corrupt)
	}

	corruptExplicit := append([]byte(nil), explicitMessage...)
	corruptExplicit[5] ^= 0x01
	parser.reset()
	if corrupt := parser.process(encodeBeastFrame(corruptExplicit), func(uint32) bool { return true }); len(corrupt) != 0 {
		t.Fatalf("corrupt DF17 must be ignored: %#v", corrupt)
	}
}

func TestForwardingParserDoesNotJoinFramesAcrossSessions(t *testing.T) {
	tracker := newAircraftTracker()
	now := time.Now()
	frame := beastFrame(t, "8D40621D58C382D690C8AC2863A7")
	split := len(frame) / 2

	tracker.beginForwarding("beast")
	tracker.recordForwarded(frame[:split], "beast", now)
	tracker.beginForwarding("beast")
	tracker.recordForwarded(frame[split:], "beast", now)
	if snapshot := tracker.snapshot(now, "active", ""); len(snapshot.Sent) != 0 {
		t.Fatalf("bytes from separate TLS sessions formed a frame: %#v", snapshot.Sent)
	}
	tracker.recordForwarded(frame, "beast", now)
	if snapshot := tracker.snapshot(now, "active", ""); len(snapshot.Sent) != 1 {
		t.Fatalf("complete frame after reconnect was not parsed: %#v", snapshot.Sent)
	}
}

func TestSBSParserHandlesChunkBoundariesAndInvalidLines(t *testing.T) {
	tracker := newAircraftTracker()
	now := time.Now()
	tracker.recordForwarded([]byte("garbage\nMSG,3,1,1,ABC"), "sbs", now)
	tracker.recordForwarded([]byte("DEF,1,2\r\nMSG,3,1,1,nothex,1\n"), "sbs", now)
	snapshot := tracker.snapshot(now, "active", "")
	if len(snapshot.Sent) != 1 || snapshot.Sent[0].ICAO != "ABCDEF" {
		t.Fatalf("unexpected SBS aircraft: %#v", snapshot.Sent)
	}
}

func TestMonitoredWriterTracksOnlySuccessfullyWrittenBytes(t *testing.T) {
	monitor := testMonitor(t)
	now := time.Now()
	messages := uint64(1)
	monitor.aircraftTracker.updateMetadata([]aircraftMetadata{{
		ICAO: "40621D", Supported: true, Messages: &messages,
	}}, "/run/readsb/aircraft.json", now)
	frame := beastFrame(t, "8D40621D58C382D690C8AC2863A7")
	sink := &shortWriter{limit: len(frame) / 2}
	writer := &monitoredWriter{Writer: sink, monitor: monitor, format: "beast"}
	first, err := writer.Write(frame)
	if err != nil || first != len(frame)/2 {
		t.Fatalf("unexpected partial write: %d, %v", first, err)
	}
	if snapshot := monitor.snapshot(); len(snapshot.Aircraft.Sent) != 0 {
		t.Fatalf("incomplete frame must not be reported as sent: %#v", snapshot.Aircraft.Sent)
	}
	sink.limit = len(frame)
	second, err := writer.Write(frame[first:])
	if err != nil || second != len(frame)-first {
		t.Fatalf("unexpected second write: %d, %v", second, err)
	}
	if snapshot := monitor.snapshot(); len(snapshot.Aircraft.Sent) != 1 {
		t.Fatalf("completed frame was not reported as sent: %#v", snapshot.Aircraft)
	}
}

func TestAircraftMetadataSupportsGroundAndOptionalFields(t *testing.T) {
	values, err := parseAircraftMetadata([]byte(`{"aircraft":[
		{"hex":"abc123","flight":" TEST ","alt_baro":"ground","gs":12.5,"track":90,"r_dst":1.25,"rssi":-8.5,"messages":42,"seen":0.2},
		{"hex":"~def456","seen":1},
		{"hex":"invalid"}
	]}`))
	if err != nil || len(values) != 2 {
		t.Fatalf("unexpected metadata: %#v, %v", values, err)
	}
	if values[0].ICAO != "ABC123" || values[0].Callsign != "TEST" || !values[0].OnGround || values[0].SpeedKnots == nil {
		t.Fatalf("metadata fields were not decoded: %#v", values[0])
	}
	if values[1].ICAO != "~DEF456" || values[1].Supported {
		t.Fatalf("non-ICAO address was not marked unsupported: %#v", values[1])
	}
}

func TestUnavailableAircraftJSONKeepsOutgoingDiagnostics(t *testing.T) {
	tracker := newAircraftTracker()
	now := time.Now()
	tracker.setMetadataUnavailable(now)
	tracker.recordForwarded(beastFrame(t, "8D40621D58C382D690C8AC2863A7"), "beast", now)
	snapshot := tracker.snapshot(now, "active", "")
	if snapshot.MetadataAvailable || len(snapshot.Sent) != 1 || len(snapshot.NotSent) != 0 {
		t.Fatalf("outgoing-only diagnostics were lost: %#v", snapshot)
	}
}

func TestLocalMessageRateRecoversAfterCounterReset(t *testing.T) {
	start := time.Now()
	points := appendLocalRate(nil, start, 100)
	points = appendLocalRate(points, start.Add(2*time.Second), 110)
	if rate := localMessageRate(points); rate != 5 {
		t.Fatalf("unexpected pre-reset rate: %f", rate)
	}
	points = appendLocalRate(points, start.Add(4*time.Second), 2)
	points = appendLocalRate(points, start.Add(6*time.Second), 8)
	if rate := localMessageRate(points); rate != 3 {
		t.Fatalf("counter reset created a rate spike: %f", rate)
	}

	points = nil
	for second := 0; second <= 12; second += 2 {
		points = appendLocalRate(points, start.Add(time.Duration(second)*time.Second), uint64(second*4))
	}
	if rate := localMessageRate(points); rate != 4 {
		t.Fatalf("local rate was not limited to the 10-second window: %f", rate)
	}
}

func TestServerHeartbeatReadTimeoutAndRefresh(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()
	reader := bufio.NewReader(client)
	go func() {
		_ = protocol.WriteControl(server, protocol.ServerMessage{Type: "HEARTBEAT", SentAt: time.Now().Format(time.RFC3339Nano)})
	}()
	message, err := readSessionMessage(reader, client, true, 25*time.Millisecond)
	if err != nil || message.Type != "HEARTBEAT" {
		t.Fatalf("heartbeat was not read: %#v, %v", message, err)
	}
	start := time.Now()
	_, err = readSessionMessage(reader, client, true, 25*time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), "server heartbeat timed out") {
		t.Fatalf("missing heartbeat did not time out: %v", err)
	}
	if time.Since(start) > 250*time.Millisecond {
		t.Fatalf("heartbeat timeout took too long: %s", time.Since(start))
	}
}

func beastFrame(t *testing.T, modeSHex string) []byte {
	t.Helper()
	message, err := hex.DecodeString(modeSHex)
	if err != nil {
		t.Fatal(err)
	}
	return encodeBeastFrame(message)
}

func encodeBeastFrame(message []byte) []byte {
	frameType := byte(0x32)
	if len(message) == 14 {
		frameType = 0x33
	}
	unescaped := append([]byte{0, 0, 0, 0, 0, 1, 0x80}, message...)
	result := []byte{0x1a, frameType}
	for _, value := range unescaped {
		result = append(result, value)
		if value == 0x1a {
			result = append(result, value)
		}
	}
	return result
}

func setModeSParity(message []byte, syndrome uint32) {
	message[len(message)-3] = 0
	message[len(message)-2] = 0
	message[len(message)-1] = 0
	base := modeSChecksum(message)
	var basis [24]uint32
	var combinations [24]uint32
	for variable := 0; variable < 24; variable++ {
		byteIndex := len(message) - 3 + variable/8
		mask := byte(1 << (7 - variable%8))
		message[byteIndex] ^= mask
		effect := modeSChecksum(message) ^ base
		message[byteIndex] ^= mask
		combination := uint32(1) << variable
		for bit := 23; bit >= 0; bit-- {
			if effect&(uint32(1)<<bit) == 0 {
				continue
			}
			if basis[bit] != 0 {
				effect ^= basis[bit]
				combination ^= combinations[bit]
				continue
			}
			basis[bit] = effect
			combinations[bit] = combination
			break
		}
	}
	target := base ^ syndrome
	var parityBits uint32
	for bit := 23; bit >= 0; bit-- {
		if target&(uint32(1)<<bit) == 0 {
			continue
		}
		if basis[bit] == 0 {
			panic("Mode-S parity matrix is not invertible")
		}
		target ^= basis[bit]
		parityBits ^= combinations[bit]
	}
	for variable := 0; variable < 24; variable++ {
		if parityBits&(uint32(1)<<variable) != 0 {
			message[len(message)-3+variable/8] |= byte(1 << (7 - variable%8))
		}
	}
}

func splitBytes(value []byte, size int) [][]byte {
	var result [][]byte
	for len(value) > 0 {
		next := size
		if next > len(value) {
			next = len(value)
		}
		result = append(result, append([]byte(nil), value[:next]...))
		value = value[next:]
	}
	return result
}

type shortWriter struct {
	limit int
}

func (writer *shortWriter) Write(value []byte) (int, error) {
	if writer.limit <= 0 {
		return 0, io.ErrShortWrite
	}
	if len(value) > writer.limit {
		return writer.limit, nil
	}
	return len(value), nil
}
