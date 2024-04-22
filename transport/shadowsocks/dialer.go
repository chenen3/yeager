package shadowsocks

import (
	"context"

	"github.com/chenen3/yeager/transport"

	otransport "github.com/Jigsaw-Code/outline-sdk/transport"
	"github.com/Jigsaw-Code/outline-sdk/transport/shadowsocks"
)

type streamDialer struct {
	dialer *shadowsocks.StreamDialer
}

var _ transport.StreamDialer = (*streamDialer)(nil)

func (d *streamDialer) Dial(ctx context.Context, raddr string) (transport.Stream, error) {
	return d.dialer.DialStream(ctx, raddr)
}

func NewStreamDialer(address, cipherName, secret string) (*streamDialer, error) {
	key, err := shadowsocks.NewEncryptionKey(cipherName, secret)
	if err != nil {
		return nil, err
	}
	endpoint := &otransport.StreamDialerEndpoint{Dialer: &otransport.TCPDialer{}, Address: address}
	dialer, err := shadowsocks.NewStreamDialer(endpoint, key)
	if err != nil {
		return nil, err
	}
	return &streamDialer{dialer}, nil
}
