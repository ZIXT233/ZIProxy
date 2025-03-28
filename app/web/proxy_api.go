package web

import (
	"strconv"
	"strings"

	"github.com/ZIXT233/ziproxy/db"
	"github.com/ZIXT233/ziproxy/manager"
	"github.com/ZIXT233/ziproxy/proxy"
	"github.com/ZIXT233/ziproxy/utils"
	"github.com/gin-gonic/gin"
)

func getAllInbound(c *gin.Context) {
	var inbounds []gin.H
	if proxyDatas, _, err := manager.DBM.ProxyData.List(db.InDir, 0, db.MAX); err != nil {
		c.JSON(500, errorR(500, "Failed to fetch proxy data"))
		return
	} else {
		userId := c.GetString("userId")
		user, err := manager.DBM.User.GetByID(userId)
		if err != nil {
			c.JSON(500, errorR(500, "Failed to fetch auth data"))
			return
		}

		for _, proxyData := range proxyDatas {
			config, _ := utils.UnmarshalConfig(proxyData.Config)
			linkConfig := ""
			if inboundInstance, ok := manager.InboundMap.Load(proxyData.ID); ok {
				linkConfig = utils.MarshalConfig(inboundInstance.(proxy.Inbound).GetLinkConfig(c.Request.Host, user.ID, user.Password))
			}
			inbounds = append(inbounds, gin.H{
				"id":         proxyData.ID,
				"scheme":     config["scheme"],
				"direction":  proxyData.Direction,
				"enabled":    proxyData.Enabled,
				"running":    manager.IsInboundProcRunning(proxyData.ID),
				"config":     proxyData.Config,
				"linkConfig": linkConfig,
			})
		}
	}
	c.JSON(200, successR(inbounds))
}
func getAllOutbound(c *gin.Context) {
	var outbounds []gin.H

	if proxyDatas, _, err := manager.DBM.ProxyData.List(db.OutDir, 0, db.MAX); err != nil {
		c.JSON(500, errorR(500, "Failed to fetch proxy data"))
		return
	} else {
		for _, proxyData := range proxyDatas {
			config, _ := utils.UnmarshalConfig(proxyData.Config)
			outbounds = append(outbounds, gin.H{
				"id":        proxyData.ID,
				"scheme":    config["scheme"],
				"direction": proxyData.Direction,
				"config":    proxyData.Config,
			})
		}
	}
	c.JSON(200, successR(outbounds))
}

func getProxyData(c *gin.Context) {
	id := c.Param("id")
	proxyData, err := manager.DBM.ProxyData.GetByID(id)
	if err != nil {
		c.JSON(500, errorR(500, "Failed to fetch proxy data"))
		return
	}
	if proxyData == nil {
		c.JSON(404, errorR(404, "Proxy data not found"))
		return
	}
	userId := c.GetString("userId")
	user, err := manager.DBM.User.GetByID(userId)
	if err != nil {
		c.JSON(500, errorR(500, "Failed to fetch auth data"))
		return
	}
	config, _ := utils.UnmarshalConfig(proxyData.Config)
	scheme := config["scheme"].(string)
	linkConfig := ""
	if inboundInstance, ok := manager.InboundMap.Load(proxyData.ID); ok {
		linkConfig = utils.MarshalConfig(inboundInstance.(proxy.Inbound).GetLinkConfig(c.Request.Host, user.ID, user.Password))
	}
	c.JSON(200, successR(gin.H{
		"id":         proxyData.ID,
		"scheme":     scheme,
		"direction":  proxyData.Direction,
		"enabled":    proxyData.Enabled,
		"config":     proxyData.Config,
		"running":    manager.IsInboundProcRunning(proxyData.ID),
		"linkConfig": linkConfig,
	}))
}
func createProxyData(c *gin.Context) {
	var proxyData db.ProxyData
	if err := c.ShouldBindJSON(&proxyData); err != nil {
		c.JSON(400, errorR(400, "Invalid request data"))
		return
	}
	if strings.Contains(c.Request.URL.Path, "inbound") {
		proxyData.Direction = db.InDir
	} else {
		proxyData.Direction = db.OutDir
	}
	if err := manager.DBM.ProxyData.Create(&proxyData); err != nil {
		c.JSON(500, errorR(500, "Failed to create proxy data"))
		return
	}
	if proxyData.Direction == db.InDir {
		manager.SyncInbound(&proxyData)
	} else {
		manager.SyncOutbound(&proxyData)
	}
	c.JSON(200, successR(gin.H{
		"id": proxyData.ID,
	}))

}

