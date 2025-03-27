package web

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/ZIXT233/ziproxy/manager"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

// JWT声明结构
type Claims struct {
	UserID      string `json:"userId"`
	UserGroupID string `json:"userGroupId"`
	jwt.RegisteredClaims
}

type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func login(c *gin.Context) {
	var req LoginRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, errorR(400, "Invalid request data"))
		return
	}

	user, err := manager.DBM.User.GetByID(req.Username)
	if err != nil || user == nil || user.Password != req.Password || !user.Enabled {
		c.JSON(401, errorR(401, "Invalid username or password"))
		return
	}
	// 生成JWT令牌
	token, err := generateToken(user.ID, user.UserGroupID)
	if err != nil {
		c.JSON(500, errorR(500, "Failed to generate token"))
		return
	}

	c.JSON(200, successR(gin.H{
		"userId":      user.ID,
		"userGroupId": user.UserGroupID,
		"token":       token,
	}))
}

func logout(c *gin.Context) {
	// JWT无需服务端注销，客户端丢弃令牌即可
	c.JSON(200, successR(gin.H{
		"message": "Logged out successfully",
	}))
}

// 生成JWT令牌
func generateToken(userId, userGroupId string) (string, error) {
	// 设置JWT声明
	claims := Claims{
		UserID:      userId,
		UserGroupID: userGroupId,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(jwtExpiration)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			NotBefore: jwt.NewNumericDate(time.Now()),
		},
	}

	// 创建令牌
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	// 签名令牌
	return token.SignedString(jwtSecret)
}

// 解析JWT令牌
func parseToken(tokenString string) (*Claims, error) {
	// 解析令牌
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		return jwtSecret, nil
	})

	if err != nil {
		return nil, err
	}

	// 验证令牌
	if claims, ok := token.Claims.(*Claims); ok && token.Valid {
		return claims, nil
	}

	return nil, jwt.ErrSignatureInvalid
}

func getClaims(c *gin.Context) (*Claims, error) {
	authHeader := c.GetHeader("Authorization")
	if authHeader == "" {
		c.JSON(http.StatusUnauthorized, errorR(http.StatusUnauthorized, "未提供授权令牌"))
		c.Abort()
		return nil, fmt.Errorf("未提供授权令牌")
	}

	// 检查Bearer前缀
	parts := strings.SplitN(authHeader, " ", 2)
	if !(len(parts) == 2 && parts[0] == "Bearer") {
		c.JSON(http.StatusUnauthorized, errorR(http.StatusUnauthorized, "授权格式无效"))
		c.Abort()
		return nil, fmt.Errorf("授权格式无效")
	}

	// 解析令牌
	claims, err := parseToken(parts[1])
	if err != nil {
		c.JSON(http.StatusUnauthorized, errorR(http.StatusUnauthorized, "无效的令牌"))
		c.Abort()
		return nil, fmt.Errorf("无效的令牌")
	}
	return claims, nil
}

// 验证中间件
func adminCheck() gin.HandlerFunc {
	return func(c *gin.Context) {
		claims, err := getClaims(c)
		if err != nil {
			return
		}
		// 检查用户组权限
		if claims.UserGroupID != "管理员" {
			c.JSON(http.StatusForbidden, errorR(http.StatusForbidden, "无权限访问"))
			c.Abort()
			return
		}

		// 将用户信息存储在上下文中
		c.Set("userId", claims.UserID)
		c.Set("userGroupId", claims.UserGroupID)

		c.Next()
	}
}
func userCheck() gin.HandlerFunc {
	return func(c *gin.Context) {
		claims, err := getClaims(c)
		if err != nil {
			return
		}
		// 将用户信息存储在上下文中
		c.Set("userId", claims.UserID)
		c.Set("userGroupId", claims.UserGroupID)
		c.Next()
	}
}
func changePassword(c *gin.Context) {
	userID, ok := c.Get("userId")
	if !ok {
		c.JSON(http.StatusUnauthorized, errorR(http.StatusUnauthorized, "未提供用户ID"))
		c.Abort()
		return
	}
	user, err := manager.DBM.User.GetByID(userID.(string))
	if err != nil || user == nil {
		c.JSON(http.StatusUnauthorized, errorR(http.StatusUnauthorized, "用户不存在"))
		c.Abort()
		return
	}
	var req struct {
		OldPassword string `json:"oldPassword"`
		NewPassword string `json:"newPassword"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, errorR(http.StatusBadRequest, "无效的请求数据"))
		return
	}
	if user.Password != req.OldPassword {
		c.JSON(http.StatusBadRequest, errorR(http.StatusBadRequest, "旧密码不正确"))
		return
	}
	user.Password = req.NewPassword
	if err := manager.DBM.User.Update(user); err != nil {
		c.JSON(http.StatusInternalServerError, errorR(http.StatusInternalServerError, "密码更新失败"))
		return
	}
	c.JSON(http.StatusOK, successR(gin.H{"message": "密码更新成功"}))
}
