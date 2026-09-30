package controller

import (
	"fmt"
	"net/http"
	"strings"
	"tgbot/config"
	"tgbot/tools"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type TgController struct {
	db *gorm.DB
}

// NewTgController 创建机器人后台控制器
func NewTgController() *TgController {
	return &TgController{db: config.Mysql}
}

// 各表可写字段白名单:过滤前端可能带来的展示字段(如 parent_title),避免脏键进 SQL
var (
	tgMenuCols    = []string{"parent_id", "lang", "title", "action_type", "action_config", "cols", "sort", "status"}
	tgHandlerCols = []string{"handler_key", "name", "param_hint", "remark", "status"}
	tgUserCols    = []string{"tg_user_id", "chat_id", "username", "first_name", "lang", "bind_user_id", "status"}
	tgChatCols    = []string{"chat_id", "title", "username", "type", "status"}
	tgCommandCols = []string{"command", "description", "action_type", "action_config", "sort", "status"}
)

// onlyFields 仅保留白名单内的键(供 Add/Save 使用)
func onlyFields(data map[string]interface{}, cols []string) map[string]interface{} {
	out := make(map[string]interface{})
	for _, c := range cols {
		if v, ok := data[c]; ok {
			out[c] = v
		}
	}
	return out
}

// ==================== 菜单树 tg_menu ====================

// GetTgMenuList 菜单列表(按 parent_id 精确、title 模糊过滤)
func (tc *TgController) GetTgMenuList(c *gin.Context) {
	var count int
	conds := "1 = 1"
	if pid := c.Query("parent_id"); pid != "" {
		conds += fmt.Sprintf(" and parent_id = %s", pid)
	}
	if title := c.Query("title"); title != "" {
		conds += fmt.Sprintf(" and title like '%%%v%%'", title)
	}
	where := " where " + conds
	data := make([]map[string]interface{}, 0)
	err := tc.db.Raw("select id,parent_id,lang,title,action_type,cast(action_config as char) as action_config,cols,sort,status,created_at,updated_at" +
		" from tg_menu" + where + " order by parent_id asc, sort asc, id asc" + pageLimit(c)).Scan(&data).Error
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "message": err.Error()})
		return
	}
	formatTimeFields(data)
	if err = tc.db.Raw("select count(id) from tg_menu" + where).Scan(&count).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 501, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "操作成功", "count": count, "data": data})
}

// AddTgMenu 新增菜单
func (tc *TgController) AddTgMenu(c *gin.Context) {
	raw := make(map[string]interface{})
	_ = c.BindJSON(&raw)
	data := onlyFields(raw, tgMenuCols)
	data["created_at"] = time.Now()
	data["updated_at"] = time.Now()
	if err := tc.db.Table("tg_menu").Create(data).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "操作成功"})
}

// SaveTgMenu 修改菜单
func (tc *TgController) SaveTgMenu(c *gin.Context) {
	raw := make(map[string]interface{})
	_ = c.BindJSON(&raw)
	id := raw["id"]
	data := onlyFields(raw, tgMenuCols)
	data["updated_at"] = time.Now()
	if err := tc.db.Table("tg_menu").Where("id = ?", id).Updates(data).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "操作成功"})
}

// DelTgMenu 删除菜单(逗号 ids):有子菜单的上级禁止直接删除,必须先删其子菜单(自底向上),
// 从根上避免“连带误删子孙”与“删父留孤儿子菜单”两类问题。
func (tc *TgController) DelTgMenu(c *gin.Context) {
	idList := tools.SplitIds(c.Query("ids"))
	if len(idList) == 0 {
		c.JSON(http.StatusOK, gin.H{"code": 400, "message": "参数错误"})
		return
	}
	// 选中项里,凡是仍被当作父级(有子菜单 parent_id 指向它)的,都不允许删
	var blocked []int64
	if err := tc.db.Table("tg_menu").Where("parent_id in (?)", idList).Pluck("distinct parent_id", &blocked).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "message": err.Error()})
		return
	}
	if len(blocked) > 0 {
		var titles []string
		tc.db.Table("tg_menu").Where("id in (?)", blocked).Pluck("title", &titles)
		msg := "请先删除子菜单，再删除上级菜单"
		if len(titles) > 0 {
			msg = "「" + strings.Join(titles, "、") + "」下还有子菜单，请先删除其子菜单"
		}
		c.JSON(http.StatusOK, gin.H{"code": 400, "message": msg})
		return
	}
	if err := tc.db.Exec("delete from tg_menu where id in (?)", idList).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "操作成功"})
}

