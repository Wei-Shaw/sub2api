package upstreamstation

import (
	"encoding/json"
	"errors"
	"fmt"

	coreservice "github.com/Wei-Shaw/sub2api/internal/service"
)

type CredentialCodec struct {
	encryptor coreservice.SecretEncryptor
}

func NewCredentialCodec(encryptor coreservice.SecretEncryptor) *CredentialCodec {
	return &CredentialCodec{encryptor: encryptor}
}

func (c *CredentialCodec) Encrypt(credentials Credentials) (string, error) {
	if c == nil || c.encryptor == nil {
		return "", errors.New("upstream credential encryptor is required")
	}
	data, err := json.Marshal(credentials)
	if err != nil {
		return "", fmt.Errorf("encode upstream credentials: %w", err)
	}
	ciphertext, err := c.encryptor.Encrypt(string(data))
	if err != nil {
		return "", fmt.Errorf("encrypt upstream credentials: %w", err)
	}
	return ciphertext, nil
}

func (c *CredentialCodec) Decrypt(ciphertext string) (Credentials, error) {
	if c == nil || c.encryptor == nil {
		return Credentials{}, errors.New("upstream credential encryptor is required")
	}
	if ciphertext == "" {
		return Credentials{}, errors.New("upstream credentials are not configured")
	}
	plaintext, err := c.encryptor.Decrypt(ciphertext)
	if err != nil {
		return Credentials{}, fmt.Errorf("decrypt upstream credentials: %w", err)
	}
	var credentials Credentials
	if err := json.Unmarshal([]byte(plaintext), &credentials); err != nil {
		return Credentials{}, fmt.Errorf("decode upstream credentials: %w", err)
	}
	return credentials, nil
}
