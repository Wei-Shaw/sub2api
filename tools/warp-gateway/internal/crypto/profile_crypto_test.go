package crypto

import "testing"

func TestProfileCipherRoundTrip(t *testing.T) {
	c, err := NewProfileCipher("test-secret-key")
	if err != nil {
		t.Fatal(err)
	}
	plain := "6LUwhhsNyuK+j10kcq3TTWzxS7iMS6VwoUAOaVXta2s="
	enc, err := c.EncryptString(plain)
	if err != nil {
		t.Fatal(err)
	}
	if enc == plain || !IsEncrypted(enc) {
		t.Fatalf("expected encrypted payload, got %q", enc)
	}
	dec, err := c.DecryptString(enc)
	if err != nil {
		t.Fatal(err)
	}
	if dec != plain {
		t.Fatalf("decrypt mismatch: %q vs %q", dec, plain)
	}
	// idempotent encrypt
	enc2, err := c.EncryptString(enc)
	if err != nil || enc2 != enc {
		t.Fatalf("re-encrypt changed ciphertext")
	}
}
