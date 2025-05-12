package http

import (
	"fmt"
	"net"
	"strings"
	"sync"

	"github.com/ZIXT233/ziproxy/proxy"
)

type Outbound struct {
	addr         string
	name         string
	upper        proxy.Outbound
	config       map[string]interface{}
	closeChanSet sync.Map
}

func (out *Outbound) Scheme() string                 { return scheme }
func (out *Outbound) Addr() string                   { return out.addr }
func (out *Outbound) Name() string                   { return out.name }
func (out *Outbound) Config() map[string]interface{} { return out.config }
func (out *Outbound) SetAddr(addr string) {
	out.addr = addr
}
func (out *Outbound) SetUpper(upper proxy.Outbound) {
	out.upper = upper
}

func init() {
	proxy.RegisterOutbound(scheme, HttpOutboundCreator)
	proxy.RegisterOutbound(scheme+"s", HttpOutboundCreator)
}
func HttpOutboundCreator(name string, config map[string]interface{}) (proxy.Outbound, error) {
	addr, ok := config["address"].(string)
	if !ok {
		return nil, fmt.Errorf("address is required")
	}
	out := &Outbound{
		addr:   addr,
		name:   name,
		config: config,
	}

	_, err := proxy.UpperOutboundCreate(out, config)
	return out, err
}

func (out *Outbound) UnregCloseChan(closeChan chan struct{}) {
	proxy.UnregCloseChan(&out.closeChanSet, closeChan)
}
func (out *Outbound) CloseAllConn() {
	proxy.CloseAllConn(&out.closeChanSet)
}

func (out *Outbound) WrapConn(underlay net.Conn, target *proxy.TargetAddr) (net.Conn, chan struct{}, error) {
	var authHead string
	if out.config["linkToken"] != nil {
		token := out.config["linkToken"].(string)
		authHead = fmt.Sprintf("linkToken:%s\r\n", token)
	}
	_, err := fmt.Fprintf(underlay, "CONNECT %s HTTP/1.1\r\n"+
		"%s"+
		"\r\n", target.String(), authHead)
	if err != nil {
		return nil, nil, err
	}
	var oriRead [1024]byte
	oriLen, err := underlay.Read(oriRead[:])
	if err != nil {
		return nil, nil, err
	}
	if !strings.Contains(string(oriRead[:oriLen]), "200") {
		return nil, nil, fmt.Errorf("not established")
	}

	if out.upper != nil {
		return out.upper.WrapConn(underlay, target)
	} else {
		closeChan := make(chan struct{})
		out.closeChanSet.LoadOrStore(closeChan, struct{}{})
		return underlay, closeChan, nil
	}
}
