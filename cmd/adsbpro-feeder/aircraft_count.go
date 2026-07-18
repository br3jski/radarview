package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
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
	Aircraft []struct {
		Seen *float64 `json:"seen"`
	} `json:"aircraft"`
}

func monitorAircraft(ctx context.Context, configuredSource string, monitor *runtimeMonitor) {
	check := func() {
		count, sourceName, ok := readAircraftCount(ctx, configuredSource)
		if ok {
			monitor.setAircraft(&count, sourceName)
		} else {
			monitor.setAircraft(nil, "")
		}
	}
	check()
	ticker := time.NewTicker(5 * time.Second)
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

func readAircraftCount(ctx context.Context, configuredSource string) (int, string, bool) {
	if configuredSource != "" {
		body, err := readAircraftDocument(ctx, configuredSource)
		if err != nil {
			return 0, "", false
		}
		count, err := countRecentAircraft(body)
		return count, configuredSource, err == nil
	}
	for _, path := range commonAircraftFiles {
		body, err := os.ReadFile(path)
		if err == nil {
			count, parseErr := countRecentAircraft(body)
			if parseErr == nil {
				return count, path, true
			}
		}
	}
	return 0, "", false
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
	return io.ReadAll(io.LimitReader(response.Body, 8*1024*1024))
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
