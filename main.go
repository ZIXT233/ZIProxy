package main

import (
	"encoding/json"
	"flag"
	"github.com/ZIXT233/ziproxy/app/router"
	"github.com/ZIXT233/ziproxy/proxy"
	_ "github.com/ZIXT233/ziproxy/proxy/direct"
	_ "github.com/ZIXT233/ziproxy/proxy/http"
	_ "github.com/ZIXT233/ziproxy/proxy/tcp"
	_ "github.com/ZIXT233/ziproxy/proxy/tls"
	"io"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"
)

var (
	configFile = flag.String("f", "config.json", "config file")
)

type Config struct {
	Inbounds  []interface{} `json:"inbounds"`
	Outbounds []interface{} `json:"outbounds"`
}

func loadConfig(file string) (*Config, error) {
	config := &Config{}
	data, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal(data, config)
	if err != nil {
		return nil, err
	}
	return config, nil
}
func loadBounds(inboundConfig []interface{}, outboundConfig []interface{}) (map[string]proxy.Inbound, map[string]proxy.Outbound, error) {
	inbounds := make(map[string]proxy.Inbound)
	outbounds := make(map[string]proxy.Outbound)
	for _, c := range inboundConfig {
		config := c.(map[string]interface{})
		inbound, err := proxy.InboundFromConfig(config)
		if err != nil {
			return nil, nil, err
		}
		inbounds[inbound.Name()] = inbound
	}
	for _, c := range outboundConfig {
		config := c.(map[string]interface{})
		outbound, err := proxy.OutboundFromConfig(config)
		if err != nil {
			return nil, nil, err
		}
		outbounds[outbound.Name()] = outbound
	}
	return inbounds, outbounds, nil
}
func launchProxy(config *Config) {
	inbounds, outbounds, err := loadBounds(config.Inbounds, config.Outbounds)
	if err != nil {
		log.Fatal(err)
	}
	//根据inbound列表创建对应的监听协程
	for _, inbound := range inbounds {
		go func() {
			listener, err := net.Listen("tcp", inbound.Addr())
			if err != nil {
				log.Fatal(err)
			}
			//创建服务连接
			for {
				tcpConn, err := listener.Accept()
				if err != nil {
					log.Println(err)
					continue
				}
				//拉一个协程处理连接
				go func() {
					defer tcpConn.Close()
					wrappedInConn, targetAddr, err := inbound.WrapConn(tcpConn)
					if err != nil {
						log.Println("wrap in conn", err)
						return
					}

					outboundName := router.MatchOutbound(targetAddr)
					outbound, ok := outbounds[outboundName]
					if !ok {
						log.Println("outbound not found")
						return
					}

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
					wrappedOutConn, err := outbound.WrapConn(outConn, targetAddr)
					if err != nil {
						log.Println("wrap out conn ", err)
						return
					}

					log.Printf("proxy from %s via %s to %s", inbound.Name(), outbound.Name(), targetAddr)
					go io.Copy(wrappedOutConn, wrappedInConn)
					io.Copy(wrappedInConn, wrappedOutConn)
				}()
			}
		}()
	}

}
func main() {
	flag.Parse()
	config, err := loadConfig(*configFile)
	if err != nil {
		log.Fatal(err)
	}
	launchProxy(config)
	osSignals := make(chan os.Signal, 1)
	signal.Notify(osSignals, os.Interrupt, os.Kill, syscall.SIGTERM)
	<-osSignals
}
