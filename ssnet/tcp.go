package ssnet

import (
	"github.com/shadowsocks/go-shadowsocks2/config"
	"github.com/shadowsocks/go-shadowsocks2/log"
	"io"
	"net"
	"strings"
	"time"

	"github.com/shadowsocks/go-shadowsocks2/socks"
)

// Create a SOCKS server listening on addr and proxy to server.
func socksLocal(addr, server string, shadow func(net.Conn) net.Conn) {
	log.Logf("SOCKS proxy %s <-> %s", addr, server)
	tcpLocal(addr, server, shadow, func(c net.Conn) (socks.Addr, error) { return socks.Handshake(c) })
}

// Create a TCP tunnel from addr to target via server.
func tcpTun(addr, server, target string, shadow func(net.Conn) net.Conn) {
	tgt := socks.ParseAddr(target)
	if tgt == nil {
		log.Logf("invalid target address %q", target)
		return
	}
	log.Logf("TCP tunnel %s <-> %s <-> %s", addr, server, target)
	tcpLocal(addr, server, shadow, func(net.Conn) (socks.Addr, error) { return tgt, nil })
}

// Listen on addr and proxy to server to reach target from getAddr.
func tcpLocal(addr, server string, shadow func(net.Conn) net.Conn, getAddr func(net.Conn) (socks.Addr, error)) {
	l, err := net.Listen("tcp", addr)
	if err != nil {
		log.Logf("failed to listen on %s: %v", addr, err)
		return
	}

	for {
		c, err := l.Accept()
		if err != nil {
			log.Logf("failed to accept: %s", err)
			continue
		}

		go func() {
			defer c.Close()
			c.(*net.TCPConn).SetKeepAlive(true)
			tgt, err := getAddr(c)
			if err != nil {

				// UDP: keep the connection until disconnect then free the UDP socket
				if err == socks.InfoUDPAssociate {
					buf := make([]byte, 1)
					// block here
					for {
						_, err := c.Read(buf)
						if err, ok := err.(net.Error); ok && err.Timeout() {
							continue
						}
						log.Logf("UDP Associate End.")
						return
					}
				}

				log.Logf("failed to get target address: %v", err)
				return
			}

			rc, err := net.Dial("tcp", server)
			if err != nil {
				log.Logf("failed to connect to server %v: %v", server, err)
				return
			}
			defer rc.Close()
			rc.(*net.TCPConn).SetKeepAlive(true)
			rc = shadow(rc)

			if _, err = rc.Write(tgt); err != nil {
				log.Logf("failed to send target address: %v", err)
				return
			}

			log.Logf("proxy %s <-> %s <-> %s", c.RemoteAddr(), server, tgt)
			_, _, err = relay(rc, c)
			if err != nil {
				if err, ok := err.(net.Error); ok && err.Timeout() {
					return // ignore i/o timeout
				}
				log.Logf("relay error: %v", err)
			}
		}()
	}
}

// Listen on addr for incoming connections.
func TcpRemote(serverInfo *config.ServerInfo, shadow func(net.Conn) net.Conn) {
	l, err := net.Listen("tcp", serverInfo.GetAddress())
	if err != nil {
		log.Logf("failed to listen on %s: %v", serverInfo.GetAddress(), err)
		return
	}

	log.Logf("listening TCP on %s", serverInfo.GetAddress())
	for {
		c, err := l.Accept()
		if err != nil {
			log.Logf("failed to accept: %v", err)
			continue
		}

		go func() {
			defer c.Close()
			orgC := c
			c.(*net.TCPConn).SetKeepAlive(true)
			c = shadow(c)

			tgt, err := socks.ReadAddr(c)
			if err != nil {
				log.Logf("failed to get target address: %v", err)
				orgC.Write([]byte("HTTP/1.1 200 OK\r\nContent-Type:text/html\r\nContent-Length:11\r\n\r\ntoken无效"))
				return
			}

			expired := serverInfo.GetExpiredDateTime()
			if time.Now().After(expired) {
				log.Logf("服务已经过期")
				c.Write([]byte("HTTP/1.1 200 OK\r\nContent-Type:text/html\r\nContent-Length:18\r\n\r\n服务已经过期"))
				return
			}

			canConnect := serverInfo.CheckMaxConnections(strings.Split(c.RemoteAddr().String(), ":")[0])
			if canConnect == false {
				log.Logf("超出最大连接数")
				c.Write([]byte("HTTP/1.1 200 OK\r\nContent-Type:text/html\r\nContent-Length:21\r\n\r\n超出最大连接数"))
				return
			}

			rc, err := net.Dial("tcp", tgt.String())
			if err != nil {
				log.Logf("failed to connect to target: %v", err)
				return
			}
			defer rc.Close()
			rc.(*net.TCPConn).SetKeepAlive(true)

			log.Logf("proxy %s <-> %s", c.RemoteAddr(), tgt)
			_, _, err = relay(c, rc)
			if err != nil {
				if err, ok := err.(net.Error); ok && err.Timeout() {
					return // ignore i/o timeout
				}
				log.Logf("relay error: %v", err)
			}
		}()
	}
}

// relay copies between left and right bidirectionally. Returns number of
// bytes copied from right to left, from left to right, and any error occurred.
func relay(left, right net.Conn) (int64, int64, error) {
	type res struct {
		N   int64
		Err error
	}
	ch := make(chan res)

	go func() {
		n, err := io.Copy(right, left)
		right.SetDeadline(time.Now()) // wake up the other goroutine blocking on right
		left.SetDeadline(time.Now())  // wake up the other goroutine blocking on left
		ch <- res{n, err}
	}()

	n, err := io.Copy(left, right)
	right.SetDeadline(time.Now()) // wake up the other goroutine blocking on right
	left.SetDeadline(time.Now())  // wake up the other goroutine blocking on left
	rs := <-ch

	if err == nil {
		err = rs.Err
	}
	return n, rs.N, err
}
