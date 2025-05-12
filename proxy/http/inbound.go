package http

import (
	"fmt"
	"io"
	"log"
	"net"
	"net/url"
	"strings"
	"sync"

	"github.com/ZIXT233/ziproxy/proxy"
	"github.com/ZIXT233/ziproxy/utils"
)

type Inbound struct {
	addr         string
	name         string
	upper        proxy.Inbound
	config       map[string]interface{}
	closeChanSet sync.Map
}

func (in *Inbound) Scheme() string                 { return scheme }
func (in *Inbound) Addr() string                   { return in.addr }
func (in *Inbound) Name() string                   { return in.name }
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
	proxy.RegisterInbound(scheme, HttpInboundCreator)
	proxy.RegisterInbound("https", HttpInboundCreator)
}
func HttpInboundCreator(name string, config map[string]interface{}) (proxy.Inbound, error) {
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

type InConn struct {
	net.Conn
	httpType string
}

func (in *Inbound) UnregCloseChan(closeChan chan struct{}) {
	proxy.UnregCloseChan(&in.closeChanSet, closeChan)
}
func (in *Inbound) CloseAllConn() {
	proxy.CloseAllConn(&in.closeChanSet)
}
func (in *Inbound) WrapConn(underlay net.Conn, authFunc func(map[string]string) string) (net.Conn, *proxy.TargetAddr, chan struct{}, error) {
	var b []byte
	peekConn := utils.NewPeekConn(underlay)
	wrappedConn := &InConn{Conn: peekConn}

	b, err := peekConn.Peek(1024)
	if err != nil {
		log.Println("read", err)
		return nil, nil, nil, err
	}
	var method, URL, address string
	n := len(b)
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
	if header["linkToken"] == "" {
		header["linkToken"] = strings.Trim(URL, "/ ")
	}

	userId := authFunc(header)
	forward := in.config["guestForward"]
	if forward != nil && userId == "guest" {
		forwardAddr, ok := forward.(string)
		if !ok {
			return nil, nil, nil, fmt.Errorf("guestForward config error")
		}
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
		tmp := make([]byte, 1024)
		wrappedConn.Read(tmp)
		fmt.Fprint(wrappedConn, "HTTP/1.1 200 Connection established\r\n\r\n")
		wrappedConn.httpType = "https"
	} else { //如果使用 http 协议，需将从客户端得到的 http 请求转发给服务端
		wrappedConn.httpType = "http"
	}

	if in.upper != nil {
		innerConn, subTarget, closeChan, err := in.upper.WrapConn(wrappedConn, authFunc)
		if err != nil {
			return nil, nil, nil, err
		}
		targetAddr.Custom = subTarget.Custom
		return innerConn, targetAddr, closeChan, err
	} else {
		closeChan := make(chan struct{})
		in.closeChanSet.LoadOrStore(closeChan, struct{}{})
		return wrappedConn, targetAddr, closeChan, nil
	}
}

func (in *Inbound) GetLinkConfig(defaultAccessAddr, token string) map[string]interface{} {
	addr := proxy.GetLinkAddr(in, defaultAccessAddr)
	config := map[string]interface{}{
		"scheme":    in.config["scheme"].(string),
		"address":   addr,
		"url":       in.config["scheme"].(string) + "://" + addr,
		"linkToken": token,
	}
	return config
}
