package transport

import (
	"context"
	"errors"
	"io"
	"net"
	"os"
	"time"
)

type Dialer interface {
	DialContext(ctx context.Context, network, addr string) (net.Conn, error)
}

type closeWriter interface {
	CloseWrite() error
}

// Relay copies data between two streams bidirectionally
func Relay(a, b net.Conn) error {
	wait := 5 * time.Second
	errc := make(chan error, 1)
	go func() {
		_, err := io.Copy(a, b)
		// unblock read on a
		if i, ok := a.(closeWriter); ok {
			i.CloseWrite()
		} else {
			a.SetReadDeadline(time.Now().Add(wait))
		}
		errc <- err
	}()
	_, err := io.Copy(b, a)
	// unblock read on b
	if i, ok := b.(closeWriter); ok {
		i.CloseWrite()
	} else {
		b.SetReadDeadline(time.Now().Add(wait))
	}
	err2 := <-errc

	if err != nil && !errors.Is(err, os.ErrDeadlineExceeded) {
		return err
	}
	if err2 != nil && !errors.Is(err2, os.ErrDeadlineExceeded) {
		return err2
	}
	return nil
}
