// Package encryptor contains the port interface for the secret-encryption
// bounded context: AES-256-GCM encryption/decryption of TOTP secrets and
// other sensitive credentials. The contract references only stdlib types so
// the repository layer can implement it without importing internal/service.
// The service package keeps a type alias to the interface so existing call
// sites and test stubs continue to satisfy the contract.
package encryptor

// SecretEncryptor defines encryption operations for TOTP secrets.
type SecretEncryptor interface {
	Encrypt(plaintext string) (string, error)
	Decrypt(ciphertext string) (string, error)
}
