package web

import (
	"sort"
	"strconv"

	"github.com/ZIXT233/ziproxy/db"
	"github.com/ZIXT233/ziproxy/manager"
	"github.com/gin-gonic/gin"
)

func getAllRouteScheme(c *gin.Context) {
	schemes, _, err := manager.DBM.RouteScheme.List(0, db.MAX)
	if err != nil {
		c.JSON(500, errorR(500, "获取路由方案失败"))
		return
	}
	var viewSchemes []gin.H
	for _, scheme := range schemes {
		// 获取规则列表
		var rules []gin.H
		rules = make([]gin.H, 0)
		for _, rule := range scheme.Rules {
			// 获取出站代理ID列表
			outboundIds := make([]string, 0)
			for _, outbound := range rule.Outbounds {
				outboundIds = append(outboundIds, outbound.ID)
			}

			rules = append(rules, gin.H{
				"id":        rule.ID,
				"name":      rule.Name,
				"type":      rule.Type,
				"pattern":   rule.Pattern,
				"outbounds": outboundIds,
				"priority":  rule.Priority,
			})
		}

		// 获取用户组列表
		userGroupIds := make([]string, 0)
		for _, group := range scheme.UserGroups {
			userGroupIds = append(userGroupIds, group.ID)
		}

		viewSchemes = append(viewSchemes, gin.H{
			"id":          scheme.ID,
			"description": scheme.Description,
			"enabled":     scheme.Enabled,
			"rules":       rules,
			"userGroups":  userGroupIds,
		})
	}
	c.JSON(200, successR(viewSchemes))
}

func getRouteScheme(c *gin.Context) {
	id := c.Param("id")
	scheme, err := manager.DBM.RouteScheme.GetByID(id)
	if err != nil {
		c.JSON(500, errorR(500, "获取路由方案失败"))
		return
	}

	// 获取规则列表
	sort.Slice(scheme.Rules, func(i, j int) bool {
		return scheme.Rules[i].Priority < scheme.Rules[j].Priority
	})
	var rules []gin.H

	rules = make([]gin.H, 0)
	for _, rule := range scheme.Rules {
		// 获取出站代理ID列表
		outboundIds := make([]string, 0)
		for _, outbound := range rule.Outbounds {
			outboundIds = append(outboundIds, outbound.ID)
		}
		rules = append(rules, gin.H{
			"id":        rule.ID,
			"name":      rule.Name,
			"type":      rule.Type,
			"pattern":   rule.Pattern,
			"outbounds": outboundIds,
			"priority":  rule.Priority,
		})
	}

	// 获取用户组列表
	userGroupIds := make([]string, 0)
	for _, group := range scheme.UserGroups {
		userGroupIds = append(userGroupIds, group.ID)
	}

	viewScheme := gin.H{
		"id":          scheme.ID,
		"description": scheme.Description,
		"enabled":     scheme.Enabled,
		"rules":       rules,
		"userGroups":  userGroupIds,
	}

	c.JSON(200, successR(viewScheme))
}

func createRouteScheme(c *gin.Context) {
	var req struct {
		Id          string `json:"id"`
		Description string `json:"description"`
		Enabled     bool   `json:"enabled"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, errorR(400, "无效的请求数据"))
		return
	}

	scheme := &db.RouteScheme{
		ID:          req.Id,
		Description: req.Description,
		Enabled:     req.Enabled,
	}

	if err := manager.DBM.RouteScheme.Create(scheme); err != nil {
		c.JSON(500, errorR(500, "创建路由方案失败"))
		return
	}
	manager.SyncRouteScheme(scheme)
	c.JSON(200, successR(gin.H{
		"id": scheme.ID,
	}))
}

func updateRouteScheme(c *gin.Context) {
	id := c.Param("id")
	var req struct {
		Description *string `json:"description"`
		Enabled     *bool   `json:"enabled"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, errorR(400, "无效的请求数据"))
		return
	}

	scheme, err := manager.DBM.RouteScheme.GetByID(id)
	if err != nil {
		c.JSON(500, errorR(500, "获取路由方案失败"))
		return
	}

	if req.Description != nil {
		scheme.Description = *req.Description
	}

	if req.Enabled != nil {
		scheme.Enabled = *req.Enabled
	}

	if err := manager.DBM.RouteScheme.Update(scheme); err != nil {
		c.JSON(500, errorR(500, "更新路由方案失败"))
		return
	}
	manager.SyncRouteScheme(scheme)
	c.JSON(200, successR(gin.H{
		"id": scheme.ID,
	}))
}

