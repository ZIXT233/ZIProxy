package direct

import (
	"github.com/ZIXT233/ziproxy/proxy"
	"io"
	"net"
)

const scheme = "direct"

type Direct struct {
	name   string
	config map[string]interface{}
}

func (d *Direct) Name() string                   { return d.name }
func (d *Direct) Scheme() string                 { return scheme }
func (d *Direct) Config() map[string]interface{} { return d.config }
func (d *Direct) Addr() string                   { return scheme }

func init() {
	proxy.RegisterOutbound(scheme, directOutboundCreator)
}

func directOutboundCreator(config map[string]interface{}) (proxy.Outbound, error) {
	return &Direct{
		name:   config["name"].(string),
		config: config,
	}, nil
}

func (d *Direct) WrapConn(underlay net.Conn, target *proxy.TargetAddr) (io.ReadWriter, error) {
	return underlay, nil
}
