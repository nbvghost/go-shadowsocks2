package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"sync"
	"time"
)

var Common = struct {
	Verbose    bool
	UDPTimeout string
	Plugin     string
	PluginOpts string
}{
	Verbose:    false,
	UDPTimeout: "5m",
}

type ServerInfo struct {
	Port            int
	Name            string
	Cipher          string
	Password        string
	ExpiredDateTime string
	MaxConnections  int
	Connections     map[string]int64
	sync.Mutex
	sync.Once
}

func (s *ServerInfo) timeTask() {

	s.Do(func() {
		go func() {
			for {

				func() {
					defer s.Unlock()
					s.Lock()

					for key := range s.Connections {
						second := s.Connections[key]
						if time.Now().Unix()-second > Server.MaxConnectionsLimitTimeOut {
							delete(s.Connections, key)
						}
						if len(s.Connections) > s.MaxConnections {
							delete(s.Connections, key)
						}

					}

				}()

				time.Sleep(time.Second)
			}

		}()
	})

}
func (s *ServerInfo) CheckMaxConnections(curRemoteAddr string) bool {
	defer s.Unlock()
	s.Lock()
	if s.Connections == nil {
		s.Connections = make(map[string]int64)
	}
	if _, ok := s.Connections[curRemoteAddr]; ok {
		s.Connections[curRemoteAddr] = time.Now().Unix()
		return true
	}
	if len(s.Connections) >= s.MaxConnections {
		return false
	}
	s.Connections[curRemoteAddr] = time.Now().Unix()
	return true
}
func (s *ServerInfo) GetExpiredDateTime() time.Time {

	t, err := time.ParseInLocation("2006-01-02 15:04:05", s.ExpiredDateTime, time.Local)
	if err != nil {
		log.Println(err)
		return time.Now().Add(-1 * time.Hour)
	}
	return t
}
func (s *ServerInfo) GetAddress() string {

	return fmt.Sprintf("0.0.0.0:%v", s.Port)
}

type ServerConfig struct {
	UDP                        bool
	Servers                    []*ServerInfo
	MaxConnectionsLimitTimeOut int64
}

var Server = &ServerConfig{}

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

	Server.MaxConnectionsLimitTimeOut = ServerInfo.MaxConnectionsLimitTimeOut

	for i := 0; i < len(ServerInfo.Servers); i++ {
		hasIndex := -1
		for x := 0; x < len(Server.Servers); x++ {
			if Server.Servers[x].Port == ServerInfo.Servers[i].Port {
				hasIndex = x
			}
		}
		if hasIndex > -1 {
			Server.Servers[hasIndex].ExpiredDateTime = ServerInfo.Servers[i].ExpiredDateTime
			Server.Servers[hasIndex].MaxConnections = ServerInfo.Servers[i].MaxConnections
		} else {
			Server.Servers = append(Server.Servers, ServerInfo.Servers[i])
			ServerInfo.Servers[i].timeTask()
			return ServerInfo.Servers[i]
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
		Server.Servers[i].timeTask()
	}
}
