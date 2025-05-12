package direct

import (
	"net"
	"sync"

	"github.com/ZIXT233/ziproxy/proxy"
)

const scheme = "direct"

type Outbound struct {
	name         string
	upper        proxy.Outbound
	config       map[string]interface{}
	closeChanSet sync.Map
}

func (d *Outbound) Name() string                   { return d.name }
func (d *Outbound) Scheme() string                 { return scheme }
func (d *Outbound) Config() map[string]interface{} { return d.config }
func (d *Outbound) Addr() string                   { return scheme }

func (out *Outbound) SetAddr(addr string) {

}
func (out *Outbound) SetUpper(upper proxy.Outbound) {
	out.upper = upper
}
func init() {
	proxy.RegisterOutbound(scheme, directOutboundCreator)
}

func directOutboundCreator(name string, config map[string]interface{}) (proxy.Outbound, error) {

	out := &Outbound{
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
	if out.upper != nil {
		return out.upper.WrapConn(underlay, target)
	} else {
		closeChan := make(chan struct{})
		out.closeChanSet.LoadOrStore(closeChan, struct{}{})
		return underlay, closeChan, nil
	}
}
