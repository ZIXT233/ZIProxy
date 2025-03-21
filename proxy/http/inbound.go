package http

import (
	"crypto/tls"
	"fmt"
	"io"
	"log"
	"net"
	"net/url"
	"strings"

	"github.com/ZIXT233/ziproxy/proxy"
)

type Inbound struct {
	addr   string
	name   string
	config map[string]interface{}
	tls    *tls.Config
}

func (in *Inbound) Scheme() string                 { return scheme }
func (in *Inbound) Addr() string                   { return in.addr }
func (in *Inbound) Name() string                   { return in.name }
func (in *Inbound) Config() map[string]interface{} { return in.config }
func (in *Inbound) Stop()                          { return }

func init() {
	proxy.RegisterInbound(scheme, HttpInboundCreator)
	proxy.RegisterInbound("https", HttpInboundCreator)
}
func HttpInboundCreator(config map[string]interface{}) (proxy.Inbound, error) {
	in := &Inbound{
		addr:   config["address"].(string),
		name:   config["name"].(string),
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
func (in *Inbound) WrapConn(underlay net.Conn) (io.ReadWriter, *proxy.TargetAddr, error) {
	wrappedConn := &InConn{Conn: underlay}
	var b [1024]byte
	n, err := underlay.Read(b[:])
	if err != nil {
		log.Println("read", err)
		return nil, nil, err
	}
	var method, URL, address string
	var header map[string]string
	header = make(map[string]string)
	//split b by '\r\n
	headLines := strings.Split(string(b[:n]), "\r\n")
	for _, line := range headLines {
		log.Println(line)
		pair := strings.SplitN(line, ":", 2)
		if len(pair) == 2 {
			header[pair[0]] = strings.Trim(pair[1], "\r\n ")
		}
	}

	if in.config["auth"] != nil {
		auth := in.config["auth"].(map[string]interface{})
		if auth["username"].(string) != header["username"] || auth["password"].(string) != header["password"] {
			fmt.Fprint(underlay,
				"HTTP/1.1 200 OK\r\n"+
					"Content-Type: text/plain\r\n"+
					"Content-Length: 11\r\n"+
					"\r\n"+
					"Hello World")
			return nil, nil, proxy.ProxyEnd
		}
	}
	fmt.Sscanf(headLines[0], "%s%s", &method, &URL)
	if method == "CONNECT" {
		address = URL
	} else { //否则为 http 协议
		hostPortURL, err := url.Parse(URL)
		if err != nil {
			return nil, nil, err
		}
		address = hostPortURL.Host
		if strings.Index(hostPortURL.Host, ":") == -1 {
			address = hostPortURL.Host + ":80"
		}
	}

	targetAddr, err := proxy.NewTargetAddr(address)

	if err != nil {
		log.Println(err)
		return nil, nil, err
	}
	if method == "CONNECT" {
		fmt.Fprint(underlay, "HTTP/1.1 200 Connection established\r\n\r\n")
		wrappedConn.httpType = "https"
		wrappedConn.headBuf = nil
	} else { //如果使用 http 协议，需将从客户端得到的 http 请求转发给服务端
		wrappedConn.httpType = "http"
		wrappedConn.headBuf = b[:n]
	}

	return wrappedConn, targetAddr, nil
}
