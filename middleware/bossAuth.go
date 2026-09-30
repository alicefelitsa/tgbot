package middleware

import (
	"github.com/gin-gonic/gin"
	"net/http"
	"tgbot/tools"
)

// BossAuth 管理员授权验证（JWT）
func BossAuth(c *gin.Context) {
	path := c.Request.URL.Path
	// 白名单路径直接放行
	if path == "/api/boss/login" || path == "/api/boss/logout" || path == "/api/boss/captcha" {
		c.Next()
		return
	}
	Authorization := c.GetHeader("Authorization")
	if Authorization == "" {
		c.JSON(http.StatusOK, gin.H{
			"code":    401,
			"message": "授权不可为空",
		})
		c.Abort()
		return
	}
	// 共用 JWT 解析（兼容 Bearer 前缀），并校验角色必须是后台管理员
	claims, err := tools.ParseAuthorization(Authorization)
	if err != nil || claims.Role != tools.RoleAdmin {
		c.JSON(http.StatusOK, gin.H{
			"code":    401,
			"message": "登录异常或已过期，请重新登录",
		})
		c.Abort()
		return
	}
	// 认证通过，把用户信息放进上下文供后续 handler 使用
	c.Set("userID", claims.UserID)
	c.Set("role", claims.Role)
	c.Next()
}