func updateProxyData(c *gin.Context) {
	id := c.Param("id")
	var proxyData db.ProxyData
	if err := c.ShouldBindJSON(&proxyData); err != nil {
		c.JSON(400, errorR(400, "Invalid request data"))
		return
	}
	dbData, err := manager.DBM.ProxyData.GetByID(id)
	if err != nil {
		c.JSON(500, errorR(500, "Failed to fetch proxy data"))
		return
	}
	utils.MergeStruct(dbData, &proxyData)
	if err := manager.DBM.ProxyData.Update(dbData); err != nil {
		c.JSON(500, errorR(500, "Failed to update proxy data"))
		return
	}
	if dbData.Direction == db.InDir {
		manager.SyncInbound(dbData)
	} else {
		manager.SyncOutbound(dbData)
	}
	c.JSON(200, successR(gin.H{
		"id": proxyData.ID,
	}))
}

func deleteProxyData(c *gin.Context) {
	id := c.Param("id")
	if id == "block" {
		c.JSON(400, errorR(400, "Cannot delete default proxy data"))
		return
	}
	dbData, err := manager.DBM.ProxyData.GetByID(id)
	if err != nil {
		c.JSON(500, errorR(500, "Failed to fetch proxy data"))
		return
	}
	if err := manager.DBM.ProxyData.Delete(id); err != nil {
		c.JSON(500, errorR(500, "Failed to delete proxy data"))
		return

	}
	if dbData.Direction == db.InDir {
		manager.RemoveInbound(id)
	} else {
		manager.RemoveOutbound(id)
	}
	c.JSON(200, successR(gin.H{
		"message": "Proxy data deleted successfully",
	}))
}

func testOutboundSpeed(c *gin.Context) {
	id := c.Param("id")
	latency, _ := manager.MeasureLatency(id)
	latencyStr := strconv.FormatInt(latency, 10) + "ms"
	if latency == -1 {
		latencyStr = "无法到达代理"
	} else if latency == -2 {
		latencyStr = "代理协议握手失败"
	}
	c.JSON(200, successR(gin.H{
		"proxyId": id,
		"latency": latencyStr,
	}))
}

func getUsableInbounds(c *gin.Context) {
	userGroupID, ok := c.Get("userGroupId")
	if !ok {
		c.JSON(401, errorR(401, "Unauthorized"))
		return
	}
	userGroup, err := manager.DBM.UserGroup.GetByID(userGroupID.(string))
	if err != nil {
		c.JSON(500, errorR(500, "Failed to fetch user group"))
		return
	}
	var inbounds []gin.H
	userId := c.GetString("userId")
	user, err := manager.DBM.User.GetByID(userId)
	if err != nil {
		c.JSON(500, errorR(500, "Failed to fetch auth data"))
		return
	}
	for _, inbound := range userGroup.AvailInbounds {
		if inbound.Enabled {
			config, _ := utils.UnmarshalConfig(inbound.Config)
			inboundInstance, ok := manager.InboundMap.Load(inbound.ID)
			linkConfig := ""
			if ok {
				config := inboundInstance.(proxy.Inbound).GetLinkConfig(c.Request.Host, user.ID, user.Password)
				linkConfig = utils.MarshalConfig(config)
			}
			inbounds = append(inbounds, gin.H{
				"id":         inbound.ID,
				"scheme":     config["scheme"],
				"linkConfig": linkConfig,
			})
		}
	}
	c.JSON(200, successR(inbounds))

}
