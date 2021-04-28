package common

import "net"

func ChoosePort() (int, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	defer ln.Close()
	if err != nil {
		return 0, err
	}
	return ln.Addr().(*net.TCPAddr).Port, nil
}
