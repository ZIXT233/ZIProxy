package web

import (
	"github.com/ZIXT233/ziproxy/manager"
	"github.com/gin-gonic/gin"
	"log"
)

func getSystemInfo(c *gin.Context) {
	sysInfo, err := manager.DBM.SystemInfo.GetbyID(1)
	if err != nil {
		c.JSON(500, errorR(500, "Failed to get system info"))
		return
	}
	c.JSON(200, successR(gin.H{
		"systemName":        sysInfo.SystemName,
		"description":       sysInfo.SystemDescription,
		"version":           manager.Version,
		"startUpTime":       manager.StartUpTime.String(),
		"trafficRecordDays": sysInfo.TrafficRecordDays,
	}))
}
func getSystemName(c *gin.Context) {
	sysInfo, err := manager.DBM.SystemInfo.GetbyID(1)
	if err != nil {
		c.JSON(500, errorR(500, "Failed to get system info"))
		return
	}
	c.JSON(200, successR(gin.H{
		"systemName": sysInfo.SystemName,
	}))
}
func clearHTTPCache(c *gin.Context) {
	err := manager.ClearHTTPCache()
	if err != nil {
		log.Println(err)
		c.JSON(500, errorR(500, "Failed to clear badger cache"))
	} else {
		log.Println("Clear HTTP cache successfully")
		c.JSON(200, successR(nil))
	}
}

func updateSystemInfo(c *gin.Context) {
	sysInfo, err := manager.DBM.SystemInfo.GetbyID(1)
	if err != nil {
		c.JSON(500, errorR(500, "Failed to get system info"))
		return
	}
	c.JSON(200, successR(gin.H{}))
	var req struct {
		SystemName        string `json:"systemName"`
		Description       string `json:"description"`
		TrafficRecordDays uint   `json:"trafficRecordDays"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, errorR(400, "Invalid request"))
		return
	}
	sysInfo.SystemName = req.SystemName
	sysInfo.SystemDescription = req.Description
	sysInfo.TrafficRecordDays = req.TrafficRecordDays
	manager.DBM.SystemInfo.Update(sysInfo)
	c.JSON(200, successR(gin.H{
		"systemName":        sysInfo.SystemName,
		"description":       sysInfo.SystemDescription,
		"trafficRecordDays": sysInfo.TrafficRecordDays,
	}))
}
