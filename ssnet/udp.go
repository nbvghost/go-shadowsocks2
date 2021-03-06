package ssnet

import (
	"fmt"
	"github.com/shadowsocks/go-shadowsocks2/config"
	"github.com/shadowsocks/go-shadowsocks2/log"
	"net"
	"time"

	"sync"

	"github.com/shadowsocks/go-shadowsocks2/socks"
)

type mode int

const (
	remoteServer mode = iota
	relayClient
	socksClient
)

const udpBufSize = 64 * 1024

// Listen on laddr for UDP packets, encrypt and send to server to reach target.
func udpLocal(laddr, server, target string, shadow func(net.PacketConn) net.PacketConn) {
	srvAddr, err := net.ResolveUDPAddr("udp", server)
	if err != nil {
		log.Logf("UDP server address error: %v", err)
		return
	}

	tgt := socks.ParseAddr(target)
	if tgt == nil {
		err = fmt.Errorf("invalid target address: %q", target)
		log.Logf("UDP target address error: %v", err)
		return
	}

	c, err := net.ListenPacket("udp", laddr)
	if err != nil {
		log.Logf("UDP local listen error: %v", err)
		return
	}
	defer c.Close()

	d, err := time.ParseDuration(config.Common.UDPTimeout)
	if err != nil {
		log.Logf("ParseDuration(config.Common.UDPTimeout) error: %v", err)
		return
	}
	nm := newNATmap(d)
	buf := make([]byte, udpBufSize)
	copy(buf, tgt)

	log.Logf("UDP tunnel %s <-> %s <-> %s", laddr, server, target)
	for {
		n, raddr, err := c.ReadFrom(buf[len(tgt):])
		if err != nil {
			log.Logf("UDP local read error: %v", err)
			continue
		}

		pc := nm.Get(raddr.String())
		if pc == nil {
			pc, err = net.ListenPacket("udp", "")
			if err != nil {
				log.Logf("UDP local listen error: %v", err)
				continue
			}

			pc = shadow(pc)
			nm.Add(raddr, c, pc, relayClient)
		}

		_, err = pc.WriteTo(buf[:len(tgt)+n], srvAddr)
		if err != nil {
			log.Logf("UDP local write error: %v", err)
			continue
		}
	}
}

// Listen on laddr for Socks5 UDP packets, encrypt and send to server to reach target.
func udpSocksLocal(laddr, server string, shadow func(net.PacketConn) net.PacketConn) {
	srvAddr, err := net.ResolveUDPAddr("udp", server)
	if err != nil {
		log.Logf("UDP server address error: %v", err)
		return
	}

	c, err := net.ListenPacket("udp", laddr)
	if err != nil {
		log.Logf("UDP local listen error: %v", err)
		return
	}
	defer c.Close()

	d, err := time.ParseDuration(config.Common.UDPTimeout)
	if err != nil {
		log.Logf("ParseDuration(config.Common.UDPTimeout) error: %v", err)
		return
	}
	nm := newNATmap(d)
	buf := make([]byte, udpBufSize)

	for {
		n, raddr, err := c.ReadFrom(buf)
		if err != nil {
			log.Logf("UDP local read error: %v", err)
			continue
		}

		pc := nm.Get(raddr.String())
		if pc == nil {
			pc, err = net.ListenPacket("udp", "")
			if err != nil {
				log.Logf("UDP local listen error: %v", err)
				continue
			}
			log.Logf("UDP socks tunnel %s <-> %s <-> %s", laddr, server, socks.Addr(buf[3:]))
			pc = shadow(pc)
			nm.Add(raddr, c, pc, socksClient)
		}

		_, err = pc.WriteTo(buf[3:n], srvAddr)
		if err != nil {
			log.Logf("UDP local write error: %v", err)
			continue
		}
	}
}

