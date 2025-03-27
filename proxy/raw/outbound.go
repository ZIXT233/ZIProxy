package raw

import (
	"io"
	"net"
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
	proxy.RegisterOutbound(scheme, RawOutboundCreator)
}
func RawOutboundCreator(proxyData *db.ProxyData) (proxy.Outbound, error) {
	config, _ := utils.UnmarshalConfig(proxyData.Config)
	out := &Outbound{
		addr:   config["address"].(string),
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
	closeChan := make(chan struct{})
	out.closeChanSet.LoadOrStore(closeChan, struct{}{})
	return underlay, closeChan, nil
}
