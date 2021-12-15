package common

import (
	"net"
	"time"
)

const (
	DialTimeout       = 4 * time.Second
	HandshakeTimeout  = 5 * time.Second
	MaxConnectionIdle = 5 * time.Minute
)

type Handler func(c net.Conn, addr string)
