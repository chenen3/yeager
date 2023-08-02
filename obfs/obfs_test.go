package obfs

import (
	"bytes"
	"io"
	"testing"
)

func TestObfuscate(t *testing.T) {
	buf := new(bytes.Buffer)
	w := Writer(buf)
	want := []byte("Hello, world!")
	io.Copy(w, bytes.NewReader(want))

	r := Reader(buf)
	// In the real world, byte slices can vary in length
	got1 := make([]byte, len(want)/2)
	if _, err := io.ReadFull(r, got1); err != nil {
		t.Fatal(err)
	}

	got2 := make([]byte, len(want)-len(got1))
	if _, err := io.ReadFull(r, got2); err != nil {
		t.Fatal(err)
	}
	got := append(got1, got2...)
	if !bytes.Equal(want, got) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestNilInput(t *testing.T) {
	w := Writer(nil)
	if _, err := w.Write([]byte("whatever")); err == nil {
		t.Fatal("expected error")
	}

	r := Reader(nil)
	if _, err := r.Read(make([]byte, 10)); err == nil {
		t.Fatal("expected error")
	}
}
