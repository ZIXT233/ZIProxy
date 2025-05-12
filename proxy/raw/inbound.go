package raw

import (
	"fmt"
	"net"
	"sync"

	"github.com/ZIXT233/ziproxy/proxy"
)

type Inbound struct {
	addr         string
	name         string
	upper        proxy.Inbound
	config       map[string]interface{}
	closeChanSet sync.Map
}

func (in *Inbound) Name() string                   { return in.name }
func (in *Inbound) Scheme() string                 { return scheme }
func (in *Inbound) Addr() string                   { return in.addr }
func (in *Inbound) Config() map[string]interface{} { return in.config }

func (in *Inbound) SetAddr(addr string) {
	in.addr = addr
}
func (in *Inbound) SetUpper(upper proxy.Inbound) {
	in.upper = upper
}
func (in *Inbound) Stop() {
	in.CloseAllConn()
	return
}

func init() {
	proxy.RegisterInbound(scheme, RawInboundCreator)
}
func RawInboundCreator(name string, config map[string]interface{}) (proxy.Inbound, error) {
	addr, ok := config["address"].(string)
	if !ok {
		return nil, fmt.Errorf("address is required")
	}
	in := &Inbound{
		addr:   addr,
		name:   name,
		config: config,
	}
	_, err := proxy.UpperInboundCreate(in, config)

	return in, err
}

func (in *Inbound) UnregCloseChan(closeChan chan struct{}) {
	proxy.UnregCloseChan(&in.closeChanSet, closeChan)
}
func (in *Inbound) CloseAllConn() {
	proxy.CloseAllConn(&in.closeChanSet)
}
func (in *Inbound) WrapConn(underlay net.Conn, authFunc func(map[string]string) string) (net.Conn, *proxy.TargetAddr, chan struct{}, error) {
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
