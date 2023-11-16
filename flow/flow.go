package flow

import (
	"errors"
	"io"
	"sync"
)

var bufPool = sync.Pool{
	New: func() any {
		// refer to 16KB maxPlaintext in crypto/tls/common.go
		b := make([]byte, 16*1024)
		return &b
	},
}

// Copy is adapted from io.Copy, and uses buffer pool to copy data from src to dst.
func Copy(dst io.Writer, src io.Reader) (written int64, err error) {
	b := bufPool.Get().(*[]byte)
	for {
		nr, er := src.Read(*b)
		if nr > 0 {
			nw, ew := dst.Write((*b)[0:nr])
			if nw < 0 || nr < nw {
				nw = 0
				if ew == nil {
					ew = errors.New("invalid write result")
				}
			}
			written += int64(nw)
			if ew != nil {
				err = ew
				break
			}
			if nr != nw {
				err = io.ErrShortWrite
				break
			}
		}
		if er != nil {
			if er != io.EOF {
				err = er
			}
			break
		}
	}
	bufPool.Put(b)
	return written, err
}

// Relay copies data in both directions between a and b,
// blocks until one of them completes.
// func Relay(a, b io.ReadWriter) error {
// 	c := make(chan error, 2)
// 	go func() {
// 		_, err := Copy(a, b)
// 		c <- err
// 	}()
// 	go func() {
// 		_, err := Copy(b, a)
// 		c <- err
// 	}()
// 	return <-c
// }
