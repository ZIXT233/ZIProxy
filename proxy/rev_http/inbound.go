package rev_http

import (
	"bytes"
	"fmt"
	"github.com/ZIXT233/ziproxy/proxy"
	"github.com/ZIXT233/ziproxy/utils"
	"log"
	"net"
	"strings"
	"sync"
)

type Inbound struct {
	addr         string
	name         string
	upper        proxy.Inbound
	config       map[string]interface{}
	closeChanSet sync.Map
}

func (in *Inbound) Scheme() string                 { return scheme }
func (in *Inbound) Addr() string                   { return in.addr }
func (in *Inbound) Name() string                   { return in.name }
func (in *Inbound) Config() map[string]interface{} { return in.config }
func (in *Inbound) Stop() {
	in.CloseAllConn()
	return
}

func (in *Inbound) SetAddr(addr string) {
	in.addr = addr
}
func (in *Inbound) SetUpper(upper proxy.Inbound) {
	in.upper = upper
}
func init() {
	proxy.RegisterInbound(scheme, RevHttpInboundCreator)
}
func RevHttpInboundCreator(name string, config map[string]interface{}) (proxy.Inbound, error) {
	addr, ok := config["address"].(string)
	if !ok {
		return nil, fmt.Errorf("address is required")
	}
	in := &Inbound{
		addr:   addr,
		name:   name,
		config: config,
	}

	_, err := proxy.UpperInboundCreate(in, config)
	return in, err
}

type InConn struct {
	net.Conn
	httpType string
}

func (in *Inbound) UnregCloseChan(closeChan chan struct{}) {
	proxy.UnregCloseChan(&in.closeChanSet, closeChan)
}
func (in *Inbound) CloseAllConn() {
	proxy.CloseAllConn(&in.closeChanSet)
}

// 反向代理模块中实现HTTP请求修改操作的包装器函数
func (in *Inbound) WrapConn(underlay net.Conn, authFunc func(map[string]string) string) (net.Conn, *proxy.TargetAddr, chan struct{}, error) {
	peekConn := utils.NewPeekConn(underlay)
	wrappedConn := &InConn{Conn: peekConn}
	var b []byte
	b, err := peekConn.Peek(1024)
	n := len(b)
	if err != nil {
		log.Println("read", err)
		return nil, nil, nil, err
	}

	request := strings.Split(string(b[:n]), "\r\n\r\n")
	headLines := strings.Split(request[0], "\r\n")
	modify := make(map[string]string)
	modify["Host"] = in.config["forward_host"].(string)
	modify["X-Forwarded-For"] = peekConn.RemoteAddr().String()
	modify["Connection"] = "close"
	targetAddr, err := proxy.NewTargetAddr(modify["Host"])
	targetAddr.UserId = "forward"
	var builder strings.Builder
	for i, line := range headLines {
		pair := strings.SplitN(line, ":", 2)
		value, ok := modify[pair[0]]
		if ok {
			headLines[i] = pair[0] + ": " + value
			delete(modify, pair[0])
		}
		builder.WriteString(headLines[i] + "\r\n")
	}
	for k, v := range modify {
		builder.WriteString(k + ": " + v + "\r\n")
	}
	builder.WriteString("\r\n")
	for i := 1; i < len(request); i++ {
		builder.WriteString(request[i])
		if i+1 < len(request) {
			builder.WriteString("\r\n\r\n")
		}
	}
	peekConn.SetPeekedBuf(bytes.NewBufferString(builder.String()))

	closeChan := make(chan struct{})
	in.closeChanSet.LoadOrStore(closeChan, struct{}{})
	return wrappedConn, targetAddr, closeChan, nil
}

func (in *Inbound) GetLinkConfig(defaultAccessAddr, token string) map[string]interface{} {
	addr := proxy.GetLinkAddr(in, defaultAccessAddr)
	config := map[string]interface{}{
		"scheme":  in.config["scheme"].(string),
		"address": addr,
		"url":     in.config["scheme"].(string) + "://" + addr,
	}
	return config
}
