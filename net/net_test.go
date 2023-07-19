package net

import (
	"bytes"
	"io"
	"net"
	"sync"
	"testing"
)

func TestCopyBufferPool(t *testing.T) {
	s := []byte{1, 2, 3}
	r := bytes.NewReader(s)
	var buf bytes.Buffer
	if _, err := Copy(&buf, r); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(buf.Bytes(), s) {
		t.Fatalf("got %v, want %v", buf.Bytes(), s)
	}
}

type readerOnly struct {
	io.Reader
}

type writerOnly struct {
	io.Writer
}

func BenchmarkCopyBuffer(b *testing.B) {
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
		buf := bufPool.Get().(*[]byte)
		io.CopyBuffer(io.Discard, conn, *buf)
		bufPool.Put(buf)
		close(done)
	}()
	for i := 0; i < b.N; i++ {
		// use wrapper to hide the bytes.Reader.WriteTo from io.CopyBuffer
		r := readerOnly{bytes.NewReader(bs)}
		// Use wrapper to hide net.TCPConn.ReadFrom from io.CopyBuffer.
		w := writerOnly{conn}
		buf := bufPool.Get().(*[]byte)
		io.CopyBuffer(w, r, *buf)
		bufPool.Put(buf)
	}
	b.StopTimer()
	conn.(*net.TCPConn).CloseWrite()
	<-done
	conn.Close()
}

// check if the adapted Copy performs better than the original io.CopyBuffer
func BenchmarkCopyAdapted(b *testing.B) {
	e, err := StartEchoServer()
	if err != nil {
		b.Fatal(err)
	}
	defer e.Close()

	// testing with network connection is closer to the actual scenario
	conn, err := net.Dial("tcp", e.Listener.Addr().String())
	if err != nil {
		b.Fatal(err)
	}

	bs := make([]byte, 1024)
	b.ResetTimer()
	done := make(chan struct{})
	go func() {
		Copy(io.Discard, conn)
		close(done)
	}()
	for i := 0; i < b.N; i++ {
		Copy(conn, bytes.NewReader(bs))
	}
	b.StopTimer()
	conn.(*net.TCPConn).CloseWrite()
	<-done
	conn.Close()
}

var testSrc = make([]byte, 8*1024*1024)

func BenchmarkBufSize16KB(b *testing.B) {
	bufPool = sync.Pool{
		New: func() any {
			s := make([]byte, 16*1024)
			return &s
		},
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r := bytes.NewReader(testSrc)
		Copy(io.Discard, r)
	}
}

func BenchmarkBufSize32KB(b *testing.B) {
	bufPool = sync.Pool{
		New: func() any {
			s := make([]byte, 32*1024)
			return &s
		},
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r := bytes.NewReader(testSrc)
		Copy(io.Discard, r)
	}
}

/*
func TestReadableBytes(t *testing.T) {
	type args struct {
		n int64
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{args: args{1}, want: "1B"},
		{args: args{2*1024 + 521}, want: "2.5KB"},
		{args: args{3*1024*1024 + 512*1024}, want: "3.5MB"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ReadableBytes(tt.args.n); got != tt.want {
				t.Errorf("ReadableBytes() = %v, want %v", got, tt.want)
			}
		})
	}
}
*/