func deleteRouteScheme(c *gin.Context) {
	id := c.Param("id")

	if err := manager.DBM.RouteScheme.Delete(id); err != nil {
		c.JSON(500, errorR(500, "删除路由方案失败"))
		return
	}
	manager.RemoveRouteScheme(id)
	c.JSON(200, successR(gin.H{
		"message": "路由方案删除成功",
	}))
}

func toggleRouteSchemeStatus(c *gin.Context) {
	id := c.Param("id")

	scheme, err := manager.DBM.RouteScheme.GetByID(id)
	if err != nil {
		c.JSON(500, errorR(500, "获取路由方案失败"))
		return
	}

	scheme.Enabled = !scheme.Enabled

	if err := manager.DBM.RouteScheme.Update(scheme); err != nil {
		c.JSON(500, errorR(500, "更新路由方案状态失败"))
		return
	}
	manager.SyncRouteScheme(scheme)
	c.JSON(200, successR(gin.H{
		"id":      scheme.ID,
		"enabled": scheme.Enabled,
	}))
}

func getRules(c *gin.Context) {
	schemeId := c.Param("id")

	scheme, err := manager.DBM.RouteScheme.GetByID(schemeId)
	if err != nil {
		c.JSON(500, errorR(500, "获取路由方案失败"))
		return
	}

	var rules []gin.H
	for _, rule := range scheme.Rules {
		outboundIds := make([]string, 0)
		for _, outbound := range rule.Outbounds {
			outboundIds = append(outboundIds, outbound.ID)
		}

		rules = append(rules, gin.H{
			"id":        rule.ID,
			"name":      rule.Name,
			"type":      rule.Type,
			"pattern":   rule.Pattern,
			"outbounds": outboundIds,
			"priority":  rule.Priority,
		})
	}

	c.JSON(200, successR(rules))
}

func addRule(c *gin.Context) {
	schemeId := c.Param("id")

	var req struct {
		Name      string   `json:"name"`
		Type      string   `json:"type"`
		Pattern   string   `json:"pattern"`
		Outbounds []string `json:"outbounds"`
		Priority  uint     `json:"priority"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, errorR(400, "无效的请求数据"))
		return
	}

	scheme, err := manager.DBM.RouteScheme.GetByID(schemeId)
	if err != nil {
		c.JSON(500, errorR(500, "获取路由方案失败"))
		return
	}

	// 创建规则
	rule := &db.Rule{
		Name:          req.Name,
		Type:          req.Type,
		Pattern:       req.Pattern,
		RouteSchemeID: schemeId,
		Priority:      req.Priority,
	}

	// 保存规则
	if err := manager.DBM.Rule.Create(rule); err != nil {
		c.JSON(500, errorR(500, "创建规则失败"))
		return
	}

	// 添加出站代理关联
	for _, outboundId := range req.Outbounds {
		outbound, err := manager.DBM.ProxyData.GetByID(outboundId)
		if err != nil {
			continue
		}
		manager.DBM.DB.Model(rule).Association("Outbounds").Append(outbound)
	}
	scheme, err = manager.DBM.RouteScheme.GetByID(schemeId)
	if err != nil {
		c.JSON(500, errorR(500, "获取路由方案失败"))
		return
	}
	manager.SyncRouteScheme(scheme)
	c.JSON(200, successR(gin.H{
		"id": rule.ID,
	}))
}

func updateRule(c *gin.Context) {
	schemeId := c.Param("id")
	ruleId := c.Param("ruleId")

	ruleIdInt, err := strconv.ParseUint(ruleId, 10, 32)
	if err != nil {
		c.JSON(400, errorR(400, "无效的规则ID"))
		return
	}
	scheme, err := manager.DBM.RouteScheme.GetByID(schemeId)
	if err != nil {
		c.JSON(500, errorR(500, "获取路由方案失败"))
		return
	}

	var req struct {
		Name      *string   `json:"name"`
		Type      *string   `json:"type"`
		Pattern   *string   `json:"pattern"`
		Outbounds *[]string `json:"outbounds"`
		Priority  *uint     `json:"priority"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, errorR(400, "无效的请求数据"))
		return
	}

	rule, err := manager.DBM.Rule.GetByID(uint(ruleIdInt))
	if err != nil {
		c.JSON(500, errorR(500, "获取规则失败"))
		return
	}

	if rule.RouteSchemeID != schemeId {
		c.JSON(400, errorR(400, "规则不属于指定的路由方案"))
		return
	}

	if req.Name != nil {
		rule.Name = *req.Name
	}

	if req.Type != nil {
		rule.Type = *req.Type
	}

	if req.Pattern != nil {
		rule.Pattern = *req.Pattern
	}

	if req.Priority != nil {
		rule.Priority = *req.Priority
	}

	if req.Outbounds != nil {
		// 清除现有关联
		manager.DBM.Rule.ClearOutbounds(rule.ID)

		// 添加新关联
		for _, outboundId := range *req.Outbounds {
			outbound, err := manager.DBM.ProxyData.GetByID(outboundId)
			if err != nil {
				continue
			}
			manager.DBM.DB.Model(rule).Association("Outbounds").Append(outbound)
		}
	}

	if err := manager.DBM.Rule.Update(rule); err != nil {
		c.JSON(500, errorR(500, "更新规则失败"))
		return
	}
	scheme, err = manager.DBM.RouteScheme.GetByID(schemeId)
	if err != nil {
		c.JSON(500, errorR(500, "获取路由方案失败"))
		return
	}
	manager.SyncRouteScheme(scheme)
	c.JSON(200, successR(gin.H{
		"id": rule.ID,
	}))
}

