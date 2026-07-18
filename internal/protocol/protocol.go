package protocol

import (
	"bufio"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
)

const Magic = "ADSBPRO/2\n"
const MaxControlSize = 16 * 1024

type Hello struct {
	Type            string `json:"type"`
	ProtocolVersion int    `json:"protocolVersion"`
	Operation       string `json:"operation"`
	InstallationID  string `json:"installationId"`
	KeyFingerprint  string `json:"keyFingerprint"`
	PublicKeySPKI   string `json:"publicKeySpki,omitempty"`
	InputFormat     string `json:"inputFormat"`
	ClientVersion   string `json:"clientVersion"`
	Label           string `json:"label,omitempty"`
}

type Challenge struct {
	Type        string `json:"type"`
	ChallengeID string `json:"challengeId"`
	Nonce       string `json:"nonce"`
	IssuedAt    string `json:"issuedAt"`
	ExpiresAt   string `json:"expiresAt"`
	ServerName  string `json:"serverName"`
}

type Proof struct {
	Type      string `json:"type"`
	Signature string `json:"signature"`
	Token     string `json:"token,omitempty"`
}

type ServerMessage struct {
	Type            string `json:"type"`
	Code            string `json:"code,omitempty"`
	ProtocolVersion int    `json:"protocolVersion,omitempty"`
	InstallationID  string `json:"installationId,omitempty"`
	ActivatedAt     string `json:"activatedAt,omitempty"`
}

func CanonicalProof(hello Hello, challenge Challenge) []byte {
	return []byte(fmt.Sprintf("ADSBPRO-FEEDER-V2\noperation=%s\nchallengeId=%s\nnonce=%s\ninstallationId=%s\nkeyFingerprint=%s\ninputFormat=%s\nclientVersion=%s\nserverName=%s\nissuedAt=%s\nexpiresAt=%s",
		hello.Operation, challenge.ChallengeID, challenge.Nonce, hello.InstallationID,
		hello.KeyFingerprint, hello.InputFormat, hello.ClientVersion, challenge.ServerName,
		challenge.IssuedAt, challenge.ExpiresAt))
}

func Fingerprint(spki []byte) string {
	sum := sha256.Sum256(spki)
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func Sign(key *ecdsa.PrivateKey, payload []byte) (string, error) {
	digest := sha256.Sum256(payload)
	r, s, err := ecdsa.Sign(rand.Reader, key, digest[:])
	if err != nil {
		return "", err
	}
	value := make([]byte, 64)
	writePadded(value[:32], r)
	writePadded(value[32:], s)
	return base64.RawURLEncoding.EncodeToString(value), nil
}

func Verify(key *ecdsa.PublicKey, payload []byte, signature string) bool {
	value, err := base64.RawURLEncoding.DecodeString(signature)
	if err != nil || len(value) != 64 {
		return false
	}
	digest := sha256.Sum256(payload)
	return ecdsa.Verify(key, digest[:], new(big.Int).SetBytes(value[:32]), new(big.Int).SetBytes(value[32:]))
}

func writePadded(destination []byte, value *big.Int) {
	bytes := value.Bytes()
	copy(destination[len(destination)-len(bytes):], bytes)
}

func WriteControl(writer io.Writer, value any) error {
	body, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if len(body) > MaxControlSize {
		return errors.New("control message too large")
	}
	header := make([]byte, 4)
	binary.BigEndian.PutUint32(header, uint32(len(body)))
	if _, err := writer.Write(header); err != nil {
		return err
	}
	_, err = writer.Write(body)
	return err
}

func ReadControl(reader *bufio.Reader, destination any) error {
	header := make([]byte, 4)
	if _, err := io.ReadFull(reader, header); err != nil {
		return err
	}
	size := binary.BigEndian.Uint32(header)
	if size < 2 || size > MaxControlSize {
		return errors.New("invalid control message length")
	}
	body := make([]byte, size)
	if _, err := io.ReadFull(reader, body); err != nil {
		return err
	}
	return json.Unmarshal(body, destination)
}
