package obfs

import (
	"crypto/rand"
	"crypto/subtle"
	"errors"
	"io"
)

const keyLen = 32
const streamBufferSize = 512

// TODO: how about accepting user specified password?

// Reader wraps an io.Reader that deobfuscates the data read from it,
// by XORing with the key from peer.
func Reader(r io.Reader) io.Reader {
	return &reader{reader: r}
}

type reader struct {
	reader io.Reader
	keys   []byte
	off    int
}

func (r *reader) Read(p []byte) (n int, err error) {
	if r.reader == nil {
		return 0, errors.New("obfs: underlying Reader is nil")
	}

	if len(r.keys) == 0 {
		key := make([]byte, keyLen)
		if _, err = io.ReadFull(r.reader, key); err != nil {
			return 0, err
		}
		// for batch XOR
		r.keys = make([]byte, streamBufferSize)
		for i := 0; i < len(r.keys); {
			nc := copy(r.keys[i:], key)
			i += nc
		}
	}
	n, err = r.reader.Read(p)
	if err != nil {
		return n, err
	}
	p = p[:n]
	for len(p) > 0 {
		nx := subtle.XORBytes(p, p, r.keys[r.off:])
		p = p[nx:]
		r.off = (r.off + nx) % len(r.keys)
	}
	return n, nil
}

// Writer wraps an io.Writer that obfuscates the data written to it,
// by XORing with a random key.
func Writer(w io.Writer) io.Writer {
	return &writer{writer: w}
}

type writer struct {
	writer io.Writer
	keys   []byte
	off    int
}

func (w *writer) Write(src []byte) (int, error) {
	if w.writer == nil {
		return 0, errors.New("obfs: underlying Writer is nil")
	}

	if len(w.keys) == 0 {
		key := make([]byte, keyLen)
		if _, err := rand.Read(key); err != nil {
			return 0, err
		}
		if _, err := w.writer.Write(key); err != nil {
			return 0, err
		}
		// for batch XOR
		w.keys = make([]byte, streamBufferSize)
		for i := 0; i < len(w.keys); {
			nc := copy(w.keys[i:], key)
			i += nc
		}
	}

	dst := make([]byte, len(src))
	for i := 0; i < len(src); {
		nx := subtle.XORBytes(dst[i:], src[i:], w.keys[w.off:])
		i += nx
		w.off = (w.off + nx) % len(w.keys)
	}
	return w.writer.Write(dst)
}
