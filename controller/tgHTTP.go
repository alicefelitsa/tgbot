package controller

// ==================== 配置化「接口取数」引擎(action_type=http) ====================
//
// 目标:让运营在后台纯配置外部接口(URL/请求体/渲染模板)即可给用户返回动态数据,
// 不写 Go 代码、不重编译。它是动作引擎的一种 action_type,不走 RegisterTg 注册表。
//
// action_config(扁平 string→string,兼容 parseConfig):
//   method    GET/POST(默认 GET)
//   url       接口地址模板,可含 {uid}{chat}{lang}{bind}{username} 及 {args键} 占位
//   headers   请求头,每行 "Key: Value"(可选)
//   body      POST 请求体模板(可选)
//   args      额外模板变量,"k=v,k2=v2"(可选,并入上下文)
//   text      显示文案模板,可对响应 JSON 取路径如 {data.balance}(可选)
//
// 内置模板变量(无需接口返回即可用):{uid}{chat}{lang}{bind}{username}{first_name}
// 及北京时间 {time}(2006-01-02 15:04:05)/{date}(2006-01-02)/{clock}(15:04:05)。
//   input_prompt 提示语(可选):配了则点击不立即调接口,先让用户输一条文本,
//              输入作为模板变量(名由 input_key 定,默认 input)参与后续 url/body/text 渲染
//   input_key 用户输入绑定的变量名(可选,默认 input)
//   image_path 图片来源,三种写法自适应:① file:<id> 图片库引用(图库按钮回填);
//              ②以 http(s) 开头 → 图片直链(可含 {uid} 等变量,如 https://x.com/{uid}.jpg);
//              ③否则视为响应 JSON 路径(如 data.pic)取直链。取到合法图则内嵌图片发送(可选)
//   list_path 指向响应里数组的路径,如 data.items;配了则每项渲染成一个按钮(可选)
//   btn_text  按钮文字模板,以当前数组元素为根,如 {title}
//   btn_url   按钮外链模板(优先),如 {link}
//   btn_cb    按钮回调模板(一般留空)
//   cols      每行按钮数(<=0 用 1)

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"tgbot/config"
)

// tgHTTPClient 专用于对外取数的独立客户端,不复用 bot 的长轮询 client,
// 避免把长轮询的超时配置牵连到这里(见记忆:长轮询专用 client 陷阱)。
var tgHTTPClient = &http.Client{Timeout: 8 * time.Second}

// httpTokenRe 匹配 {key} 占位。key 限定为合法变量名/JSON 路径(字母下划线开头,
// 仅含 字母/数字/下划线/点/方括号下标),避免把整段 JSON 字面量(如 {"title":"x"})
// 误当成一个变量吞掉——否则扁平 JSON 请求体会被整体替换成空串。
var httpTokenRe = regexp.MustCompile(`\{([A-Za-z_][A-Za-z0-9_.\[\]]*)\}`)

