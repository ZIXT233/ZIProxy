package web

import (
	"github.com/ZIXT233/ziproxy/db"
	"github.com/ZIXT233/ziproxy/manager"
	"github.com/ZIXT233/ziproxy/utils"
	"github.com/gin-gonic/gin"
)

func getAllUser(c *gin.Context) {
	users, _, err := manager.DBM.User.List(0, db.MAX)
	if err != nil {
		c.JSON(500, errorR(500, "Failed to fetch users"))
		return
	}
	var viewUsers []gin.H
	for _, user := range users {
		viewUsers = append(viewUsers, gin.H{
			"id":          user.ID,
			"email":       user.Email,
			"enabled":     user.Enabled,
			"userGroupId": user.UserGroupID,
		})
	}
	c.JSON(200, successR(viewUsers))
}
func getAllUserGroup(c *gin.Context) {
	usergroups, _, err := manager.DBM.UserGroup.List(0, db.MAX)
	if err != nil {
		c.JSON(500, errorR(500, "Failed to fetch usergroups"))
		return
	}
	var viewUserGroups []gin.H
	for _, group := range usergroups {
		//select id[] from Inbound[]
		inboundProxyIds := make([]string, 0)
		for _, proxy := range group.AvailInbounds {
			inboundProxyIds = append(inboundProxyIds, proxy.ID)
		}
		viewUserGroups = append(viewUserGroups, gin.H{
			"id":              group.ID,
			"inboundProxyIds": inboundProxyIds,
			"routeSchemeId":   group.RouteSchemeID,
			"userCount":       len(group.Users),
		})
	}
	c.JSON(200, successR(viewUserGroups))
}

func getUser(c *gin.Context) {
	id := c.Param("id")
	user, err := manager.DBM.User.GetByID(id)
	if err != nil {
		c.JSON(500, errorR(500, "Failed to fetch user"))
		return
	}
	if user == nil {
		c.JSON(404, errorR(404, "User not found"))
		return
	}
	viewUser := gin.H{
		"id":          user.ID,
		"email":       user.Email,
		"enabled":     user.Enabled,
		"userGroupId": user.UserGroupID,
	}
	c.JSON(200, successR(viewUser))
}
func createUser(c *gin.Context) {
	var user db.User
	if err := c.ShouldBindJSON(&user); err != nil {
		c.JSON(400, errorR(400, "Invalid input"))
		return
	}
	if err := manager.DBM.User.Create(&user); err != nil {
		c.JSON(500, errorR(500, "Failed to create user"))
		return
	}

	manager.SyncUser()
	c.JSON(201, successR(gin.H{"id": user.ID}))
}

func updateUser(c *gin.Context) {
	id := c.Param("id")
	var user db.User
	if err := c.ShouldBindJSON(&user); err != nil {
		c.JSON(400, errorR(400, "Invalid input"))
		return
	}
	dbData, err := manager.DBM.User.GetByID(id)
	if err != nil {
		c.JSON(500, errorR(500, "Failed to fetch proxy data"))
		return
	}
	utils.MergeStruct(dbData, &user)
	if err := manager.DBM.User.Update(dbData); err != nil {
		c.JSON(500, errorR(500, "Failed to update user"))
		return
	}
	manager.SyncUser()
	c.JSON(200, successR(gin.H{"id": user.ID}))
}

func deleteUser(c *gin.Context) {
	id := c.Param("id")
	user, err := manager.DBM.User.GetByID(id)
	if err != nil {
		c.JSON(500, errorR(500, "Failed to fetch user"))
		return
	}
	if user.UserGroupID == "管理员" {
		userGroup, err := manager.DBM.UserGroup.GetByID(user.UserGroupID)
		if err != nil {
			c.JSON(500, errorR(500, "获取用户组失败"))
			return
		}
		if len(userGroup.Users) <= 1 {
			c.JSON(400, errorR(500, "管理员用户组至少保留一个用户"))
			return
		}
	}
	if err := manager.DBM.User.Delete(id); err != nil {
		c.JSON(500, errorR(500, "Failed to delete user"))
		return
	}
	manager.RemoveUser(id)
	c.JSON(200, successR(gin.H{"message": "User deleted successfully"}))
}

func userResetPassword(c *gin.Context) {
	id := c.Param("id")
	user, err := manager.DBM.User.GetByID(id)
	if err != nil {
		c.JSON(500, errorR(500, "Failed to fetch user"))
		return
	}
	if user == nil {
		c.JSON(404, errorR(404, "User not found"))
		return
	}
	user.Password = utils.SHA256([]byte(user.ID))
	if err := manager.DBM.User.Update(user); err != nil {
		c.JSON(500, errorR(500, "Failed to update user"))
		return
	}
	manager.SyncUser()
	c.JSON(200, successR(gin.H{"message": "Password reset successfully"}))
}

