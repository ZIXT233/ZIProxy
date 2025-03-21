package tcp

import (
	"github.com/ZIXT233/ziproxy/proxy"
	"io"
	"net"
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
	proxy.RegisterOutbound(scheme, TcpOutboundCreator)
}
func TcpOutboundCreator(config map[string]interface{}) (proxy.Outbound, error) {
	out := &Outbound{
		addr:   config["address"].(string),
		name:   config["name"].(string),
		config: config,
	}
	return out, nil
}

func (out *Outbound) WrapConn(underlay net.Conn, target *proxy.TargetAddr) (io.ReadWriter, error) {
	return underlay, nil
}