// httpResult 调用外部接口并把结果渲染成 TgResult。任何环节失败都回友好文案,
// 绝不返回 nil(否则 onCallback 只会 answer"暂无数据"而不编辑消息,用户看不到反馈)。
// extraCtx 为额外模板变量(如等待输入后注入的 {input}),优先级高于内置用户变量。
func (r *TgBotRuntime) httpResult(cfg map[string]string, u *TgUser, extraCtx map[string]string) *TgResult {
	fail := &TgResult{Text: "⚠️ 暂时取不到数据，请稍后再试"}
	ctx := httpCtx(u, parseArgs(cfg["args"]))
	for k, v := range extraCtx { // 用户输入变量最后覆盖(优先级最高)
		ctx[k] = v
	}

	// 没配接口地址:不发请求,直接把模板(显示文案/配图)渲染回给用户。
	// 响应路径变量(如 {data.xxx})因无 resp 取空;内置变量与 {input}/{payee} 等 extraCtx 照常可用。
	var parsed interface{}
	if urlTpl := strings.TrimSpace(cfg["url"]); urlTpl != "" {
		rawURL := strings.TrimSpace(httpRenderURL(urlTpl, ctx, nil))
		if !validTgURL(rawURL) {
			config.LogWarning("http fetch: 非法 URL %q", urlTpl)
			return fail
		}
		method := strings.ToUpper(strings.TrimSpace(cfg["method"]))
		if method == "" {
			method = http.MethodGet
		}
		body := ""
		if method == http.MethodPost {
			body = httpRender(cfg["body"], ctx, nil)
		}
		var ok bool
		parsed, ok = httpFetch(rawURL, method, cfg["headers"], body)
		if !ok {
			return fail
		}
	}

	res := &TgResult{}
	if text := strings.TrimSpace(cfg["text"]); text != "" {
		res.Text = httpRender(text, ctx, parsed)
	} else {
		res.Text = "🌐 数据已更新"
	}

	// 配图三种写法自适应:① file: 前缀 → 图片库引用,直接当 FileID 用(发送端 photoFile 识别);
	// ② http(s) 开头 → 图片直链(先过 httpRender,支持 {uid} 等变量占位);
	// ③ 其余 → 按响应 JSON 路径取直链(如 data.pic、list[0].url)。合法才设为配图,
	// 取不到自动退化纯文案(发送端 safeImage 还会再校验)
	if ip := strings.TrimSpace(cfg["image_path"]); ip != "" {
		if strings.HasPrefix(ip, "file:") {
			res.Image = ip
		} else if strings.HasPrefix(ip, "http://") || strings.HasPrefix(ip, "https://") {
			if u := strings.TrimSpace(httpRender(ip, ctx, parsed)); validTgURL(u) {
				res.Image = u
			}
		} else if s, ok := jsonPath(parsed, strings.Trim(ip, "{}")); ok && validTgURL(s) {
			// 兼容用户写成 {pci} 占位符风格:去掉外层花括号再按路径取
			res.Image = s
		}
	}

	// 配了 list_path 且解析出数组 → 每项一个按钮
	if lp := strings.TrimSpace(cfg["list_path"]); lp != "" {
		if v, ok := jsonValue(parsed, lp); ok {
			if arr, ok := v.([]interface{}); ok {
				for _, item := range arr {
					btn := TgButton{Text: strings.TrimSpace(httpRender(cfg["btn_text"], ctx, item))}
					if btn.Text == "" {
						continue
					}
					if bu := strings.TrimSpace(httpRender(cfg["btn_url"], ctx, item)); bu != "" {
						btn.Url = bu
					} else if bc := strings.TrimSpace(httpRender(cfg["btn_cb"], ctx, item)); bc != "" {
						btn.Cb = bc
					}
					res.Buttons = append(res.Buttons, btn)
				}
			}
		}
	}

	if c := toInt(cfg["cols"]); c > 0 {
		res.Cols = c
	} else {
		res.Cols = 1
	}
	// http 结果也可套配图/富文本(读同一份 cfg):未取到图时回退 cfg["image"],format=html 则开富文本
	if res.Image == "" {
		res.Image = cfg["image"]
	}
	if cfg["format"] == "html" {
		res.HTML = true
	}
	return res
}

// httpFetch 执行一次对外请求并把响应解析为 JSON。入参为已渲染好的最终值。
// 任何环节失败返回 (nil,false),由调用方决定回退策略。httpResult 与根横幅取数共用它。
func httpFetch(rawURL, method, headers, body string) (interface{}, bool) {
	var br io.Reader
	if method == http.MethodPost {
		br = strings.NewReader(body)
	}
	reqCtx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, method, rawURL, br)
	if err != nil {
		config.LogError("http fetch 建请求 %s: %v", rawURL, err)
		return nil, false
	}
	applyHTTPHeaders(req, headers)
	resp, err := tgHTTPClient.Do(req)
	if err != nil {
		config.LogError("http fetch 请求 %s: %v", rawURL, err)
		return nil, false
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		config.LogWarning("http fetch %s 非 2xx: %d", rawURL, resp.StatusCode)
		return nil, false
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 512*1024)) // 响应体上限 512KB
	if err != nil {
		config.LogError("http fetch 读体 %s: %v", rawURL, err)
		return nil, false
	}
	var parsed interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		config.LogWarning("http fetch %s 返回非 JSON: %v", rawURL, err)
		return nil, false
	}
	return parsed, true
}

// bannerAPIText 用根横幅的 api_* 配置调接口并渲染出引导语文本(只取文本,不涉 list_path 按钮/图)。
// 失败返回 ("",false),由调用方回退静态文本。url/body/text 均经 httpRender 支持 {uid}{first_name} 等变量。
func bannerAPIText(cfg map[string]string, u *TgUser) (string, bool) {
	ctx := httpCtx(u, parseArgs(cfg["api_args"]))
	rawURL := strings.TrimSpace(httpRenderURL(cfg["api_url"], ctx, nil))
	if rawURL == "" || !validTgURL(rawURL) {
		config.LogWarning("banner http: 非法或缺失接口地址 %q", cfg["api_url"])
		return "", false
	}
	method := strings.ToUpper(strings.TrimSpace(cfg["api_method"]))
	if method == "" {
		method = http.MethodGet
	}
	body := ""
	if method == http.MethodPost {
		body = httpRender(cfg["api_body"], ctx, nil)
	}
	parsed, ok := httpFetch(rawURL, method, cfg["api_headers"], body)
	if !ok {
		return "", false
	}
	text := strings.TrimSpace(httpRender(cfg["api_text"], ctx, parsed))
	if text == "" {
		return "", false
	}
	return text, true
}

