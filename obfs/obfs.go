package obfs

import (
	"crypto/rand"
	"errors"
	"io"
)

const keyLen = 8

type reader struct {
	io.Reader
	key []byte
}

// Reader wraps an io.Reader that deobfuscates the data read from it,
// by XORing with a key from peer.
func Reader(r io.Reader) io.Reader {
	return &reader{Reader: r}
}

func (r *reader) Read(p []byte) (n int, err error) {
	if r.Reader == nil {
		return 0, errors.New("obfs: underlying Reader is nil")
	}

	if len(r.key) == 0 {
		key := make([]byte, keyLen)
		if _, err = io.ReadFull(r.Reader, key); err != nil {
			return 0, err
		}
		r.key = key
	}
	n, err = r.Reader.Read(p)
	if err != nil {
		return n, err
	}
	for i := 0; i < n; i++ {
		p[i] ^= r.key[i%keyLen]
	}
	return n, nil
}

type writer struct {
	io.Writer
	key []byte
}

// Writer wraps an io.Writer that obfuscates the data written to it,
// by XORing with a random key.
func Writer(w io.Writer) io.Writer {
	return &writer{Writer: w}
}

func (w *writer) Write(p []byte) (n int, err error) {
	if w.Writer == nil {
		return 0, errors.New("obfs: underlying Writer is nil")
	}

	if len(w.key) == 0 {
		key := make([]byte, keyLen)
		if _, err = rand.Read(key); err != nil {
			return 0, err
		}
		if _, err = w.Writer.Write(key); err != nil {
			return 0, err
		}
		w.key = key
	}
	xor := make([]byte, len(p))
	for i := 0; i < len(p); i++ {
		xor[i] = p[i] ^ w.key[i%keyLen]
	}
	return w.Writer.Write(xor)
}
