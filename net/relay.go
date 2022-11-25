package net

import (
	"errors"
	"io"
	"sync"
)

// A Relayer relay data between src and dst bidirectionally
type Relayer struct {
	src io.ReadWriter
	dst io.ReadWriter
}

// NewRelayer returns a Relayer that relays data between src and dst bidirectionally
func NewRelayer(src, dst io.ReadWriter) *Relayer {
	return &Relayer{src: src, dst: dst}
}

// ToDst relays from src to dst, sends the first error encountered to ch, if any
func (c *Relayer) ToDst(ch chan<- error) {
	_, err := copyBufferred(c.dst, c.src)
	ch <- err
}

// FromDst relays from dst to src, sends the first error encountered to ch, if any
func (c *Relayer) FromDst(ch chan<- error) {
	_, err := copyBufferred(c.src, c.dst)
	ch <- err
}

var slicePool = sync.Pool{
	New: func() any {
		slice := make([]byte, 32*1024)
		// A pointer can be put into the return interface value without an allocation.
		return &slice
	},
}

// copyBufferred is mostly taken from the actual implementation of io.Copy and io.CopyBuffer,
// except that the buffer used to perform the copy will come from slicePool to avoid allocation.
//
// If either src implements io.WriterTo or dst implements io.ReaderFrom,
// no buffer will be used to perform the copy.
func copyBufferred(dst io.Writer, src io.Reader) (written int64, err error) {
	// If the reader has a WriteTo method, use it to do the copy.
	// Avoids an allocation and a copy.
	if wt, ok := src.(io.WriterTo); ok {
		return wt.WriteTo(dst)
	}
	// Similarly, if the writer has a ReadFrom method, use it to do the copy.
	if rt, ok := dst.(io.ReaderFrom); ok {
		return rt.ReadFrom(src)
	}

	s := slicePool.Get().(*[]byte)
	for {
		nr, er := src.Read(*s)
		if nr > 0 {
			nw, ew := dst.Write((*s)[0:nr])
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
	slicePool.Put(s)
	return written, err
}
