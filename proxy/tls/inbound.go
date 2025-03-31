package tls

import (
	stdtls "crypto/tls"
	"io"
	"log"
	"net"
	"strings"
	"sync"

	"github.com/ZIXT233/ziproxy/db"
	"github.com/ZIXT233/ziproxy/proxy"
	"github.com/ZIXT233/ziproxy/utils"
)

type Inbound struct {
	addr         string
	name         string
	config       map[string]interface{}
	tlsConfig    *stdtls.Config
	upper        proxy.Inbound
	closeChanSet sync.Map
	verifyByPsk  string
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
	proxy.RegisterInbound(scheme, TlsInboundCreator)
}
func TlsInboundCreator(proxyData *db.ProxyData) (proxy.Inbound, error) {
	config, _ := utils.UnmarshalConfig(proxyData.Config)

	certFile := config["cert"].(string)
	keyFile := config["key"].(string)
	cert, err := stdtls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		log.Printf("%s tls.LoadX509KeyPair err: %v", proxyData.ID, err)
		return nil, err
	}

	in := &Inbound{
		name:   proxyData.ID,
		config: config,
	}
	if v := config["verifyByPsk"]; v != nil {
		in.verifyByPsk = v.(string)
	} else {
		in.verifyByPsk = ""
	}
	in.tlsConfig = &stdtls.Config{
		InsecureSkipVerify: in.verifyByPsk != "",
		Certificates:       []stdtls.Certificate{cert},
	}

	if up, ok := config["upper"]; ok {
		upperConfig := utils.MarshalConfig(up.(map[string]interface{}))
		upper, err := proxy.InboundFromConfig(&db.ProxyData{
			ID:     proxyData.ID,
			Config: upperConfig,
		})
		if err != nil {
			return nil, err
		}
		in.upper = upper
		in.addr = upper.Addr()
	} else {
		in.addr = config["address"].(string)
	}
	return in, nil
}

func (in *Inbound) UnregCloseChan(closeChan chan struct{}) {
	if in.upper != nil {
		in.upper.UnregCloseChan(closeChan)
	} else {
		proxy.UnregCloseChan(&in.closeChanSet, closeChan)
	}
}
func (in *Inbound) CloseAllConn() {
	if in.upper != nil {
		in.upper.CloseAllConn()
	} else {
		proxy.CloseAllConn(&in.closeChanSet)
	}
}
func (in *Inbound) WrapConn(underlay net.Conn, authFunc func(map[string]string) string) (io.ReadWriter, *proxy.TargetAddr, chan struct{}, error) {
	tlsConn := stdtls.Server(underlay, in.tlsConfig)
	err := tlsConn.Handshake()
	if err != nil {
		return nil, nil, nil, err
	}
	if in.verifyByPsk != "" {
		mlen, _ := utils.CryptoRandomInRange(900, 1400)
		mess := strings.Repeat("233", mlen/3)
		_, err = tlsConn.Write([]byte(in.verifyByPsk + "\n" + mess + "\n"))
		if err != nil {
			return nil, nil, nil, err
		}
		utils.ReadUtil(tlsConn, '\n')
	}
	if in.upper != nil {
		return in.upper.WrapConn(tlsConn, authFunc)
	} else {
		closeChan := make(chan struct{})
		in.closeChanSet.LoadOrStore(closeChan, struct{}{})
		return tlsConn, nil, closeChan, nil
	}
}

func (in *Inbound) GetLinkConfig(defaultAccessAddr, token string) map[string]interface{} {
	config := make(map[string]interface{})
	config["scheme"] = scheme
	config["verifyByPsk"] = in.verifyByPsk
	if in.upper != nil {
		upperConfig := in.upper.GetLinkConfig(defaultAccessAddr, token)
		config["upper"] = upperConfig
	} else {
		config["address"] = proxy.GetLinkAddr(in, defaultAccessAddr)
	}
	return config
}
