package reject

import (
	"encoding/json"
	"errors"
	"net"
	"yeager/protocol"
)

func init() {
	protocol.RegisterOutboundBuilder(Tag, func(json.RawMessage) (protocol.Outbound, error) {
		return new(Client), nil
	})
}

const Tag = "reject"

var Err = errors.New("traffic rejected")

type Client struct{}

func (f *Client) Dial(addr *protocol.Address) (net.Conn, error) {
	return nil, Err
}
