package direct

import "net"

const Tag = "direct"

// Direct implements proxy.Outbound by making network connections directly using net.DialContext
var Direct = new(net.Dialer)
