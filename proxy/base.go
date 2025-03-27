package proxy

import (
	"errors"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"

	"github.com/ZIXT233/ziproxy/db"
	"github.com/ZIXT233/ziproxy/utils"
)

var (
	ProxyEnd = errors.New("ProxyEnd")
)

type Inbound interface {
	Scheme() string
	Addr() string
	Name() string
	Config() map[string]interface{}
	WrapConn(underlay net.Conn, authFunc func(map[string]string) string) (io.ReadWriter, *TargetAddr, chan struct{}, error)
	UnregCloseChan(closeChan chan struct{})
	GetLinkConfig(defaultHost, userId, passwd string) map[string]interface{}
	CloseAllConn()
	Stop()
}
type InboundCreator func(proxyData *db.ProxyData) (Inbound, error)

var (
	InboundMap = make(map[string]InboundCreator)
)

func RegisterInbound(scheme string, c InboundCreator) {
	InboundMap[scheme] = c
}
func InboundFromConfig(proxyData *db.ProxyData) (Inbound, error) {
	config, _ := utils.UnmarshalConfig(proxyData.Config)
	if scheme, ok := config["scheme"].(string); ok {
		if c, ok := InboundMap[scheme]; ok {
			return c(proxyData)
		}
	}
	return nil, errors.New("unknown scheme")
}

type Outbound interface {
	Scheme() string
	Addr() string
	Name() string
	Config() map[string]interface{}
	WrapConn(underlay net.Conn, target *TargetAddr) (io.ReadWriter, chan struct{}, error)
	UnregCloseChan(closeChan chan struct{})
	CloseAllConn()
}
type OutboundCreator func(proxyData *db.ProxyData) (Outbound, error)

var (
	OutboundMap = make(map[string]OutboundCreator)
)

func RegisterOutbound(Scheme string, c OutboundCreator) {
	OutboundMap[Scheme] = c
}
func OutboundFromConfig(proxyData *db.ProxyData) (Outbound, error) {
	config, _ := utils.UnmarshalConfig(proxyData.Config)
	if scheme, ok := config["scheme"].(string); ok {
		if c, ok := OutboundMap[scheme]; ok {
			return c(proxyData)
		}
	}
	return nil, errors.New("unknown scheme")
}

type TargetAddr struct {
	Hostname string // fully-qualified domain name
	IP       net.IP
	Port     int
	UserId   string
}

func (a *TargetAddr) String() string {
	return a.Host() + ":" + strconv.Itoa(a.Port)
}

func (a *TargetAddr) Host() string {
	if a.Hostname == "" {
		return a.IP.String()
	}
	return a.Hostname
}

func NewTargetAddr(addr string) (*TargetAddr, error) {
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	if host == "" {
		host = "127.0.0.1"
	}
	port, _ := strconv.Atoi(portStr)

	target := &TargetAddr{Port: port}
	if ip := net.ParseIP(host); ip != nil {
		target.IP = ip
		target.Hostname = ""
	} else {
		target.Hostname = host
		ips, err := net.LookupIP(host)
		if err != nil {
			target.IP = nil
		} else {
			target.IP = ips[0]
		}
	}
	return target, nil
}

func UnregCloseChan(closeChanSet *sync.Map, closeChan chan struct{}) {
	if _, ok := closeChanSet.Load(closeChan); ok {
		closeChanSet.Delete(closeChan)
	}
}
func CloseAllConn(closeChanSet *sync.Map) {
	closeChanSet.Range(func(key, value interface{}) bool {
		key.(chan struct{}) <- struct{}{}
		return true
	})
}

func GetLinkAddr(inbound Inbound, defaultHost string) (addr string) {
	hostport := inbound.Addr()
	host, port, err := net.SplitHostPort(hostport)
	if err != nil {
		return ""
	}
	if sni, ok := inbound.Config()["sni"]; ok {
		return sni.(string) + ":" + port
	}
	if strings.Contains(host, "0.0.0.0") {
		hostname, _, err := net.SplitHostPort(defaultHost)
		if err != nil {
			return ""
		}
		return hostname + ":" + port
	}
	return hostport
}
