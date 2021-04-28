package common

import "net"

func RandomPort() int {
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}
