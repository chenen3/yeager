package net

import (
	"bytes"
	"io"
	"sync"
	"testing"
)

type readerOnly struct {
	io.Reader
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
		b := bufPool.Get().(*[]byte)
		r := readerOnly{bytes.NewReader(testSrc)}
		io.CopyBuffer(writerOnly{io.Discard}, r, *b)
		bufPool.Put(b)
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
		b := bufPool.Get().(*[]byte)
		r := readerOnly{bytes.NewReader(testSrc)}
		io.CopyBuffer(writerOnly{io.Discard}, r, *b)
		bufPool.Put(b)
	}
}

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
