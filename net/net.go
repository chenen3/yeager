package net

import "time"

const (
	DialTimeout      = 10 * time.Second
	HandshakeTimeout = 5 * time.Second
	KeepAlive        = 30 * time.Second
	IdleConnTimeout  = 90 * time.Second
)
