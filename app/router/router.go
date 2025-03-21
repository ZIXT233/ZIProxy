package router

import (
	"github.com/ZIXT233/ziproxy/proxy"
	"github.com/metacubex/geo/geoip"
	"github.com/metacubex/geo/geosite"
	"log"
)

var (
	siteDb *geosite.Database

	ipDb *geoip.Database
)

func init() {
	var err error
	siteDb, err = geosite.FromFile("geosite.dat")
	if err != nil {
		log.Fatal("failed to load geosite.dat:", err)
	}
	ipDb, err = geoip.FromFile("geoip.dat")
	if err != nil {
		log.Fatal("failed to load geoip.dat:", err)
	}
}
func MatchOutbound(target *proxy.TargetAddr) string {
	var codes []string
	if target.Name != "" {
		codes = siteDb.LookupCodes(target.Name)
	} else {
		codes = ipDb.LookupCode(target.IP)
	}
	for _, code := range codes {
		if code == "geolocation-!cn" {
			return "v2ray"
		}
	}
	return "direct"
}