// httpCtx 构造模板上下文:先放管理员自填 args,再注入内置时间变量,最后用内置用户变量覆盖(内置优先且可预期)。
func httpCtx(u *TgUser, args map[string]string) map[string]string {
	ctx := map[string]string{}
	for k, v := range args {
		ctx[k] = v
	}
	// 内置北京时间变量:用固定 UTC+8 时区,不依赖服务器本地时区、也不依赖接口返回。
	// 若接口自身返回了时间字段,用路径写法 {data.time} 取(不会被这里的 {time} 遮挡)。
	bj := time.FixedZone("CST", 8*3600)
	now := time.Now().In(bj)
	ctx["time"] = now.Format("2006-01-02 15:04:05")
	ctx["date"] = now.Format("2006-01-02")
	ctx["clock"] = now.Format("15:04:05")
	if u != nil {
		ctx["uid"] = strconv.FormatInt(u.TgUserID, 10)
		ctx["chat"] = strconv.FormatInt(u.ChatID, 10)
		ctx["lang"] = u.Lang
		ctx["bind"] = strconv.FormatInt(u.BindID, 10)
		ctx["username"] = u.Username
		ctx["first_name"] = u.FirstName
	}
	return ctx
}

// httpRender 把 tpl 里的 {key} 依次解析为:上下文变量 → resp 的 JSON 路径 → 空串。
// resp 传 nil 表示仅用上下文(如渲染 URL/请求体阶段)。
func httpRender(tpl string, ctx map[string]string, resp interface{}) string {
	if tpl == "" {
		return ""
	}
	return httpTokenRe.ReplaceAllStringFunc(tpl, func(m string) string {
		key := strings.TrimSpace(m[1 : len(m)-1])
		if v, ok := ctx[key]; ok {
			return v
		}
		if s, ok := jsonPath(resp, key); ok {
			return s
		}
		return ""
	})
}

// httpRenderURL 渲染「接口地址」模板:与 httpRender 同,但把每个替换进去的变量值做 URL 编码。
// 用于避免 {first_name}/{username} 等自由文本含空格/中文/&/# 时把整条 URL 撑坏——
// 否则 http.NewRequest 的 url.Parse 会报错、请求根本发不出去(表现为“暂时取不到数据”)。
// 纯 ASCII 常规值(如 uid、数字)编码后不变,零回归;仅对替换值编码,模板里的 ?&= 等结构字符不受影响。
func httpRenderURL(tpl string, ctx map[string]string, resp interface{}) string {
	if tpl == "" {
		return ""
	}
	return httpTokenRe.ReplaceAllStringFunc(tpl, func(m string) string {
		key := strings.TrimSpace(m[1 : len(m)-1])
		if v, ok := ctx[key]; ok {
			return url.QueryEscape(v)
		}
		if s, ok := jsonPath(resp, key); ok {
			return url.QueryEscape(s)
		}
		return ""
	})
}

// applyHTTPHeaders 解析每行 "Key: Value" 的请求头并设置到 req。
func applyHTTPHeaders(req *http.Request, headers string) {
	for _, line := range strings.Split(headers, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		i := strings.IndexByte(line, ':')
		if i <= 0 {
			continue
		}
		k := strings.TrimSpace(line[:i])
		v := strings.TrimSpace(line[i+1:])
		if k != "" {
			req.Header.Set(k, v)
		}
	}
}

// jsonPath 按路径取响应里的值并转成字符串(标量直取,对象/数组给紧凑 JSON)。
func jsonPath(root interface{}, path string) (string, bool) {
	v, ok := jsonValue(root, path)
	if !ok {
		return "", false
	}
	return jsonScalarStr(v), true
}

// jsonValue 按 "a.b[0].c" 形式逐段走 map/slice,返回原始值。
func jsonValue(root interface{}, path string) (interface{}, bool) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, false
	}
	cur := root
	for _, seg := range strings.Split(path, ".") {
		name := seg
		var idxs []int
		if i := strings.IndexByte(seg, '['); i >= 0 {
			name = seg[:i]
			rest := seg[i:]
			for strings.HasPrefix(rest, "[") {
				j := strings.IndexByte(rest, ']')
				if j < 0 {
					return nil, false
				}
				n, err := strconv.Atoi(rest[1:j])
				if err != nil {
					return nil, false
				}
				idxs = append(idxs, n)
				rest = rest[j+1:]
			}
		}
		if name != "" {
			m, ok := cur.(map[string]interface{})
			if !ok {
				return nil, false
			}
			v, ok := m[name]
			if !ok {
				return nil, false
			}
			cur = v
		}
		for _, n := range idxs {
			arr, ok := cur.([]interface{})
			if !ok || n < 0 || n >= len(arr) {
				return nil, false
			}
			cur = arr[n]
		}
	}
	return cur, true
}

// jsonScalarStr 把 JSON 值转成适合塞进文案的字符串。
func jsonScalarStr(v interface{}) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case bool:
		return strconv.FormatBool(t)
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	default:
		b, err := json.Marshal(t)
		if err != nil {
			return fmt.Sprintf("%v", t)
		}
		return string(b)
	}
}
