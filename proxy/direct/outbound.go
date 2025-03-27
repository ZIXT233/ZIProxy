package direct

import (
	"io"
	"net"
	"sync"

	"github.com/ZIXT233/ziproxy/db"
	"github.com/ZIXT233/ziproxy/proxy"
	"github.com/ZIXT233/ziproxy/utils"
)

const scheme = "direct"

type Outbound struct {
	name         string
	config       map[string]interface{}
	closeChanSet sync.Map
}

func (d *Outbound) Name() string                   { return d.name }
func (d *Outbound) Scheme() string                 { return scheme }
func (d *Outbound) Config() map[string]interface{} { return d.config }
func (d *Outbound) Addr() string                   { return scheme }

func init() {
	proxy.RegisterOutbound(scheme, directOutboundCreator)
}

func directOutboundCreator(proxyData *db.ProxyData) (proxy.Outbound, error) {
	config, _ := utils.UnmarshalConfig(proxyData.Config)
	return &Outbound{
		name:   proxyData.ID,
		config: config,
	}, nil
}

func (out *Outbound) UnregCloseChan(closeChan chan struct{}) {
	proxy.UnregCloseChan(&out.closeChanSet, closeChan)
}
func (out *Outbound) CloseAllConn() {
	proxy.CloseAllConn(&out.closeChanSet)
}
func (out *Outbound) WrapConn(underlay net.Conn, target *proxy.TargetAddr) (io.ReadWriter, chan struct{}, error) {
	closeChan := make(chan struct{})
	out.closeChanSet.LoadOrStore(closeChan, struct{}{})
	return underlay, closeChan, nil
}
