package tcp

import (
	"github.com/ZIXT233/ziproxy/proxy"
	"io"
	"net"
)

type Inbound struct {
	addr   string
	name   string
	config map[string]interface{}
}

func (in *Inbound) Name() string                   { return in.name }
func (in *Inbound) Scheme() string                 { return scheme }
func (in *Inbound) Addr() string                   { return in.addr }
func (in *Inbound) Config() map[string]interface{} { return in.config }
func (in *Inbound) Stop()                          { return }

func init() {
	proxy.RegisterInbound(scheme, TcpInboundCreator)
}
func TcpInboundCreator(config map[string]interface{}) (proxy.Inbound, error) {
	in := &Inbound{
		addr:   config["address"].(string),
		name:   config["name"].(string),
		config: config,
	}
	return in, nil
}

func (in *Inbound) WrapConn(underlay net.Conn) (io.ReadWriter, *proxy.TargetAddr, error) {

	return underlay, nil, nil
}
