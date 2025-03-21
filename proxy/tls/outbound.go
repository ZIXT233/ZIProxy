package tls

import (
	stdtls "crypto/tls"
	"github.com/ZIXT233/ziproxy/proxy"
	"io"
	"net"
)

type Outbound struct {
	addr      string
	name      string
	config    map[string]interface{}
	tlsConfig *stdtls.Config
	upper     proxy.Outbound
}

func init() {
	proxy.RegisterOutbound(scheme, TlsOutboundCreator)
}
func TlsOutboundCreator(config map[string]interface{}) (proxy.Outbound, error) {

	out := &Outbound{
		config: config,
	}

	upperConfig := config["upper"].(map[string]interface{})
	if upperConfig != nil {
		upper, err := proxy.OutboundFromConfig(upperConfig)
		if err != nil {
			return nil, err
		}
		out.upper = upper
		out.addr = upper.Addr()
		out.name = upper.Name()
	} else {
		out.addr = config["address"].(string)
		out.name = config["name"].(string)
	}

	sni, _, _ := net.SplitHostPort(out.addr)
	out.tlsConfig = &stdtls.Config{
		ServerName:         sni,
		InsecureSkipVerify: false,
	}
	return out, nil
}

func (out *Outbound) WrapConn(underlay net.Conn, target *proxy.TargetAddr) (io.ReadWriter, error) {
	tlsConn := stdtls.Client(underlay, out.tlsConfig)
	err := tlsConn.Handshake()
	if err != nil {
		return nil, err
	}
	if out.upper != nil {
		return out.upper.WrapConn(tlsConn, target)
	} else {
		return tlsConn, nil
	}
}
