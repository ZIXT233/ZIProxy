package proxy

import (
	"errors"
	"io"
	"net"
	"strconv"
)

var (
	ProxyEnd = errors.New("ProxyEnd")
)

type Inbound interface {
	Scheme() string
	Addr() string
	Name() string
	Config() map[string]interface{}
	WrapConn(underlay net.Conn) (io.ReadWriter, *TargetAddr, error)
	Stop()
}
type InboundCreator func(config map[string]interface{}) (Inbound, error)

var (
	InboundMap = make(map[string]InboundCreator)
)

func RegisterInbound(scheme string, c InboundCreator) {
	InboundMap[scheme] = c
}
func InboundFromConfig(config map[string]interface{}) (Inbound, error) {
	if scheme, ok := config["scheme"].(string); ok {
		if c, ok := InboundMap[scheme]; ok {
			return c(config)
		}
	}
	return nil, errors.New("unknown scheme")
}

type Outbound interface {
	Scheme() string
	Addr() string
	Name() string
	Config() map[string]interface{}
	WrapConn(underlay net.Conn, target *TargetAddr) (io.ReadWriter, error)
}
type OutboundCreator func(config map[string]interface{}) (Outbound, error)

var (
	OutboundMap = make(map[string]OutboundCreator)
)

func RegisterOutbound(Scheme string, c OutboundCreator) {
	OutboundMap[Scheme] = c
}
func OutboundFromConfig(config map[string]interface{}) (Outbound, error) {
	if scheme, ok := config["scheme"].(string); ok {
		if c, ok := OutboundMap[scheme]; ok {
			return c(config)
		}
	}
	return nil, errors.New("unknown scheme")
}

type TargetAddr struct {
	Name string // fully-qualified domain name
	IP   net.IP
	Port int
}

func (a *TargetAddr) String() string {
	port := strconv.Itoa(a.Port)
	if a.IP == nil {
		return net.JoinHostPort(a.Name, port)
	}
	return net.JoinHostPort(a.IP.String(), port)
}

func (a *TargetAddr) Host() string {
	if a.IP == nil {
		return a.Name
	}
	return a.IP.String()
}

func NewTargetAddr(addr string) (*TargetAddr, error) {
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	if host == "" {
		host = "127.0.0.1"
	}
	port, err := strconv.Atoi(portStr)

	target := &TargetAddr{Port: port}
	if ip := net.ParseIP(host); ip != nil {
		target.IP = ip
	} else {
		target.Name = host
	}
	return target, nil
}
