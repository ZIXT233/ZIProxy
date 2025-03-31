package tls

import (
	stdtls "crypto/tls"
	"fmt"
	"io"
	"net"
	"strings"
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
	verifyByPsk  string
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
	if v := config["verifyByPsk"]; v != nil {
		out.verifyByPsk = v.(string)
	} else {
		out.verifyByPsk = ""
	}
	out.tlsConfig = &stdtls.Config{
		ServerName:         sni,
		InsecureSkipVerify: out.verifyByPsk != "",
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
