package controller

// ==================== 自建 Mock 调试接口(不鉴权,仅供联调) ====================
//
// 背景:后台 http / pick_user 菜单调试时要依赖第三方 mock 站(如 jsonplaceholder),
// 而它的写接口(POST)时常 500/404,不稳定。这里在自家后端挂两个稳定接口,
// 后台把「接口地址」填成 http://127.0.0.1:8200/api/test/xxx(部署后换成服务器地址)即可调试。
//
// 两个接口:
//   GET  /api/test/get    把收到的查询参数原样放进 data 回显,测 GET 取数 / {data.xxx} 路径
//   POST /api/test/post   把收到的 JSON 请求体原样放进 data 回显,测 pick_user + 输入金额 的 POST 流程
//
// 二者恒定返回 200(即使请求体不是合法 JSON 也照收不误),避免像第三方那样被「非 2xx」判失败。

import (
	"encoding/json"
	"io"

	"github.com/gin-gonic/gin"
)

// MockGet GET /api/test/get?k=v&...
// 与 POST 一致:把收到的查询参数原样放进 data 回显(不额外加字段),code/message 固定。
// 例:?name=张三&balance=100 → {"code":0,"message":"ok","data":{"name":"张三","balance":"100"}},
// 后台「显示文案」用 {data.name} {data.balance} 即可取到。需测数组/list_path 请用 POST(请求体可带数组)。
func MockGet(c *gin.Context) {
	data := gin.H{}
	for k, v := range c.Request.URL.Query() {
		if len(v) > 0 {
			data[k] = v[0] // 同名多值取第一个
		}
	}
	c.JSON(200, gin.H{
		"code":    0,
		"message": "ok",
		"data":    data,
	})
}

// MockPost POST /api/test/post
// 把收到的 JSON 请求体原样放进 data 回显(不额外加任何字段),这样后台「显示文案」里用
// {data.price} {data.payee} 等即可取到你 POST 过去的字段,方便端到端验证 pick_user 流程。
// 无论请求体是否合法 JSON 都返回 200(非法则把原文字符串当作 data 原样回传)。
func MockPost(c *gin.Context) {
	body, _ := io.ReadAll(c.Request.Body)
	var data interface{}
	if err := json.Unmarshal(body, &data); err != nil {
		data = string(body) // 非 JSON 则原样回传字符串
	}
	c.JSON(200, gin.H{
		"code":    0,
		"message": "ok",
		"data":    data,
	})
}