func deleteRule(c *gin.Context) {
	schemeId := c.Param("id")
	ruleId := c.Param("ruleId")

	ruleIdInt, err := strconv.ParseUint(ruleId, 10, 32)
	if err != nil {
		c.JSON(400, errorR(400, "无效的规则ID"))
		return
	}
	scheme, err := manager.DBM.RouteScheme.GetByID(schemeId)
	if err != nil {
		c.JSON(500, errorR(500, "获取路由方案失败"))
		return
	}

	rule, err := manager.DBM.Rule.GetByID(uint(ruleIdInt))
	if err != nil {
		c.JSON(500, errorR(500, "获取规则失败"))
		return
	}

	if rule.RouteSchemeID != schemeId {
		c.JSON(400, errorR(400, "规则不属于指定的路由方案"))
		return
	}

	if err := manager.DBM.Rule.Delete(rule.ID); err != nil {
		c.JSON(500, errorR(500, "删除规则失败"))
		return
	}
	scheme, err = manager.DBM.RouteScheme.GetByID(schemeId)
	if err != nil {
		c.JSON(500, errorR(500, "获取路由方案失败"))
		return
	}
	manager.SyncRouteScheme(scheme)
	c.JSON(200, successR(gin.H{
		"message": "规则删除成功",
	}))
}

func updateRuleOrder(c *gin.Context) {
	schemeId := c.Param("id")

	var req struct {
		RuleIds []int `json:"ruleIds"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, errorR(400, "无效的请求数据"))
		return
	}
	scheme, err := manager.DBM.RouteScheme.GetByID(schemeId)
	if err != nil {
		c.JSON(500, errorR(500, "获取路由方案失败"))
		return
	}

	// 更新规则优先级
	for i, ruleId := range req.RuleIds {

		rule, err := manager.DBM.Rule.GetByID(uint(ruleId))
		if err != nil || rule.RouteSchemeID != schemeId {
			continue
		}

		rule.Priority = uint(i)
		manager.DBM.Rule.Update(rule)
	}
	scheme, err = manager.DBM.RouteScheme.GetByID(schemeId)
	if err != nil {
		c.JSON(500, errorR(500, "获取路由方案失败"))
		return
	}
	manager.SyncRouteScheme(scheme)
	c.JSON(200, successR(gin.H{
		"message": "规则顺序更新成功",
	}))
}