// ==================== 动态数据类白名单 tg_handler ====================

// GetTgHandlerList 数据类列表(供菜单编辑下拉选择,不带分页参数时返回全部启用项)
func (tc *TgController) GetTgHandlerList(c *gin.Context) {
	var count int
	conds := "1 = 1"
	if kw := c.Query("name"); kw != "" {
		conds += fmt.Sprintf(" and (name like '%%%v%%' or handler_key like '%%%v%%')", kw, kw)
	}
	where := " where " + conds
	data := make([]map[string]interface{}, 0)
	err := tc.db.Raw("select id,handler_key,name,param_hint,remark,status,created_at,updated_at" +
		" from tg_handler" + where + " order by id asc" + pageLimit(c)).Scan(&data).Error
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "message": err.Error()})
		return
	}
	formatTimeFields(data)
	if err = tc.db.Raw("select count(id) from tg_handler" + where).Scan(&count).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 501, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "操作成功", "count": count, "data": data})
}

// AddTgHandler 新增数据类
func (tc *TgController) AddTgHandler(c *gin.Context) {
	raw := make(map[string]interface{})
	_ = c.BindJSON(&raw)
	data := onlyFields(raw, tgHandlerCols)
	data["created_at"] = time.Now()
	data["updated_at"] = time.Now()
	if err := tc.db.Table("tg_handler").Create(data).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "操作成功"})
}

// SaveTgHandler 修改数据类
func (tc *TgController) SaveTgHandler(c *gin.Context) {
	raw := make(map[string]interface{})
	_ = c.BindJSON(&raw)
	id := raw["id"]
	data := onlyFields(raw, tgHandlerCols)
	data["updated_at"] = time.Now()
	if err := tc.db.Table("tg_handler").Where("id = ?", id).Updates(data).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "操作成功"})
}

// DelTgHandler 删除数据类(逗号 ids)
func (tc *TgController) DelTgHandler(c *gin.Context) {
	idList := tools.SplitIds(c.Query("ids"))
	if len(idList) == 0 {
		c.JSON(http.StatusOK, gin.H{"code": 400, "message": "参数错误"})
		return
	}
	if err := tc.db.Exec("delete from tg_handler where id in (?)", idList).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "操作成功"})
}

// ==================== Telegram 用户 tg_user ====================

// GetTgUserList 用户列表(按用户名模糊、tg_user_id 精确过滤)
func (tc *TgController) GetTgUserList(c *gin.Context) {
	var count int
	conds := "1 = 1"
	if kw := c.Query("username"); kw != "" {
		conds += fmt.Sprintf(" and username like '%%%v%%'", kw)
	}
	if tuid := c.Query("tg_user_id"); tuid != "" {
		conds += fmt.Sprintf(" and tg_user_id = %s", tuid)
	}
	where := " where " + conds
	data := make([]map[string]interface{}, 0)
	err := tc.db.Raw("select id,tg_user_id,chat_id,username,first_name,lang,bind_user_id,status,created_at,updated_at" +
		" from tg_user" + where + " order by id desc" + pageLimit(c)).Scan(&data).Error
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "message": err.Error()})
		return
	}
	formatTimeFields(data)
	if err = tc.db.Raw("select count(id) from tg_user" + where).Scan(&count).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 501, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "操作成功", "count": count, "data": data})
}

// SaveTgUser 修改用户(启停状态/绑定业务账号等)
func (tc *TgController) SaveTgUser(c *gin.Context) {
	raw := make(map[string]interface{})
	_ = c.BindJSON(&raw)
	id := raw["id"]
	data := onlyFields(raw, tgUserCols)
	data["updated_at"] = time.Now()
	if err := tc.db.Table("tg_user").Where("id = ?", id).Updates(data).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "操作成功"})
}

// DelTgUser 删除用户(逗号 ids)
func (tc *TgController) DelTgUser(c *gin.Context) {
	idList := tools.SplitIds(c.Query("ids"))
	if len(idList) == 0 {
		c.JSON(http.StatusOK, gin.H{"code": 400, "message": "参数错误"})
		return
	}
	if err := tc.db.Exec("delete from tg_user where id in (?)", idList).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "操作成功"})
}

