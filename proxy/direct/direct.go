package direct

import "net"

const Tag = "direct"

// Direct implements the proxy.Outbounder
var Direct = new(net.Dialer)
