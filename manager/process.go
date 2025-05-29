package manager

import (
	"errors"
	"io"
	"log"
	"net"
	"runtime"
	"time"

	"github.com/ZIXT233/ziproxy/proxy"
	_ "github.com/ZIXT233/ziproxy/proxy/direct"
	_ "github.com/ZIXT233/ziproxy/proxy/http"
	_ "github.com/ZIXT233/ziproxy/proxy/raw"
	_ "github.com/ZIXT233/ziproxy/proxy/tls"
)

func addActiveUserLink(userId string) {
	ActiveUserLinkMu.Lock()
	ActiveUserLink[userId]++
	ActiveUserLinkMu.Unlock()
}
func subActiveUserLink(userId string) {
	ActiveUserLinkMu.Lock()
	ActiveUserLink[userId]--
	if ActiveUserLink[userId] == 0 {
		delete(ActiveUserLink, userId)
	}
	ActiveUserLinkMu.Unlock()
}

func InboundProcess(inbound proxy.Inbound) (net.Listener, error) {
	//根据入站代理配置监听对应网络地址和端口
	listener, err := net.Listen("tcp", inbound.Addr())
	if err != nil {
		log.Println(err)
		return nil, err
	}
	//创建监听协程
	go func() {
		log.Printf("Inbound %s process listening on %s", inbound.Name(), listener.Addr())
		//循环进行连接的建立
		for {
			inConn, err := listener.Accept()
			if err != nil {
				if errors.Is(err, net.ErrClosed) {
					log.Printf("Inbound %s process end", inbound.Name())
					inbound.Stop()
					break
				} else {
					log.Printf("Inbound %s accept fail", inbound.Name())
					continue
				}
			}
			//新建一个协程处理连接，建立流量通道
			go func() {
				defer inConn.Close()
				// 通过入站代理实例对应的包装器函数包装代理流量，从而可对流量进行入站代理协议处理，函数返回包装后IO流
				wrappedInConn, targetAddr, inCloseChan, err := inbound.WrapConn(inConn, proxyAuth)
				defer inbound.UnregCloseChan(inCloseChan)
				if err != nil {
					log.Printf("inbound %s recieve %s fail", inbound.Name(), inConn.RemoteAddr().String())
					return
				}
				//通过路由模块进行出站代理匹配
				outboundName := RouteOutbound(targetAddr, inbound.Name())
				//通过出站代理ID获取出站代理实例
				val, ok := OutboundMap.Load(outboundName)
				if !ok {
					if outboundName == "block" {
						log.Printf("Block %s@%s ---> %s\t\tNow Goroutine:%d", targetAddr.UserId, inbound.Name(), targetAddr, runtime.NumGoroutine())
					} else {
						log.Printf("Outbound %s not found", outboundName)
					}

					return
				}
				outbound := val.(proxy.Outbound)

				//建立与下一级网络目标的连接
				var dialAddr string
				if outbound.Addr() == "direct" {
					dialAddr = targetAddr.String()
				} else {
					dialAddr = outbound.Addr()
				}

				outConn, err := net.Dial("tcp", dialAddr)
				if err != nil {
					log.Println("dial out conn ", err)
					return
				}

				//连接超时统计模块
				outConn = createConnWithTimeout(outConn, time.Second*10)
				defer outConn.Close()

				// 通过出站代理实例对应的包装器函数包装代理流量，从而可对流量进行出站代理协议处理，函数返回包装后IO流
				wrappedOutConn, outCloseChan, err := outbound.WrapConn(outConn, targetAddr)
				defer outbound.UnregCloseChan(outCloseChan)
				if err != nil {
					log.Println("wrap out conn ", err)
					return
				}

				commonCloseChan := make(chan struct{})
				log.Printf("Start %s@%s ---> %s ---> %s\t\tNow Goroutine:%d", targetAddr.UserId, inbound.Name(), outbound.Name(), targetAddr, runtime.NumGoroutine())
				//用于监听关闭信号，及时关闭当前流量通道的协程，确保并发可靠性
				go func() {
					var reason string
					select {
					case <-outCloseChan:
						reason = "outbound closed"
					case <-inCloseChan:
						reason = "inbound closed"
					case <-commonCloseChan:
						if outConn.(*ConnWithTimeout).IsTimeout {
							reason = "no data transfer in 10s"
						} else {
							reason = "transport finished"
						}
					}
					inConn.Close()
					outConn.Close()
					log.Printf("End   %s@%s ---> %s ---> %s\t\tdue to %s\tNow Goroutine:%d", targetAddr.UserId, inbound.Name(), outbound.Name(), targetAddr, reason, runtime.NumGoroutine())
				}()
				//流量统计模块
				statisticOutConn := StatisticWrap(wrappedOutConn)
				//更新用户连接数
				addActiveUserLink(targetAddr.UserId)
				//将Inbound侧IO流与Outbound侧IO流进行连接，完成流量转发
				{
					tmp, ok := inbound.Config()["use_http_cache"]
					use_http_cache := false
					if ok {
						use_http_cache, _ = tmp.(bool)
					}

					if use_http_cache {
						//如果使用HTTP缓存代理
						//MITM中间人解密模块
						decryptInConn, isTLS, err := TLS_MITM_to_client(wrappedInConn)
						if err != nil {
							log.Println("TLS_MITM_to_client", err)
							return
						}
						decryptOutConn, err := TLS_MITM_to_server(statisticOutConn, targetAddr.Host(), isTLS)
						if err != nil {
							log.Println("TLS_MITM_to_server", err)
							return
						}
						//在httpCache模块中完成对两侧流量的解析、缓存和转发
						httpCache(decryptInConn, decryptOutConn)
					} else {
						//如果不使用HTTP缓存代理
						//建立隧道代理，直接连接两侧IO流
						go io.Copy(statisticOutConn, wrappedInConn)
						io.Copy(wrappedInConn, statisticOutConn)
					}
				}
				subActiveUserLink(targetAddr.UserId)
				//连接正常结束时发送连接正常关闭信号给流量通道关闭协程
				select {
				case commonCloseChan <- struct{}{}:
				default:
				}
				//将流量统计信息记录到数据库
				statisticOutConn.AddToDB(inbound.Name(), outbound.Name(), targetAddr.UserId, targetAddr.String())

			}()
		}
	}()
	return listener, nil
}

