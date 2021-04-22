package reject

import (
	"encoding/json"
	"errors"
	"net"
	"yeager/protocol"
)

func init() {
	protocol.RegisterOutboundBuilder("reject", func(json.RawMessage) (protocol.Outbound, error) {
		return new(Client), nil
	})
}

var Err = errors.New("traffic rejected")

type Client struct{}

func (f *Client) Dial(addr *protocol.Address) (net.Conn, error) {
	return nil, Err
}
