package identity

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

type Identity struct {
	InstallationID string
	PrivateKey     *ecdsa.PrivateKey
}

func LoadOrCreate(dataDir string) (*Identity, error) {
	if err := os.MkdirAll(dataDir, 0700); err != nil {
		return nil, err
	}
	idPath := filepath.Join(dataDir, "installation-id")
	keyPath := filepath.Join(dataDir, "identity-key.pem")
	id, idErr := os.ReadFile(idPath)
	keyPEM, keyErr := os.ReadFile(keyPath)
	if idErr == nil && keyErr == nil {
		if err := os.Chmod(keyPath, 0600); err != nil {
			return nil, err
		}
		if err := os.Chmod(idPath, 0600); err != nil {
			return nil, err
		}
		block, _ := pem.Decode(keyPEM)
		if block == nil {
			return nil, errors.New("invalid identity key PEM")
		}
		key, err := x509.ParseECPrivateKey(block.Bytes)
		if err != nil {
			return nil, err
		}
		return &Identity{InstallationID: string(id), PrivateKey: key}, nil
	}
	if !errors.Is(idErr, os.ErrNotExist) || !errors.Is(keyErr, os.ErrNotExist) {
		return nil, errors.New("partial or unreadable feeder identity; refusing to replace it")
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	installationID, err := randomUUID()
	if err != nil {
		return nil, err
	}
	der, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, err
	}
	if err := atomicWrite(keyPath, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: der}), 0600); err != nil {
		return nil, err
	}
	if err := atomicWrite(idPath, []byte(installationID), 0600); err != nil {
		return nil, err
	}
	return &Identity{InstallationID: installationID, PrivateKey: key}, nil
}

func (i *Identity) PublicSPKI() ([]byte, error) {
	return x509.MarshalPKIXPublicKey(&i.PrivateKey.PublicKey)
}

func randomUUID() (string, error) {
	value := make([]byte, 16)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	value[6] = (value[6] & 0x0f) | 0x40
	value[8] = (value[8] & 0x3f) | 0x80
	hexValue := hex.EncodeToString(value)
	return fmt.Sprintf("%s-%s-%s-%s-%s", hexValue[0:8], hexValue[8:12], hexValue[12:16], hexValue[16:20], hexValue[20:32]), nil
}

func atomicWrite(path string, data []byte, mode os.FileMode) error {
	temp := path + ".new"
	if err := os.WriteFile(temp, data, mode); err != nil {
		return err
	}
	if err := os.Chmod(temp, mode); err != nil {
		return err
	}
	return os.Rename(temp, path)
}
