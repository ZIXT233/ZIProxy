package manager

import (
	"log"
	"math/rand"
	"net"
	"sort"
	"strings"

	"github.com/ZIXT233/ziproxy/db"
	"github.com/ZIXT233/ziproxy/proxy"
	"github.com/metacubex/geo/geoip"
	"github.com/metacubex/geo/geosite"
)

var (
	siteDb *geosite.Database

	ipDb *geoip.Database
)

func initRouter(geoDir string) {
	var err error
	siteDb, err = geosite.FromFile(geoDir + "/geosite.dat")
	if err != nil {
		log.Fatal("failed to load geosite.dat:", err)
	}
	ipDb, err = geoip.FromFile(geoDir + "/geoip.dat")
	if err != nil {
		log.Fatal("failed to load geoip.dat:", err)
	}
}
func matchDomain(pattern, domain string) bool {
	if pattern == "*" {
		return true // 匹配任何域名
	}
	patternParts := strings.Split(pattern, ".")
	domainParts := strings.Split(domain, ".")

	if len(patternParts) != len(domainParts) {
		return false
	}

	for i := range patternParts {
		if patternParts[i] != "*" && patternParts[i] != domainParts[i] {
			return false
		}
	}

	return true
}

func matchGeo(pattern string, codes []string) bool {
	for _, code := range codes {
		if code == pattern {
			return true
		}
	}
	return false
}

func matchIP(pattern string, ip net.IP) bool {
	// 如果pattern是*，则匹配任何IP
	if pattern == "*" {
		return true
	}
	// 如果pattern是CIDR，则比较IP地址和CIDR
	if strings.Contains(pattern, "/") {
		_, cidr, err := net.ParseCIDR(pattern)
		if err != nil {
			return false
		}
		return cidr.Contains(ip)
	}
	// 如果pattern是IP地址，则直接比较
	if strings.Contains(pattern, ".") {
		return pattern == ip.String()
	}
	return false
}

func RouteOutbound(target *proxy.TargetAddr, inboundName string) string {
	var geoCodes []string
	//查询代理目标的地理位置或着组织信息
	if target.Hostname != "" {
		geoCodes = siteDb.LookupCodes(target.Hostname)
	} else {
		geoCodes = ipDb.LookupCode(target.IP)
	}

	if user, ok := UserMap.Load(target.UserId); ok {
		//根据代理用户查询对应代理用户组
		if userGroup, ok := UserGroupMap.Load(user.(*db.User).UserGroupID); ok {
			avail_inbound := false
			//验证访问的入站代理是否在当前用户组可用范围内
			for _, inbound := range userGroup.(*db.UserGroup).AvailInbounds {
				if inbound.ID == inboundName {
					avail_inbound = true
				}
			}
			if !avail_inbound {
				return "block"
			}
			//根据代理用户组查询对应路由发难
			if routeScheme, ok := RouteSchemeMap.Load(userGroup.(*db.UserGroup).RouteSchemeID); ok {
				if !routeScheme.(*db.RouteScheme).Enabled {
					return "block"
				}
				//关联路由规则已经由GORM框架的Preload机制自动装载
				rules := routeScheme.(*db.RouteScheme).Rules
				sort.Slice(rules, func(i, j int) bool {
					return rules[i].Priority < rules[j].Priority
				})
				//迭代匹配路由规则
				for _, r := range rules {
					var match bool
					patterns := strings.Split(r.Pattern, ",")
					for _, pattern := range patterns {
						switch r.Type {
						case "geosite":
							match = matchGeo(pattern, geoCodes)
						case "domain":
							match = matchDomain(pattern, target.Hostname)
						case "ip":
							match = matchIP(pattern, target.IP)
						case "any":
							match = true
						default:
							match = false
						}
						if match {
							break
						}
					}
					if match {
						//出站代理ID列表已经由GORM框架的Preload机制装载，在多个入站代理之间进行随机负载均衡
						id := r.Outbounds[rand.Intn(len(r.Outbounds))].ID
						return id
					}
				}
			}
		}
	}
	return "direct"
}
