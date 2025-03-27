package manager

import (
	"io"
	"log"
	"net"
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

func InboundProcess(inbound proxy.Inbound) (chan struct{}, error) {
	stopChan := make(chan struct{})
	listener, err := net.Listen("tcp", inbound.Addr())
	if err != nil {
		log.Println(err)
		return nil, err
	}
	go func() {
		acceptEnd := false
		go func() {
			log.Printf("%s Listening on %s", inbound.Name(), listener.Addr())
			<-stopChan
			log.Printf("%s End Listening on %s", inbound.Name(), listener.Addr())
			acceptEnd = true
			listener.Close()
			inbound.Stop()
			delete(InboundProcStopChanMap, inbound.Name())
		}()
		//创建服务连接
		for {
			inConn, err := listener.Accept()
			if err != nil {
				if acceptEnd {
					log.Printf("inbound %s monitor end", inbound.Name())
				} else {
					log.Printf("inbound %s accept %s fail", inbound.Name(), inConn.RemoteAddr().String())
				}
				break
			}
			//拉一个协程处理连接
			go func() {
				defer inConn.Close()
				wrappedInConn, targetAddr, inCloseChan, err := inbound.WrapConn(inConn, proxyAuth)
				if err != nil {
					log.Printf("inbound %s recieve %s fail", inbound.Name(), inConn.RemoteAddr().String())
					return
				}

				outboundName := RouteOutbound(targetAddr)

				val, ok := OutboundMap.Load(outboundName)
				if !ok {
					log.Println("outbound not found")
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
				defer outConn.Close()
				wrappedOutConn, outCloseChan, err := outbound.WrapConn(outConn, targetAddr)
				if err != nil {
					log.Println("wrap out conn ", err)
					return
				}

				log.Printf("Start %s@%s ---> %s ---> %s", targetAddr.UserId, inbound.Name(), outbound.Name(), targetAddr)
				go func() {
					select {
					case <-outCloseChan:
						log.Printf("outbound %s closed", outbound.Name())
					case <-inCloseChan:

						log.Printf("inbound %s closed", inbound.Name())
					}
					outConn.Close()
					inConn.Close()
				}()

				statisticOutConn := StatisticWrap(wrappedOutConn)
				addActiveUserLink(targetAddr.UserId)
				{
					go io.Copy(statisticOutConn, wrappedInConn)
					io.Copy(wrappedInConn, statisticOutConn)
				}
				subActiveUserLink(targetAddr.UserId)
				inbound.UnregCloseChan(inCloseChan)
				outbound.UnregCloseChan(outCloseChan)

				statisticOutConn.AddToDB(inbound.Name(), outbound.Name(), targetAddr.UserId, targetAddr.String())

				log.Printf("End   %s@%s ---> %s ---> %s", targetAddr.UserId, inbound.Name(), outbound.Name(), targetAddr)
			}()
		}
	}()
	return stopChan, nil
}

var InboundProcStopChanMap = make(map[string]chan struct{})

func attachInboundProc(id string) (err error) {
	stopInboundProc(id)
	inbound, ok := InboundMap.Load(id)
	if ok {
		InboundProcStopChanMap[id], err = InboundProcess(inbound.(proxy.Inbound))
		if err != nil {
			delete(InboundProcStopChanMap, id)
		}
	}
	return err
}
func stopInboundProc(id string) {
	stopChan, ok := InboundProcStopChanMap[id]
	if ok {
		log.Printf("send stop inbound proc %s", id)
		stopChan <- struct{}{}
		log.Printf("sned ok stop inbound proc %s", id)
		for {
			_, alive := InboundProcStopChanMap[id]
			if !alive {
				break
			}
			time.Sleep(time.Millisecond * 10)
		}
	}
}

func IsInboundProcRunning(id string) (running bool) {
	_, ok := InboundProcStopChanMap[id]
	return ok
}
