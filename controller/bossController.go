package controller

import (
	"net/http"
	"tgbot/config"
	"tgbot/tools"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type BossController struct {
	db *gorm.DB
}

// NewBossController 创建管理员控制器
func NewBossController() *BossController {
	return &BossController{
		db: config.Mysql,
	}
}

// AdminLogin 管理员登录
func (bc *BossController) AdminLogin(c *gin.Context) {
	data := make(map[string]interface{})
	_ = c.BindJSON(&data)
	// Pluck 进 []int：GORM 内部完成列类型→int 转换（避开 map 断言与 .Row() 的 nil panic）
	var ids []int
	bc.db.Table("admin").Where("account = ? and password = ?", data["account"], data["password"]).Pluck("id", &ids)
	if len(ids) == 0 {
		c.JSON(http.StatusOK, gin.H{"code": 400, "message": "账户或密码不正确"})
		return
	}
	token, err := tools.GenerateToken(ids[0], tools.RoleAdmin)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 400, "message": "创建授权令牌失败：" + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "登录成功",
		"token":   token,
		"account": data["account"],
	})
}

// AdminLogout 管理员退出（JWT 无状态，前端清除本地 token 即可）
func (bc *BossController) AdminLogout(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "退出登录"})
}

// AuthUser 获取当前登录管理员信息
func (bc *BossController) AuthUser(c *gin.Context) {
	var code int
	var message string
	data := make(map[string]interface{})
	uid := c.GetInt("userID")
	resData := make([]map[string]interface{}, 0)
	err := bc.db.Raw("select * from admin where id = ?", uid).Scan(&resData).Error
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "message": err.Error()})
		return
	}
	if len(resData) > 0 {
		code = 0
		message = "操作成功"
		data["userId"] = resData[0]["id"]
		data["account"] = resData[0]["account"]
		data["nickname"] = resData[0]["account"]
	} else {
		code = 400
		message = "管理员不存在"
	}
	c.JSON(http.StatusOK, gin.H{"code": code, "message": message, "data": data})
}
