package freedom

import (
	"encoding/json"
	"net"
	"yeager/protocol"
)

func init() {
	protocol.RegisterOutboundBuilder("freedom", func(json.RawMessage) (protocol.Outbound, error) {
		return new(Client), nil
	})
}

type Client struct{}

func (f *Client) Dial(addr net.Addr) (net.Conn, error) {
	return net.Dial("tcp", addr.String())
}
