package main

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/br3jski/radarview/internal/identity"
	"github.com/br3jski/radarview/internal/protocol"
	"github.com/br3jski/radarview/internal/source"
)

var version = "2.1.0"

type config struct {
	ServerAddress string
	ServerName    string
	Source        source.Config
	DataDir       string
	TokenFile     string
	Label         string
}

type status struct {
	State          string    `json:"state"`
	InstallationID string    `json:"installationId,omitempty"`
	InputFormat    string    `json:"inputFormat,omitempty"`
	UpdatedAt      time.Time `json:"updatedAt"`
	Error          string    `json:"error,omitempty"`
}

func main() {
	configuration := loadConfig()
	command := "run"
	if len(os.Args) > 1 {
		command = os.Args[1]
		switch command {
		case "run", "service":
		case "status":
			value, err := os.ReadFile(filepath.Join(configuration.DataDir, "status.json"))
			if err != nil {
				log.Fatal(err)
			}
			fmt.Println(string(value))
			return
		case "version":
			fmt.Println(version)
			return
		default:
			log.Fatalf("usage: %s [run|service|status|version]", filepath.Base(os.Args[0]))
		}
	}
	if command == "service" {
		if err := configureServiceLogging(configuration.DataDir); err != nil {
			log.Fatal(err)
		}
	}
	identityValue, err := identity.LoadOrCreate(configuration.DataDir)
	if err != nil {
		log.Fatal(err)
	}
	run := func(ctx context.Context) error {
		return runLoop(ctx, configuration, identityValue)
	}
	if command == "service" {
		if err := runPlatformService(run); err != nil {
			log.Fatal(err)
		}
		return
	}
	if err := run(context.Background()); err != nil && !errors.Is(err, context.Canceled) {
		log.Fatal(err)
	}
}

func runLoop(ctx context.Context, configuration config, identityValue *identity.Identity) error {
	delay := time.Second
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := runSession(ctx, configuration, identityValue, func() { delay = time.Second }); err != nil {
			if errors.Is(err, context.Canceled) {
				return err
			}
			writeStatus(configuration.DataDir, status{State: "disconnected", InstallationID: identityValue.InstallationID, UpdatedAt: time.Now(), Error: err.Error()})
			log.Printf("session ended: %v", err)
		}
		jitter := time.Duration(rand.Int63n(int64(delay)))
		timer := time.NewTimer(jitter)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
		if delay < 60*time.Second {
			delay *= 2
			if delay > 60*time.Second {
				delay = 60 * time.Second
			}
		}
	}
}