var InboundProcListenerMap = make(map[string]net.Listener)

func attachInboundProc(id string) (err error) {
	stopInboundProc(id)
	inbound, ok := InboundMap.Load(id)
	if ok {
		listener, err := InboundProcess(inbound.(proxy.Inbound))
		if err != nil {
			return err
		}
		InboundProcListenerMap[id] = listener
	}
	return err
}
func stopInboundProc(id string) {
	listener, alive := InboundProcListenerMap[id]
	if alive {
		listener.Close()
		delete(InboundProcListenerMap, id)
		time.Sleep(time.Millisecond * 10)
	}
}

func IsInboundProcRunning(id string) (running bool) {
	_, ok := InboundProcListenerMap[id]
	return ok
}

type ConnWithTimeout struct {
	net.Conn
	lastActiveTime time.Time
	nowTime        time.Time
	timeout        time.Duration
	IsTimeout      bool
	tickerStop     chan struct{}
}

func createConnWithTimeout(conn net.Conn, timeout time.Duration) *ConnWithTimeout {
	c := &ConnWithTimeout{
		Conn:           conn,
		lastActiveTime: time.Now(),
		nowTime:        time.Now(),
		timeout:        timeout,
		IsTimeout:      false,
		tickerStop:     make(chan struct{}),
	}
	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case c.nowTime = <-ticker.C:
				if c.nowTime.Sub(c.lastActiveTime) > c.timeout {
					c.Close()
					c.IsTimeout = true
					return
				}
			case <-c.tickerStop:
				return
			}
		}
	}()
	return c
}
func (c *ConnWithTimeout) Read(b []byte) (n int, err error) {
	n, err = c.Conn.Read(b)
	c.lastActiveTime = c.nowTime

	return n, err
}
func (c *ConnWithTimeout) Write(b []byte) (n int, err error) {
	n, err = c.Conn.Write(b)
	c.lastActiveTime = c.nowTime
	return n, err
}

func (c *ConnWithTimeout) Close() error {
	select {
	case c.tickerStop <- struct{}{}:
	default:
	}
	return c.Conn.Close()
}
