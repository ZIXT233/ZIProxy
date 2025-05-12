package tls

import (
	stdtls "crypto/tls"
	"fmt"
	"net"
	"strings"
	"sync"

	"github.com/ZIXT233/ziproxy/proxy"
	"github.com/ZIXT233/ziproxy/utils"
)

type Outbound struct {
	addr         string
	name         string
	config       map[string]interface{}
	upper        proxy.Outbound
	tlsConfig    *stdtls.Config
	closeChanSet sync.Map
	verifyByPsk  string
}

func (out *Outbound) Scheme() string                 { return scheme }
func (out *Outbound) Addr() string                   { return out.addr }
func (out *Outbound) Name() string                   { return out.name }
func (out *Outbound) Config() map[string]interface{} { return out.config }
func (out *Outbound) SetAddr(addr string) {
	out.addr = addr
}
func (out *Outbound) SetUpper(upper proxy.Outbound) {
	out.upper = upper
}

func init() {
	proxy.RegisterOutbound(scheme, TlsOutboundCreator)
}
func TlsOutboundCreator(name string, config map[string]interface{}) (proxy.Outbound, error) {
	addr, ok := config["address"].(string)
	if !ok {
		return nil, fmt.Errorf("address is required")
	}
	out := &Outbound{
		addr:   addr,
		name:   name,
		config: config,
	}
	_, err := proxy.UpperOutboundCreate(out, config)
	if err != nil {
		return nil, err
	}
	if v := config["verifyByPsk"]; v != nil {
		out.verifyByPsk = v.(string)
	} else {
		out.verifyByPsk = ""
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
func (out *Outbound) WrapConn(underlay net.Conn, target *proxy.TargetAddr) (net.Conn, chan struct{}, error) {
	var sni string
	if out.addr != "direct" {
		sni, _, _ = net.SplitHostPort(out.addr)
	} else {
		sni = target.Host()
	}
	out.tlsConfig = &stdtls.Config{
		ServerName:         sni,
		InsecureSkipVerify: out.verifyByPsk != "",
	}
	tlsConn := stdtls.Client(underlay, out.tlsConfig)
	err := tlsConn.Handshake()
	if err != nil {
		return nil, nil, err
	}
	if out.verifyByPsk != "" {
		buf, err := utils.ReadUtil(tlsConn, '\n')
		if err != nil {
			return nil, nil, err
		}
		pskLen := len(out.verifyByPsk)
		if string(buf[:pskLen]) != out.verifyByPsk {
			return nil, nil, fmt.Errorf("invalid psk")
		}
		utils.ReadUtil(tlsConn, '\n')
		mlen, _ := utils.CryptoRandomInRange(100, 200)
		mess := strings.Repeat("233", mlen/3)
		_, err = tlsConn.Write([]byte(mess + "\n"))
	}
	if out.upper != nil {
		return out.upper.WrapConn(tlsConn, target)
	} else {
		closeChan := make(chan struct{})
		out.closeChanSet.LoadOrStore(closeChan, struct{}{})
		return tlsConn, closeChan, nil
	}
}