func runSession(parent context.Context, configuration config, identityValue *identity.Identity, onActive func()) error {
	sourceConnection, err := source.Connect(configuration.Source)
	if err != nil {
		return fmt.Errorf("ADS-B source: %w", err)
	}
	defer sourceConnection.Close()
	spki, err := identityValue.PublicSPKI()
	if err != nil {
		return err
	}
	pairedPath := filepath.Join(configuration.DataDir, "paired")
	pairedValue, pairedErr := os.ReadFile(pairedPath)
	paired := pairedErr == nil
	if paired && strings.TrimSpace(string(pairedValue)) != identityValue.InstallationID {
		return errors.New("paired marker does not match installation identity")
	}
	if pairedErr != nil && !errors.Is(pairedErr, os.ErrNotExist) {
		return pairedErr
	}
	token, tokenErr := os.ReadFile(configuration.TokenFile)
	operation := "authenticate"
	if !paired {
		if tokenErr != nil {
			return errors.New("pairing token is required for the first connection")
		}
		operation = "pair"
	}
	hello := protocol.Hello{
		Type: "HELLO", ProtocolVersion: 2, Operation: operation,
		InstallationID: identityValue.InstallationID, KeyFingerprint: protocol.Fingerprint(spki),
		InputFormat: sourceConnection.Format, ClientVersion: version, Label: configuration.Label,
	}
	if operation == "pair" {
		hello.PublicKeySPKI = base64.RawURLEncoding.EncodeToString(spki)
	}
	dialer := &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 60 * time.Second}
	tlsConnection, err := tls.DialWithDialer(dialer, "tcp", configuration.ServerAddress, &tls.Config{
		ServerName: configuration.ServerName, MinVersion: tls.VersionTLS12,
	})
	if err != nil {
		return err
	}
	defer tlsConnection.Close()
	reader := bufio.NewReader(tlsConnection)
	if _, err := io.WriteString(tlsConnection, protocol.Magic); err != nil {
		return err
	}
	if err := protocol.WriteControl(tlsConnection, hello); err != nil {
		return err
	}
	var challenge protocol.Challenge
	if err := protocol.ReadControl(reader, &challenge); err != nil {
		return err
	}
	if challenge.Type != "CHALLENGE" || challenge.ServerName != configuration.ServerName {
		return errors.New("invalid server challenge")
	}
	expiresAt, err := time.Parse(time.RFC3339Nano, challenge.ExpiresAt)
	if err != nil || !expiresAt.After(time.Now()) || expiresAt.After(time.Now().Add(time.Minute)) {
		return errors.New("invalid server challenge expiry")
	}
	signature, err := protocol.Sign(identityValue.PrivateKey, protocol.CanonicalProof(hello, challenge))
	if err != nil {
		return err
	}
	proof := protocol.Proof{Type: "PROOF", Signature: signature}
	if operation == "pair" {
		proof.Token = strings.TrimSpace(string(token))
	}
	if err := protocol.WriteControl(tlsConnection, proof); err != nil {
		return err
	}
	var ready protocol.ServerMessage
	if err := protocol.ReadControl(reader, &ready); err != nil {
		return err
	}
	if ready.Type == "ERROR" {
		return fmt.Errorf("server rejected connection: %s", ready.Code)
	}
	if ready.Type != "READY" {
		return errors.New("server did not confirm readiness")
	}
	if operation == "pair" {
		if err := os.WriteFile(pairedPath, []byte(ready.InstallationID), 0600); err != nil {
			return err
		}
	}
	writeStatus(configuration.DataDir, status{State: "ready", InstallationID: identityValue.InstallationID, InputFormat: sourceConnection.Format, UpdatedAt: time.Now()})

	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	var closeOnce sync.Once
	closeConnection := func() { closeOnce.Do(func() { cancel(); _ = tlsConnection.Close() }) }
	serverResult := make(chan error, 1)
	activeSignal := make(chan struct{}, 1)
	go func() {
		active := false
		for {
			var message protocol.ServerMessage
			if err := protocol.ReadControl(reader, &message); err != nil {
				serverResult <- err
				return
			}
			if message.Type == "ERROR" {
				serverResult <- fmt.Errorf("server ended session: %s", message.Code)
				return
			}
			if message.Type != "ACTIVE" || active {
				serverResult <- errors.New("unexpected server message")
				return
			}
			active = true
			if err := os.Remove(configuration.TokenFile); err != nil && !errors.Is(err, os.ErrNotExist) {
				serverResult <- err
				return
			}
			writeStatus(configuration.DataDir, status{State: "active", InstallationID: identityValue.InstallationID, InputFormat: sourceConnection.Format, UpdatedAt: time.Now()})
			activeSignal <- struct{}{}
		}
	}()
	if len(sourceConnection.Prefetched) > 0 {
		if _, err := tlsConnection.Write(sourceConnection.Prefetched); err != nil {
			return err
		}
	}
	copyResult := make(chan error, 1)
	go func() { _, err := io.Copy(tlsConnection, sourceConnection); copyResult <- err }()
	for {
		select {
		case err := <-serverResult:
			closeConnection()
			return err
		case <-activeSignal:
			onActive()
			activeSignal = nil
		case err := <-copyResult:
			closeConnection()
			if err != nil {
				return err
			}
			return errors.New("ADS-B source closed")
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func loadConfig() config {
	values := map[string]string{}
	path := env("ADSBPRO_CONFIG", defaultConfigPath())
	if data, err := os.ReadFile(path); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 {
				values[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
			}
		}
	}
	get := func(key, fallback string) string {
		if value := os.Getenv(key); value != "" {
			return value
		}
		if value := values[key]; value != "" {
			return value
		}
		return fallback
	}
	port := func(key string, fallback int) int {
		value, err := strconv.Atoi(get(key, fmt.Sprint(fallback)))
		if err != nil {
			return fallback
		}
		return value
	}
	dataDir := get("DATA_DIR", defaultDataDir())
	return config{
		ServerAddress: get("SERVER_ADDR", "feed.ads-b.pro:48582"), ServerName: get("SERVER_NAME", "feed.ads-b.pro"),
		Source:  source.Config{Host: get("SOURCE_HOST", "127.0.0.1"), Mode: get("SOURCE_MODE", "auto"), BeastPort: port("BEAST_PORT", 30005), SBSPort: port("SBS_PORT", 30003)},
		DataDir: dataDir, TokenFile: get("TOKEN_FILE", filepath.Join(dataDir, "pairing-token")), Label: get("FEEDER_LABEL", "ADS-B feeder"),
	}
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
func writeStatus(dataDir string, value status) {
	body, _ := json.Marshal(value)
	temporary := filepath.Join(dataDir, "status.json.new")
	if os.WriteFile(temporary, body, 0600) == nil {
		_ = os.Rename(temporary, filepath.Join(dataDir, "status.json"))
	}
}
