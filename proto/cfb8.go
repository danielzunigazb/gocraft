package proto

import (
	"crypto/aes"
	"crypto/cipher"
	"io"
	"net"
)

// Online-mode connections are encrypted with AES-128 in CFB8 mode, the shared secret doubling as the
// IV. The Go standard library has only full-block CFB, so the byte-wise mode is written here.

type cfb8 struct {
	b       cipher.Block
	iv      [aes.BlockSize]byte
	decrypt bool
}

// NewCFB8 returns a CFB8 stream over the block cipher with the given IV.
func NewCFB8(block cipher.Block, iv []byte, decrypt bool) cipher.Stream {
	x := &cfb8{b: block, decrypt: decrypt}
	copy(x.iv[:], iv)
	return x
}

func (x *cfb8) XORKeyStream(dst, src []byte) {
	var ks [aes.BlockSize]byte
	for i, c := range src {
		x.b.Encrypt(ks[:], x.iv[:])
		out := c ^ ks[0]
		copy(x.iv[:], x.iv[1:])
		if x.decrypt {
			x.iv[aes.BlockSize-1] = c
		} else {
			x.iv[aes.BlockSize-1] = out
		}
		dst[i] = out
	}
}

// Encrypted wraps a connection so that everything read is decrypted and everything written encrypted,
// with the 16-byte shared secret as key and IV.
func Encrypted(c net.Conn, secret []byte) (io.Reader, io.Writer, error) {
	block, err := aes.NewCipher(secret)
	if err != nil {
		return nil, nil, err
	}
	r := cipher.StreamReader{S: NewCFB8(block, secret, true), R: c}
	w := cipher.StreamWriter{S: NewCFB8(block, secret, false), W: c}
	return r, w, nil
}
