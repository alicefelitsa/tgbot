package controller

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	tg "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"gorm.io/gorm"

	"tgbot/config"
)

// ==================== handler 注册表(§4) ====================
//
// 菜单里 action_type=handler 指向的「数据类」,就是这里注册的函数。
// 写一个函数、注册一个 key,后台就能选它;纯配置改动无需重新编译。

// TgButton 内联键盘按钮:Cb 走站内回调,Url 走外链,二者取一
type TgButton struct {
	Text string // 按钮文字
	Cb   string // callback_data(站内跳转,只放 域:ID)
	Url  string // 或外链
}

// TgResult 一次动作的执行结果:要渲染的文案 + 可选配图 + 按钮 + 排版列数
type TgResult struct {
	Text       string
	Image      string // 可选配图直链 URL;非空则这条消息以「图片+文案(caption)+按钮」形式呈现(不发纯文本)
	HTML       bool   // 文案/标题按 HTML 富文本渲染(Telegram parse_mode=HTML)
	Buttons    []TgButton
	Cols       int    // <=0 用默认
	ForceReply bool   // 发送后高亮回复输入框(等待用户输入场景),仅新发消息生效
	ReplyHint  string // ForceReply 时输入框内的占位提示(可选)
	// ImageData/ImageName:群发等场景在循环外预下载一次的图片字节。非空则 sendPhoto 直接以
	// multipart 上传,不再依赖 Telegram 去抓 URL(规避海外服务器拉不到国内 CDN 图)。
	ImageData []byte
	ImageName string
}

// TgUser 当前操作机器人的 Telegram 用户(分发层从 tg_user 读出)
type TgUser struct {
	TgUserID  int64
	ChatID    int64
	Lang      string
	BindID    int64
	Username  string // 供 http 取数的模板变量 {username}
	FirstName string // 供 http 取数的模板变量 {first_name}
}

// TgHandlerFunc 数据处理器:拿 *gorm.DB、当前用户、参数,返回要渲染的内容(照 shortplay 用 GORM)
type TgHandlerFunc func(db *gorm.DB, u *TgUser, args map[string]string) (*TgResult, error)

var tgHandlers = map[string]TgHandlerFunc{}

// RegisterTg 注册一个数据处理器到白名单
func RegisterTg(key string, fn TgHandlerFunc) { tgHandlers[key] = fn }

// GetTgHandler 按 key 取数据处理器
func GetTgHandler(key string) (TgHandlerFunc, bool) {
	fn, ok := tgHandlers[key]
	return fn, ok
}

// ==================== 内联键盘与工具函数 ====================

// buildKeyboard 按 cols 把按钮排成多行内联键盘。
// 无按钮时返回空键盘(内联键盘为空),配合 EditMessageText 可清除上一条消息残留的按钮。
// 返回指针以适配 EditMessageTextConfig.ReplyMarkup(*InlineKeyboardMarkup)与 MessageConfig.ReplyMarkup(interface)。
func buildKeyboard(buttons []TgButton, cols int) *tg.InlineKeyboardMarkup {
	if cols <= 0 {
		cols = 2
	}
	rows := make([][]tg.InlineKeyboardButton, 0, len(buttons)/cols+1)
	var cur []tg.InlineKeyboardButton
	for _, b := range buttons {
		var btn tg.InlineKeyboardButton
		if b.Url != "" {
			// 非法外链(如含中文的占位 URL)会被 Telegram 拒绝整条消息,直接跳过该按钮并告警
			if !validTgURL(b.Url) {
				config.LogWarning("tg 跳过非法按钮 URL:%q(按钮:%q)", b.Url, b.Text)
				continue
			}
			btn = tg.NewInlineKeyboardButtonURL(b.Text, b.Url)
		} else {
			btn = tg.NewInlineKeyboardButtonData(b.Text, b.Cb)
		}
		cur = append(cur, btn)
		if len(cur) == cols {
			rows = append(rows, cur)
			cur = nil
		}
	}
	if len(cur) > 0 { // 收尾剩余不足一行的按钮(跳过后末尾判断不再可靠,单独 flush)
		rows = append(rows, cur)
	}
	if len(rows) == 0 { // 无任何按钮:返回 nil,调用方据此不带 reply_markup。
		// (库对空键盘会序列化成 {},Telegram 报 "inline_keyboard must be of type Array" 而拒发整条消息)
		return nil
	}
	markup := tg.NewInlineKeyboardMarkup(rows...)
	return &markup
}

