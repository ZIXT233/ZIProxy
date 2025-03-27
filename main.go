package main

import (
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/ZIXT233/ziproxy/app/web"
	"github.com/ZIXT233/ziproxy/manager"
	_ "github.com/ZIXT233/ziproxy/proxy/direct"
	_ "github.com/ZIXT233/ziproxy/proxy/http"
	_ "github.com/ZIXT233/ziproxy/proxy/raw"
	_ "github.com/ZIXT233/ziproxy/proxy/tls"
	"github.com/ZIXT233/ziproxy/utils"
)

var (
	configFile = flag.String("c", "config.json", "config file")
)

const (
	Version = "v1.0.0"
)

func main() {
	flag.Parse()
	config, err := utils.LoadRootConfig(*configFile)
	if err != nil {
		log.Fatal(err)
	}
	manager.Start(config, Version)
	web.StartWeb(config)
	osSignals := make(chan os.Signal, 1)
	signal.Notify(osSignals, os.Interrupt, os.Kill, syscall.SIGTERM)
	<-osSignals
}
