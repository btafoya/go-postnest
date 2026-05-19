package crypto

import (
	"crypto/rand"
	"encoding/base64"
	"strings"
	"testing"
)

func testKey(t *testing.T) string {
	t.Helper()
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(b)
}

func TestRoundTrip(t *testing.T) {
	c, err := NewCipher(testKey(t))
	if err != nil {
		t.Fatal(err)
	}
	for _, pt := range []string{"hello", "CF_DNS_API_TOKEN=abc123", strings.Repeat("x", 4096)} {
		enc, err := c.Encrypt(pt)
		if err != nil {
			t.Fatalf("encrypt: %v", err)
		}
		if enc == pt {
			t.Fatal("ciphertext equals plaintext")
		}
		dec, err := c.Decrypt(enc)
		if err != nil {
			t.Fatalf("decrypt: %v", err)
		}
		if dec != pt {
			t.Fatalf("round trip mismatch: got %q want %q", dec, pt)
		}
	}
}

func TestEmptyPassthrough(t *testing.T) {
	c, _ := NewCipher(testKey(t))
	enc, err := c.Encrypt("")
	if err != nil || enc != "" {
		t.Fatalf("empty encrypt: %q %v", enc, err)
	}
	dec, err := c.Decrypt("")
	if err != nil || dec != "" {
		t.Fatalf("empty decrypt: %q %v", dec, err)
	}
}

func TestTamperDetected(t *testing.T) {
	c, _ := NewCipher(testKey(t))
	enc, _ := c.Encrypt("secret")
	raw, _ := base64.StdEncoding.DecodeString(enc)
	raw[len(raw)-1] ^= 0xFF
	if _, err := c.Decrypt(base64.StdEncoding.EncodeToString(raw)); err == nil {
		t.Fatal("expected tamper to fail decryption")
	}
}

func TestWrongKeyFails(t *testing.T) {
	c1, _ := NewCipher(testKey(t))
	c2, _ := NewCipher(testKey(t))
	enc, _ := c1.Encrypt("secret")
	if _, err := c2.Decrypt(enc); err == nil {
		t.Fatal("expected decryption with wrong key to fail")
	}
}

func TestBadKey(t *testing.T) {
	if _, err := NewCipher(""); err != ErrNoKey {
		t.Fatalf("empty key: got %v want ErrNoKey", err)
	}
	if _, err := NewCipher("not-base64!!!"); err == nil {
		t.Fatal("expected decode error")
	}
	short := base64.StdEncoding.EncodeToString(make([]byte, 16))
	if _, err := NewCipher(short); err == nil {
		t.Fatal("expected key length error")
	}
}
