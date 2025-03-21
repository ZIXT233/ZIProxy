package http

import (
	"fmt"
	"github.com/ZIXT233/ziproxy/proxy"
	"io"
	"net"
	"strings"
)

type Outbound struct {
	addr   string
	name   string
	config map[string]interface{}
}

func (out *Outbound) Scheme() string                 { return scheme }
func (out *Outbound) Addr() string                   { return out.addr }
func (out *Outbound) Name() string                   { return out.name }
func (out *Outbound) Config() map[string]interface{} { return out.config }

func init() {
	proxy.RegisterOutbound(scheme, HttpOutboundCreator)
	proxy.RegisterOutbound(scheme+"s", HttpOutboundCreator)
}
func HttpOutboundCreator(config map[string]interface{}) (proxy.Outbound, error) {
	out := &Outbound{
		addr:   config["address"].(string),
		name:   config["name"].(string),
		config: config,
	}
	return out, nil
}

func (out *Outbound) WrapConn(underlay net.Conn, target *proxy.TargetAddr) (io.ReadWriter, error) {
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
		return nil, err
	}
	var oriRead [1024]byte
	oriLen, err := underlay.Read(oriRead[:])
	if err != nil {
		return nil, err
	}
	if !strings.Contains(string(oriRead[:oriLen]), "200") {
		return nil, fmt.Errorf("not established")
	}
	return underlay, nil
}