func getUserGroup(c *gin.Context) {
	id := c.Param("id")
	group, err := manager.DBM.UserGroup.GetByID(id)
	if err != nil {
		c.JSON(500, errorR(500, "获取用户组失败"))
		return
	}
	if group == nil {
		c.JSON(404, errorR(404, "用户组不存在"))
		return
	}

	// 获取入站代理ID列表
	inboundProxyIds := make([]string, 0)
	for _, proxy := range group.AvailInbounds {
		inboundProxyIds = append(inboundProxyIds, proxy.ID)
	}

	// 获取用户ID列表
	userIds := make([]string, 0)
	for _, user := range group.Users {
		userIds = append(userIds, user.ID)
	}

	viewUserGroup := gin.H{
		"id":              group.ID,
		"inboundProxyIds": inboundProxyIds,
		"routeSchemeId":   group.RouteSchemeID,
		"userCount":       len(group.Users),
		"userIds":         userIds,
	}

	c.JSON(200, successR(viewUserGroup))
}

func createUserGroup(c *gin.Context) {
	var req struct {
		ID            string   `json:"id"`
		RouteSchemeID string   `json:"routeSchemeId"`
		InboundIds    []string `json:"inboundProxyIds"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, errorR(400, "无效的请求数据"))
		return
	}

	// 检查路由方案是否存在
	routeScheme, err := manager.DBM.RouteScheme.GetByID(req.RouteSchemeID)
	if err != nil {
		c.JSON(400, errorR(400, "指定的路由方案不存在"))
		return
	}

	// 创建用户组
	userGroup := &db.UserGroup{
		ID:            req.ID,
		RouteSchemeID: routeScheme.ID,
	}

	// 添加入站代理关联
	for _, inboundId := range req.InboundIds {
		inbound, err := manager.DBM.ProxyData.GetByID(inboundId)
		if err != nil {
			continue
		}
		userGroup.AvailInbounds = append(userGroup.AvailInbounds, *inbound)
	}

	if err := manager.DBM.UserGroup.Create(userGroup); err != nil {
		c.JSON(500, errorR(500, "创建用户组失败"))
		return
	}
	manager.SyncUserGroup()
	c.JSON(201, successR(gin.H{"id": userGroup.ID}))
}

func updateUserGroup(c *gin.Context) {
	id := c.Param("id")
	var req struct {
		RouteSchemeID *string   `json:"routeSchemeId"`
		InboundIds    *[]string `json:"inboundProxyIds"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, errorR(400, "无效的请求数据"))
		return
	}

	userGroup, err := manager.DBM.UserGroup.GetByID(id)
	if err != nil {
		c.JSON(500, errorR(500, "获取用户组失败"))
		return
	}

	if req.RouteSchemeID != nil {
		// 检查路由方案是否存在
		routeScheme, err := manager.DBM.RouteScheme.GetByID(*req.RouteSchemeID)
		if err != nil {
			c.JSON(400, errorR(400, "指定的路由方案不存在"))
			return
		}
		//改RouteScheme对象的ID才能更新关联，改RouteSchemeID会与对象ID冲突而不修改
		userGroup.RouteScheme.ID = routeScheme.ID
	}

	if req.InboundIds != nil {
		// 清除现有关联
		manager.DBM.UserGroup.ClearInbounds(userGroup.ID)

		// 添加新关联
		for _, inboundId := range *req.InboundIds {
			inbound, err := manager.DBM.ProxyData.GetByID(inboundId)
			if err != nil {
				continue
			}
			manager.DBM.DB.Model(userGroup).Association("AvailInbounds").Append(inbound)
		}
	}

	if err := manager.DBM.UserGroup.Update(userGroup); err != nil {
		c.JSON(500, errorR(500, "更新用户组失败"))
		return
	}
	manager.SyncUserGroup()
	c.JSON(200, successR(gin.H{"id": userGroup.ID}))
}

func deleteUserGroup(c *gin.Context) {
	id := c.Param("id")
	if id == "管理员" {
		c.JSON(400, errorR(400, "不能删除管理员用户组"))
		return
	}
	if err := manager.DBM.UserGroup.Delete(id); err != nil {
		c.JSON(500, errorR(500, "删除用户组失败"))
		return
	}
	manager.RemoveUserGroup(id)
	c.JSON(200, successR(gin.H{"message": "用户组删除成功"}))
}
