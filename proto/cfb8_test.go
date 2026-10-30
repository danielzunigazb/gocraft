package proto

import (
	"bytes"
	"crypto/aes"
	"encoding/hex"
	"testing"
)

// NIST SP 800-38A, F.3.7: CFB8-AES128.Encrypt, the first 18 bytes.
func TestCFB8KnownVector(t *testing.T) {
	key, _ := hex.DecodeString("2b7e151628aed2a6abf7158809cf4f3c")
	iv, _ := hex.DecodeString("000102030405060708090a0b0c0d0e0f")
	plain, _ := hex.DecodeString("6bc1bee22e409f96e93d7e117393172aae2d")
	want, _ := hex.DecodeString("3b79424c9c0dd436bace9e0ed4586a4f32b9")
	block, _ := aes.NewCipher(key)
	got := make([]byte, len(plain))
	NewCFB8(block, iv, false).XORKeyStream(got, plain)
	if !bytes.Equal(got, want) {
		t.Fatalf("encrypted %x, want %x", got, want)
	}
	back := make([]byte, len(got))
	NewCFB8(block, iv, true).XORKeyStream(back, got)
	if !bytes.Equal(back, plain) {
		t.Fatalf("decrypted %x", back)
	}
}

// The stream carries state across calls: writing in pieces gives the same bytes as writing at once.
func TestCFB8PiecesMatchWhole(t *testing.T) {
	key := bytes.Repeat([]byte{7}, 16)
	block, _ := aes.NewCipher(key)
	msg := bytes.Repeat([]byte("minecraft "), 20)
	whole := make([]byte, len(msg))
	NewCFB8(block, key, false).XORKeyStream(whole, msg)
	s := NewCFB8(block, key, false)
	pieces := make([]byte, len(msg))
	for i := 0; i < len(msg); i += 7 {
		j := min(i+7, len(msg))
		s.XORKeyStream(pieces[i:j], msg[i:j])
	}
	if !bytes.Equal(whole, pieces) {
		t.Fatal("pieces differ from the whole")
	}
}
