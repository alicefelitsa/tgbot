package controller

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"tgbot/config"
	"time"

	"github.com/gin-gonic/gin"
)

// ==================== 机器人资料(名称 / 简介) ====================
//
// 通过 Telegram Bot API 的 setMyName / setMyShortDescription / setMyDescription 设置,读取用同名 get 方法。
// 说明:
//  1. go-telegram-bot-api v5.5.1 尚未封装这几个较新的方法,且其 Config 接口用非导出方法(method()/params())
//     无法从外部包实现,故这里直接对 Bot API 发原生 HTTP POST(带 token),版本无关、稳定可控。
//  2. 头像**没有**对应的 Bot API 接口,只能用 @BotFather 手动上传,后台无法改。
//  3. 名称/简介同时写一份到 sys_setting(botName/botShortDescription/botDescription)供后台回显。
//     保存时「留空 = 不改动该项」(既不写库也不推 Telegram),避免误清空线上资料。

// tgCall 向 Telegram Bot API 发一次 POST 表单请求。成功返回 result 原始 JSON(可能是 true 或对象),失败返回 error。
func (r *TgBotRuntime) tgCall(method string, params url.Values) (json.RawMessage, error) {
	token := strings.TrimSpace(config.Conf.GetString("telegram.botToken"))
	if !tgTokenPattern.MatchString(token) {
		return nil, fmt.Errorf("未配置有效 botToken")
	}
	endpoint := "https://api.telegram.org/bot" + token + "/" + method
	// 后台触发的同步操作,给一个明确的总超时;沿用环境代理(与主 bot 一致的出网策略)
	client := &http.Client{
		Timeout:   20 * time.Second,
		Transport: &http.Transport{Proxy: http.ProxyFromEnvironment},
	}
	resp, err := client.PostForm(endpoint, params)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var out struct {
		OK          bool            `json:"ok"`
		Result      json.RawMessage `json:"result"`
		Description string          `json:"description"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	if !out.OK {
		if out.Description == "" {
			out.Description = "Telegram 返回失败"
		}
		return nil, fmt.Errorf("Telegram: %s", out.Description)
	}
	return out.Result, nil
}

// upsertSetting 写一条 sys_setting(存在则更新 svalue/name/remark,否则插入),供机器人资料回显。
func (r *TgBotRuntime) upsertSetting(skey, svalue, name, remark string) error {
	return r.db.Exec("insert into sys_setting (skey,svalue,name,remark,updated_at) values (?,?,?,?,?)"+
		" on duplicate key update svalue=values(svalue), name=values(name), remark=values(remark), updated_at=values(updated_at)",
		skey, svalue, name, remark, time.Now()).Error
}

// SaveBotProfile 保存机器人资料:每个非空字段先写库回显,再推送到 Telegram;留空的字段跳过不动。
// body: { name, shortDescription, description }
func (r *TgBotRuntime) SaveBotProfile(c *gin.Context) {
	var body struct {
		Name             string `json:"name"`
		ShortDescription string `json:"shortDescription"`
		Description      string `json:"description"`
	}
	_ = c.BindJSON(&body)

	type field struct {
		skey, label, remark, val, method, paramKey string
	}
	fields := []field{
		{"botName", "名称", "通过 setMyName 同步到 Telegram", strings.TrimSpace(body.Name), "setMyName", "name"},
		{"botShortDescription", "短简介", "显示在资料页「简介」,可多行,≤120字(setMyShortDescription)", strings.TrimSpace(body.ShortDescription), "setMyShortDescription", "short_description"},
		{"botDescription", "欢迎语", "与bot空聊天窗口的开场白,可多行,≤512字(setMyDescription)", strings.TrimSpace(body.Description), "setMyDescription", "description"},
	}

	var okList, skipList, failList []string
	for _, f := range fields {
		if f.val == "" {
			skipList = append(skipList, f.label)
			continue // 留空 = 不改动该项
		}
		if err := r.upsertSetting(f.skey, f.val, f.label, f.remark); err != nil {
			failList = append(failList, fmt.Sprintf("%s(存库失败:%v)", f.label, err))
			continue
		}
		if _, err := r.tgCall(f.method, url.Values{f.paramKey: {f.val}}); err != nil {
			failList = append(failList, fmt.Sprintf("%s(%v)", f.label, err))
			continue
		}
		okList = append(okList, f.label)
	}

	code := 0
	var msg string
	switch {
	case len(okList) == 0 && len(failList) == 0:
		msg = "未填写任何资料(留空表示不修改)"
	case len(failList) > 0:
		code = 500
		msg = "同步失败:" + strings.Join(failList, "、")
		if len(okList) > 0 {
			msg = "已同步:" + strings.Join(okList, "、") + ";失败:" + strings.Join(failList, "、")
		}
	default:
		msg = "已同步到 Telegram:" + strings.Join(okList, "、")
		if len(skipList) > 0 {
			msg += ";未填跳过:" + strings.Join(skipList, "、")
		}
	}
	c.JSON(http.StatusOK, gin.H{"code": code, "message": msg,
		"data": gin.H{"ok": okList, "skipped": skipList, "failed": failList}})
}

// GetBotProfile 从 Telegram 实时读取当前机器人资料(名称/短简介/简介),用于后台「拉取当前」回填。
// 尽力而为:某项读取失败(如网络不通)不影响其它项,失败原因随 data.failed 返回。
func (r *TgBotRuntime) GetBotProfile(c *gin.Context) {
	data := gin.H{"name": "", "shortDescription": "", "description": ""}
	var failed []string

	if res, err := r.tgCall("getMyName", url.Values{}); err != nil {
		failed = append(failed, "名称:"+err.Error())
	} else {
		var o struct {
			Name string `json:"name"`
		}
		_ = json.Unmarshal(res, &o)
		data["name"] = o.Name
	}
	if res, err := r.tgCall("getMyShortDescription", url.Values{}); err != nil {
		failed = append(failed, "短简介:"+err.Error())
	} else {
		var o struct {
			ShortDescription string `json:"short_description"`
		}
		_ = json.Unmarshal(res, &o)
		data["shortDescription"] = o.ShortDescription
	}
	if res, err := r.tgCall("getMyDescription", url.Values{}); err != nil {
		failed = append(failed, "简介:"+err.Error())
	} else {
		var o struct {
			Description string `json:"description"`
		}
		_ = json.Unmarshal(res, &o)
		data["description"] = o.Description
	}

	msg := "已获取当前资料"
	if len(failed) > 0 {
		msg = "部分获取失败:" + strings.Join(failed, ";")
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": msg, "data": data, "failed": failed})
}
