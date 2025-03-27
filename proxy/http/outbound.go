package http

import (
	"fmt"
	"io"
	"net"
	"strings"
	"sync"

	"github.com/ZIXT233/ziproxy/db"
	"github.com/ZIXT233/ziproxy/proxy"
	"github.com/ZIXT233/ziproxy/utils"
)

type Outbound struct {
	addr         string
	name         string
	config       map[string]interface{}
	closeChanSet sync.Map
}

func (out *Outbound) Scheme() string                 { return scheme }
func (out *Outbound) Addr() string                   { return out.addr }
func (out *Outbound) Name() string                   { return out.name }
func (out *Outbound) Config() map[string]interface{} { return out.config }

func init() {
	proxy.RegisterOutbound(scheme, HttpOutboundCreator)
	proxy.RegisterOutbound(scheme+"s", HttpOutboundCreator)
}
func HttpOutboundCreator(proxyData *db.ProxyData) (proxy.Outbound, error) {
	config, _ := utils.UnmarshalConfig(proxyData.Config)
	addr, ok := config["address"].(string)
	if !ok {
		return nil, fmt.Errorf("address is required")
	}
	out := &Outbound{
		addr:   addr,
		name:   proxyData.ID,
		config: config,
	}
	return out, nil
}

func (out *Outbound) UnregCloseChan(closeChan chan struct{}) {
	proxy.UnregCloseChan(&out.closeChanSet, closeChan)
}
func (out *Outbound) CloseAllConn() {
	proxy.CloseAllConn(&out.closeChanSet)
}

func (out *Outbound) WrapConn(underlay net.Conn, target *proxy.TargetAddr) (io.ReadWriter, chan struct{}, error) {
	var authHead string
	if out.config["auth"] != nil {
		auth := out.config["auth"].(map[string]interface{})
		authHead = fmt.Sprintf("username:%s\r\npassword:%s\r\n",
			auth["username"].(string), auth["password"].(string))
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

	closeChan := make(chan struct{})
	out.closeChanSet.LoadOrStore(closeChan, struct{}{})
	return underlay, closeChan, nil
}
