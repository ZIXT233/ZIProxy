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
	DBM              *db.RepoManager
	StatisticDBM     *db.StatisticRepoManager
	ActiveUserLinkMu sync.Mutex
	ActiveUserLink   = make(map[string]uint)
	Version          string
	StartUpTime      time.Time
)

func SyncBounds() error {
	inboundData, _, _ := DBM.ProxyData.List(db.InDir, 0, db.MAX)
	outboundData, _, _ := DBM.ProxyData.List(db.OutDir, 0, db.MAX)
	for _, d := range inboundData {
		SyncInbound(&d)
	}
	for _, d := range outboundData {
		SyncOutbound(&d)
	}
	return nil
}

func SyncInbound(d *db.ProxyData) {
	stopInboundProc(d.ID) //will close old conn
	inbound, err := proxy.InboundFromConfig(d)
	if err != nil {
		log.Printf("Failed to load inbound %s err: %v", d.ID, err)
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
	outbound, err := proxy.OutboundFromConfig(d)
	if err != nil {
		log.Printf("Failed to load outbound %s err: %v", d.ID, err)
	}
	OutboundMap.Store(d.ID, outbound)
}
func RemoveOutbound(id string) {
	if old, exists := OutboundMap.Load(id); exists {
		old.(proxy.Outbound).CloseAllConn()
	}
	OutboundMap.Delete(id)
}
func SyncRouteScheme() {
	routeSchemes, _, _ := DBM.RouteScheme.List(0, db.MAX)
	for _, scheme := range routeSchemes {
		RouteSchemeMap.Store(scheme.ID, &scheme)
	}
}
func RemoveRouteScheme(id string) {
	RouteSchemeMap.Delete(id)
}
func SyncUser() {
	users, _, _ := DBM.User.List(0, db.MAX)

	for _, user := range users {
		UserMap.Store(user.ID, &user)
	}
}
func RemoveUser(id string) {
	UserMap.Delete(id)
}
func SyncUserGroup() {
	userGroups, _, _ := DBM.UserGroup.List(0, db.MAX)
	for _, group := range userGroups {
		UserGroupMap.Store(group.ID, &group)
	}
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
	Version = version

	SyncUser()
	SyncUserGroup()
	SyncRouteScheme()
	SyncBounds()
	TrafficCleanCron()
	StartUpTime = time.Now().Truncate(time.Second)
}
