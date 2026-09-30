package controller

import (
	"fmt"
	"strconv"
	"strings"
	"tgbot/config"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// 本文件存放各控制器共用的辅助函数（照搬 shortplay controller/common.go 约定），
// 后台 CRUD 与 Bot 分发层都可复用；当前初稿先落骨架，后续接口按需调用。

// formatTimeFields 将结果集中的 time.Time 字段格式化为字符串
func formatTimeFields(rows []map[string]interface{}) {
	for _, row := range rows {
		for col, val := range row {
			if t, ok := val.(time.Time); ok {
				row[col] = t.Format("2006-01-02 15:04:05")
			}
		}
	}
}

// pageLimit 处理Mysql数据分页：读取 page/limit 查询参数，拼成 limit 子句（同包共用，无需导出）。
func pageLimit(c *gin.Context) string {
	limit, err := strconv.Atoi(c.Query("limit"))
	page, err := strconv.Atoi(c.Query("page"))
	if page == 0 || limit == 0 || err != nil {
		return ""
	} else {
		page = (page - 1) * limit
		res := fmt.Sprintf(" limit %v,%v", page, limit)
		return res
	}
}

// ==================== 系统设置 sys_setting(数据库优先、config.yaml 回落) ====================
//
// 运行时才读取的配置(如图片库中转 chat_id)存数据库,后台改完立即生效、无需重启;
// 读取时「DB 有值用 DB,否则回落 config.yaml,再回落调用方默认」,保证删库行也不炸。

// settingGet 取一条设置的字符串值。返回(值, 是否存在有效值)。
func settingGet(db *gorm.DB, key string) (string, bool) {
	if db == nil {
		return "", false
	}
	var rows []map[string]interface{}
	if err := db.Raw("select svalue from sys_setting where skey = ?", key).Scan(&rows).Error; err != nil || len(rows) == 0 {
		return "", false
	}
	v := strings.TrimSpace(fmt.Sprintf("%v", rows[0]["svalue"]))
	if v == "" || v == "<nil>" {
		return "", false
	}
	return v, true
}

// settingInt64 取一条设置并解析为 int64:优先 sys_setting,空/非法则回落 config.yaml 的 cfgKey,
// 仍为 0 则由调用方判默认。chat_id / 数值型开关走这里。
func settingInt64(db *gorm.DB, key, cfgKey string) int64 {
	if v, ok := settingGet(db, key); ok {
		if n := toInt64(v); n != 0 {
			return n
		}
	}
	return config.Conf.GetInt64(cfgKey)
}
