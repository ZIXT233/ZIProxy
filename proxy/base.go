package proxy

import (
	"errors"
	"net"
	"strconv"
	"strings"
	"sync"
)

var (
	ProxyEnd = errors.New("ProxyEnd")
)

type Inbound interface {
	Scheme() string
	Addr() string
	SetAddr(string)
	SetUpper(Inbound)
	Name() string
	Config() map[string]interface{}
	WrapConn(underlay net.Conn, authFunc func(map[string]string) string) (net.Conn, *TargetAddr, chan struct{}, error)
	UnregCloseChan(closeChan chan struct{})
	GetLinkConfig(defaultHost, token string) map[string]interface{}
	CloseAllConn()
	Stop()
}
type InboundCreator func(name string, config map[string]interface{}) (Inbound, error)

var (
	InboundMap = make(map[string]InboundCreator)
)

func RegisterInbound(scheme string, c InboundCreator) {
	InboundMap[scheme] = c
}

func InboundFromConfig(name string, config map[string]interface{}) (Inbound, error) {
	if scheme, ok := config["scheme"].(string); ok {
		if c, ok := InboundMap[scheme]; ok {
			return c(name, config)
		}
	}
	return nil, errors.New("unknown scheme")
}

func UpperInboundCreate(in Inbound, config map[string]interface{}) (Inbound, error) {
	if up, ok := config["upper"]; ok {
		upperConfig, ok := up.(map[string]interface{})
		if !ok {
			return nil, errors.New("upper config is not map")
		}
		upperConfig["address"] = in.Addr()
		upperBound, err := InboundFromConfig(in.Name(), upperConfig)
		if err != nil {
			return nil, err
		}
		in.SetUpper(upperBound)
		return upperBound, nil
	}
	return nil, nil
}

type Outbound interface {
	Scheme() string
	Addr() string
	SetAddr(string)
	SetUpper(Outbound)
	Name() string
	Config() map[string]interface{}
	WrapConn(underlay net.Conn, target *TargetAddr) (net.Conn, chan struct{}, error)
	UnregCloseChan(closeChan chan struct{})
	CloseAllConn()
}
type OutboundCreator func(name string, config map[string]interface{}) (Outbound, error)

var (
	OutboundMap = make(map[string]OutboundCreator)
)

func RegisterOutbound(Scheme string, c OutboundCreator) {
	OutboundMap[Scheme] = c
}

func OutboundFromConfig(name string, config map[string]interface{}) (Outbound, error) {
	if scheme, ok := config["scheme"].(string); ok {
		if c, ok := OutboundMap[scheme]; ok {
			return c(name, config)
		}
	}
	return nil, errors.New("unknown scheme")
}
func UpperOutboundCreate(out Outbound, config map[string]interface{}) (Outbound, error) {
	if up, ok := config["upper"]; ok {
		upperConfig, ok := up.(map[string]interface{})
		if !ok {
			return nil, errors.New("upper config is not map")
		}
		upperConfig["address"] = out.Addr()
		upperBound, err := OutboundFromConfig(out.Name(), upperConfig)
		if err != nil {
			return nil, err
		}
		out.SetUpper(upperBound)
		return upperBound, nil
	}
	return nil, nil
}

type TargetAddr struct {
	Hostname string // fully-qualified domain name
	IP       net.IP
	Port     int
	UserId   string
	Custom   map[string]interface{}
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
		select {
		case key.(chan struct{}) <- struct{}{}: //如果不能发送代表已经关闭但是还未取消注册，忽略
		default:
		}
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
