package manager

import (
	"github.com/ZIXT233/ziproxy/db"
	"github.com/ZIXT233/ziproxy/utils"
)

func loadDefaultData(dbm *db.RepoManager) {
	sysInfo := &db.SystemInfo{
		ID:                1,
		SystemName:        "ZIProxy",
		SystemDescription: "ZIProxy是一款多用户集成代理系统。\n无验证http代理可直接使用地址连接。\n前置代理出站配置请复制完整配置。",
		TrafficRecordDays: 30,
	}
	dbm.SystemInfo.Create(sysInfo)
	// 创建默认路由方案
	routeScheme := &db.RouteScheme{
		ID:          "默认分流",
		Description: "默认路由方案",
		Enabled:     true,
	}
	dbm.RouteScheme.Create(routeScheme)

	// 创建默认入站代理
	inboundProxy1 := &db.ProxyData{
		ID:        "default",
		Direction: db.InDir,
		Enabled:   true,
		Config: `{ 
      "scheme": "http",
      "address": "localhost:8080"
    }`,
	}
	inboundProxy2 := &db.ProxyData{
		ID:        "https",
		Direction: db.InDir,
		Enabled:   true,
		Config: `{
      "scheme": "tls",
      "cert": "static/cert/server.crt",
      "key": "static/cert/server.key",
      "upper": { 
        "scheme": "https",
        "address": "0.0.0.0:8083",
        "guestForward": "localhost:2339"
      }
    }`,
	}
	dbm.ProxyData.Create(inboundProxy1)
	dbm.ProxyData.Create(inboundProxy2)

	// 创建默认出站代理
	outboundProxy1 := &db.ProxyData{
		ID:        "direct",
		Direction: db.OutDir,
		Enabled:   true, //已弃用
		Config: `{ 
      "scheme": "direct"
    }`,
	}
	outboundProxy2 := &db.ProxyData{
		ID:        "美国节点",
		Direction: db.OutDir,
		Enabled:   true, //已弃用
		Config: `{ 
      "scheme": "http",
      "address": "localhost:1087"
    }`,
	}
	outboundProxy3 := &db.ProxyData{
		ID:        "block",
		Direction: db.OutDir,
		Enabled:   true,
		Config: `{ 
      "scheme": "block" 
    }`,
	}
	dbm.ProxyData.Create(outboundProxy1)
	dbm.ProxyData.Create(outboundProxy2)
	dbm.ProxyData.Create(outboundProxy3)
	// 创建默认规则
	rule1 := &db.Rule{
		Name:          "任何",
		Type:          "any",
		Pattern:       "*",
		RouteSchemeID: routeScheme.ID,
		Priority:      1,
	}
	dbm.Rule.Create(rule1)
	// 使用Association添加出站代理关联
	dbm.DB.Model(rule1).Association("Outbounds").Append(outboundProxy1)

	rule2 := &db.Rule{
		Name:          "国外网站",
		Type:          "geosite",
		Pattern:       "geolocation-!cn",
		RouteSchemeID: routeScheme.ID,
		Priority:      0,
	}
	dbm.Rule.Create(rule2)
	// 使用Association添加出站代理关联
	dbm.DB.Model(rule2).Association("Outbounds").Append(outboundProxy2)

	// 创建管理员用户组
	adminGroup := &db.UserGroup{
		ID:            "管理员",
		RouteSchemeID: routeScheme.ID,
	}
	dbm.UserGroup.Create(adminGroup)
	// 使用Association添加入站代理关联
	dbm.DB.Model(adminGroup).Association("AvailInbounds").Append(inboundProxy1, inboundProxy2)

	// 创建普通用户组
	userGroup := &db.UserGroup{
		ID:            "普通用户",
		RouteSchemeID: routeScheme.ID,
	}
	dbm.UserGroup.Create(userGroup)
	// 使用Association添加入站代理关联
	dbm.DB.Model(userGroup).Association("AvailInbounds").Append(inboundProxy1)

	// 创建普通用户组
	guestGroup := &db.UserGroup{
		ID:            "游客",
		RouteSchemeID: routeScheme.ID,
	}
	dbm.UserGroup.Create(guestGroup)
	// 使用Association添加入站代理关联
	dbm.DB.Model(guestGroup).Association("AvailInbounds").Append(inboundProxy1)

	// 创建管理员用户
	admin := &db.User{
		ID:          "admin",
		Email:       "admin@example.com",
		Password:    utils.SHA256([]byte("admin")),
		Enabled:     true,
		UserGroupID: adminGroup.ID,
	}
	dbm.User.Create(admin)

	// 创建普通用户
	user := &db.User{
		ID:          "user",
		Email:       "user@example.com",
		Password:    utils.SHA256([]byte("user")),
		Enabled:     true,
		UserGroupID: userGroup.ID,
	}
	dbm.User.Create(user)
	guest := &db.User{
		ID:          "guest",
		Email:       "guest@guest.com",
		Password:    utils.SHA256([]byte("guest")),
		Enabled:     true,
		UserGroupID: guestGroup.ID,
	}
	dbm.User.Create(guest)
}
