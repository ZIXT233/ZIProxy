package tls

import (
	stdtls "crypto/tls"
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
	tlsConfig    *stdtls.Config
	upper        proxy.Outbound
	closeChanSet sync.Map
}

func init() {
	proxy.RegisterOutbound(scheme, TlsOutboundCreator)
}
func TlsOutboundCreator(proxyData *db.ProxyData) (proxy.Outbound, error) {
	config, _ := utils.UnmarshalConfig(proxyData.Config)
	out := &Outbound{
		name:   proxyData.ID,
		config: config,
	}

	if up, ok := config["upper"]; ok {
		upperConfig := utils.MarshalConfig(up.(map[string]interface{}))
		upper, err := proxy.OutboundFromConfig(&db.ProxyData{
			ID:     proxyData.ID,
			Config: upperConfig,
		})
		if err != nil {
			return nil, err
		}
		out.upper = upper
		out.addr = upper.Addr()
	} else {
		out.addr = config["address"].(string)
	}

	sni, _, _ := net.SplitHostPort(out.addr)
	out.tlsConfig = &stdtls.Config{
		ServerName:         sni,
		InsecureSkipVerify: false,
	}
	return out, nil
}

func (out *Outbound) UnregCloseChan(closeChan chan struct{}) {
	if out.upper != nil {
		out.upper.UnregCloseChan(closeChan)
	} else {
		proxy.UnregCloseChan(&out.closeChanSet, closeChan)
	}
}
func (out *Outbound) CloseAllConn() {
	if out.upper != nil {
		out.upper.CloseAllConn()
	} else {
		proxy.CloseAllConn(&out.closeChanSet)
	}
}
func (out *Outbound) WrapConn(underlay net.Conn, target *proxy.TargetAddr) (io.ReadWriter, chan struct{}, error) {
	tlsConn := stdtls.Client(underlay, out.tlsConfig)
	err := tlsConn.Handshake()
	if err != nil {
		return nil, nil, err
	}
	if out.upper != nil {
		return out.upper.WrapConn(tlsConn, target)
	} else {
		closeChan := make(chan struct{})
		out.closeChanSet.LoadOrStore(closeChan, struct{}{})
		return tlsConn, closeChan, nil
	}
}