// Listen on addr for encrypted packets and basically do UDP NAT.
func UdpRemote(serverInfo *config.ServerInfo, shadow func(net.PacketConn) net.PacketConn) {
	c, err := net.ListenPacket("udp", serverInfo.GetAddress())
	if err != nil {
		log.Logf("UDP remote listen error: %v", err)
		return
	}
	defer c.Close()
	c = shadow(c)

	d, err := time.ParseDuration(config.Common.UDPTimeout)
	if err != nil {
		log.Logf("ParseDuration(config.Common.UDPTimeout) error: %v", err)
		return
	}
	nm := newNATmap(d)
	buf := make([]byte, udpBufSize)

	log.Logf("listening UDP on %s", serverInfo.GetAddress())
	for {
		n, raddr, err := c.ReadFrom(buf)
		if err != nil {
			log.Logf("UDP remote read error: %v", err)
			continue
		}

		tgtAddr := socks.SplitAddr(buf[:n])
		if tgtAddr == nil {
			log.Logf("failed to split target address from packet: %q", buf[:n])
			continue
		}

		tgtUDPAddr, err := net.ResolveUDPAddr("udp", tgtAddr.String())
		if err != nil {
			log.Logf("failed to resolve target UDP address: %v", err)
			continue
		}

		payload := buf[len(tgtAddr):n]

		pc := nm.Get(raddr.String())
		if pc == nil {
			pc, err = net.ListenPacket("udp", "")
			if err != nil {
				log.Logf("UDP remote listen error: %v", err)
				continue
			}

			nm.Add(raddr, c, pc, remoteServer)
		}

		_, err = pc.WriteTo(payload, tgtUDPAddr) // accept only UDPAddr despite the signature
		if err != nil {
			log.Logf("UDP remote write error: %v", err)
			continue
		}
	}
}

// Packet NAT table
type natmap struct {
	sync.RWMutex
	m       map[string]net.PacketConn
	timeout time.Duration
}

func newNATmap(timeout time.Duration) *natmap {
	m := &natmap{}
	m.m = make(map[string]net.PacketConn)
	m.timeout = timeout
	return m
}

func (m *natmap) Get(key string) net.PacketConn {
	m.RLock()
	defer m.RUnlock()
	return m.m[key]
}

func (m *natmap) Set(key string, pc net.PacketConn) {
	m.Lock()
	defer m.Unlock()

	m.m[key] = pc
}

func (m *natmap) Del(key string) net.PacketConn {
	m.Lock()
	defer m.Unlock()

	pc, ok := m.m[key]
	if ok {
		delete(m.m, key)
		return pc
	}
	return nil
}

func (m *natmap) Add(peer net.Addr, dst, src net.PacketConn, role mode) {
	m.Set(peer.String(), src)

	go func() {
		timedCopy(dst, peer, src, m.timeout, role)
		if pc := m.Del(peer.String()); pc != nil {
			pc.Close()
		}
	}()
}

// copy from src to dst at target with read timeout
func timedCopy(dst net.PacketConn, target net.Addr, src net.PacketConn, timeout time.Duration, role mode) error {
	buf := make([]byte, udpBufSize)

	for {
		src.SetReadDeadline(time.Now().Add(timeout))
		n, raddr, err := src.ReadFrom(buf)
		if err != nil {
			return err
		}

		switch role {
		case remoteServer: // server -> client: add original packet source
			srcAddr := socks.ParseAddr(raddr.String())
			copy(buf[len(srcAddr):], buf[:n])
			copy(buf, srcAddr)
			_, err = dst.WriteTo(buf[:len(srcAddr)+n], target)
		case relayClient: // client -> user: strip original packet source
			srcAddr := socks.SplitAddr(buf[:n])
			_, err = dst.WriteTo(buf[len(srcAddr):n], target)
		case socksClient: // client -> socks5 program: just set RSV and FRAG = 0
			_, err = dst.WriteTo(append([]byte{0, 0, 0}, buf[:n]...), target)
		}

		if err != nil {
			return err
		}
	}
}
