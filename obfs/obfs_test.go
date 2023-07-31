package obfs

import (
	"bytes"
	"testing"
)

func TestObfuscate(t *testing.T) {
	buf := new(bytes.Buffer)
	w := Writer(buf)
	want := []byte("Hello, world!")
	if _, err := w.Write(want); err != nil {
		t.Fatal(err)
	}

	r := Reader(buf)
	got := make([]byte, len(want))
	if _, err := r.Read(got); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestEmptyReadWriter(t *testing.T) {
	w := Writer(nil)
	if _, err := w.Write([]byte("whatever")); err == nil {
		t.Fatal("expected error")
	}

	r := Reader(nil)
	if _, err := r.Read(make([]byte, 10)); err == nil {
		t.Fatal("expected error")
	}
}
