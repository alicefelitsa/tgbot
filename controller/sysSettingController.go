package controller

import (
	"fmt"
	"net/http"
	"strings"
	"tgbot/tools"
	"time"

	"github.com/gin-gonic/gin"
)

// ==================== 系统设置 sys_setting ====================
//
// key-value 形式的运行时配置存库,后台「系统设置」页可增删改,改完即生效(读取处走 settingGet/settingInt64)。
// 与固定表不同:这里面向运维自助调参,不预设字段结构,一行业务=一个配置项。

var sysSettingCols = []string{"skey", "svalue", "name", "remark"}

// GetSysSettingList 设置列表(按 key/名称模糊过滤;数据少,一次全量、不分页)
func (tc *TgController) GetSysSettingList(c *gin.Context) {
	var count int
	conds := "1 = 1"
	if kw := c.Query("keyword"); kw != "" {
		conds += fmt.Sprintf(" and (skey like '%%%v%%' or name like '%%%v%%')", kw, kw)
	}
	where := " where " + conds
	data := make([]map[string]interface{}, 0)
	err := tc.db.Raw("select id,skey,svalue,name,remark,updated_at from sys_setting" + where + " order by id asc").Scan(&data).Error
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "message": err.Error()})
		return
	}
	formatTimeFields(data)
	if err = tc.db.Raw("select count(id) from sys_setting" + where).Scan(&count).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 501, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "操作成功", "count": count, "data": data})
}

// AddSysSetting 新增设置项(skey 唯一,重复则拒绝)
func (tc *TgController) AddSysSetting(c *gin.Context) {
	raw := make(map[string]interface{})
	_ = c.BindJSON(&raw)
	data := onlyFields(raw, sysSettingCols)
	skey, _ := data["skey"].(string)
	skey = strings.TrimSpace(skey)
	if skey == "" {
		c.JSON(http.StatusOK, gin.H{"code": 400, "message": "参数错误:配置键(skey)不能为空"})
		return
	}
	data["skey"] = skey
	var dup int
	tc.db.Raw("select count(id) from sys_setting where skey = ?", skey).Scan(&dup)
	if dup > 0 {
		c.JSON(http.StatusOK, gin.H{"code": 400, "message": "配置键已存在:" + skey})
		return
	}
	data["updated_at"] = time.Now()
	if err := tc.db.Table("sys_setting").Create(data).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "操作成功"})
}

// SaveSysSetting 修改设置项(按 id;skey 若改动需保持唯一)
func (tc *TgController) SaveSysSetting(c *gin.Context) {
	raw := make(map[string]interface{})
	_ = c.BindJSON(&raw)
	id := raw["id"]
	data := onlyFields(raw, sysSettingCols)
	if s, ok := data["skey"].(string); ok {
		sk := strings.TrimSpace(s)
		if sk == "" {
			c.JSON(http.StatusOK, gin.H{"code": 400, "message": "配置键(skey)不能为空"})
			return
		}
		data["skey"] = sk
		var dup int
		tc.db.Raw("select count(id) from sys_setting where skey = ? and id <> ?", sk, id).Scan(&dup)
		if dup > 0 {
			c.JSON(http.StatusOK, gin.H{"code": 400, "message": "配置键已被其它项占用:" + sk})
			return
		}
	}
	data["updated_at"] = time.Now()
	if err := tc.db.Table("sys_setting").Where("id = ?", id).Updates(data).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "操作成功"})
}

// DelSysSetting 删除设置项(逗号 ids)
func (tc *TgController) DelSysSetting(c *gin.Context) {
	idList := tools.SplitIds(c.Query("ids"))
	if len(idList) == 0 {
		c.JSON(http.StatusOK, gin.H{"code": 400, "message": "参数错误"})
		return
	}
	if err := tc.db.Exec("delete from sys_setting where id in (?)", idList).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "操作成功"})
}

// GetSysSettingMap 一次性返回所有设置的 key→value 映射(供固定表单页回填)
func (tc *TgController) GetSysSettingMap(c *gin.Context) {
	data := make([]map[string]interface{}, 0)
	if err := tc.db.Raw("select skey,svalue from sys_setting order by id asc").Scan(&data).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "message": err.Error()})
		return
	}
	m := make(map[string]string, len(data))
	for _, row := range data {
		k := fmt.Sprintf("%v", row["skey"])
		v := fmt.Sprintf("%v", row["svalue"])
		if v == "<nil>" {
			v = ""
		}
		m[k] = v
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "操作成功", "data": m})
}

// SaveSysSettingBatch 批量保存固定设置项:body {"items":[{"skey","svalue","name","remark"}]}
// 按 skey upsert(存在则只更新 svalue,不存在则插入并带上 name/remark),固定表单一次提交多项。
func (tc *TgController) SaveSysSettingBatch(c *gin.Context) {
	var body struct {
		Items []struct {
			Skey   string `json:"skey"`
			Svalue string `json:"svalue"`
			Name   string `json:"name"`
			Remark string `json:"remark"`
		} `json:"items"`
	}
	_ = c.BindJSON(&body)
	now := time.Now()
	for _, it := range body.Items {
		sk := strings.TrimSpace(it.Skey)
		if sk == "" {
			continue
		}
		err := tc.db.Exec("insert into sys_setting (skey,svalue,name,remark,updated_at) values (?,?,?,?,?)"+
			" on duplicate key update svalue=values(svalue), updated_at=values(updated_at)",
			sk, it.Svalue, it.Name, it.Remark, now).Error
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"code": 500, "message": err.Error()})
			return
		}
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "保存成功"})
}
