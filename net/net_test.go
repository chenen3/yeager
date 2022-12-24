package net

import (
	"bytes"
	"io"
	"net"
	"testing"
)

// reader implements the io.Reader interface,
// but does not implement the io.WriterTo interface.
// It is intended to replace bytes.Reader for testing,
// so that io.Copy will copy with buffer, instead of calling WriteTo
type reader struct {
	s   []byte
	off int
}

func (r *reader) Read(p []byte) (int, error) {
	if r.off >= len(r.s) {
		return 0, io.EOF
	}
	n := copy(p, r.s[r.off:])
	r.off += n
	return n, nil
}

func TestCopyBufferPool(t *testing.T) {
	s := []byte{1, 2, 3}
	r := &reader{s: s}
	var buf bytes.Buffer
	if _, err := CopyBufferPool(&buf, r); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(buf.Bytes(), s) {
		t.Fatalf("got %v, want %v", buf.Bytes(), s)
	}
}

func BenchmarkIOCopy(b *testing.B) {
	e, err := StartEchoServer()
	if err != nil {
		b.Fatal(err)
	}
	defer e.Close()

	conn, err := net.Dial("tcp", e.Listener.Addr().String())
	if err != nil {
		b.Fatal(err)
	}

	bs := make([]byte, 1024)
	b.ResetTimer()
	done := make(chan struct{})
	go func() {
		io.Copy(io.Discard, conn)
		close(done)
	}()
	for i := 0; i < b.N; i++ {
		r := &reader{s: bs}
		io.Copy(conn, r)
	}
	b.StopTimer()
	conn.(*net.TCPConn).CloseWrite()
	<-done
	conn.Close()
}

func BenchmarkCopyBufferPool(b *testing.B) {
	e, err := StartEchoServer()
	if err != nil {
		b.Fatal(err)
	}
	defer e.Close()

	// benchmark testing copyBufferPool with network connection is more realistic
	conn, err := net.Dial("tcp", e.Listener.Addr().String())
	if err != nil {
		b.Fatal(err)
	}

	bs := make([]byte, 1024)
	b.ResetTimer()
	done := make(chan struct{})
	go func() {
		CopyBufferPool(io.Discard, conn)
		close(done)
	}()
	for i := 0; i < b.N; i++ {
		r := &reader{s: bs}
		CopyBufferPool(conn, r)
	}
	b.StopTimer()
	conn.(*net.TCPConn).CloseWrite()
	<-done
	conn.Close()
}
