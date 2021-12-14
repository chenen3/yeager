package socks

import (
	"context"
	"errors"
	"io"
	"net"
	"sync"

	"github.com/chenen3/yeager/config"
	"github.com/chenen3/yeager/log"
)

// UDPServer implements protocol.Inbound interface
type UDPServer struct {
	ctx    context.Context
	cancel context.CancelFunc
	conf   *config.SOCKSProxy
	pc     net.PacketConn
	wg     sync.WaitGroup // counts active Serve goroutines for graceful close

	ready chan struct{} // imply that server is ready to accept connection, testing only
}

func NewUDPServer(conf *config.SOCKSProxy) (*UDPServer, error) {
	if conf == nil || conf.Listen == "" {
		return nil, errors.New("config missing listening address")
	}

	ctx, cancel := context.WithCancel(context.Background())
	return &UDPServer{
		conf:   conf,
		ready:  make(chan struct{}),
		ctx:    ctx,
		cancel: cancel,
	}, nil
}

const udpBufSize = 64 * 1024

var pool = sync.Pool{
	New: func() interface{} {
		return make([]byte, udpBufSize)
	},
}

func (us *UDPServer) ListenAndServe(handle func(ctx context.Context, conn net.Conn, network, addr string)) error {
	pc, err := net.ListenPacket("udp", us.conf.Listen)
	if err != nil {
		return nil
	}
	us.pc = pc
	log.L().Infof("socks5 UDP proxy listening %s", us.conf.Listen)

	close(us.ready)
	for {
		buf := pool.Get().([]byte)
		n, raddr, err := pc.ReadFrom(buf)
		if err != nil {
			select {
			case <-us.ctx.Done():
				return nil
			default:
				log.L().Warnf(err.Error())
				continue
			}
		}

		us.wg.Add(1)
		go func() {
			defer us.wg.Done()
			udpConn, err := newUDPConn(pc, raddr, buf[:n])
			if err != nil {
				log.L().Error(err)
				return
			}
			handle(us.ctx, udpConn, "udp", udpConn.datagram.dst.String())
			pool.Put(buf)
		}()
	}
}

func (us *UDPServer) Close() error {
	defer us.wg.Wait()
	us.cancel()
	if us.pc != nil {
		return us.pc.Close()
	}
	return nil
}

// ServerUDPConn implement net.Conn interface, wraps net.PacketConn
type ServerUDPConn struct {
	net.PacketConn
	raddr    net.Addr
	datagram *datagram
	off      int
}

func newUDPConn(pc net.PacketConn, raddr net.Addr, req []byte) (*ServerUDPConn, error) {
	var dg datagram
	if err := dg.Unmarshal(req); err != nil {
		return nil, err
	}

	return &ServerUDPConn{
		PacketConn: pc,
		raddr:      raddr,
		datagram:   &dg,
	}, nil
}

func (uc *ServerUDPConn) Read(b []byte) (int, error) {
	if uc.off >= len(uc.datagram.data) {
		return 0, io.EOF
	}

	n := copy(b, uc.datagram.data[uc.off:])
	uc.off += n
	return n, nil
}

func (uc *ServerUDPConn) Write(b []byte) (int, error) {
	dg := &datagram{
		dst:  uc.datagram.dst,
		data: b,
	}
	bs, err := dg.Marshal()
	if err != nil {
		return 0, err
	}

	return uc.PacketConn.WriteTo(bs, uc.raddr)
}

func (uc *ServerUDPConn) RemoteAddr() net.Addr {
	return uc.raddr
}

// Close not actually close the inner PackageConn
// which would be done by UDPServer
func (uc *ServerUDPConn) Close() error {
	return nil
}
