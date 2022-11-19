package relay

import "io"

// A Relayer relay data between src and dst bidirectionally
type Relayer struct {
	src io.ReadWriter
	dst io.ReadWriter
}

// New returns a Relayer that relays data between src and dst bidirectionally
func New(src, dst io.ReadWriter) *Relayer {
	return &Relayer{src: src, dst: dst}
}

// ToDst relays from src to dst, sends the first error encountered to the given ch
func (c *Relayer) ToDst(ch chan<- error) {
	_, err := io.Copy(c.dst, c.src)
	ch <- err
}

// FromDst relays from dst to src, sends the first error encountered to the given ch
func (c *Relayer) FromDst(ch chan<- error) {
	_, err := io.Copy(c.src, c.dst)
	ch <- err
}
