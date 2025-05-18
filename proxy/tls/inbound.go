package tls

import (
	stdtls "crypto/tls"
	"fmt"
	"log"
	"net"
	"strings"
	"sync"

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
	proxy.RegisterInbound(scheme, TlsInboundCreator)
}

// TLS入站代理实例的创建函数，初始化TLS证书、PSK信息。
func TlsInboundCreator(name string, config map[string]interface{}) (proxy.Inbound, error) {
	certFile := config["cert"].(string)
	keyFile := config["key"].(string)
	cert, err := stdtls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		log.Printf("%s tls.LoadX509KeyPair err: %v", name, err)
		return nil, err
	}
	addr, ok := config["address"].(string)
	if !ok {
		return nil, fmt.Errorf("address is required")
	}
	in := &Inbound{
		addr:   addr,
		name:   name,
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

	_, err = proxy.UpperInboundCreate(in, config)
	return in, err
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

// TLS入站代理模块中实现TLS握手，加密解密的IO流包装器函数
func (in *Inbound) WrapConn(underlay net.Conn, authFunc func(map[string]string) string) (net.Conn, *proxy.TargetAddr, chan struct{}, error) {
	//利用crypto/tls包处理TLS IO流
	tlsConn := stdtls.Server(underlay, in.tlsConfig)
	//完成TLS握手
	err := tlsConn.Handshake()
	if err != nil {
		return nil, nil, nil, err
	}
	//可选的PSK认证
	if in.verifyByPsk != "" {
		//填充psk到随机长度，发送给客户端进行认证
		mlen, _ := utils.CryptoRandomInRange(900, 1400)
		mess := strings.Repeat("233", mlen/3)
		_, err = tlsConn.Write([]byte(in.verifyByPsk + "\n" + mess + "\n"))
		if err != nil {
			return nil, nil, nil, err
		}
		utils.ReadUtil(tlsConn, '\n')
	}
	//处理上层叠加协议，返回包装后IO流
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
	config["address"] = proxy.GetLinkAddr(in, defaultAccessAddr)
	if in.upper != nil {
		upperConfig := in.upper.GetLinkConfig(defaultAccessAddr, token)
		config["upper"] = upperConfig
	}
	return config
}
