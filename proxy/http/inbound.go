package http

import (
	"fmt"
	"io"
	"log"
	"net"
	"net/url"
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
	closeChanSet sync.Map
}

func (in *Inbound) Scheme() string                 { return scheme }
func (in *Inbound) Addr() string                   { return in.addr }
func (in *Inbound) Name() string                   { return in.name }
func (in *Inbound) Config() map[string]interface{} { return in.config }
func (in *Inbound) Stop() {
	in.CloseAllConn()
	return
}

func init() {
	proxy.RegisterInbound(scheme, HttpInboundCreator)
	proxy.RegisterInbound("https", HttpInboundCreator)
}
func HttpInboundCreator(proxyData *db.ProxyData) (proxy.Inbound, error) {
	config, _ := utils.UnmarshalConfig(proxyData.Config)
	in := &Inbound{
		addr:   config["address"].(string),
		name:   proxyData.ID,
		config: config,
	}
	return in, nil
}

type InConn struct {
	net.Conn
	headBuf  []byte
	httpType string
}

func (c *InConn) Read(b []byte) (n int, err error) {
	if c.headBuf != nil {
		n = copy(b, c.headBuf)
		c.headBuf = nil
		return n, nil
	} else {
		n, err = c.Conn.Read(b)
		return n, err
	}
}
func (c *InConn) Write(b []byte) (n int, err error) {
	return c.Conn.Write(b)
}

func (in *Inbound) UnregCloseChan(closeChan chan struct{}) {
	proxy.UnregCloseChan(&in.closeChanSet, closeChan)
}
func (in *Inbound) CloseAllConn() {
	proxy.CloseAllConn(&in.closeChanSet)
}
func (in *Inbound) WrapConn(underlay net.Conn, authFunc func(map[string]string) string) (io.ReadWriter, *proxy.TargetAddr, chan struct{}, error) {
	wrappedConn := &InConn{Conn: underlay}
	var b [1024]byte
	n, err := underlay.Read(b[:])
	if err != nil {
		log.Println("read", err)
		return nil, nil, nil, err
	}
	var method, URL, address string

	//split b by '\r\n
	headLines := strings.Split(string(b[:n]), "\r\n")
	fmt.Sscanf(headLines[0], "%s%s", &method, &URL)
	header := make(map[string]string)
	for _, line := range headLines {
		pair := strings.SplitN(line, ":", 2)
		if len(pair) == 2 {
			header[pair[0]] = strings.Trim(pair[1], "\r\n ")
		}
	}
	header["linkToken"] = strings.Trim(URL, "/ ")
	userId := authFunc(header)
	forward := in.config["guestForward"]
	if forward != nil && userId == "guest" {
		forwardAddr, ok := forward.(string)
		if !ok {
			return nil, nil, nil, fmt.Errorf("guestForward config error")
		}
		wrappedConn.headBuf = b[:n]
		forwardConn, err := net.Dial("tcp", forwardAddr)
		if err != nil {
			log.Println("dial", err)
			return nil, nil, nil, err
		}
		log.Printf("auth fail, forward to %s", forwardAddr)
		go io.Copy(forwardConn, wrappedConn)
		io.Copy(wrappedConn, forwardConn)
		return nil, nil, nil, fmt.Errorf("auth fail")
	}
	if method == "CONNECT" {
		address = URL
	} else { //否则为 http 协议
		hostPortURL, err := url.Parse(URL)
		if err != nil {
			return nil, nil, nil, err
		}
		address = hostPortURL.Host
		if strings.Index(hostPortURL.Host, ":") == -1 {
			address = hostPortURL.Host + ":80"
		}
	}

	targetAddr, err := proxy.NewTargetAddr(address)
	targetAddr.UserId = userId

	if err != nil {
		log.Println(err)
		return nil, nil, nil, err
	}
	if method == "CONNECT" {
		fmt.Fprint(underlay, "HTTP/1.1 200 Connection established\r\n\r\n")
		wrappedConn.httpType = "https"
		wrappedConn.headBuf = nil
	} else { //如果使用 http 协议，需将从客户端得到的 http 请求转发给服务端
		wrappedConn.httpType = "http"
		wrappedConn.headBuf = b[:n]
	}
	closeChan := make(chan struct{})
	in.closeChanSet.LoadOrStore(closeChan, struct{}{})
	return wrappedConn, targetAddr, closeChan, nil
}

func (in *Inbound) GetLinkConfig(defaultAccessAddr, token string) map[string]interface{} {
	addr := proxy.GetLinkAddr(in, defaultAccessAddr)
	config := map[string]interface{}{
		"scheme":    in.Scheme(),
		"address":   addr,
		"url":       in.config["scheme"].(string) + "://" + in.Addr(),
		"linkToken": token,
	}
	return config
}
