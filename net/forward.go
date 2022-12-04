package net

import (
	"errors"
	"io"
	"sync"
)

// Forwarder forwards data from client to remote, and from remote to client,
// whichever completes, sends an error on C.
type Forwarder struct {
	client io.ReadWriter
	remote io.ReadWriter
	C      chan error
}

// NewForwarder creates a new Forwarder that will send errors
// on its channel after FromClient or ToClient completes.
func NewForwarder(client, remote io.ReadWriter) *Forwarder {
	return &Forwarder{
		client: client,
		remote: remote,
		C:      make(chan error, 2),
	}
}

// FromClient forwards data from client to remote,
// sends the first error encountered to C, if any.
func (f *Forwarder) FromClient() {
	_, err := copyBufferred(f.remote, f.client)
	f.C <- err
}

// ToClient forwards data from remote to client,
// sends the first error encountered to C, if any.
func (f *Forwarder) ToClient() {
	_, err := copyBufferred(f.client, f.remote)
	f.C <- err
}

var slicePool = sync.Pool{
	New: func() any {
		slice := make([]byte, 32*1024)
		// A pointer can be put into the return interface value without an allocation.
		return &slice
	},
}

// copyBufferred is mostly taken from the actual implementation of io.Copy and io.CopyBuffer,
// except that the buffer used to perform the copy will come from slicePool.
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
