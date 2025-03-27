package web

import (
	"fmt"
	"sort"
	"time"

	"github.com/ZIXT233/ziproxy/manager"
	"github.com/gin-gonic/gin"
)

func getTrafficHistory(c *gin.Context) {
	timeRange := c.Param("timeRange")
	startTime := time.Now()
	endTime := time.Now()
	timeStep := time.Hour
	timeTable := []string{}
	downBytes := []uint64{}
	upBytes := []uint64{}
	switch timeRange {
	case "hour":
		startTime = startTime.Add(-time.Hour * 23)
		timeStep = time.Hour
	case "day":
		startTime = startTime.Add(-time.Hour * 24 * 6)
		timeStep = time.Hour * 24
	case "week":
		startTime = startTime.Add(-time.Hour * 24 * 7 * 3)
		timeStep = time.Hour * 24 * 7
	}
	weekCount := 1
	for t := startTime; !t.After(endTime); t = t.Add(timeStep) {
		startOfTime := t.Truncate(timeStep)
		endOfTime := startOfTime.Add(timeStep)
		bytesIn, bytesOut, err := manager.StatisticDBM.Traffic.GetTrafficStats(startOfTime, endOfTime)
		if err != nil {
			c.JSON(500, errorR(500, "获取流量统计失败"))
			return
		}
		downBytes = append(downBytes, bytesIn)
		upBytes = append(upBytes, bytesOut)

		switch timeRange {
		case "hour":
			timeTable = append(timeTable, t.Format("15:00"))
		case "day":
			timeTable = append(timeTable, t.Format("01-02"))
		case "week":
			timeTable = append(timeTable, fmt.Sprintf("第%d周", weekCount))
			weekCount++
		}
	}
	c.JSON(200, successR(gin.H{
		"labels":   timeTable,
		"download": downBytes,
		"upload":   upBytes,
	}))
}

func getTrafficStatus(c *gin.Context) {
	today := time.Now().Truncate(time.Hour * 24)
	totalBytesIn, totalBytesOut, err := manager.StatisticDBM.Traffic.GetTrafficStats(today, time.Now())
	if err != nil {
		c.JSON(500, errorR(500, "获取流量统计失败"))
		return
	}
	c.JSON(200, successR(gin.H{
		"download": totalBytesIn,
		"upload":   totalBytesOut,
		"total":    totalBytesIn + totalBytesOut,
	}))
}

func getProxyTrafficRank(c *gin.Context) {
	direction := c.Param("direction")
	startTime := time.Now().Add(-time.Hour * 24 * 7)
	startTime = startTime.Truncate(time.Hour * 24)
	endTime := time.Now()
	stats, err := manager.StatisticDBM.Traffic.GetProxyTrafficRank(direction, startTime, endTime)
	if err != nil {
		c.JSON(500, errorR(500, "获取流量统计失败"))
		return
	}
	c.JSON(200, successR(stats))
}

func getUserTrafficRank(c *gin.Context) {
	startTime := time.Now().Add(-time.Hour * 24 * 7)
	startTime = startTime.Truncate(time.Hour * 24)
	endTime := time.Now()
	stats, err := manager.StatisticDBM.Traffic.GetUserTrafficRank(startTime, endTime)
	if err != nil {
		c.JSON(500, errorR(500, "获取流量统计失败"))
		return
	}
	c.JSON(200, successR(stats))
}

func getActiveUserLink(c *gin.Context) {
	manager.ActiveUserLinkMu.Lock()
	stats := make([]gin.H, len(manager.ActiveUserLink))
	count := 0
	for id, links := range manager.ActiveUserLink {
		stats[count] = gin.H{
			"userId": id,
			"links":  links,
		}
		count++
	}
	sort.Slice(stats, func(i, j int) bool {
		return stats[i]["links"].(uint) > stats[j]["links"].(uint)
	})
	manager.ActiveUserLinkMu.Unlock()
	c.JSON(200, successR(stats))
}

func getMyTraffic(c *gin.Context) {
	userID, ok := c.Get("userId")
	if !ok {
		c.JSON(401, errorR(401, "Unauthorized"))
		return
	}
	startTime := time.Now().Add(-time.Hour * 24 * 7)
	startTime = startTime.Truncate(time.Hour * 24)
	endTime := time.Now()
	downBytes, upBytes, err := manager.StatisticDBM.Traffic.GetUserTrafficStats(userID.(string), startTime, endTime)
	if err != nil {
		c.JSON(500, errorR(500, "获取流量统计失败"))
		return
	}
	manager.ActiveUserLinkMu.Lock()
	links, ok := manager.ActiveUserLink[userID.(string)]
	manager.ActiveUserLinkMu.Unlock()
	if !ok {
		links = 0
	}
	c.JSON(200, successR(gin.H{
		"download": downBytes,
		"upload":   upBytes,
		"total":    downBytes + upBytes,
		"links":    links,
	}))
}
