package direct

import "net"

const Tag = "direct"

// Direct implements the proxy.Outbounder interface
var Direct = new(net.Dialer)
