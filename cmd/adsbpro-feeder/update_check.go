package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type latestRelease struct {
	Version string `json:"version"`
}

func monitorUpdates(ctx context.Context, updateURL string, monitor *runtimeMonitor) {
	check := func() {
		checkCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		if latest, err := fetchLatestVersion(checkCtx, updateURL); err == nil {
			monitor.setLatest(latest, time.Now())
		}
	}
	check()
	ticker := time.NewTicker(6 * time.Hour)
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

func fetchLatestVersion(ctx context.Context, updateURL string) (string, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, updateURL, nil)
	if err != nil {
		return "", err
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", io.ErrUnexpectedEOF
	}
	var release latestRelease
	if err := json.NewDecoder(io.LimitReader(response.Body, 4096)).Decode(&release); err != nil {
		return "", err
	}
	if _, ok := semanticVersion(release.Version); !ok {
		return "", strconv.ErrSyntax
	}
	return release.Version, nil
}

func newerVersion(candidate, current string) bool {
	candidateParts, candidateOK := semanticVersion(candidate)
	currentParts, currentOK := semanticVersion(current)
	if !candidateOK || !currentOK {
		return false
	}
	for index := range candidateParts {
		if candidateParts[index] != currentParts[index] {
			return candidateParts[index] > currentParts[index]
		}
	}
	return false
}

func semanticVersion(value string) ([3]int, bool) {
	var result [3]int
	value = strings.TrimPrefix(strings.TrimSpace(value), "v")
	if dash := strings.IndexByte(value, '-'); dash >= 0 {
		value = value[:dash]
	}
	parts := strings.Split(value, ".")
	if len(parts) != 3 {
		return result, false
	}
	for index, part := range parts {
		parsed, err := strconv.Atoi(part)
		if err != nil || parsed < 0 {
			return result, false
		}
		result[index] = parsed
	}
	return result, true
}
