package raw

import (
	"io"
	"net"
	"sync"

	"github.com/ZIXT233/ziproxy/db"
	"github.com/ZIXT233/ziproxy/proxy"
	"github.com/ZIXT233/ziproxy/utils"
)

type Inbound struct {
	addr         string
	name         string
	config       map[string]interface{}
	closeChanSet sync.Map
}

func (in *Inbound) Name() string                   { return in.name }
func (in *Inbound) Scheme() string                 { return scheme }
func (in *Inbound) Addr() string                   { return in.addr }
func (in *Inbound) Config() map[string]interface{} { return in.config }
func (in *Inbound) Stop() {
	in.CloseAllConn()
	return
}

func init() {
	proxy.RegisterInbound(scheme, RawInboundCreator)
}
func RawInboundCreator(proxyData *db.ProxyData) (proxy.Inbound, error) {
	config, _ := utils.UnmarshalConfig(proxyData.Config)
	in := &Inbound{
		addr:   config["address"].(string),
		name:   proxyData.ID,
		config: config,
	}
	return in, nil
}

func (in *Inbound) UnregCloseChan(closeChan chan struct{}) {
	proxy.UnregCloseChan(&in.closeChanSet, closeChan)
}
func (in *Inbound) CloseAllConn() {
	proxy.CloseAllConn(&in.closeChanSet)
}
func (in *Inbound) WrapConn(underlay net.Conn, authFunc func(map[string]string) string) (io.ReadWriter, *proxy.TargetAddr, chan struct{}, error) {
	closeChan := make(chan struct{})
	in.closeChanSet.LoadOrStore(closeChan, struct{}{})
	return underlay, nil, closeChan, nil
}

func (in *Inbound) GetLinkConfig(defaultAccessAddr, token string) map[string]interface{} {
	config := make(map[string]interface{})
	for key, value := range in.config {
		config[key] = value
	}
	config["address"] = proxy.GetLinkAddr(in, defaultAccessAddr)
	return config
}
