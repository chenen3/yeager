package shadowsocks

import (
	"context"
	"net"

	sdk "github.com/Jigsaw-Code/outline-sdk/transport"
	"github.com/Jigsaw-Code/outline-sdk/transport/shadowsocks"
)

type adaptor struct {
	*shadowsocks.StreamDialer
}

func (d *adaptor) DialContext(ctx context.Context, network, raddr string) (net.Conn, error) {
	return d.DialStream(ctx, raddr)
}

func NewDialer(address, cipherName, secret string) (*adaptor, error) {
	key, err := shadowsocks.NewEncryptionKey(cipherName, secret)
	if err != nil {
		return nil, err
	}
	endpoint := &sdk.StreamDialerEndpoint{Dialer: &sdk.TCPDialer{}, Address: address}
	dialer, err := shadowsocks.NewStreamDialer(endpoint, key)
	if err != nil {
		return nil, err
	}
	return &adaptor{dialer}, nil
}