// validTgURL 校验 Telegram 可接受的外链:必须是 http/https 且主机名为合法 ASCII 域名/IP。
// 种子数据里的占位(如 https://你的域名)含非 ASCII,会被 Telegram 判 Wrong HTTP URL 并拒绝整条消息。
func validTgURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return false
	}
	host := u.Hostname()
	if host == "" {
		return false
	}
	for _, r := range host {
		if r > 127 { // 非 ASCII 主机名
			return false
		}
	}
	return true
}

// splitCb 拆分 callback_data 为「域:参数」(§5,只放 ID 不塞数据)
func splitCb(data string) (domain, arg string) {
	if i := strings.Index(data, ":"); i >= 0 {
		return data[:i], data[i+1:]
	}
	return data, ""
}

// parseConfig 解析 JSON 列(读出为 string/[]byte)为扁平字符串 map;非法或空返回空 map
func parseConfig(v interface{}) map[string]string {
	out := map[string]string{}
	var s string
	switch b := v.(type) {
	case string:
		s = b
	case []byte:
		s = string(b)
	}
	if s == "" || s == "null" {
		return out
	}
	_ = json.Unmarshal([]byte(s), &out)
	return out
}

// parseArgs 解析 handler 参数串(如 "limit=5,key=v")为 map
func parseArgs(s string) map[string]string {
	out := map[string]string{}
	for _, kv := range strings.Split(s, ",") {
		kv = strings.TrimSpace(kv)
		if kv == "" {
			continue
		}
		if i := strings.Index(kv, "="); i >= 0 {
			out[strings.TrimSpace(kv[:i])] = strings.TrimSpace(kv[i+1:])
		} else {
			out[kv] = ""
		}
	}
	return out
}

// toInt64 归一化 GORM 扫描进 map 的数值列(BIGINT→int64 等),供 id/parent_id 使用
func toInt64(v interface{}) int64 {
	s := strings.TrimSpace(fmt.Sprintf("%v", v))
	if s == "" || s == "<nil>" {
		return 0
	}
	n, _ := strconv.ParseInt(s, 10, 64)
	return n
}

// toInt 归一化数值列为 int(如 cols 排版列)
func toInt(v interface{}) int { return int(toInt64(v)) }

// ==================== 通用占位 handler(初稿,不依赖业务表) ====================

func init() {
	// demo_list:示例动态列表。接真实业务时把 mock 循环换成
	// db.Raw("select ... from 你的表 where ... limit ?", limit).Scan(&list) 即可,分发逻辑不动。
	RegisterTg("demo_list", func(db *gorm.DB, u *TgUser, args map[string]string) (*TgResult, error) {
		limit := 5
		if v, ok := args["limit"]; ok {
			fmt.Sscanf(v, "%d", &limit)
		}
		res := &TgResult{Text: "📦 示例动态列表(占位,替换成你的查询):", Cols: 1}
		for i := 1; i <= limit; i++ {
			res.Buttons = append(res.Buttons, TgButton{
				Text: fmt.Sprintf("示例条目 #%d", i),
				Cb:   fmt.Sprintf("item:%d", i), // 只放 ID,不塞数据
			})
		}
		return res, nil
	})

	// item_detail:示例条目详情(demo_list 按钮的二级动作)
	RegisterTg("item_detail", func(db *gorm.DB, u *TgUser, args map[string]string) (*TgResult, error) {
		return &TgResult{Text: "🔎 条目 #" + args["id"] + " 的详情占位"}, nil
	})
}