// ==================== Telegram 群组/频道 tg_chat ====================

// GetTgChatList 群组列表(bot 被拉进群时自动落库;按标题/用户名模糊、类型精确过滤)
func (tc *TgController) GetTgChatList(c *gin.Context) {
	var count int
	conds := "1 = 1"
	if kw := c.Query("title"); kw != "" {
		conds += fmt.Sprintf(" and (title like '%%%v%%' or username like '%%%v%%')", kw, kw)
	}
	if ctype := c.Query("type"); ctype != "" {
		conds += fmt.Sprintf(" and type = '%s'", ctype)
	}
	where := " where " + conds
	data := make([]map[string]interface{}, 0)
	err := tc.db.Raw("select id,chat_id,title,username,type,status,created_at,updated_at" +
		" from tg_chat" + where + " order by status desc, id desc" + pageLimit(c)).Scan(&data).Error
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "message": err.Error()})
		return
	}
	formatTimeFields(data)
	if err = tc.db.Raw("select count(id) from tg_chat" + where).Scan(&count).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 501, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "操作成功", "count": count, "data": data})
}

// SaveTgChat 修改群组(仅改 status:用于人工修正失效/恢复可推送记录)
func (tc *TgController) SaveTgChat(c *gin.Context) {
	raw := make(map[string]interface{})
	_ = c.BindJSON(&raw)
	id := raw["id"]
	data := onlyFields(raw, tgChatCols)
	data["updated_at"] = time.Now()
	if err := tc.db.Table("tg_chat").Where("id = ?", id).Updates(data).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "操作成功"})
}

// DelTgChat 删除群组(逗号 ids;仅删本地记录,不影响 bot 在群里的实际成员状态)
func (tc *TgController) DelTgChat(c *gin.Context) {
	idList := tools.SplitIds(c.Query("ids"))
	if len(idList) == 0 {
		c.JSON(http.StatusOK, gin.H{"code": 400, "message": "参数错误"})
		return
	}
	if err := tc.db.Exec("delete from tg_chat where id in (?)", idList).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "操作成功"})
}

// ==================== 命令菜单 tg_command ====================

// GetTgCommandList 命令列表
func (tc *TgController) GetTgCommandList(c *gin.Context) {
	var count int
	conds := "1 = 1"
	if kw := c.Query("command"); kw != "" {
		conds += fmt.Sprintf(" and (command like '%%%v%%' or description like '%%%v%%')", kw, kw)
	}
	where := " where " + conds
	data := make([]map[string]interface{}, 0)
	err := tc.db.Raw("select id,command,description,action_type,cast(action_config as char) as action_config,sort,status,created_at,updated_at" +
		" from tg_command" + where + " order by sort asc, id asc").Scan(&data).Error
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "message": err.Error()})
		return
	}
	formatTimeFields(data)
	if err = tc.db.Raw("select count(id) from tg_command" + where).Scan(&count).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 501, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "操作成功", "count": count, "data": data})
}

// AddTgCommand 新增命令
func (tc *TgController) AddTgCommand(c *gin.Context) {
	raw := make(map[string]interface{})
	_ = c.BindJSON(&raw)
	data := onlyFields(raw, tgCommandCols)
	data["created_at"] = time.Now()
	data["updated_at"] = time.Now()
	if err := tc.db.Table("tg_command").Create(data).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "操作成功"})
}

// SaveTgCommand 修改命令
func (tc *TgController) SaveTgCommand(c *gin.Context) {
	raw := make(map[string]interface{})
	_ = c.BindJSON(&raw)
	id := raw["id"]
	data := onlyFields(raw, tgCommandCols)
	data["updated_at"] = time.Now()
	if err := tc.db.Table("tg_command").Where("id = ?", id).Updates(data).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "操作成功"})
}

// DelTgCommand 删除命令(逗号 ids)
func (tc *TgController) DelTgCommand(c *gin.Context) {
	idList := tools.SplitIds(c.Query("ids"))
	if len(idList) == 0 {
		c.JSON(http.StatusOK, gin.H{"code": 400, "message": "参数错误"})
		return
	}
	if err := tc.db.Exec("delete from tg_command where id in (?)", idList).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "操作成功"})
}
