package main

import (
	"crypto/rand"
	"encoding/base64"
	"flag"
	"fmt"
	"github.com/shadowsocks/go-shadowsocks2/config"
	"github.com/shadowsocks/go-shadowsocks2/core"
	"github.com/shadowsocks/go-shadowsocks2/plugin"
	"github.com/shadowsocks/go-shadowsocks2/ssnet"
	"io"
	"log"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
)

func main() {

	var flags struct {
		Keygen int
	}
	flag.IntVar(&flags.Keygen, "keygen", 0, "generate a base64url-encoded random key of given length in byte")

	log.Println("Cipher:", strings.Join(core.ListCipher(), " "))

	config.LoadCommon()
	config.LoadServer()

	/*var flags struct {
		Client     string
		Server     string
		Cipher     string
		Key        string
		Password   string
		Keygen     int
		Socks      string
		RedirTCP   string
		RedirTCP6  string
		TCPTun     string
		UDPTun     string
		UDPSocks   bool

		Plugin     string
		PluginOpts string
	}

	flag.BoolVar(&config.Verbose, "verbose", false, "verbose mode")
	flag.StringVar(&flags.Cipher, "cipher", "AEAD_CHACHA20_POLY1305", "available ciphers: "+strings.Join(core.ListCipher(), " "))


	flag.StringVar(&flags.Password, "password", "", "password")
	flag.StringVar(&flags.Server, "s", "", "server listen address or url")
	flag.StringVar(&flags.Client, "c", "", "client connect address or url")
	flag.StringVar(&flags.Socks, "socks", "", "(client-only) SOCKS listen address")
	flag.BoolVar(&flags.UDPSocks, "u", false, "(client-only) Enable UDP support for SOCKS")

	flag.StringVar(&flags.TCPTun, "tcptun", "", "(client-only) TCP tunnel (laddr1=raddr1,laddr2=raddr2,...)")
	flag.StringVar(&flags.UDPTun, "udptun", "", "(client-only) UDP tunnel (laddr1=raddr1,laddr2=raddr2,...)")
	flag.StringVar(&flags.Plugin, "plugin", "", "Enable SIP003 plugin. (e.g., v2ray-plugin)")

	flag.BoolVar(&flags.UDP, "udp", false, "(server-only) enable UDP support")
	flag.DurationVar(&config.UDPTimeout, "udptimeout", 5*time.Minute, "UDP tunnel timeout")

	//not supported
	flag.StringVar(&flags.RedirTCP, "redir", "", "(client-only) redirect TCP from this address")
	flag.StringVar(&flags.RedirTCP6, "redir6", "", "(client-only) redirect TCP IPv6 from this address")*/

	flag.Parse()

	if flags.Keygen > 0 {
		key := make([]byte, flags.Keygen)
		io.ReadFull(rand.Reader, key)
		fmt.Println(base64.URLEncoding.EncodeToString(key))
		return
	}

	for i := range config.Server.Servers {
		server := config.Server.Servers[i]
		startServer(&server)
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh
	plugin.KillPlugin()
}
func startServer(serverInfo *config.ServerInfo)  {
	//addr := flags.Server
	//cipher := flags.Cipher
	//password := flags.Password
	var err error

	addr := fmt.Sprintf("0.0.0.0:%v", serverInfo.Port)
	udpAddr := addr
	password := serverInfo.Password
	cipher := serverInfo.Cipher

	if config.Common.Plugin != "" {
		addr, err = plugin.StartPlugin(config.Common.Plugin, config.Common.PluginOpts, addr, true)
		if err != nil {
			log.Fatal(err)
		}
	}

	ciph, err := core.PickCipher(cipher, password)
	if err != nil {
		log.Fatal(err)
	}

	if config.Server.UDP {
		go ssnet.UdpRemote(udpAddr, ciph.PacketConn)
	}
	go ssnet.TcpRemote(addr, ciph.StreamConn)
}
func parseURL(s string) (addr, cipher, password string, err error) {
	u, err := url.Parse(s)
	if err != nil {
		return
	}

	addr = u.Host
	if u.User != nil {
		cipher = u.User.Username()
		password, _ = u.User.Password()
	}
	return
}
