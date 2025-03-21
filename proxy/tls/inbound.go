package tls

import (
	stdtls "crypto/tls"
	"github.com/ZIXT233/ziproxy/proxy"
	"io"
	"log"
	"net"
)

type Inbound struct {
	addr      string
	name      string
	config    map[string]interface{}
	tlsConfig *stdtls.Config
	upper     proxy.Inbound
}

func (in *Inbound) Name() string                   { return in.name }
func (in *Inbound) Scheme() string                 { return scheme }
func (in *Inbound) Addr() string                   { return in.addr }
func (in *Inbound) Config() map[string]interface{} { return in.config }
func (in *Inbound) Stop()                          { return }

func init() {
	proxy.RegisterInbound(scheme, TlsInboundCreator)
}
func TlsInboundCreator(config map[string]interface{}) (proxy.Inbound, error) {
	certFile := config["cert"].(string)
	keyFile := config["key"].(string)
	cert, err := stdtls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		log.Fatalf("tls.LoadX509KeyPair err: %v", err)
		return nil, err
	}

	in := &Inbound{
		config: config,
	}
	in.tlsConfig = &stdtls.Config{
		InsecureSkipVerify: false,
		Certificates:       []stdtls.Certificate{cert},
	}

	upperConfig := config["upper"].(map[string]interface{})
	if upperConfig != nil {
		upper, err := proxy.InboundFromConfig(upperConfig)
		if err != nil {
			return nil, err
		}
		in.upper = upper
		in.addr = upper.Addr()
		in.name = upper.Name()
	} else {
		in.addr = config["address"].(string)
		in.name = config["name"].(string)
	}
	return in, nil
}

func (in *Inbound) WrapConn(underlay net.Conn) (io.ReadWriter, *proxy.TargetAddr, error) {
	tlsConn := stdtls.Server(underlay, in.tlsConfig)
	err := tlsConn.Handshake()
	if err != nil {
		return nil, nil, err
	}
	if in.upper != nil {
		return in.upper.WrapConn(tlsConn)
	} else {
		return tlsConn, nil, nil
	}
}
