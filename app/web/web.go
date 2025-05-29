package web

import (
	"github.com/ZIXT233/ziproxy/utils"
	"github.com/gin-gonic/gin"
	"log"
	"net/http"
	"time"
)

// JWT密钥和过期时间
var (
	jwtSecret     []byte
	jwtExpiration = 24 * time.Hour // 令牌有效期为24小时
)

func StartWeb(config *utils.RootConfig) {
	address := config.WebAddress
	staticPath := config.StaticPath

	if staticPath == "" {
		staticPath = "./static"
	}
	if address == "" {
		address = ":8079"
	}
	if config.WebSecret == "" {
		log.Fatal("请在config.json中设置web_secret密钥用于加密会话")
		return
	} else {
		jwtSecret = []byte(config.WebSecret)
	}

	go func() {
		gin.SetMode(gin.ReleaseMode)
		r := gin.Default()
		r.Static("/assets", staticPath+"/dist/assets")
		r.StaticFile("/favicon.ico", staticPath+"/dist/favicon.ico")
		r.NoRoute(func(c *gin.Context) {
			// 如果没有匹配到其他路由，则返回 Vue 的 index.html
			c.File(staticPath + "/dist/index.html")
		})
		/* 配置CORS
		r.Use(cors.New(cors.Config{
			AllowOrigins:     []string{"http://localhost:5173"},
			AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
			AllowHeaders:     []string{"Content-Type", "Authorization"},
			ExposeHeaders:    []string{"Content-Length"},
			AllowCredentials: true,
			MaxAge:           12 * time.Hour,
		}))*/
		r.GET("/api/system/name", getSystemName)
		toAuth := r.Group("/api/auth")
		{
			toAuth.POST("/login", login)
			toAuth.POST("/logout", logout) // 客户端实现，服务端无需处理

		}
		common := r.Group("/api")
		common.Use(userCheck())
		{
			common.POST("/auth/change-password", changePassword)
			common.GET("/proxies/usable-inbounds", getUsableInbounds)
			common.GET("/dashboard/my-traffic", getMyTraffic)
			common.GET("/system/info", getSystemInfo)
			common.GET("/user/my-token", getMyToken)
			common.PUT("/user/my-token", updateMyToken)
		}
		admin := r.Group("/api")
		admin.Use(adminCheck())
		{
			proxies := admin.Group("/proxies")
			{
				proxies.GET("/inbound", getAllInbound)
				proxies.GET("/outbound", getAllOutbound)

				proxies.GET("/inbound/:id", getProxyData)
				proxies.GET("/outbound/:id", getProxyData)
				proxies.POST("/inbound", createProxyData)
				proxies.POST("/outbound", createProxyData)
				proxies.PUT("/inbound/:id", updateProxyData)
				proxies.PUT("/outbound/:id", updateProxyData)
				proxies.DELETE("/inbound/:id", deleteProxyData)
				proxies.DELETE("/outbound/:id", deleteProxyData)
				proxies.POST("/outbound/:id/test-speed", testOutboundSpeed)
			}
			admin.GET("/users", getAllUser)
			admin.POST("/users", createUser)
			admin.GET("/users/:id", getUser)
			admin.PUT("/users/:id", updateUser)
			admin.DELETE("/users/:id", deleteUser)
			admin.POST("/users/:id/reset-password", userResetPassword)
			admin.GET("/user-groups", getAllUserGroup)
			admin.GET("/user-groups/:id", getUserGroup)
			admin.POST("/user-groups", createUserGroup)
			admin.PUT("/user-groups/:id", updateUserGroup)
			admin.DELETE("/user-groups/:id", deleteUserGroup)

			routes := admin.Group("/routes")
			{
				schemes := routes.Group("/schemes")
				{
					schemes.GET("", getAllRouteScheme)
					schemes.GET("/:id", getRouteScheme)
					schemes.POST("", createRouteScheme)
					schemes.PUT("/:id", updateRouteScheme)
					schemes.DELETE("/:id", deleteRouteScheme)
					schemes.POST("/:id/toggle-status", toggleRouteSchemeStatus)

					// 规则相关
					schemes.GET("/:id/rules", getRules)
					schemes.POST("/:id/rules", addRule)
					schemes.PUT("/:id/rules/:ruleId", updateRule)
					schemes.DELETE("/:id/rules/:ruleId", deleteRule)
					schemes.POST("/:id/rules/reorder", updateRuleOrder)
				}
			}
			dashboard := admin.Group("/dashboard")
			{
				dashboard.GET("/traffic-history/:timeRange", getTrafficHistory)
				dashboard.GET("/traffic-status", getTrafficStatus)
				dashboard.GET("/proxy-traffic-rank/:direction", getProxyTrafficRank)
				dashboard.GET("/user-traffic-rank", getUserTrafficRank)

				dashboard.GET("/active-user-link", getActiveUserLink)
			}

			admin.PUT("/system/info", updateSystemInfo)
			admin.POST("/system/clear-badger-cache", clearHTTPCache)
		}

		log.Printf("Web面板启动，地址：http://%s，静态文件路径：%s", address, staticPath)
		r.Run(address)
	}()
}

// ApiResponse 定义API响应结构
type ApiResponse struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"` // 使用omitempty选项，当Data为空时不序列化该字段
}

func successR(data interface{}) ApiResponse {
	return ApiResponse{
		Code:    http.StatusOK,
		Message: "success",
		Data:    data,
	}
}

func errorR(code int, message string) ApiResponse {
	return ApiResponse{
		Code:    code,
		Message: message,
	}
}
