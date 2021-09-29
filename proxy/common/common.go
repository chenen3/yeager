package common

import (
	"time"
)

const (
	DialTimeout       = 4 * time.Second
	HandshakeTimeout  = 5 * time.Second
	MaxConnectionIdle = 5 * time.Minute
)
