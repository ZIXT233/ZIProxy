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
	Scheme() string                 //获取入站代理协议
	Addr() string                   //获取入站代理监听地址
	SetAddr(string)                 //设置入站代理监听地址
	SetUpper(Inbound)               //设置入站代理上层协议
	Name() string                   //获取入站代理实例名称
	Config() map[string]interface{} //获取入站代理配置参数
	//入站代理流量包装器方法，接受下层连接IO流和用户认证回调函数进行处理；返回包装后连接、代理目标信息、已注册的连接关闭消息通道、错误信息。
	WrapConn(underlay net.Conn, authFunc func(map[string]string) string) (net.Conn, *TargetAddr, chan struct{}, error)
	//入站代理相关连接取消注册的方法，当某一相关连接主动关闭时调用，这样在实例关闭时，就不会重复通知该连接关闭。
	UnregCloseChan(closeChan chan struct{})
	//获取入站代理的连接配置信息。
	GetLinkConfig(defaultHost, token string) map[string]interface{}
	//通过广播连接关闭消息通道的方式，关闭该入站代理实例所有相关连接。
	CloseAllConn()
	//关闭该入站代理实例，除了关闭上述相关连接外，还会停止相关监听协程。
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
	Scheme() string                 //获取出站代理协议
	Addr() string                   //获取出站代理下级网络地址
	SetAddr(string)                 //设置出站代理下级网络地址
	SetUpper(Outbound)              //设置出站代理上层协议
	Name() string                   //设置出站代理实例名称
	Config() map[string]interface{} //获取出站代理配置参数
	//出站代理流量包装器方法，接受下层连接IO流和代理目标信息进行处理；返回包装后连接、已注册的连接关闭消息通道、错误信息。
	WrapConn(underlay net.Conn, target *TargetAddr) (net.Conn, chan struct{}, error)
	//出站代理相关连接取消注册的方法，当某一相关连接主动关闭时调用，这样在实例关闭时，就不会重复通知该连接关闭。
	UnregCloseChan(closeChan chan struct{})
	//通过广播连接关闭消息通道的方式，关闭该出站代理实例所有相关连接。
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
