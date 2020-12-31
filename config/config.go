package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"time"
)

var Common = struct {
	Verbose    bool
	UDPTimeout time.Duration
	Plugin     string
	PluginOpts string
}{
	Verbose:    false,
	UDPTimeout: 0,
}

type ServerInfo struct {
	Port            int
	Name            string
	Cipher          string
	Password        string
	ExpiredDateTime string
}

type ServerConfig struct {
	UDP     bool
	Servers []ServerInfo
}

var Server = ServerConfig{}

func LoadCommon() {
	b, err := ioutil.ReadFile("common.json")
	if err != nil {
		panic(err)
	}
	err = json.Unmarshal(b, &Common)
	if err != nil {
		panic(err)
	}
}
func ReadNewServerConfig() *ServerInfo {
	b, err := ioutil.ReadFile("server.json")
	if err != nil {
		println(err)
	}
	var ServerInfo = ServerConfig{}
	err = json.Unmarshal(b, &ServerInfo)
	if err != nil {
		println(err)
	}

	for i := 0; i < len(ServerInfo.Servers); i++ {
		hasIndex := -1
		for x := 0; x < len(Server.Servers); x++ {
			if Server.Servers[x].Port == ServerInfo.Servers[i].Port {
				hasIndex = x
			}
		}
		if hasIndex > -1 {
			Server.Servers[hasIndex].ExpiredDateTime = ServerInfo.Servers[i].ExpiredDateTime
		} else {
			Server.Servers = append(Server.Servers, ServerInfo.Servers[i])
			return &ServerInfo.Servers[i]
		}
	}
	return nil
}
func LoadServer() {
	b, err := ioutil.ReadFile("server.json")
	if err != nil {
		panic(err)
	}
	err = json.Unmarshal(b, &Server)
	if err != nil {
		panic(err)
	}

	ports := make([]int, 0)
	for i := 0; i < len(Server.Servers); i++ {
		has := false
		for x := 0; x < len(ports); x++ {
			if Server.Servers[i].Port == ports[x] {
				has = true
				break
			}
		}
		if has {
			panic(errors.New(fmt.Sprintf("重复的端口:%v", Server.Servers[i].Port)))
		}
	}
}
