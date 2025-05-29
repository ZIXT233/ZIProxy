package manager

import (
	"errors"
	"log"
	"net"
	"sync"
	"time"

	"github.com/ZIXT233/ziproxy/db"
	"github.com/ZIXT233/ziproxy/proxy"
	"github.com/ZIXT233/ziproxy/utils"
)

var (
	InboundMap       sync.Map
	OutboundMap      sync.Map
	RouteSchemeMap   sync.Map
	UserGroupMap     sync.Map
	UserMap          sync.Map
	UserTokenMap     sync.Map
	DBM              *db.RepoManager
	StatisticDBM     *db.StatisticRepoManager
	ActiveUserLinkMu sync.Mutex
	ActiveUserLink   = make(map[string]uint)
	Version          string
	StartUpTime      time.Time
	HttpCacheEnable  bool
)

func SyncInbound(d *db.ProxyData) {
	stopInboundProc(d.ID) //will close old conn
	config, _ := utils.UnmarshalConfig(d.Config)
	inbound, err := proxy.InboundFromConfig(d.ID, config)
	if err != nil {
		log.Printf("Failed to load inbound %s err: %v", d.ID, err)
		return
	}
	InboundMap.Store(d.ID, inbound)
	if d.Enabled {
		attachInboundProc(d.ID)
	}
}
func RemoveInbound(id string) {
	stopInboundProc(id)
	InboundMap.Delete(id)
}
func SyncOutbound(d *db.ProxyData) {
	if old, exists := OutboundMap.Load(d.ID); exists {
		old.(proxy.Outbound).CloseAllConn()
	}
	if d.ID == "block" {
		return
	}
	config, _ := utils.UnmarshalConfig(d.Config)
	outbound, err := proxy.OutboundFromConfig(d.ID, config)
	if err != nil {
		log.Printf("Failed to load outbound %s err: %v", d.ID, err)
		return
	}
	OutboundMap.Store(d.ID, outbound)
}
func RemoveOutbound(id string) {
	if old, exists := OutboundMap.Load(id); exists {
		old.(proxy.Outbound).CloseAllConn()
	}
	OutboundMap.Delete(id)
}
func SyncRouteScheme(d *db.RouteScheme) {
	RouteSchemeMap.Store(d.ID, d)
}
func RemoveRouteScheme(id string) {
	RouteSchemeMap.Delete(id)
}
func SyncUser(d *db.User) {
	UserMap.Store(d.ID, d)
	if d.ID != "forward" && d.ID != "guest" {
		UserTokenMap.Store(d.LinkToken, d)
	}

}
func RemoveUser(id string) {
	UserMap.Delete(id)
}
func SyncUserGroup(d *db.UserGroup) {
	UserGroupMap.Store(d.ID, d)
}
func RemoveUserGroup(id string) {
	UserGroupMap.Delete(id)
}

func MeasureLatency(proxyID string) (int64, error) {
	if val, exists := OutboundMap.Load(proxyID); exists {
		outbound := val.(proxy.Outbound)
		start := time.Now()
		timeout := 5 * time.Second
		conn, err := net.DialTimeout("tcp", outbound.Addr(), timeout)
		if err != nil {
			return -1, errors.New("failed to connect to outbound")
		}
		defer conn.Close()
		duration := time.Since(start)
		milliseconds := duration.Milliseconds()
		target, _ := proxy.NewTargetAddr("baidu.com:80")
		_, _, err = outbound.WrapConn(conn, target)
		if err != nil {
			return -2, errors.New("failed to handshake with outbound")
		}
		return milliseconds, nil
	}
	return 0, errors.New("proxy not found")
}
func TrafficCleanCron() {
	go func() {
		for {
			sysInfo, err := DBM.SystemInfo.GetbyID(1)
			if err != nil {
				return
			}
			beforeTime := time.Now().Add(-time.Duration(sysInfo.TrafficRecordDays) * 24 * time.Hour)
			StatisticDBM.Traffic.Clean(beforeTime)
			log.Printf("Traffic records before %s have been cleaned", beforeTime.Format("2006-01-02"))
			time.Sleep(time.Hour * 24)
		}
	}()
}
func UpdateUserToken(id string) (string, error) {
	user, err := DBM.User.GetByID(id)
	if err != nil {
		log.Printf("Failed to fetch user %s err: %v", id, err)
		return "", err
	}
	if _, exists := UserTokenMap.Load(id); exists {
		UserTokenMap.Delete(id)
	}
	var token string
	for {
		token, _ = utils.GenerateBase64RandomString(16)
		if _, exists := UserTokenMap.Load(token); !exists {
			break
		}
	}
	user.LinkToken = token
	if err := DBM.User.Update(user); err != nil {
		log.Printf("Failed to update user %s err: %v", id, err)
		return "", err
	}
	SyncUser(user)
	return token, nil
}

func Start(config *utils.RootConfig, version string) {
	var err error
	var isNewDB bool
	DBM, isNewDB, err = db.InitRepo(config.DB)
	if err != nil {
		panic(err)
	}
	if isNewDB {
		loadDefaultData(DBM)
	}

	StatisticDBM, _, err = db.InitStatisticRepo(config.StatisticDB)
	if err != nil {
		panic(err)
	}
	initRouter(config.StaticPath)
	HttpCacheEnable = true
	err = InitTlsMITM(config.MITMCACert, config.MITMCAKey)
	if err != nil {
		log.Printf("Failed to init tls mitm cert err: %v", err)
		HttpCacheEnable = false
	}
	if config.BadgerSize <= 0 {
		HttpCacheEnable = false
	} else {
		err = initHttpCacheDB(config.BadgerDir, config.BadgerSize)
		if err != nil {
			log.Printf("Failed to init badger err: %v", err)
			HttpCacheEnable = false
		}
	}

	if HttpCacheEnable {
		log.Printf("Http proxy cache enabled")
	} else {
		log.Printf("Http proxy cache disabled because mitm or badger is not configured")
	}
	Version = version

	users, _, _ := DBM.User.List(0, db.MAX)
	userGroups, _, _ := DBM.UserGroup.List(0, db.MAX)
	routeSchemes, _, _ := DBM.RouteScheme.List(0, db.MAX)
	inboundData, _, _ := DBM.ProxyData.List(db.InDir, 0, db.MAX)
	outboundData, _, _ := DBM.ProxyData.List(db.OutDir, 0, db.MAX)

	for _, d := range users {
		SyncUser(&d)
	}
	for _, d := range userGroups {
		SyncUserGroup(&d)
	}
	for _, d := range routeSchemes {
		SyncRouteScheme(&d)
	}
	for _, d := range inboundData {
		SyncInbound(&d)
	}
	for _, d := range outboundData {
		SyncOutbound(&d)
	}
	TrafficCleanCron()
	StartUpTime = time.Now().Truncate(time.Second)
	LaunchRealTimeStatistic()
}
