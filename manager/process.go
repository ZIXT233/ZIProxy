package manager

import (
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
	listener, err := net.Listen("tcp", inbound.Addr())
	if err != nil {
		log.Println(err)
		return nil, err
	}
	go func() {
		//创建服务连接
		for {
			inConn, err := listener.Accept()
			if err != nil {
				if err == net.ErrClosed {
					log.Printf("Inbound %s process end", inbound.Name())
					inbound.Stop()
					break
				} else {
					log.Printf("Inbound %s accept %s fail", inbound.Name(), inConn.RemoteAddr().String())
					continue
				}
			}
			//拉一个协程处理连接
			go func() {
				defer inConn.Close()
				wrappedInConn, targetAddr, inCloseChan, err := inbound.WrapConn(inConn, proxyAuth)
				defer inbound.UnregCloseChan(inCloseChan)
				if err != nil {
					log.Printf("inbound %s recieve %s fail", inbound.Name(), inConn.RemoteAddr().String())
					return
				}

				outboundName := RouteOutbound(targetAddr)

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

				outConn = createConnWithTimeout(outConn, time.Second*10)
				defer outConn.Close()
				wrappedOutConn, outCloseChan, err := outbound.WrapConn(outConn, targetAddr)
				defer outbound.UnregCloseChan(outCloseChan)
				if err != nil {
					log.Println("wrap out conn ", err)
					return
				}

				commonCloseChan := make(chan struct{})
				log.Printf("Start %s@%s ---> %s ---> %s\t\tNow Goroutine:%d", targetAddr.UserId, inbound.Name(), outbound.Name(), targetAddr, runtime.NumGoroutine())
				//用于监听关闭信号的协程
				go func() {
					var reason string
					select {
					case <-outCloseChan:
						reason = "outbound closed"
						outConn.Close()
					case <-inCloseChan:
						reason = "inbound closed"
						inConn.Close()
					case <-commonCloseChan:
						if outConn.(*ConnWithTimeout).IsTimeout {
							reason = "no data transfer in 10s"
						} else {
							reason = "transport finished"
						}
					}
					log.Printf("End   %s@%s ---> %s ---> %s\t\tdue to %s\tNow Goroutine:%d", targetAddr.UserId, inbound.Name(), outbound.Name(), targetAddr, reason, runtime.NumGoroutine())
				}()

				statisticOutConn := StatisticWrap(wrappedOutConn)
				addActiveUserLink(targetAddr.UserId)
				{
					go io.Copy(statisticOutConn, wrappedInConn)
					io.Copy(wrappedInConn, statisticOutConn)
				}
				subActiveUserLink(targetAddr.UserId)
				select {
				case commonCloseChan <- struct{}{}:
				default:
				}

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
		for {
			_, alive = InboundProcListenerMap[id]
			if !alive {
				return
			}
			time.Sleep(time.Millisecond * 10)
		}
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
