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

// TLS出站代理模块中实现TLS握手，加密解密的IO流包装器函数
func (out *Outbound) WrapConn(underlay net.Conn, target *proxy.TargetAddr) (net.Conn, chan struct{}, error) {
	var sni string

	if out.addr != "direct" {
		//如果下一站是次级代理，SNI设置为从出站代理配置获取的次级代理域名
		sni, _, _ = net.SplitHostPort(out.addr)
	} else {
		//如果下一站时代理目标，SNI设置为代理目标的域名
		sni = target.Host()
	}
	//创建TLS Config对象
	out.tlsConfig = &stdtls.Config{
		ServerName:         sni,
		InsecureSkipVerify: out.verifyByPsk != "",
	}
	//利用crypto/tls包处理TLS IO流
	tlsConn := stdtls.Client(underlay, out.tlsConfig)
	err := tlsConn.Handshake()
	if err != nil {
		return nil, nil, err
	}
	//可选的PSK认证
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
	//处理上层叠加协议，返回包装后IO流
	if out.upper != nil {
		return out.upper.WrapConn(tlsConn, target)
	} else {
		closeChan := make(chan struct{})
		out.closeChanSet.LoadOrStore(closeChan, struct{}{})
		return tlsConn, closeChan, nil
	}
}
