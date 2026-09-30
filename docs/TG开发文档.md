# Telegram 灵活自定义菜单机器人 · 开发文档（初稿）

> 一句话:做一个 Telegram 机器人,**菜单和「点每个菜单返回什么」全部在后台可视化配置**,改配置不改代码、不重启;需要新的动态数据时,只写一个函数注册进白名单即可。
>
> 项目定位:**独立项目**(独立仓库、独立进程、独立数据库),不并进任何现有项目。
>
> 但**开发方式完全沿用 `D:\GoLand\shortplay` 的约定**:Go 后端照搬它的分层与响应/命名/DB 习惯;管理后台直接以它的 `web/admin`(Vue2 + Element)为模板另起一套。只是「照抄风格 + 复制样板」,不 import、不修改、不共用 shortplay 的代码或数据库。

---

## 1. 核心设计:菜单即数据

机器人本身不含任何写死的菜单。所有按钮、层级、点击后的行为都存在一张 `tg_menu` 表里,后台增删改这张表就等于改机器人。

「点某个菜单返回什么」由两个字段决定:

- `action_type`:动作类型,四种——`menu`(打开子菜单) / `text`(返回固定文案) / `url`(跳转链接) / `handler`(调动态数据)。
- `action_config`:JSON,按 `action_type` 解释,存该动作需要的参数。

| action_type | 点击行为 | action_config 示例 |
|---|---|---|
| `menu` | 渲染 `parent_id=本条id` 下所有启用按钮,形成子菜单页 | `{}` |
| `text` | 直接回一段固定文案(支持 Markdown) | `{"text":"客服时间 9:00-21:00"}` |
| `url` | 内联链接按钮,不产生回调 | `{"url":"https://你的域名"}` |
| `handler` | 调注册的 Go 函数拿数据,动态渲染文案+按钮 | `{"handler":"demo_list","args":"limit=5"}` |

这就是「灵活自定义」的落点:运营在后台选动作类型、填参数,就能拼出任意菜单树;`handler` 类型让菜单能返回实时数据。

---

## 2. 架构分层(照搬 shortplay 风格)

三层职责:

```
┌───────────────────────────────────────────────┐
│ 通道层  POST /api/tg/webhook (Gin handler)       │
│   收 Telegram 的 Update → 丢进队列 → 秒回 200     │
└───────────────────┬───────────────────────────┘
                    ▼
┌───────────────────────────────────────────────┐
│ 分发层  消费队列(goroutine)                       │
│   解析 callback_data → 查 tg_menu → 按 action_type │
│   派活;handler 走注册表 key→func                    │
└───────────────────┬───────────────────────────┘
                    ▼
┌───────────────────────────────────────────────┐
│ 后台层  /api/boss/tg/* CRUD (BossAuth) + admin    │
└───────────────────────────────────────────────┘
```

**Go 端刻意不过度分层**,与 shortplay 一致:

| 维度 | 约定(照抄 shortplay) |
|---|---|
| 入口 | `web.go` 里 `main()`:`gin.SetMode` → `route.SetupRouter()` → `router.Run(":"+config.Conf.GetString("server.webPort"))`;config/mysql/logger 靠各自 `config` 包的 `init()` 自动初始化 |
| 分层 | 无 `model/`、无 `service/`;DB 操作写在 controller 里,用 `map[string]interface{}` + 原生 SQL,GORM 只当执行器 |
| 路由 | 全集中在 `route/route.go` 的 `SetupRouter()`;后台组 `/api/boss` 挂 `middleware.BossAuth`,Bot 回调组 `/api/tg` 公开(用 secret token 头校验) |
| 响应 | 无封装,直接 `c.JSON(200, gin.H{"code":0,"message":"操作成功","data":...,"count":...})`;HTTP 恒 200,错误看 `code`,`code===0` 即成功,字段是 `message` 不是 msg |
| DB | GORM;全局 `config.Mysql *gorm.DB`,controller 构造注入 `db`;查询 `db.Raw(...).Scan(&[]map...)`,增改 `db.Table("x").Create/Updates(map)`,删除 `db.Exec("delete ... in (?)", tools.SplitIds(...))`;取单列用 `Pluck/Scan`(**禁 `Raw().Row().Scan(&int)`,会 nil panic**);`Scan` 进 map 的 `id` 是 `uint32`,用 `fmt.Sprintf("%v",...)` 归一化 |
| 时间 | 每表 `id/created_at/updated_at`,手工 `time.Now()` 赋值,查询后 `formatTimeFields` 统一格式化 |
| 命名 | 表前缀 `tg_`;字段 snake_case;接口大驼峰 `GetXxxList/AddXxx/SaveXxx/DelXxx`(删除走 GET + 逗号 `ids`) |
| 鉴权 | JWT(HS256)`tools.GenerateToken/ParseAuthorization`,后台 `middleware.BossAuth` 校验 `RoleAdmin` |
| 配置 | 基础设施放 `config.yaml`(viper,`config.Conf.GetString("...")`) |

> 落地:把 shortplay 里跟业务无关的通用文件(`config/` 的 viper 加载 + mysql + logger、`tools/` 的 jwt + SplitIds、`middleware/` 的 cors + bossAuth、controller 的 `pageLimit`/`formatTimeFields`)**复制**到新仓库改改即用。

---

## 3. 数据库(独立库)

手工建表(无 AutoMigrate)。前缀 `tg_`,snake_case,公共列 `id/created_at/updated_at`。

```sql
-- 菜单树:驱动一切
CREATE TABLE tg_menu (
  id            BIGINT PRIMARY KEY AUTO_INCREMENT,
  parent_id     BIGINT      NOT NULL DEFAULT 0,    -- 0=首页根菜单
  lang          VARCHAR(8)  NOT NULL DEFAULT 'all',-- all/en/es/zh 多语言预留
  title         VARCHAR(64) NOT NULL,              -- 按钮文字(可含 emoji)
  action_type   VARCHAR(16) NOT NULL DEFAULT 'menu',-- menu/text/url/handler
  action_config JSON        NULL,                  -- 见 §1
  cols          TINYINT     NOT NULL DEFAULT 2,    -- 该页每行几列(排版)
  sort          INT         NOT NULL DEFAULT 0,    -- 排序,小在前
  status        TINYINT     NOT NULL DEFAULT 1,    -- 1启用 0停用
  created_at    DATETIME    NULL,
  updated_at    DATETIME    NULL,
  KEY idx_parent (parent_id, status, sort)
);

-- 动态数据类白名单:后台配 handler 时只能从这里选,防手填错
CREATE TABLE tg_handler (
  id          BIGINT PRIMARY KEY AUTO_INCREMENT,
  handler_key VARCHAR(64)  NOT NULL,              -- 与代码 RegisterTg 的 key 一致
  name        VARCHAR(64)  NOT NULL,              -- 中文名
  param_hint  VARCHAR(255) NOT NULL DEFAULT '',   -- 参数提示,如 "limit=5"
  remark      VARCHAR(255) NOT NULL DEFAULT '',
  status      TINYINT      NOT NULL DEFAULT 1,
  created_at  DATETIME     NULL,
  updated_at  DATETIME     NULL,
  UNIQUE KEY uk_key (handler_key)
);

-- Telegram 用户
CREATE TABLE tg_user (
  id           BIGINT PRIMARY KEY AUTO_INCREMENT,
  tg_user_id   BIGINT      NOT NULL,              -- Telegram 用户 ID
  chat_id      BIGINT      NOT NULL,              -- 回消息用
  username     VARCHAR(64) NOT NULL DEFAULT '',
  first_name   VARCHAR(64) NOT NULL DEFAULT '',
  lang         VARCHAR(8)  NOT NULL DEFAULT 'en',
  bind_user_id BIGINT      NOT NULL DEFAULT 0,    -- 预留:绑定业务账号
  status       TINYINT     NOT NULL DEFAULT 1,
  created_at   DATETIME    NULL,
  updated_at   DATETIME    NULL,
  UNIQUE KEY uk_tg (tg_user_id)
);

-- 后台管理员(照 shortplay,login 用)
CREATE TABLE admin (
  id         BIGINT PRIMARY KEY AUTO_INCREMENT,
  account    VARCHAR(64) NOT NULL,
  password   VARCHAR(64) NOT NULL,   -- 初稿沿用明文比对;上线建议改 bcrypt
  status     TINYINT     NOT NULL DEFAULT 1,
  created_at DATETIME    NULL,
  updated_at DATETIME    NULL,
  UNIQUE KEY uk_account (account)
);

-- 命令菜单:驱动输入框旁那个原生「菜单」按钮(通过 setMyCommands 注册)
CREATE TABLE tg_command (
  id            BIGINT PRIMARY KEY AUTO_INCREMENT,
  command       VARCHAR(32) NOT NULL,             -- 不含斜杠、小写,如 start/help/support
  description   VARCHAR(64) NOT NULL,             -- 「菜单」按钮里显示的说明(如"主菜单")
  action_type   VARCHAR(16) NOT NULL DEFAULT 'menu',-- 复用 menu/text/url/handler
  action_config JSON        NULL,                 -- 复用 §1 的解释
  sort          INT         NOT NULL DEFAULT 0,
  status        TINYINT     NOT NULL DEFAULT 1,
  created_at    DATETIME    NULL,
  updated_at    DATETIME    NULL,
  UNIQUE KEY uk_cmd (command)
);
```

---

## 4. handler 注册表(加一类动态数据 = 写一个函数)

菜单里 `action_type=handler` 指向的「数据类」,就是这里注册的函数。写一个函数、注册一个 key,后台就能选它。

```go
// controller/tgBotHandler.go
type TgButton struct {
	Text string // 按钮文字
	Cb   string // callback_data(站内跳转)
	Url  string // 或外链
}
type TgResult struct {
	Text    string
	Buttons []TgButton
	Cols    int // <=0 用默认
}
type TgUser struct {
	TgUserID int64
	ChatID   int64
	Lang     string
	BindID   int64
}

// 数据处理器:拿 *gorm.DB、当前用户、参数,返回要渲染的内容(照 shortplay 用 GORM)
type TgHandlerFunc func(db *gorm.DB, u *TgUser, args map[string]string) (*TgResult, error)

var tgHandlers = map[string]TgHandlerFunc{}

func RegisterTg(key string, fn TgHandlerFunc) { tgHandlers[key] = fn }
func GetTgHandler(key string) (TgHandlerFunc, bool) {
	fn, ok := tgHandlers[key]
	return fn, ok
}
```

初稿放两个**通用占位 handler**(不依赖任何业务表,先跑通;将来换成真实查询):

```go
func init() {
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

	RegisterTg("item_detail", func(db *gorm.DB, u *TgUser, args map[string]string) (*TgResult, error) {
		return &TgResult{Text: "🔎 条目 #" + args["id"] + " 的详情占位"}, nil
	})
}
```

接真实业务时,把 mock 循环换成 `db.Raw("select ... from 你的表 where ... limit ?", limit).Scan(&list)`(list 是 `[]map[string]interface{}`),分发逻辑不动。

---

## 4.5 配置化「接口取数」(action_type=http,无需写代码)

§4 的 handler 要开发写 Go 函数、注册、重编译才能用。对"把外部接口的数据取来展示给用户"这类需求,这太重。于是新增一个**通用 http 引擎**(`controller/tgHTTP.go`):Go 里只实现一次,之后运营在后台**纯配置**就能加"点按钮→调接口→把返回 JSON 渲染成文字/按钮",**不写代码、不重编译**。

**后台怎么填**(菜单编辑弹窗「点击行为」选「接口取数(http)」):

| 字段 | 说明 |
|---|---|
| 请求方法 | GET / POST |
| 接口地址 | 支持模板变量,如 `https://api.x.com/{uid}/bal?lang={lang}` |
| 请求头 | 每行一条 `Key: Value`(如 `Authorization: Bearer xxx`) |
| 请求体 | POST 正文,可用模板变量 |
| 参数 | 额外模板变量 `k=v,k2=v2`,成为 `{k}` 供上面引用 |
| 显示文案 | 用 `{字段路径}` 取响应值,如 `你的余额 {data.balance} 元` |
| 列表路径 | ⚠️ 后端已支持、当前 UI 暂未开放:如 `data.items`;填了则数组每个元素生成一个按钮 |
| 按钮文字 / 按钮链接 | ⚠️ 同上、UI 暂未开放:以当前元素为根的模板,如 `{title}` / `{link}` |
| 每行按钮数 | ⚠️ 同上、UI 暂未开放:列表按钮排版列数 |

**可用模板变量**:`{uid}` `{chat}` `{lang}` `{bind}` `{username}` `{first_name}`(均来自当前 TG 用户)+「参数」里自定义的 k=v + **等待输入场景的 `{input}`**(见下 await-input)。渲染时 `{key}` 先查这些上下文,查不到再按响应 JSON 路径取(支持点号与下标,如 `{data.list[0].name}`);都没有则替换为空串。

**await-input(先问用户要输入)**:配了 `input_prompt` 后,用户点该菜单不会立即调接口,而是先收到这句提示语(以 ForceReply 新消息发出、高亮输入框),并登记一个内存态「等待输入」(默认 5 分钟超时)。用户下一条普通文本会被捕获,以变量 `{input}` 代入 url/body/text 后再发请求(后台 UI 固定用 `input`;`input_key` 键后端仍支持,需自定义变量名时直接写进库即可)。

**action_config 结构**(必须扁平 string→string,兼容 `parseConfig`):`{method,url,headers,body,args,text,image_path,input_prompt,input_key,list_path,btn_text,btn_url,cols}`。

**执行链路**:菜单里 http 按钮回调仍是 `h:{菜单id}`(与 handler 同域,`runHandlerByMenu` 回查该行后转 `menuHTTPResult`)→若配了 `input_prompt` 且本次无输入→登记等待态+回提示语(ForceReply 新消息);否则→`httpResult` 渲染 URL/请求体(含已注入的 `{input}` 变量)→ 发请求 → 解析 JSON → 按 text/image_path/list_path 组装 `TgResult` → 展示。用户输入的文本由 `onMessage` 普通文本分支捕获后重新走同一条链路。

**安全边界**:仅允许 http/https;独立 `http.Client` 超时 8s(不复用长轮询 client);响应体上限 512KB;非 2xx / 非 JSON / 请求失败一律回友好文案"⚠️ 暂时取不到数据"且不崩(onCallback 自动补返回按钮)。URL 由后台(已过 BossAuth)管理员配置,SSRF 视为自管;未来可加 `telegram.httpAllowHosts` 白名单。首期不含本库查询(db_query)。

**配置示例**(后台 http 配置块顶部有「GET · 查余额」「POST · 查资料」两个一键填入按钮,点一下自动填好下表,再照改;当前 UI 只做纯文案展示):

*示例一 · GET 取单值(个性化余额)*——接口返回 `{"data":{"balance":128.5}}`:

| 字段 | 值 |
|---|---|
| 请求方法 | GET |
| 接口地址 | `https://api.example.com/user/{uid}/balance` |
| 请求头 | `Authorization: Bearer your-api-token` |
| 显示文案 | `💰 你好 {first_name}，当前余额 {data.balance} 元` |

*示例二 · POST 取单值(提交参数拿文案)*——接口返回 `{"data":{"level":"VIP3","points":820}}`:

| 字段 | 值 |
|---|---|
| 请求方法 | POST |
| 接口地址 | `https://api.example.com/user/{uid}/profile` |
| 请求头 | `Content-Type: application/json` |
| 请求体 | `{"lang":"{lang}"}` |
| 显示文案 | `👤 你好 {first_name}，当前等级 {data.level}，积分 {data.points}` |

---

## 4.6 富文本(HTML)与配图(图片+文案+按钮)

菜单/命令的文案支持两样新能力,均由 `action_config` 的两个扁平键控制(仍满足 string→string):

- **`format` = `"html"`**:该条文案按 Telegram `parse_mode=HTML` 渲染,可写 `<b>粗</b>` `<i>斜</i>` `<a href="url">文字链</a>` `<code>` 等。默认空=纯文本(存量数据不受影响)。
- **`image` = 图片直链 URL**:非空则这条消息以 **`sendPhoto`/`editMessageMedia`** 发送,呈现「图片在上、文案(caption)在中、按钮在下」——即常见 bot 的图文菜单形态。

**UI**:「显示文字(text)」与「菜单导航(menu)」配置块各有「配图URL」输入 + 「富文本」开关;menu 页另可填「页面引导语」当 caption(默认「请选择：」)。

**关键实现与坑**(`controller/tgBotRuntime.go`):
- `TgResult` 新增 `Image string` / `HTML bool`;`send`/`editMessage` 据 `Image` 决定发文本还是图片、据 `HTML` 决定 `ParseMode`。
- **HTML 失败自动回退**:开了 HTML 若因文案里有非法标签导致 Telegram 报错,`send`/`editMessage` 会去掉 `parse_mode` 再发一次,避免整条消息发不出去。
- **图文互转限制**:Telegram 的 `editMessageMedia` 只能改「本就是媒体」的消息,`editMessageText` 改带图消息会残留旧图。故 `editMessage` 按「目标形态 vs 当前消息形态(`messageHasMedia`)」分派:一致→原地编辑;不一致→**删旧消息再发新消息**(视觉正确,位置会跳到底部)。
- 配图 URL 先过 `validTgURL` 校验,非法则告警并退化为纯文本。
- caption 上限 1024 字(纯文本消息 4096),超长需自行拆分。

---

## 5. callback_data 约定(关键)

Telegram 的 `callback_data` 上限 **64 字节**,只放「域:ID」,业务数据一律回查 DB。

| 前缀 | 含义 | 例 |
|---|---|---|
| `m:<menuId>` | 打开该菜单作为父页的子菜单 | `m:5` |
| `h:<menuId>` | 触发该菜单配置的 handler | `h:8` |
| `<域>:<id>` | handler 的二级动作 | `item:3` |

---

## 5.5 命令菜单(输入框旁的「菜单」按钮)

注意区分两种「菜单」:

- **消息里的九宫格**:是内联键盘(`InlineKeyboardMarkup`),由 `tg_menu` 驱动,点 `/start` 后机器人发消息画出来(§1、§6)。
- **输入框左下角的「菜单」按钮**(截图那个):是 **Telegram 客户端原生**功能,机器人不画它。客户端根据机器人**注册的命令列表**自动显示这个按钮,点开就是 `/start 主菜单`、`/help 帮助`、`/support 客服` 这样的命令 + 说明。命令列表由 Bot API 的 `setMyCommands` 设置(也可在 BotFather `/setcommands` 手填,但我们走后台配置)。

所以「菜单」按钮里显示哪些命令、每条的中文说明,都来自 `tg_command` 表。做法:启动时把表里的启用命令读出来调 `setMyCommands` 注册;后台增删改命令后,再同步一次即可刷新按钮内容。命令被点击时,Telegram 会把 `/xxx` 当普通消息发给机器人,在 `onMessage` 里按命令查 `tg_command`,复用同一套 `action_type` 执行引擎。

```go
// 启动时 + 后台改命令后调用:把 tg_command 注册成原生「菜单」按钮内容
func (r *TgBotRuntime) syncCommands() {
	list := make([]map[string]interface{}, 0)
	r.db.Raw("select command, description from tg_command where status = 1 order by sort").Scan(&list)
	cmds := make([]tg.BotCommand, 0, len(list))
	for _, m := range list {
		cmds = append(cmds, tg.BotCommand{
			Command:     fmt.Sprintf("%v", m["command"]),     // 小写、不含斜杠
			Description: fmt.Sprintf("%v", m["description"]), // 「菜单」按钮里显示的说明
		})
	}
	_, _ = r.bot.Request(tg.NewSetMyCommands(cmds))
}

// SyncCommands 后台「同步命令」接口:改完 tg_command 后刷新原生「菜单」按钮
func (r *TgBotRuntime) SyncCommands(c *gin.Context) {
	r.syncCommands()
	c.JSON(200, gin.H{"code": 0, "message": "命令已同步"})
}

// 收到消息:是命令就按 tg_command 分发,否则忽略(初稿)
func (r *TgBotRuntime) onMessage(up *tg.Update) {
	m := up.Message
	u := r.ensureUser(m.From, m.Chat.ID)
	if !strings.HasPrefix(m.Text, "/") {
		return // 普通文本:初稿忽略,后续可接关键词回复
	}
	cmd := strings.Fields(m.Text)[0]           // "/start 额外参数" → "/start"
	if i := strings.Index(cmd, "@"); i >= 0 {  // 群里常见 /start@yourbot
		cmd = cmd[:i]
	}
	row := r.loadCommand(cmd) // 查 tg_command:command = 去掉斜杠的 cmd
	if row == nil {
		if cmd == "/start" { // 兜底:没配 start 也渲染根菜单,保证能用
			text, kb := r.menuPage(0, u.Lang)
			r.send(u.ChatID, text, kb)
		}
		return
	}
	res := r.execAction(row, u) // 复用 menu/text/url/handler 执行引擎
	if res == nil {
		return
	}
	r.send(u.ChatID, res.Text, buildKeyboard(res.Buttons, res.Cols))
}
```

`execAction(row, u)` 是命令和按钮**共用**的动作执行器:按 `action_type` 分派——`menu` 渲染该菜单页、`text` 直接返回文案、`handler` 调注册表、`url` 不产生消息(仅内联按钮用)。`onCallback` 也改成走 `execAction`,这样「点按钮」和「点命令」行为完全一致、都由后台配置决定。

> 命令名限制:小写字母/数字/下划线,不含斜杠,全局唯一;说明文字别太长。`setMyCommands` 是**全局**设置(对所有用户一致),改完 Telegram 有缓存,通常几秒内刷新。

---

## 6. 后端落地(文件清单 + 关键代码)

新建独立仓库,目录照 shortplay:

```
tgbot/
├── go.mod                     # module tgbot
├── web.go                     # 入口 main()
├── config.yaml                # server/mysql/jwt/telegram
├── config/                    # viper 加载、mysql(config.Mysql)、logger(config.LogXxx)
├── route/route.go             # SetupRouter()
├── middleware/                # cors.go、bossAuth.go
├── tools/                     # jwt.go、function.go(SplitIds 等)
├── controller/
│   ├── bossController.go      # 后台登录 + pageLimit/formatTimeFields 等公共函数
│   ├── tgController.go        # 后台 CRUD(tg_menu/tg_handler/tg_user)
│   ├── tgBotHandler.go        # handler 注册表 + 示例 handler
│   └── tgBotRuntime.go        # 通道层 webhook + 分发层 + Telegram 调用
└── web/admin/                 # 以 shortplay web/admin 为模板另起
```

### 6.1 config.yaml

```yaml
# ===========================
# 服务器端口配置
# ===========================
server:
  # Web服务端口
  webPort: 8200            # 与 shortplay 8100 区分,避免本机端口撞
  # 限流队列容量
  queueCapacity: 1000
  # 限流突发容量
  suddenCapacity: 2000

# ===========================
# MySQL数据库配置
# ===========================
mysql:
  # 数据库地址
  address: "82.158.225.190:3306"
  # 数据库名称
  database: "tgbot"
  # 数据库用户
  user: "tgbot"
  # 数据库密码
  password: "@Tt37233535"   # ⚠️ 明文仅供开发,正式上线前务必改密码并从仓库/文档移除

# ===========================
# JWT登录鉴权配置
# ===========================
jwt:
  # 签名密钥,务必修改为随机长字符串并妥善保管
  secretKey: "改成随机64位hex"
  # token有效期(小时)
  expireHours: 24

# ===========================
# Redis配置(初稿暂未使用,保留与 shortplay 一致的结构)
# ===========================
redis:
  # Redis服务器地址
  address: "localhost:6379"
  # 密码,没有则为空
  password: ""
  # 默认数据库
  db: 0

# ===========================
# Telegram 机器人配置
# ===========================
telegram:
  botToken: "123456:AAA..."      # BotFather 拿的 token,待填
  webhookPath: "/api/tg/webhook"
  secretToken: "改成随机长串"      # 校验回调来源,防伪造
  publicBase: "https://你的域名"    # 注册 webhook 用的公网 HTTPS 域名
```

### 6.2 route/route.go

```go
func SetupRouter() *gin.Engine {
	router := gin.New()
	router.Use(gin.Logger(), gin.Recovery(), middleware.Cors())

	// Bot 运行时先建(供后台「同步命令」接口调用)
	tgBot := controller.NewTgBotRuntime()

	boss := router.Group("/api/boss", middleware.BossAuth)
	{
		bossController := controller.NewBossController()
		boss.POST("/login", bossController.AdminLogin) // login 在 BossAuth 白名单里放行

		tg := controller.NewTgController()
		boss.GET("/GetTgMenuList", tg.GetTgMenuList)
		boss.POST("/AddTgMenu", tg.AddTgMenu)
		boss.POST("/SaveTgMenu", tg.SaveTgMenu)
		boss.GET("/DelTgMenu", tg.DelTgMenu)
		boss.GET("/GetTgHandlerList", tg.GetTgHandlerList)
		boss.GET("/GetTgUserList", tg.GetTgUserList)
		// 命令菜单(原生「菜单」按钮)CRUD
		boss.GET("/GetTgCommandList", tg.GetTgCommandList)
		boss.POST("/AddTgCommand", tg.AddTgCommand)
		boss.POST("/SaveTgCommand", tg.SaveTgCommand)
		boss.GET("/DelTgCommand", tg.DelTgCommand)
		boss.POST("/SyncTgCommand", func(c *gin.Context) { // 改完命令后刷新按钮
			tgBot.SyncCommands(c)
		})
	}

	// Telegram 回调:公开组,不挂 BossAuth,用 secret token 头校验
	router.POST("/api/tg/webhook", tgBot.Webhook)

	return router
}
```

### 6.3 通道层 + 分发层(tgBotRuntime.go)

```go
type TgBotRuntime struct {
	db    *gorm.DB
	bot   *tg.BotAPI
	queue chan *tg.Update
}

func NewTgBotRuntime() *TgBotRuntime {
	bot, _ := tg.NewBotAPI(config.Conf.GetString("telegram.botToken"))
	r := &TgBotRuntime{db: config.Mysql, bot: bot, queue: make(chan *tg.Update, 1000)}
	go r.consume()     // 启动分发循环
	r.syncCommands()   // 启动时注册原生「菜单」按钮内容(见 §5.5)
	return r
}

// 通道层:Telegram POST 过来,校验来源、入队、秒回 200(否则会被重推)
func (r *TgBotRuntime) Webhook(c *gin.Context) {
	if c.GetHeader("X-Telegram-Bot-Api-Secret-Token") !=
		config.Conf.GetString("telegram.secretToken") {
		c.AbortWithStatus(403)
		return
	}
	var up tg.Update
	if err := c.ShouldBindJSON(&up); err != nil {
		c.JSON(200, gin.H{"code": 400, "message": err.Error()})
		return
	}
	select {
	case r.queue <- &up:
	default: // 队列满丢弃,别阻塞
		config.LogWarning("tg queue full, drop %d", up.UpdateID)
	}
	c.JSON(200, gin.H{"code": 0, "message": "ok"}) // 恒 200
}

func (r *TgBotRuntime) consume() {
	for up := range r.queue {
		switch {
		case up.Message != nil:
			r.onMessage(up)
		case up.CallbackQuery != nil:
			r.onCallback(up.CallbackQuery)
		}
	}
}

// onMessage 见 §5.5(命令感知版:先判命令走 tg_command 分发,再兜底渲染根菜单)

// 点按钮 → 按 action_type 派活
func (r *TgBotRuntime) onCallback(q *tg.CallbackQuery) {
	u := r.ensureUser(q.From, q.Message.Chat.ID)
	domain, arg := splitCb(q.Data)
	var res *TgResult

	switch domain {
	case "m":
		res = r.menuPageResult(arg, u.Lang)   // 子菜单
	case "h":
		res = r.runHandlerByMenu(arg, u)      // 菜单配的 handler
	case "item":
		res = r.run("item_detail", u, map[string]string{"id": arg})
	default:
		r.answer(q, "未知操作")
		return
	}
	if res == nil {
		r.answer(q, "暂无数据")
		return
	}
	// 原地编辑消息实现「页面跳转」,不刷新高消息
	edit := tg.NewEditMessageText(q.Message.Chat.ID, q.Message.ID, res.Text)
	edit.ReplyMarkup = buildKeyboard(res.Buttons, res.Cols)
	if _, err := r.bot.Request(edit); err != nil {
		config.LogWarning("edit msg: %v", err)
	}
	r.answer(q, "") // 必须应答 callback,否则按钮转圈
}

func (r *TgBotRuntime) run(key string, u *TgUser, args map[string]string) *TgResult {
	fn, ok := GetTgHandler(key)
	if !ok {
		return nil
	}
	res, err := fn(r.db, u, args)
	if err != nil {
		config.LogError("handler %s: %v", key, err)
		return nil
	}
	return res
}

func (r *TgBotRuntime) answer(q *tg.CallbackQuery, text string) {
	a := tg.NewCallback(q.ID)
	if text != "" {
		a.Text = text
	}
	r.bot.Request(a)
}
```

`menuPage` / `menuPageResult`:查 `tg_menu where parent_id=? and status=1 order by sort`,把每条转成按钮(`menu`→`cb="m:<id>"`,`handler`→`cb="h:<id>"`,`url`→`Url`,`text`→指向返回文案的动作),返回文案+键盘。`runHandlerByMenu`:查该菜单 `action_config.handler` 再 `run`。`buildKeyboard`:按 `cols` 排成多行内联键盘。`ensureUser`:按 `tg_user_id` 查不到就 `Create` 落库。

### 6.4 后台 CRUD(tgController.go,照 bossController 写法)

```go
type TgController struct{ db *gorm.DB }
func NewTgController() *TgController { return &TgController{db: config.Mysql} }

// GetTgMenuList 菜单列表(按 parent_id 过滤)
func (tc *TgController) GetTgMenuList(c *gin.Context) {
	data := make([]map[string]interface{}, 0)
	err := tc.db.Raw(
		"select id,parent_id,lang,title,action_type,action_config,cols,sort,status,created_at,updated_at"+
			" from tg_menu where parent_id = ? order by sort asc, id desc", c.Query("parent_id")).Scan(&data).Error
	if err != nil {
		c.JSON(200, gin.H{"code": 500, "message": err.Error()})
		return
	}
	formatTimeFields(data)
	c.JSON(200, gin.H{"code": 0, "message": "操作成功", "data": data})
}

// AddTgMenu 新增
func (tc *TgController) AddTgMenu(c *gin.Context) {
	data := make(map[string]interface{})
	_ = c.BindJSON(&data)
	delete(data, "id")
	data["created_at"] = time.Now()
	data["updated_at"] = time.Now()
	if err := tc.db.Table("tg_menu").Create(data).Error; err != nil {
		c.JSON(200, gin.H{"code": 500, "message": err.Error()})
		return
	}
	c.JSON(200, gin.H{"code": 0, "message": "操作成功"})
}

// SaveTgMenu 修改
func (tc *TgController) SaveTgMenu(c *gin.Context) {
	data := make(map[string]interface{})
	_ = c.BindJSON(&data)
	id := data["id"]
	delete(data, "id"); delete(data, "created_at")
	data["updated_at"] = time.Now()
	if err := tc.db.Table("tg_menu").Where("id = ?", id).Updates(data).Error; err != nil {
		c.JSON(200, gin.H{"code": 500, "message": err.Error()})
		return
	}
	c.JSON(200, gin.H{"code": 0, "message": "操作成功"})
}

// DelTgMenu 删除(逗号 ids)
func (tc *TgController) DelTgMenu(c *gin.Context) {
	if err := tc.db.Exec("delete from tg_menu where id in (?)", tools.SplitIds(c.Query("ids"))).Error; err != nil {
		c.JSON(200, gin.H{"code": 500, "message": err.Error()})
		return
	}
	c.JSON(200, gin.H{"code": 0, "message": "操作成功"})
}
```

`action_config` 是 JSON 列,前端传字符串即可入库;读出来是字符串,分发层用 `json.Unmarshal` 解析成 `map[string]string`。

#### 动作类型配置速查表(后台“机器人菜单/命令菜单”直接对照着填)

| action_type | action_config 写法 | 触发后行为 | 备注 |
| --- | --- | --- | --- |
| **menu** | `{}` 或 `{"page":"root"}` | 展开“本按钮自身 id”下的子菜单;`page:"root"` 则回首页根菜单 | 作命令时配 `{"page":"root"}` 即渲染主菜单(如 /start);用作“返回主菜单”也靠它 |
| **text** | `{"text":"要发的文字"}` | 直接回一段纯文本(无按钮) | 换行用 `\n`;action_config 必须是合法 JSON |
| **url** | `{"url":"https://..."}` | 按钮为外链,由 Telegram 直接打开(不产生回调) | 当命令触发时会回一条带“🔗 打开链接”按钮的消息;URL 主机名必须 ASCII 合法(中文域名会被跳过) |
| **handler** | `{"handler":"demo_list","args":"limit=5"}` | 调用代码注册表里的数据处理器,返回动态文案+按钮 | `handler` 需在“数据类”白名单(tg_handler)且代码 `RegisterTg` 注册过同名 key;`args` 可选,逗号分隔 `k=v` |

> 现成参考:后台“机器人菜单”有一个 **🧪 动作类型示例** 容器(id=32),子按钮 33–38 分别演示 text/url/handler/menu(含二级页与返回主菜单);“命令菜单”的 `/demo` 演示 handler 类型命令。

---

## 7. 后台管理页(以 shortplay web/admin 为模板)

把 shortplay 的 `web/admin` 整个复制成新项目,清掉 drama/chapter/category 业务页,保留 `api/request.js`、`layout/`、`views/login.vue`、`views/setting.vue` 骨架,再按它的 CRUD 三件套加机器人菜单页:

```
web/admin/src/api/tgMenu.js          # 仿 api/drama.js
web/admin/src/views/tgMenu/index.vue # 仿 views/category/index.vue(列表+dialog编辑)
web/admin/src/router/index.js        # children 加一条
web/admin/src/layout/sidebar.vue     # menuList 加一项
```

### 7.1 api/tgMenu.js

```js
import request from './request'
export function getTgMenuList(params) { return request.get('/GetTgMenuList', { params }) }
export function addTgMenu(data)      { return request.post('/AddTgMenu', data) }
export function saveTgMenu(data)     { return request.post('/SaveTgMenu', data) }
export function delTgMenu(ids)       { return request.get('/DelTgMenu', { params: { ids } }) }
export function getTgHandlerList()   { return request.get('/GetTgHandlerList') }
// 命令菜单(原生「菜单」按钮)
export function getTgCommandList()   { return request.get('/GetTgCommandList') }
export function addTgCommand(data)   { return request.post('/AddTgCommand', data) }
export function saveTgCommand(data)  { return request.post('/SaveTgCommand', data) }
export function delTgCommand(ids)    { return request.get('/DelTgCommand', { params: { ids } }) }
export function syncTgCommand()      { return request.post('/SyncTgCommand') }
```

### 7.2 router/index.js(children 内加)

```js
{ path: 'tgMenu', name: 'tgMenu',
  component: () => import('@/views/tgMenu/index'),
  meta: { title: '机器人菜单' } },
```

### 7.3 sidebar.vue(menuList 加一项)

```js
{ path: '/tgMenu', title: '机器人', icon: 'el-icon-chat-dot-round' },
```

### 7.4 views/tgMenu/index.vue

照 `category/index.vue` 结构:搜索栏(按 title 过滤)+ 工具栏(添加/删除)+ `el-table`(列:ID、父菜单、标题、action_type、排序、状态开关、操作)+ 分页 + `el-dialog` 编辑表单。核心是**选不同 `action_type` 显示不同配置项**——这就是「自定义返回数据」的入口:

```vue
<el-form-item label="动作类型">
  <el-select v-model="form.action_type">
    <el-option label="子菜单"   value="menu"/>
    <el-option label="固定文案" value="text"/>
    <el-option label="链接按钮" value="url"/>
    <el-option label="动态数据" value="handler"/>
  </el-select>
</el-form-item>

<!-- 动态数据:从 tg_handler 白名单选,填参数 -->
<template v-if="form.action_type==='handler'">
  <el-form-item label="数据类">
    <el-select v-model="cfg.handler">
      <el-option v-for="h in handlerList" :key="h.handler_key" :label="h.name" :value="h.handler_key"/>
    </el-select>
  </el-form-item>
  <el-form-item label="参数"><el-input v-model="cfg.args" placeholder="如 limit=5"/></el-form-item>
</template>

<el-form-item v-if="form.action_type==='text'" label="文案">
  <el-input type="textarea" v-model="cfg.text"/>
</el-form-item>
<el-form-item v-if="form.action_type==='url'" label="链接">
  <el-input v-model="cfg.url"/>
</el-form-item>
```

保存时把 `cfg` 序列化成 JSON 存进 `action_config`;`parent_id` 用下拉选已有菜单实现层级。列表页支持按父菜单筛选、排序、启停开关。

### 7.5 命令管理页(原生「菜单」按钮)

同样按三件套再加一个 `views/tgCommand/index.vue`,管理 `tg_command` 表:列表列(命令名、说明、action_type、排序、状态、操作)+ 编辑弹窗(命令名、说明、动作类型/动作配置——复用 §7.4 那套「选动作类型显示不同配置项」)。工具栏放一个**「同步到 Telegram」按钮**,点击调 `syncTgCommand()`(即 `POST /SyncTgCommand`),把当前启用命令推到 `setMyCommands`,输入框旁的「菜单」按钮随即刷新。

> 建议:菜单编辑页保存后自动顺带调一次同步,省得运营忘记点按钮。

### 7.6 后台页面与 CRUD 统一规范(务必遵守,避免返工)

**所有后台列表页风格必须一致、增删改查功能齐全。** 新增任何管理页(如后续的 tg_user 用户页)都照此办理,不要各页各写一套。

前端页面结构(`web/admin/src/views/xxx/index.vue`)统一为三段式,顺序与写法不得随意变:

1. **`el-card` 的 `slot="header"` 一律放「搜索栏」**——`<div class="queryForm">` 内套 `<el-form :inline="true" :model="where" class="query-form-inline" size="small">`,含查询字段 `el-input`(`class="queryElInput"`) + 查询/重置按钮。
   - ⚠️ **反面教训(曾犯错)**:header 里只放一行标题文字(如「命令菜单」+提示语),会和别的页面(搜索栏有固定行高)对不齐,导致各页 header 高度不一致。**header 禁止只放标题文字。**
2. **header 下方 `.toolbar` 放操作按钮**:`添加`(type=primary)/`删除`(type=danger)/其它动作(如 `同步到 Telegram`)。提示性文案放工具栏行尾的 `span`,**不要塞进 header**。
3. **再下方 `el-table`**(`height="calc(100vh - 210px)")` + `el-pagination`,列布局各页保持一致。
4. **每页都要有 添加 / 修改(编辑)/ 删除 全套功能**;涉及 `action_type` 的,编辑弹窗按其动态显示配置项(见 §7.4)。

后端 CRUD(`controller/tgController.go`)统一约定:

- 命名 `GetXxxList / AddXxx / SaveXxx / DelXxx?ids=`;GORM 注入 + `map` + 原生 SQL;HTTP 恒 200,返回 `{code, message, data, count}`。
- 复用 `pageLimit / formatTimeFields / SplitIds / onlyFields`(列白名单过滤脏键)。
- `JSON` 列查询必须 `cast(xxx as char) as xxx`,否则 GORM 扫进 `map` 会变 `[]byte` 被 gin 转 base64,前端 `JSON.parse` 失败。
- 列表接口按需支持关键字过滤:`conds := "1 = 1"` 起,拼 `and xxx like '%kw%'`,同一 `where` 用于数据查询和 `count`。

> 改前端源码后需重新构建才生效:`npm run serve`(8080,热更新)或 `npm run build`(发布 dist)。

### 7.7 后台使用说明(每个功能都有示例数据,照着改就行)

> 登录:浏览器开后台地址,账号 `admin` + 密码(见附录 A / 已手动设置)。登录后左侧有两个功能页:**机器人菜单**、**命令菜单**。
> 下面每个功能都对应库里已造好的示例行,直接在页面上看/改它们即可上手。

#### A. 机器人菜单页(管 `tg_menu`——用户在 TG 里看到的按钮)

**列表默认只显示根菜单(上级ID=0)。** 想看某个按钮下面的子按钮,点该行右侧的**「下钻」**;点工具栏**「返回根菜单」**回到顶层。

| 功能/字段 | 怎么用 | 对应示例行 |
| --- | --- | --- |
| 搜索 | 顶部输「按钮名」关键字 → 查询/重置 | 任意 |
| 下钻 / 返回根 | 进入某按钮的子菜单层级 / 回到上级ID=0 | 下钻 id=32「🧪 动作类型示例」看四类子按钮 |
| 添加 | 点「添加」→ 弹窗填字段(见下)→ 确定 | —— |
| 编辑 | 点某行「编辑」改任意字段 | —— |
| 删除 | 勾选行 → 点「删除」(**会连带删除其子按钮**) | —— |
| 状态开关 | 列表里直接拨动即启用/停用;停用(status=0)的按钮**不会出现在机器人里** | id=43「🔒 已停用示例」 |
| 上级菜单ID | 0=挂在根菜单下;填某父按钮的 ID=做它的子按钮(先「下钻」到某层再点添加会自动带上当前层级) | id=33~38 上级=32 |
| 按钮名 | TG 上显示的按钮文字,可加 emoji | —— |
| 语言(lang) | `all`=所有人可见;`zh`/`en`/…=**仅对应 Telegram 客户端语言**的用户才看得到 | id=40(all)/41(zh)/42(en) |
| 动作类型 | 核心,4 选 1(见下) | id=33/34/35/36 |
| 每行按钮数(cols) | 一排显示几个按钮,1~6 | 根菜单=2、示例区=1 |
| 排序 | 数字小的排前面 | —— |

**四种动作类型(编辑弹窗选不同值会显示不同配置项):**
- **menu(打开子菜单)**:点一下进入下一级按钮。跳转目标选「本节点的子菜单」=往下钻;选「返回根主菜单」=做「⬅️ 返回」按钮。示例:id=32(进子菜单)、id=38(返回根)。
- **text(固定文案)**:点一下原地显示一段固定文字(无按钮)。示例:id=33。
- **url(内链按钮)**:一个外链按钮,点了直接打开网页(不产生回复)。示例:id=34。
- **handler(动态数据)**:点了调用「数据处理器」返回动态内容(列表/详情等);下拉选数据类 + 选填参数如 `limit=5`。示例:id=35。

#### B. 命令菜单页(管 `tg_command`——输入框左侧原生「菜单」里的斜杠命令)

| 功能 | 怎么用 | 对应示例行 |
| --- | --- | --- |
| 搜索 | 顶部输「命令名」→ 查询/重置 | —— |
| 添加/编辑/删除 | 同常规;命令名**小写、不含斜杠**(如 `start`),说明是「菜单」里显示的文字 | 见下四类 |
| 动作类型 | 与 A 页同样 4 种 | `/start`=menu、`/help`·`/support`=text、`/alice`=url、`/demo`=handler |
| **同步到 Telegram** | **改完命令必须点它**,才会把启用命令推到手机「菜单」按钮 | —— |
| 状态开关 | 停用(status=0)的命令不会进原生菜单 | —— |

> 说明:菜单类示例(机器人菜单页)是实时读库的,**无需同步、无需重启**;只有**命令**新增/改动才需点「同步到 Telegram」。

#### C. 「数据处理器(handler)」从哪来?

下拉里的 `demo_list`/`item_detail` 是代码内置的占位处理器(登记在 `tg_handler` 表)。要做**真实业务数据**(订单/商品等),需开发在 `controller/tgBotHandler.go` 用 `RegisterTg("你的key", fn)` 注册 + `tg_handler` 加一行,后台下拉才会出现该选项(见「附:新增一类返回数据的标准动作」)。

#### D. 快速上手三步

1. 机器人菜单页 → 对 id=32「🧪 动作类型示例」点**下钻**,看父子结构;再对它子项逐个点**编辑**,感受四种动作类型的配置差异。
2. 改一条 text 的文案或 url 的链接保存。
3. 打开 Telegram 给自己(或机器人)发 `/start` → 点「🧪 动作类型示例」→ 逐一体验 text/url/handler/menu;发 `/demo` 体验 handler 命令(需先同步)。

---

## 8. Telegram 硬限制与坑

- **导航用 `EditMessageText` 不用 `Send`**:点按钮刷新当前消息,否则点几次刷几条。
- **每个 callback 必须 `AnswerCallbackQuery`**:几秒内应答,否则按钮转圈。
- **Webhook 秒回 200**:耗时逻辑丢队列/goroutine,别在 handler 里同步查库/调第三方。
- **webhook 只能 443/80/8443/8080,且必须公网 HTTPS**。
- **网络可达性**:服务器要能出站访问 `api.telegram.org`(境内直连不通,需代理或部署到境外)。
- **webhook 与 getUpdates 互斥**:本地先用 getUpdates 验证,上线切 webhook 前先 `DeleteWebhook`/停轮询。

---

## 9. 跑通步骤

阶段 0 · 本地验证 token:用 getUpdates 长轮询(临时替换 `consume` 的取数入口),给自己发 `/start` 能收到根菜单即通(挂代理)。

阶段 1 · 建表配菜单:建独立库执行 §3 SQL;`tg_menu` 插几条根菜单,其中一条 `action_type=handler, action_config={"handler":"demo_list","args":"limit=5"}`;`tg_handler` 插 `handler_key=demo_list`;`tg_command` 插 `start/主菜单`、`help/帮助`、`support/客服` 三条;`admin` 插一个管理员。起后端 `go run web.go`(启动即 `syncCommands`),输入框旁「菜单」按钮应出现这三条命令;调 `/api/boss/GetTgMenuList` 验证 CRUD。

阶段 2 · 接分发层:`web.go` 里 `controller.NewTgBotRuntime()` 构造时已 `go consume()`;本地长轮询点「示例动态菜单」,能看到 handler 返回的 mock 列表 → 闭环成立。

阶段 3 · 切 webhook 上线:部署到能直连 Telegram 的机器,配 HTTPS 反代到 8200。`config.yaml` 把 `debugPolling` 置 false、`publicBase` 改为真实域名、`secretToken` 换随机长串;启动时 `registerWebhook()` 会自动 `SetWebhook(publicBase+webhookPath, secret_token=secretToken, drop_pending_updates=true)`(因库 v5.5.1 的 WebhookConfig 无 secret_token 字段,改走原生 HTTP POST 带上密钥),无需手动 curl。后台加菜单管理页。

---

## 10. 后续可加(backlog)

绑定业务账号(`bind_user_id`)、真实数据 handler(订单/商品/物流)、多语言(`lang` 生效)、图文卡片富媒体、转人工客服、群发/推送任务表、点击统计埋点、后台菜单预览。

---

## 11. 安全红线

- token / secret 不进 Git,用环境变量注入;泄露去 BotFather `/revoke` 重生成。
- webhook 校验 `X-Telegram-Bot-Api-Secret-Token`,防伪造回调。
- 可支持所有行业的业务。

---

## 12. 开发日志(持续追加,方便续开发)

> **约定**:每次开发新功能或修 BUG,都在本节**追加一条**,格式:`日期 · 类型(功能/修复/规范) · 背景/根因 · 改动点(文件) · 验证方式 · 遗留项`。目的是让后续接手(含 AI)能快速还原上下文、不重复踩坑。

### 2026-09-29 · 修复+验收 · 『测试测试』点不进去的根因 + 全功能端到端跑通
- **背景**:用户新增的『测试测试』菜单在 Telegram 里点不开,要求“全面把功能跑通再交付”。
- **根因**(非代码 bug,是数据配置错):『测试测试』(id=45)的 `action_config` 被配成 `{"page":"root"}`。按 `menuPageResult`,menu 类型且 page==root 时按钮回调是 `m:0`(回根菜单),用户本就在根菜单→点它等于重新渲染根菜单,看似“点不进去”。它的子项『哈哈哈』(id=46)本身没问题。
- **修正**:通过后台 `POST /api/boss/SaveTgMenu {id:45, action_config:"{\"page\":\"\"}"}` 改为展开子菜单(同时验证了 CRUD 接口)。修正后根菜单里『测试测试』回调由 `m:0` 变 `m:45`,点开得到子项 `t:46`。
- **验证方法**(新增经验):在 `controller` 包内写临时只读测试 `zz_verify_test.go` 直接调非导出函数(`loadCommand`/`execAction`/`menuPageResult`/`run`),`TgBotRuntime{db:config.Mysql}` 且 `bot=nil`(send/edit/answer 自动跳过,不碰真实 Telegram)。因 `go test` 工作目录是包目录找不到 `config.yaml`,改用 `go test -c -o _verify.test ./controller` 编译后从项目根 `Rename` 为 `.exe` 再 `& '.\_verify.exe' '-test.run=...' '-test.v'` 运行。覆盖结果:A 命令(start/help/alice/demo)✅、B 四类型(menu/text/url/handler)✅、C 进入子菜单+返回根✅、D item_detail✅、E 测试测试修正后 PASS。
- **清理**:临时产物 `zz_verify_test.go`/`_verify.exe`/`_fixmenu.ps1` 已删;并补删了上一轮漏删的 `cmd/seeddemo2/` 残留。`go build ./...` 退出码 0。
- **提醒**:改完菜单数据后,用户已打开的旧根菜单消息里按钮的 callback 是旧值(仍 `m:0`),需**重新发 /start** 才能拿到新键盘。

### 2026-09-29 · 优化 · 后台全功能逻辑审查(孤儿数据/cols 语义/子菜单可达性)
- **背景**:用户要求把后台全部功能检查一遍,修正不符合逻辑的功能。梳理路由/CRUD/登录/前端两页后发现三处逻辑问题。
- **问题与修正**:
  1. **删除只删一层留孤儿**(`controller/tgController.go` `DelTgMenu`):旧实现只 `delete where parent_id in(ids)` 删直接子菜单,但前端承诺“删除及其所有子菜单”;三层嵌套时孙子菜单会变孤儿(parent_id 指向已删行、永远不可见)。改为逐层 BFS 收集全部子孙 id 一并删除。
  2. **cols 语义颠倒**(`controller/tgBotRuntime.go` `menuPageResult`):页面每行列数旧代码从**子项**读 cols 且最后一个子项覆盖全页——容器自身的 cols 根本没用。改为取**本容器菜单自身**的 cols(根页无容器行→默认 2)。
  3. **给非文件夹挂子菜单→子项永远不可达**(`views/tgMenu/index.vue`):bot 只有 `action_type=menu 且 page≠root` 的按钮才产生 `m:{id}` 回调去加载子级;text/url/handler/返回按钮永不加载子级。旧 UI 每行都显“添加子菜单”→可给叶子节点挂上永远看不到的死数据。修正:新增 `isFolder(row)`,“添加子菜单”仅对文件夹行显示;“每行”列非文件夹显示“—”;弹窗“每行按钮数”仅文件夹类型显示(附说明)。
- **保存前双守卫**(`save()`仅对已存在菜单):①空文件夹(展开子菜单但 0 子项)→提醒点开是空页;②把有子项的文件夹改成非文件夹/返回按钮→提醒“N 个子菜单将无法被点到”。
- **未改(维持现状)**:tg_handler/tg_user 有后端 CRUD 但无前端页(handler 靠代码注册、user 由 TG 自动落库,本轮不新增页面);GetTgMenuList 的 parent_id/title 字符串拼接 SQL 仅后台鉴权后可达且菜单搜索实为前端过滤(不传该参),未改。
- **验证**:`go build ./...`+`go vet ./...` exit 0;`npm run build` 通过;`GetProblems` 三文件无错。

### 2026-09-29 · 优化 · 修正 menu 动作类型的逻辑矛盾(命名+保存校验)
- **矛盾**:用户发现后台弹窗里“点击行为=打开子菜单”与“打开目标=返回根主菜单”自相矛盾。根因:`action_type=menu` 本质是“导航到一个菜单页”,有两种目标(展开本级子菜单 / 返回根),但旧命名“打开子菜单”只描述了其中一种,导致可选出矛盾配置(正是“测试测试”点不进去的根源)。
- **修法**(`web/admin/src/views/tgMenu/index.vue`):① menu 类型显示名“打开子菜单”→“菜单导航”(`actionTypeOptions`/`actionLabel`/`actionHint` 同步),不再预设目标;② 子选项“打开目标”→“导航到”,两选项写清语义(📂 展开本级子菜单(当文件夹) / ↩️ 返回根主菜单(当返回按钮));③ `save()` 新增逻辑校验——编辑**已有**菜单且“导航到=展开子菜单”但无任何子菜单时 `$confirm` 提醒“点开会是无按钮空页”。命令页 menu 只有→根一种目标,无矛盾不改。列表“点击后效果”列(`configSummary`)本已区分两种目标,无需改。
- **验证**:`npm run build` 通过,`GetProblems` 无错。

### 2026-09-29 · 优化 · 机器人菜单页改树形 + 友好命名(易用性)
- **背景**:用户反馈原列表看不懂——“按钮名/上级ID/下钻”等术语晦涩、层级靠手填数字 ID、找东西辛苦。
- **改动**:`web/admin/src/views/tgMenu/index.vue` 重写为 `el-table` **树形**(`row-key=id` + `:tree-props`,前端 `getTgMenuList({})` 取全量后 `buildTree`);列名「按钮名→菜单名称」「上级ID→上级菜单(显示上级名称)」「下钻→行内“添加子菜单”」;编辑弹窗「上级菜单ID」数字输入→**下拉选已有菜单**(排除自身及子孙防成环);搜索改为前端过滤列出所有匹配;去掉分页(树一屏展示);加全部展开/折叠、刷新。后端无改动。
- **验证**:`npm run build` 通过(仅体积良性 warning)。
- **追加**：“动作类型/配置”两列用户看不懂→改为中文可读:列头“动作类型→点击行为”、“配置→点击后效果”;单元格用 `actionLabel`/`configSummary` 把英文类型和原始 JSON 翻译成大白话(如“打开子菜单”、“↩️ 返回根主菜单”、“💬 显示文字：…”、“⚙️ 调用数据：示例动态列表（参数 limit=5）”);语言列 all/zh/en → 全部/中文/英文。
- **遗留**:命令页 tgCommand 是扁平表、无层级,本次未改。
- **再优化**：“上级菜单”列在树形模式下全是重复值(根菜单)无意义→用 `v-if="searching"` 只在搜索(平铺)时显示;树形靠缩进体现层级。“菜单名称”列由 `min-width` 改为固定 `width=240px`(不再被拉伸)。
- **同改造应用到命令页**:`views/tgCommand/index.vue` 同样把“动作类型→触发行为”、“配置→触发后效果”两列改中文可读(`actionLabel`/`configSummary`);menu 类型按命令语义显示为“📋 打开主菜单（九宫格）”;handler 显示中文名+参数。
- **表单自解释(两页弹窗)**:用户反馈不懂“动态数据(handler)/数据处理器/参数”用途→编辑弹窗加 `.form-tip` 动态说明:①`actionHint` computed 随选中动作类型实时显示该类型用途(菜单页说“点击…”、命令页说“命令触发…”);②选 handler 时根据 `selectedHandler` 展示其 `remark`(用途)与 `param_hint`(可用参数),参数框 placeholder 也取 `param_hint`;③新增 scoped `.form-tip` 样式。handler 选项只能选代码已注册的(`RegisterTg`+`tg_handler` 行)。

### 2026-09-29 · 文档 · 后台逐功能使用说明 + 补齐 lang/status 示例
- **背景**:用户反馈不会用后台,要求每个功能都配例子。
- **改动**:①`tg_menu` 新增“🚧 更多功能示例”容器(id=39)及子按钮 40–43,补齐之前没覆盖的 **lang**(all/zh/en 各一条)与 **status=0**(停用)两个字段;②文档新增 **§7.7 后台使用说明**,逐功能(搜索/下钻/添加/编辑/删除/状态/各字段/四种动作类型/同步)列表讲解并指向具体示例行 id。
- **方式**:一次性 `cmd/seeddemo2` GORM 脚本写入(`json.Marshal` 生成合法 JSON),跑完删除。
- **验证**:脚本回查 id=39–43、lang/status 值正确;菜单类实时读库,后台刷新即见、无需重启。
- **提醒**:lang=zh/en 的按钮只对相应 TG 客户端语言的用户可见(由 `loadMenuChildren` 的 `lang='all' or lang=?` 过滤),用户若看不到属正常。

### 2026-09-29 · 功能 · 补充各动作类型示例数据(供后台参考)
- **背景**:用户希望每种 action_type 都有现成例子参考后台配置。
- **改动**:线上库 `tg_command` 加 `/demo`(handler,`{"handler":"demo_list","args":"limit=5"}`);`tg_menu` 加“🧪 动作类型示例”容器(id=32)及子按钮 33–38,分别演示 text/url/handler/menu(含二级页与返回主菜单)。文档 §6 新增“动作类型配置速查表”。
- **方式**:一次性 `cmd/seeddemo` GORM 脚本写入(用 `json.Marshal` 生成合法 JSON 避免 Error3140),跑完删除。
- **验证**:脚本回查 id 与 action_config 均正确;后台刷新即可见;TG 端 /start → “🧪 动作类型示例”可逐一体验四类。
- **遗留**:`/demo` 需在后台点“同步到 Telegram”才进原生「菜单」按钮(菜单类示例无需同步)。

### 2026-09-29 · 修复 · `url` 类型命令(如 /alice)无响应
- **根因**:`execAction` 的 `url` 分支直接 `return nil`(url 本意是消息上的外链按钮,不产生回复)。但当命令 action_type=url 时,`onMessage` 拿到 nil 就什么都不发。
- **改动**:`controller/tgBotRuntime.go` `execAction` 的 url 分支改为回一条带外链按钮的消息(`Text:"🔗 "+url`,一个 `Url` 按钮);url 为空仍返回 nil。菜单里的 url 按钮不受影响(外链由 Telegram 直开、不产生回调)。
- **验证**:`go build ./...` 通过;重发 `/alice` 应收到带“🔗 打开链接”按钮的消息。

### 2026-09-29 · 决策 · admin 密码明文存储 + 对齐文档
- **决定**:`admin.password` 保**明文存储**,不做 bcrypt(用户拍板);线上密码已手动改为 `qwer1324`。
- **改动**:附录 A 种子 `admin` 密码 `admin123`→`qwer1324`;§10 backlog 移除“`admin.password` 改 bcrypt”一项;占位说明改为标注明文决策。
- **影响**:登录校验仍为 `where account=? and password=?` 明文比对(无需改代码)。

### 2026-09-29 · 优化 · 运行时健壮性 + 长轮询超时回归(全项目自测)
- **修复回归**:上一轮为防挂起给 bot 的 `http.Client` 设了 `Timeout:10s`,但这个 client 也用于 `getUpdates` 长轮询(`cfg.Timeout=50s`),会被整体超时提前掐断。改为**只在连接阶段设短超时**:`http.Transport{DialContext: net.Dialer{Timeout:5s}, TLSHandshakeTimeout:5s, ResponseHeaderTimeout:60s, Proxy:ProxyFromEnvironment}`,不再设 `http.Client.Timeout`。无出网时 GetMe 约 5s 失败,有出网时长轮询能挂满 50s。
- **健壮性**:`consume` 拆出 `handleUpdate`,每条 Update 单独 `recover`(+`debug.Stack()` 日志),避免单条消息 panic 拖垮整个分发循环。
- **UX**:点 `text`/`item_detail` 等无按钮的终端页时,`onCallback` 自动补一个「⬅️ 返回主菜单」(`cb=m:0`),避免原地编辑保留上一屏陈旧按钮、用户被困。
- **验证**:`go build ./...`+`go vet ./...` 绿;服务冷启动监听 8200+轮询启动无错;后台 API 全链路通(登录/四表列表/命令 Add→过滤→Save→Del/Sync/AuthUser/无token 401);`SyncTgCommand` 返回 code=0 兼证明 Go 进程能出网调 Telegram;前端 `npm run build` 成功(仅模板自带体积/空 style 良性 warning)。
- **发现(待用户定)**:线上 `admin` 表实际密码为 `qwer1324`,与附录 A 种子写的 `admin123` 不一致(登录测试用的是前者)。建议统一:要么改库要么改文档;且 `admin.password` 为明文,backlog 已列改 bcrypt。

### 2026-09-29 · 修复 · /help /support 等纯文本命令无响应(空内联键盘被拒发)
- **根因**:`text` 动作无按钮,`buildKeyboard` 对空按钮会返回一个空 `InlineKeyboardMarkup`;而 go-telegram-bot-api v5 的 `InlineKeyboardMarkup.MarshalJSON` 在零按钮时输出 `{}`,Telegram 报 `Bad Request: field "inline_keyboard" must be of type Array` 拒发整条消息。`/start`(menu)、`demo_list`(handler)有按钮故正常。
- **改动**:① `controller/tgBotHandler.go` 的 `buildKeyboard` 无按钮时返回 `nil`;② `controller/tgBotRuntime.go` 的 `send` 改为 `if kb != nil` 才挂 `msg.ReplyMarkup`(纯文本消息不带键盘)。
- **验证**:`go build ./...` 通过;重发 `/help`、`/support` 应正常回复固定文案。`onCallback` 的 `edit.ReplyMarkup` 是 `*InlineKeyboardMarkup` 类型字段,`omitempty` 对 nil 自动省略,无需改。
- **遗留(非阻断)**:点 `text` 类型菜单按钮时,因 edit 省略 reply_markup 会保留上一屏按钮(陈旧),如需导航时清除按钮要显式传 `ReplyKeyboardRemove`。

### 2026-09-29 · 修复 · /start 无响应(整条菜单被 Telegram 拒发)
- **根因**:根菜单「访问官网」是 `url` 按钮,种子数据填了非法占位 `https://你的域名`(含中文)。Telegram 规则:内联键盘里任一按钮 URL 非法(`Wrong HTTP URL`)会**拒发整条消息**,故用户收不到任何回复(轮询其实一直在收 update)。
- **改动**:① `controller/tgBotHandler.go` 的 `buildKeyboard` 新增 `validTgURL` 校验,跳过非法外链按钮并告警;② 种子数据 `tg_menu` id=4 的 url 改为 `https://t.me/lianggou_ai_bot`。
- **验证**:直连 `getMe`/`getWebhookInfo` 正常、webhook 为空;重发 `/start` 出九宫格根菜单。
- **排查线索**:`getUpdates` 返回空 + 无回复 = update 已被消费但发送失败,优先看 `logs/app.log` 里的 `tg send msg` 报错。

### 2026-09-29 · 改进 · 启动防挂起(GetMe 加超时)
- **背景**:原 `tg.NewBotAPI(token)` 内部 `GetMe` 无超时,连不上 Telegram 时长时间阻塞,整个服务起不来。
- **改动**:`controller/tgBotRuntime.go` 的 `NewTgBotRuntime` 改用 `tg.NewBotAPIWithClient(token, tg.APIEndpoint, &http.Client{Timeout:10*time.Second})`;失败仅 `LogWarning`、不 panic、不阻断后台接口。
- **备注**:`NewBotAPIWithClient` 签名为 `(token, apiEndpoint string, client HTTPClient)`,`*http.Client` 满足接口,`GetMe` 走该 client 故超时生效。

### 2026-09-29 · 规范 · 后台页面风格统一
- **问题**:命令菜单页 header 只放标题文字,与机器人菜单页(搜索栏)高度不一致。
- **改动**:命令页 header 改为同款 `queryForm` 搜索栏;`GetTgCommandList` 加 `command` 关键字过滤。规范固化见 §7.6。

### 2026-09-29 · 优化 · 菜单表 ID 列/上级下拉过滤/手动树形(箭头位置)
- **背景**:用户对机器人菜单页(`web/admin/src/views/tgMenu/index.vue`)提了三点:①放出 ID 自增列;②“上级菜单”下拉把全部菜单都列出来了(含返回按钮/文字/链接/动态数据等不能当父级的项),应只显示可当父级的;③想让 ID 放最左、但 Element 自带树形把展开箭头死绑在第一个数据列→箭头跑到了 ID 上。
- **改动**:
  1. **上级下拉过滤**:`parentOptions` 加 `if (!this.isFolder(x)) return`,只列「🏠根菜单 + 文件夹」(仍排除自身及子孙防成环)——与“只有文件夹能渲染子级页”的模型一致,从源头杜绝把子菜单挂到叶子/返回按钮下形成不可达死数据。
  2. **弃用 Element 自带树形→手动拍平**:去掉 `el-table` 的 `:tree-props`/`:default-expand-all`/`row-key`;data 用 `expandedIds:[]`(数组以保证 Vue2 响应)替代 `expandAll`/`tableKey`;`displayData` 先 `buildTree` 再按 `expandedIds` 深度优先拍平成带 `__level`/`__hasChildren` 的行(搜索态直接平铺匹配项 level=0);「菜单名称」列用 scoped slot 自绘缩进(`paddingLeft=__level*18`)+可点箭头(`toggleNode` push/splice `expandedIds`),「全部展开/折叠」= 收集所有含子节点的 `parent_id` 填入或清空 `expandedIds`。这样 **ID 列放最左、展开箭头在名称列** 两个诉求同时满足。
  3. **操作列去掉 `fixed="right"`**:固定列浮层导致其与相邻“状态”列之间竖线边框缺失;列能完整显示无需横向滚动,去 fixed 后边框恢复。
  4. **箭头美化+树形连接线**:收起态箭头为圆角徽标(浅蓝底 `#ecf5ff`+蓝字),展开态加 `.is-open` 变实心蓝底 `#409EFF` 白字带投影(“变深”区分);子菜单用树形连接线关联父级——**正确算法**(踩坑:初版每行只画上半截竖线→兄弟间断开、像漂浮小括号):DFS 拍平时每行记录 `__isLast`(是否本组末子)与 `__ancLast`(长度=depth-1 的布尔数组,各祖先是否末子;根层不占主干列,递归时 `childAnc = depth===0?[]:ancLast.concat([isLast])`)。渲染:祖先列 `!cont` 才画贯穿竖线 `.tree-guide--vline`(top0 bottom0);折角列(最后一列)画 `::before` 竖线 top0→50% + `::after` 横线接箭头,非末子时竖线续到 100%(连向下一个兄弟)、末子只到 50%——主干连续成串并接父级。关键坑:竖线要撑满整行高度,需给该列 `class-name="col-title"` + `.tableData ::v-deep td.col-title{padding:0}` 与 `.cell{padding:0;height:100%}`,且必须去掉该列 `show-overflow-tooltip`(否则 `.cell` overflow:hidden 会裁掉连接线),长标题改 `.tree-title` 手写 `nowrap+ellipsis`。
- **注意**:`serve`(8080)从 `src` 实时编译,改完需浏览器 Ctrl+F5 硬刷新;`npm run build` 产出 dist 是给部署用的、与 serve 的实时视图是两套。
- **验证**:`npm run build` 通过;`GetProblems` 无错。经验沉淀见记忆「tgbot 菜单树三处逻辑约束」第 4 点。

### 2026-09-29 · 修复 · 编辑弹窗“数据处理器”串入“导航到”选项文案(Vue el-select 无 key 复用)
- **现象**:用户反馈“动态数据是什么逻辑”时附带截图——选“点击行为=动态数据(handler)”后,“数据处理器”下拉框里竟显示“📂 展开本级子菜单（当文件夹用）”(那是“菜单导航”才有的选项),展开列表却是正常的 demo_list/item_detail。
- **根因**:`tgMenu`/`tgCommand` 编辑弹窗里多个动作类型块(`v-if`)各含一个结构相同的 `el-select`;切换 `action_type` 时 Vue 虚拟 DOM 无 `key` 直接**复用同一 el-select 实例**,把它缓存的旧选中文字带过来(`filterable` 下拉尤其明显),并非数据错(`cfg.handler` 实为空)。
- **修法**:给各动态块里的 `el-select` 加唯一 `key`(导航块 `key="sel-page"`、处理器块 `key="sel-handler"`),强制 Vue 销毁重建、不再复用。两页都改。
- **顺带说明 handler 逻辑**(回用户提问):动态数据=点击时调后端 `RegisterTg(key,fn)` 注册的函数实时算内容;数据处理器选项来自 `tg_handler` 表但必须代码已注册同名 key 才可选;参数 `k=v` 逗号分隔存入 `action_config{handler,args}`。链路:h:{菜单id}→runHandlerByMenu 回查→execAction handler 分支→run(key,用户,parseArgs(args))→回文案+按钮原地编辑。新增一类:开发写函数+表加行即可,纯配置免重编。
- **验证**:`npm run build` 通过;`GetProblems` 两文件无错。

### 2026-09-29 · 功能 · 配置化「接口取数」(action_type=http,无需写代码)
- **背景**:§4 的 handler 动态数据要开发写 Go 函数+注册+重编译,对"调外部接口给用户看数据"这类需求太重。新增通用 http 引擎:后台填接口地址+渲染模板即可,零代码零重编译。
- **改动点**:
  - 新增 `controller/tgHTTP.go`:`execHTTPFetch`(建请求/URL 校验/超时 8s/响应体上限 512KB/非 2xx 与非 JSON 兜底)+ `httpRender`(`{key}` 先查上下文再按 JSON 路径取)+ `jsonValue/jsonPath`(支持 `a.b[0].c`)+ `httpCtx`(内置 uid/chat/lang/bind/username/first_name + args)+ `applyHTTPHeaders`。
  - `controller/tgBotRuntime.go`:`execAction` 加 `case "http"`;`menuPageResult` 按钮回调对 http 复用 `h:{id}` 域;`ensureUser` 填 Username/FirstName。`controller/tgBotHandler.go`:`TgUser` 加 Username/FirstName 字段。
  - `web/admin/src/views/tgMenu/index.vue`:动作类型下拉加「接口取数(http)」;新增 http 配置块(请求方法/地址/头/体/参数/显示文案/列表路径/按钮文字/按钮链接/每行数);`buildActionConfig`/`edit`/`configSummary`/`actionLabel`/`actionTagType`/`actionHint`/`save` 校验同步。
- **配置结构**:action_config 扁平 string→string,键 method/url/headers/body/args/text/list_path/btn_text/btn_url/cols。
- **安全**:仅 http/https、超时、限流、失败回友好文案不崩;URL 由后台(已鉴权)管理员配置,SSRF 记为自管,未来可加 host 白名单。
- **验证**:`go build ./...`+`go vet ./...` 通过;临时 `httptest` 测试(编译成 exe 从根跑)覆盖 GET+模板渲染、list_path 生成按钮、`a[0].b` 下标取路径三条路径全 PASS,验完删测试文件与二进制(复查无残留);`npm run build` 通过、GetProblems 无错。
- **遗留**:命令页 tgCommand 暂未加 http UI(后端已天然支持);db_query(本库查询)引擎未做;非 JSON 响应、结果缓存、host 白名单未做。详见 §4.5。

### 2026-09-29 · 变更 · 后台移除「动态数据(handler)」动作类型(改用接口取数)
- **背景**:用户反馈 handler 动态数据用不上(需开发写函数+重编译),统一改用配置化的「接口取数(http)」。
- **范围决策**:仅从**后台新建/编辑 UI** 移除 handler;**保留后端执行链路**(execAction handler 分支/run/注册表/demo_list与item_detail/tg_handler CRUD/h: 回调域)。理由:h: 回调域现被 http 共用不能删;且线上已有 handler 演示菜单(🔥热门动态/🎮电竞装备//demo 等)不能突然失效。
- **改动点**:
  - `web/admin/src/views/tgMenu/index.vue`:删 handler 选项与配置块、handlerOptions/loadHandlers/selectedHandler/handlerName、getTgHandlerList import、buildActionConfig handler 分支、save 的 handler 校验、cfg.handler 字段。保留 actionLabel/actionTagType 的 handler 静态项 + configSummary handler 分支(改为直接取 cfg.handler,不依赖 handlerOptions),以便遗留行仍可读。
  - `web/admin/src/views/tgCommand/index.vue`:同样删 handler 选项/块/相关方法与校验(命令页无 http UI)。
- **遗留**:后端 handler 代码与 tg_handler 表变为休眠(不再能从后台新建);如需彻底拆除后端+删演示菜单数据,待确认再做。
- **验证**:`npm run build` 通过、GetProblems 两文件无错;grep 确认两页无 handlerOptions/selectedHandler/handlerName/loadHandlers/getTgHandlerList 残留。

### 2026-09-29 · 优化 · 菜单弹窗表单改整体两列布局(免浏览器滚动条)
- **背景**:选“接口取数”后字段多、单列把弹窗拉得超出屏高,浏览器出现滚动条。
- **改动**(`web/admin/src/views/tgMenu/index.vue`):整个新增/编辑弹窗表单改为两列 el-row/el-col——上部 上级菜单|菜单名称、语言|点击行为;http 字段全部两列(请求方法|参数、接口地址|请求头、显示文案|请求体、列表路径|每行按钮数、按钮文字|按钮链接);底部 排序|状态 合并一行;弹窗固定宽度 520px→720px;精简冗余 form-tip。`App.vue` 全局响应式弹窗新增 `≤768px` 时 `el-col` 堆成单列。
- **验证**:`npm run build` 通过、GetProblems 无错。

### 2026-09-29 · 修复 · 菜单弹窗两列文字撑破/提示浮动错位
- **现象**:http 字段中 `rows=1` 的 textarea(请求头/请求体)文字撑出格子并露出拖拽角;底部变量提示用 `margin:-6px 0 0 90px` 浮到右半区换行错位。
- **修复**(`web/admin/src/views/tgMenu/index.vue`):请求头改回普通单行 el-input;请求体移到单独整行 textarea(rows=2);变量提示改为整行浅灰说明块新增样式 `.http-help`;“每行按钮数”从 http 列表行移出、复用底部共享表单项(v-if 扩展为 menu文件夹 或 http+列表路径,提示文案按类型动态)。
- **验证**:`npm run build` 通过、GetProblems 无错。

### 2026-09-29 · 优化 · 菜单弹窗加宽 + 变量说明块缩进对齐输入栏
- **背景**:弹窗不够宽;底部 `.http-help` 变量说明块从最左边开始,与顶部 actionHint(已缩进 90px)不一致。
- **改动**(`web/admin/src/views/tgMenu/index.vue`):弹窗宽度 720px→880px;`.http-help` 的 margin 改为 `0 0 14px 90px`,左缩进对齐到输入栏起始线(与 label-width=90px 一致)。
- **验证**:`npm run build` 通过、GetProblems 无错。

### 2026-09-29 · 功能 · 接口取数新增 GET/POST 一键填入示例
- **背景**:方便使用者新增 http 菜单时有可参考的完整配置。
- **改动**(`web/admin/src/views/tgMenu/index.vue`):http 配置块顶部新增 `.http-examples` 两个按钮“GET · 查余额”/“POST · 查订单列表”,新方法 `fillHttpExample(type)` 一键把整份示例 cfg(含 method/url/headers/body/text/list_path/btn_*/cols)填入表单;`docs/TG开发文档.md` §4.5 补两个配置示例表。
- **验证**:`npm run build` 通过、GetProblems 无错。

### 2026-09-29 · 优化 · http 变量说明块扩写 + 字体加大
- **背景**:`.http-help` 变量说明太简略且 12px 字体偏小。
- **改动**(`web/admin/src/views/tgMenu/index.vue`):内容改写为分点详解(当前用户变量逐个释义/自定义变量/取返回值写法/列表模式),变量名用 `<code>` 包;样式 font-size 12→13px、line-height 1.6→1.9、颜色加深,新增 `.hh-title`/`.hh-row`/`.http-help code`(代码底色)。
- **验证**:`npm run build` 通过、GetProblems 无错。

### 2026-09-29 · 优化 · http 表单简化为只显示文案(隐藏列表渲染)
- **背景**:用户觉得列表模式(列表路径/按钮文字/按钮链接)太复杂,“暂时只显示文案就行”。
- **改动**(`web/admin/src/views/tgMenu/index.vue`):移除 http 配置块的列表相关字段(列表路径/按钮文字/按钮链接行),“显示文案”改整行;“每行按钮数”共享项的 http+list_path 条件去掉(仅留 menu 文件夹);`fillHttpExample('post')` 改为纯文案示例(查资料),按钮文案改“POST · 查资料”;`.http-help` 删“列表模式”那条。后端 execHTTPFetch 的 list_path 能力保留不动(“暂时”),`buildActionConfig` 仍会传空串。`docs/TG开发文档.md` §4.5 同步(列表字段标注 UI 暂未开放、POST 示例改纯文案)。
- **验证**:`npm run build` 通过、GetProblems 无错。

### 2026-09-29 · 优化 · http 显示文案改 rows=2 + 删除变量说明块
- **背景**:用户要求“显示文案”也用多行框,并去掉底部“变量与取值说明”整块。
- **改动**(`web/admin/src/views/tgMenu/index.vue`):显示文案 el-input 改 `type="textarea" :rows="2"`;删除 `.http-help` 整块及其样式(`.http-help`/`.hh-title`/`.hh-row`/`.http-help code`);修正 `.http-examples` 注释不再引用已删块。
- **验证**:`npm run build` 通过、GetProblems 无错。

### 2026-09-29 · 修复 · http 请求头改多行 textarea(支持多个请求头)
- **背景**:用户问“请求头有两个参数怎么填”。后端 `applyHTTPHeaders`(controller/tgHTTP.go)是**按换行 `\n` 拆分、每行 `Key: Value`**,但 UI 是单行 el-input 根本敲不进换行 → 无法填第二个。
- **改动**(`web/admin/src/views/tgMenu/index.vue`):请求头 el-input 改 `type="textarea" :rows="2"`,placeholder 改成多行示例(`Authorization: Bearer xxx` 换行 `Content-Type: application/json`)。后端无需改动。
- **验证**:`npm run build` 通过、GetProblems 无错。

### 2026-09-29 · 优化 · http 接口地址改 rows=2(与请求头对称)
- **背景**:用户觉得请求头已是多行框,接口地址单行不对称,要求也改两行。
- **改动**(`web/admin/src/views/tgMenu/index.vue`):接口地址 el-input 改 `type="textarea" :rows="2"`。纯 UI,后端无需改动。
- **验证**:`npm run build` 通过、GetProblems 无错。

### 2026-09-29 · 验证 · http 引擎端到端测试 + 落两条真实公网示例交付
- **背景**:用户要求“加两个真实例子数据、做两个测试接口跑一遍看是否有 BUG”。
- **方法**(临时测试 `controller/zz_http_e2e_test.go`,已删):用 `httptest` 起两个本地接口,走**真实代码路径**(插 tg_menu → loadMenuByID → execAction → execHTTPFetch)精确断言。因 config 包 init 连远程 MySQL,需 `go test -c -o http_e2e.exe ./controller` 后从项目根跑二进制(GOOS 临时切 windows)。
- **结果**:①GET PASS——验证 URL 模板 `{uid}`/args `{type}`、**多请求头换行拆分**(Authorization+X-Test 均生效)、JSON 路径含数组下标 `data.tags[0]`、小数 `128.5`;②POST PASS——验证请求体模板 `{uid}/{first_name}`、Content-Type 头、响应回显。引擎无 BUG。
- **交付**:向 `tg_menu` 落两条 parent_id=0 的真实示例(现场拉取均成功):id=51 `🧪 测试·查用户资料(GET)` → `jsonplaceholder.typicode.com/users/1`(文案 `{name}/{email}/{address.city}` 验证嵌套路径);id=52 `🧪 测试·发帖回显(POST)` → `jsonplaceholder.typicode.com/posts`(body `{"title":"hello {first_name}",...}`,文案 `{id}/{title}`)。临时测试文件与 exe 已删、复查无残留; httptest 临时行测完自动删除。

### 2026-09-29 · 修复 · http 模板正则吞掉扁平 JSON 请求体(导致变量取不到值)
- **现象**:用户 POST 示例 body 填 `{"title":"张申然"}`,文案 `标题 {title}` 渲染为空(只显 ID=101)。
- **根因**(`controller/tgHTTP.go`):`httpTokenRe` 旧为 `\{([^{}]+)\}`,会把**任何**不含内层花括号的 `{...}` 当变量。扁平 JSON body `{"title":"..."}` 整体被当成一个占位符,查不到→替换成空串→实际发出空 body→接口只回 id。之前测试 body 带嵌套花括号才侥幸未暴露。
- **修复**:`httpTokenRe` 收紧为 `\{([A-Za-z_][A-Za-z0-9_.\[\]]*)\}`——只认合法变量名/JSON 路径(字母下划线开头,仅含字母/数字/下划线/点/方括号),`{uid}`/`{data.balance}`/`{data.list[0].name}` 照常替换,JSON 字面量不再被吞。
- **验证**(临时 httptest 测,跑完已删):扁平 body `ID=101 T=张申然 RAW={"title":"张申然"}` PASS;嵌套回归 `GOT={"tg":42,"name":"Alice"} TAG=gold` PASS。
- **遗留**:**后端改动,需重编译+重启才生效**(用户当前进程仍旧正则)。命令页 tgCommand 的 http 配置同引擎,一并受益。

### 2026-09-29 · 调整 · 菜单删除改为“先删子菜单才能删上级”(不再连带递归删)
- **背景**:用户要求有子菜单的上级不允许直接删,必须先删子菜单。
- **后端**(`controller/tgController.go` DelTgMenu):去掉原逐层收集子孙一起删的递归;改为先查选中项中仍被当作父级的(`select distinct parent_id ... where parent_id in (ids)`),命中则拒绝并回错因文案(拼上阻塞项标题);无子菜单才直接删选中 ids。新增 `strings` 导入。
- **前端**(`web/admin/src/views/tgMenu/index.vue` del()):确认文案改为“若菜单下仍有子菜单需先删子菜单”;修复原“无论 code 成败都弹 success”的 bug,改为 code===0 才 success+刷新,否则 error。
- **验证**:`go build ./...` 通过、`npm run build` 通过、GetProblems 无错。后端改动需重启生效。

### 2026-09-29 · 优化 · 菜单树折角横线延长接到子菜单
- **背景**:叶子子菜单前是 28px 透明占位 `.tree-arrow-hollow`,折角横线只画到引导列右边缘(18px),与标题间留白。
- **改动**(`web/admin/src/views/tgMenu/index.vue`):`.tree-guide--elbow::after` 横线 width 9px→26px 跨过间距;`.tree-node` 加 `position:relative;z-index:1` 使延长后的横线走箭头/占位下方不压住徽标。后续又修正粗细不一致:横线原用 `top:50%` 定位,行高为奇数时落在半像素被抗锯齿糊成~2px(看起来比竖线粗),改用 `top:0;bottom:0;margin:auto 0` 让浏览器整数对齐居中。纯前端。
- **验证**:`npm run build` 通过、GetProblems 无错。

### 2026-09-29 · 优化 · 根级叶子菜单标题贴左对齐
- **背景**:id 51/52 等 `parent_id=0` 且无子项的根级菜单,因叶子占位 `.tree-arrow-hollow`(28px,本用于让子级叶子与同级文件夹标题对齐)被整体右推,看着像缩进;用户要求一级菜单贴左。
- **改动**(`web/admin/src/views/tgMenu/index.vue`):模板给 `.tree-arrow-hollow` 加 `tree-arrow-hollow--root` 修饰(`row.__level === 0`);新增 CSS 使根级叶子占位 width 归 0,标题左缘与根级文件夹箭头左缘齐平。子级叶子占位不变。纯前端。
- **验证**:`npm run build` 通过、GetProblems 无错;数据侧经临时 Go 查询确认 51/52 确为 `parent_id=0`(临时文件已删)。

### 2026-09-29 · 优化 · “接口取数”标签换为紫色
- **背景**:`action_type=http` 标签原用 Element `info`(灰),与“菜单导航”等灰色易混淆。
- **改动**(`web/admin/src/views/tgMenu/index.vue`):`actionTagType` 的 http 由 `info` 改为 `''`(去掉 --info 灰修饰);el-tag 按 `action_type==='http'` 加 `tag-http` 类,新增作用域 CSS 定为紫色(底 #f2e9ff/框 #ddc6f7/字 #7a3ff0)。纯前端。
- **验证**:`npm run build` 通过、GetProblems 无错。

### 2026-09-29 · 优化 · “菜单导航”标签换为柔和玫红
- **背景**:menu 标签原用 Element 默认 `''`(偏灰);换红后用户反馈“太红”,改柔和玫红。
- **改动**(`web/admin/src/views/tgMenu/index.vue`):el-tag 对 `action_type==='menu'` 加 `tag-menu` 类,新增作用域 CSS 定为柔和玫红(底 #fef0f0/框 #fbc4c4/字 #e0686d);`actionTagType` 的 menu 仍为 `''`。纯前端。
- **验证**:`npm run build` 通过、GetProblems 无错。

### 2026-09-29 · 优化 · “打开链接”标签换为浅蓝色
- **背景**:url 标签原用 Element `warning`(橙),用户要求换浅蓝。
- **改动**(`web/admin/src/views/tgMenu/index.vue`):`actionTagType` 的 url 由 `warning` 改为 `''`(去橙色修饰);el-tag 对 `action_type==='url'` 加 `tag-url` 类,新增作用域 CSS 定为浅蓝(底 #f2f8ff/框 #d9ecff/字 #79b8ff，按用户反馈调得更淡)。纯前端。
- **验证**:`npm run build` 通过、GetProblems 无错。

### 2026-09-29 · 新增 · 点击行为加“返回(back)”与“取消(cancel)”两种动作
- **背景**:用户要求在同一级子菜单页里可放“返回”“取消”按钮当导航/关闭用。经确认:返回=回上一级与回根菜单事都要(弹窗里二选一);取消=删除当前这条菜单消息。
- **后端**(`controller/tgBotRuntime.go`):①新增两个 action_type。`back` 在 `menuPageResult` 渲染期直接编成 `m:` 回调(target=root→`m:0`;否则 `m:{本容器父级}`,用新增的 `backTarget`——从 `loadMenuByID` 读容器自身 `parent_id` 得到),不依赖历史栈;`cancel` 编成 `c:0`。②`onCallback` 新增 `c` 域:调 `bot.Request(tg.DeleteMessageConfig{ChatID,MessageID})` 删本条消息后 `answer("已取消")` 并 return(v5.5.1 无 NewDeleteMessage 构造函数,直用结构体)。③`execAction` 加 `back`/`cancel` 兑底(命令路径无当前页上下文:back 回根、cancel 回“已取消”文案)。④`loadMenuByID` select 补 `parent_id` 列。
- **前端**(`web/admin/src/views/tgMenu/index.vue`):actionTypeOptions 加 back/cancel;cfg 新增 `target`(默认 parent,data/add/edit/onActionTypeChange 四处同步);弹窗 back 块=“返回到”下拉(上一级/根菜单,key=sel-back 防串味),cancel 块=提示文案;buildActionConfig 分别序 `{target}`/`{}`;actionLabel/actionTagType(back=info灰/cancel=danger红)/configSummary/actionHint 均补两类。
- **验证**:`go vet ./controller` + `go build ./...` 通过、`npm run build` 通过、GetProblems 无错。未做端到端真实回调测试(需重启后端 + TG 实机点按钮验证删消息/逐级返回)。
- **遗留**:**后端改动,需重编译+重启才生效**。命令页 tgCommand 未加 back/cancel UI(后端已支持,但命令无“当前页”概念,back 统一回根、cancel 仅回文案,意义不大)。上一级返回依赖容器 `parent_id`,若容器本身被删则 `backTarget=0` 退化回根。

### 2026-09-29 · 调整 · 往「菜单导航」去冗余:移除其“导航到”下拉里的“返回根主菜单”选项
- **背景**:新增独立「返回」动作后,菜单导航的“导航到”=【展开子菜单 / 返回根主菜单】与返回动作重叠。用户指出“返回根”在此已无意义。
- **改动**(`web/admin/src/views/tgMenu/index.vue`,纯前端):删除 menu 的“导航到” el-select 整块(菜单导航现固定=当文件夹展开子菜单);“每行按钮数”条件从 `menu && cfg.page!=='root'` 简化为 `menu`;actionHint.menu 与 save() 两处“导航到”相关提示文案改指「返回」动作。`cfg.page`/isFolder/buildActionConfig 保留(新 menu 项 page 恒为 ''→仍为文件夹)。
- **兼容**:旧数据里 action_type=menu+page=root 的“返回”按钮(如种子 id30/31/38)仍可正常渲染(后端保留该分支),configSummary 仍显示“↩️ 返回根主菜单”;仅不能再通过菜单导航新建此类。想改可切成「返回」动作。
- **验证**:`npm run build` 通过、GetProblems 无错。纯前端,无需重编译后端。

### 2026-09-29 · 功能 · 菜单/命令文案支持富文本(HTML)与配图(图片+文案+按钮)
- **背景**:用户参考其它 bot 的「图在上、文在中、按钮在下」形态,要求本项目也支持富文本与配图。此前 `send`/`edit` 只发纯文本、无 `parse_mode`,写 `**粗**`/`<b>` 也原样显示。
- **改动**:
  - 后端 `controller/tgBotHandler.go`:`TgResult` 加 `Image string`、`HTML bool`。
  - 后端 `controller/tgBotRuntime.go`:`send` 改签名收 `*TgResult`,按 `Image` 走 `NewPhoto`+`FileURL`(caption+键盘)否则 `NewMessage`;按 `HTML` 设 `ParseMode=tg.ModeHTML`,失败自动去 parse_mode 重试;新增 `editMessage`(处理文本↔图片互转:形态不一致→`deleteMessage`+`send`,一致→`EditMessageMedia`/`EditMessageText`)、`deleteMessage`、`safeImage`、`messageHasMedia` 辅助;`onCallback` 的编辑块与 `c:` 域改用这些辅助;`execAction` 的 text/http 读 `cfg["image"]`/`cfg["format"]`;`menuPageResult` 读容器自身 `action_config` 的 text/image/format;移除已不再使用的 `menuPage`。
  - 前端 `web/admin/src/views/tgMenu/index.vue`:cfg 加 `image`/`format`(四处默认值 + edit 回填);menu 配置块新增「页面引导语/配图URL/富文本」,text 配置块新增「配图URL/富文本」;`buildActionConfig` 对 menu/text 序列化 `{text,image,format}`(menu 不再写 page);`configSummary` 的 menu/text 反映引导语与含配图;`actionHint` 补富文本/配图说明。
- **验证**:`go build ./...` + `go vet ./controller` 通过、`npm run build` 通过、GetProblems 无错。未做 TG 实机端到端(需重启后端 + 真图 URL 验证图文互转与 HTML)。
- **遗留**:**后端改动,需重编译+重启才生效**。命令页 tgCommand 未加配图/富文本 UI(后端 execAction 通用读 cfg,数据层已支持,后续可照 text 块补 UI)。http 结果的 image/format 后端已读但 UI 暂未暴露。图文互转「删旧发新」会使消息跳到底部,是 Telegram 无法原地文本↔媒体互转的固有限制。

### 2026-09-29 · 功能 · 后台文案输入框加「富文本工具栏 + 实时预览」
- **背景**:用户看到「页面引导语」里手写 `<b>` 标签,问能否做成富文本。因 Telegram 只认极小一撮 HTML 标签,全 WYSIWYG 会产出大量不支持的标签导致发送失败,故取「受限工具栏 + 预览」方案(纯前端,存的仍是 HTML 字符串,后端不动)。
- **改动**(`web/admin/src/views/tgMenu/index.vue`):menu 的「页面引导语」与 text 的「回复文案」两处,将「富文本」开关上移到字段上方;开关为 html 时字段上方显工具栏(B/I/U/S/代码/剧透/🔗链接)、下方显实时预览(`v-html`,`white-space:pre-wrap` 保留换行)。页面引导语改为 textarea。新增方法 `_richEl`(取 el-input 内部真实 textarea/input DOM)、`insertTag(ref,open,close)`(在选区两侧包标签、无选区插占位文字、$nextTick 回写光标)、`insertLink(ref)`(`$prompt` 要 URL 包 `<a href>`)。新增样式 `.rich-bar/.rb/.rich-preview`(预览内 `::v-deep a/code` 上色)。工具栏仅包 Telegram 支持的标签,不产生脏 HTML。
- **验证**:`npm run build` 通过、GetProblems 无错。纯前端,无需重编译后端;8080 上 Ctrl+F5 生效。
- **遗留**:预览是浏览器渲染,与 TG 客户端实际样式略有差异(剧透/部分标签浏览器不还原),仅作排版参考。工具栏未做选中包裹已有文字的反向解析(再次编辑手贴的 HTML 时选区包标签仍适用)。

### 2026-09-29 · 调整 · 接口取数(http)的「显示文案」也接富文本工具栏
- **背景**:用户问 http 显示文案能否也用富文本。后端上轮做 http 时 `execAction` 已读 `cfg["format"]=="html"` 置 `res.HTML`,只是 UI 未暴露——纯前端补 UI,无需重编译后端。
- **改动**(`web/admin/src/views/tgMenu/index.vue`):http 块「显示文案」上方加「富文本」开关,开启后复用 `insertTag/insertLink`(ref=`richHttp`)工具栏 + 实时预览;`buildActionConfig` 的 http 序列化补 `format`;`fillHttpExample` 两处 cfg 补 `format: ''`。
- **验证**:`npm run build` 通过、GetProblems 无错。纯前端,Ctrl+F5 生效。注:预览会原样显示 `{data.xxx}` 占位符(未代入),仅看排版。

### 2026-09-29 · 功能 · 根菜单页(/start)也支持横幅配图 + 富文本引导语
- **背景**:用户希望 `/start` 打开的根菜单页(原写死「请选择：」、无图)也能配横幅图 + 富文本引导语。根页在数据模型里没有对应的 `tg_menu` 行(它是 `parent_id=0` 的特殊页),故其展示配置最自然的归属是 **`/start` 命令本身**(`tg_command`)。
- **后端**(`controller/tgBotRuntime.go` `menuPageResult`):`parentID==0` 分支新增 `r.loadCommand("start")` 读其 `action_config`,取 `text`(非空才覆盖默认「请选择：」)/`image`/`format=="html"` 填入 `res`,复用已有的 send/editMessage 图文+HTML 链路;`/start` 与点「返回根」(m:0)都走此分支,统一带横幅。
- **前端**(`web/admin/src/views/tgCommand/index.vue`):menu 类型块加「富文本」开关 + 「菜单引导语」(工具栏/实时预览,ref=`richCmd`,复用 `insertTag/insertLink`/`_richEl`) + 「菜单配图URL」;`cfg` 默认与 `edit()` 回填补 `image/format`;`buildActionConfig` menu 序列化补 `text/image/format`;`configSummary` menu 分支含配图时追加「🖼️含配图」;补 `.rich-bar/.rich-preview` 样式。
- **验证**:`go build ./... && go vet ./controller` 通过、`npm run build` 通过、GetProblems 无错。**含后端改动,须重启后端方生效**;图 URL 拉不到会自动退回纯文本。

### 2026-09-29 · 规范 · 命令弹窗对齐菜单弹窗(880px 两列)
- **背景**:用户要求「命令菜单」的添加/修改弹窗做成与「机器人菜单」一致——两列、宽度相同。菜单弹窗已是 `width=880px` + `el-row/el-col :span="12"` 两列,命令弹窗仍是 `520px` 单列。
- **改动**(`web/admin/src/views/tgCommand/index.vue`):弹窗宽度 520→880px;顶部「命令名｜说明」同行两列,「触发行为」整行 + `actionHint` 改为独立 `.form-tip`(左缩进 90px与输入框对齐,同菜单页);menu 块重排为「富文本/菜单引导语」整行 + 「跳转目标｜配图URL」同行两列(标签「菜单配图URL」缩短为「配图URL」避免 90px 下换行);底部「排序｜状态」同行两列;text 回复文案 rows 3→4。
- **验证**:`npm run build` 通过、GetProblems 无错。纯前端,Ctrl+F5 生效。

### 2026-09-29 · 调整 · 命令的 text「回复文案」也接富文本工具栏
- **背景**:用户要求命令菜单里 text 类型的「回复文案」也加富文本。后端 `execAction` 的 text 分支已读 `cfg["format"]=="html"` 置 `res.HTML`(与菜单共用同一引擎),只是 UI 未暴露——纯前端补 UI,无需重编译后端。
- **改动**(`web/admin/src/views/tgCommand/index.vue`):text 块「回复文案」上方加「富文本」开关,开启后复用 `insertTag/insertLink`(ref=`richText`)工具栏 + 实时预览;`buildActionConfig` 的 text 序列化补 `format`。
- **验证**:`npm run build` 通过、GetProblems 无错。纯前端,Ctrl+F5 生效。

### 2026-09-29 · 调整 · 命令的 text「回复文案」也支持配图
- **背景**:紧接上条,用户要求 text 也能配图。后端 `execAction` 的 `case "text"` 早已读 `cfg["image"]` 填入 `res.Image`(图文+HTML 链路就绪),纯前端补 UI。
- **改动**(`web/admin/src/views/tgCommand/index.vue`):text 块「回复文案」下方加「配图URL」输入框;`buildActionConfig` 的 text 序列化补 `image`;`configSummary` text 分支改为含图时前缀「🖼️」、无文案时显示「(仅配图)」。
- **验证**:`npm run build` 通过、GetProblems 无错。纯前端,Ctrl+F5 生效。

### 2026-09-29 · 功能 · 命令跳转目标可选一级菜单,横幅字段仅根菜单时显示
- **背景**:用户建议命令 menu 的「跳转目标」不应只有根主菜单,应能选机器人菜单里的一级菜单;并与触发行为并列一行;且只有目标是根主菜单时才需填引导语/配图(因为选具体菜单时,那个菜单页的横幅应在「机器人菜单」配那一行本身)。
- **后端**(`controller/tgBotRuntime.go` `execAction` `case "menu"`):改为按 `cfg["page"]` 决定 parentID(root/空→0,否则 `toInt64(page)` 直接展开目标一级菜单);修正旧逻辑误用命令自身 id 当 parentID 的隐患(execAction 的 menu 分支仅命令路径会走到,菜单树文件夹走 m: 回调)。
- **前端**(`web/admin/src/views/tgCommand/index.vue`):引入 `getTgMenuList`,新增 `topMenus`+`loadTopMenus()`(mounted 拉 parent_id=0 且 action_type=menu 的一级文件夹);「触发行为」与「跳转目标」同行两列(非 menu 时触发行为占满 24);跳转下拉=🏠根主菜单 + 一级菜单(value=String(id));menu 配置块包 `v-if="cfg.page==='root'"` 才显富文本/引导语/配图,否则显提示行;`edit()` page 默认 root;`buildActionConfig` menu 非 root 只存 `{page}`;`configSummary` menu 区分根菜单/具体一级菜单名。
- **验证**:`go build ./... && go vet ./controller` 通过、`npm run build` 通过、GetProblems 无错。**含后端改动,需重启后端生效**。

### 2026-09-29 · 调整 · 根主菜单横幅改为按「当前触发命令」各自读取
- **背景**:上轮遗留——`menuPageResult(0)` 把根页横幅写死读 `start` 命令。用户确认改为按当前触发的命令自身配置渲染。
- **改动**(`controller/tgBotRuntime.go` `execAction` `case "menu"`):当 `parentID==0`(命令触发打开根主菜单)时,用当前命令的 `cfg` 完全覆盖横幅(text 非空才覆盖默认「请选择：」/image/format=="html");未配则回落默认。导航回根(`m:0` 回调)无命令上下文,仍由 `menuPageResult` 兜底读 start,不受影响。纯后端,前端无需改。
- **验证**:`go build ./... && go vet ./controller` 通过、GetProblems 无错。需重启后端生效。

### 2026-09-29 · 修复 · 命令切到具体菜单保存会抹掉根菜单横幅数据
- **现象**:用户反馈「切回根主菜单后引导语/配图都没了」。查库确认 `/start` 的 `action_config` 变成 `{"page":"root","text":"","image":"","format":""}`。
- **根因**:上轮 `buildActionConfig` 对 menu 类型,当 `page!=root` 时只返回 `{page}`。用户切到具体一级菜单→点保存,旧的 text/image/format 被丢弃写回库;再切回根自然为空。
- **修复**(`web/admin/src/views/tgCommand/index.vue`):menu 类型无论选哪个目标都固定序列化 `{page,text,image,format}`(选具体菜单时后端本就忽略横幅,留着无害且切回根不丢)。并用临时脚本 `TestZZRestoreStart` 将 `/start` 演示横幅恢复回库(rows affected=1)。
- **验证**:`npm run build` 通过、GetProblems 无错;临时脚本/exe 已删并复查无 `zz_*` 残留。纯前端,Ctrl+F5 生效。

### 2026-09-29 · 规范 · 命令 menu 选项改名为「打开菜单」并统一标签玫红
- **背景**:用户要求命令页 menu 的文字由「打开主菜单」改为「打开菜单」,并换个标签颜色。
- **改动**(`web/admin/src/views/tgCommand/index.vue`):`actionTypeOptions` menu label 与 `actionLabel` menu 均改为「打开菜单」;表格 el-tag 新增 `:class="{ 'tag-menu': row.action_type === 'menu' }"` 并补 `.tag-menu` 样式(#fef0f0/#fbc4c4/#e0686d),与「机器人菜单」页同一玫红配色(遵循标签配色规范)。
- **验证**:`npm run build` 通过、GetProblems 无错。纯前端,Ctrl+F5 生效。

### 2026-09-29 · 调整 · 命令列表「说明」列改固定宽度
- **背景**:用户反馈命令列表「说明」列太宽。原因为该列用 `min-width="140px"`,与同为弹性列的「触发后效果」一起分摊剩余宽度。
- **改动**(`web/admin/src/views/tgCommand/index.vue`):「说明」列 `min-width="140px"` → 固定 `width="160px"`,弹性宽度归「触发后效果」。
- **验证**:`npm run build` 通过、GetProblems 无错。纯前端,Ctrl+F5 生效。

### 2026-09-29 · 规范 · 菜单树收起态展开箭头颜色与展开态统一
- **背景**:用户要求一级菜单的收起态展开图标颜色改成与展开时一致。
- **改动**(`web/admin/src/views/tgMenu/index.vue`):`.tree-arrow` 基础样式由浅蓝底蓝字(#ecf5ff/#409EFF)改为实心蓝底白字(#409EFF/#fff),与 `.tree-arrow.is-open` 同色(展开态保留投影作细微区分);hover 改为加深蓝 #337ecc 保留悬停反馈。
- **验证**:`npm run build` 通过、GetProblems 无错。纯前端,Ctrl+F5 生效。

### 2026-09-29 · 交互 · 富文本常驻预览改为工具栏「预览」按钮弹窗
- **背景**:用户反馈文案框下方的常驻预览很占地方,希望改成工具栏一个按钮、点击才弹出预览。
- **改动**(`tgMenu/index.vue` 3 处工具栏 + `tgCommand/index.vue` 2 处):删除文案框下方常驻的 `.rich-preview` div;在 `.rich-bar` 的「链接」右侧新增 `👁 预览` 按钮(`.rb-preview`,`margin-left:auto` 靠右),点击置 `previewVisible=true`;新增一个 `append-to-body` 的预览 `el-dialog`(复用 `.rich-preview` 样式渲染 `cfg.text`)。两页 data 各加 `previewVisible: false`。
- **验证**:`npm run build` 通过、GetProblems 无错。纯前端,Ctrl+F5 生效。

### 2026-09-29 · 调整 · 富文本预览弹窗字号/尺寸调大
- **背景**:用户反馈预览字太小、看着费力,要求字体调大、框做大。
- **改动**(`tgMenu/index.vue` + `tgCommand/index.vue`):预览 `el-dialog` 宽度 520px → 760px;`.rich-preview` 字号 13px→16px、line-height 1.6→2、padding 8px 10px→18px 20px、新增 min-height:160px、border-radius 4px→6px。(预览现仅用于弹窗,改 `.rich-preview` 不影响其他地方)
- **验证**:`npm run build` 通过、GetProblems 无错。纯前端,Ctrl+F5 生效。

### 2026-09-29 · 功能 · 菜单列表新增「点击行为」筛选
- **背景**:用户要求菜单页搜索栏加一个按点击行为(action_type)查询。
- **改动**(`tgMenu/index.vue`):搜索栏新增 clearable `el-select`(复用 `actionTypeOptions` 选项,placeholder=全部);`where` 加 `action_type`;`searching()` 改为标题或行为任一非空即平铺;`displayData()` 平铺分支同时按 kw 与 action_type 过滤;`reset()` 一并清空 action_type。纯前端过滤,无需后端。
- **验证**:`npm run build` 通过、GetProblems 无错。Ctrl+F5 生效。

### 2026-09-29 · 调整 · 终端页自动返回按钮改为按层级回「上一级」
- **背景**:用户发现未配返回的 text 菜单(如热门动态)点进去会自动多一个「返回主菜单」(源于 onCallback 对无按钮终端页的兜底),要求改成回上一级。
- **改动**(`controller/tgBotRuntime.go` `onCallback` 终端页补按钮处):不再写死 `m:0`;按当前 `domain` 取 `arg` 对应菜单行的 `parent_id` 作为目标(t/h=被点菜单、m=当前展示文件夹),`parent_id>0` → 回该父页且文案「⬅️ 返回上一级」;`parent_id==0`(本就是根下) → 仍回 `m:0` 且文案「⬅️ 返回主菜单」;`item` 域无菜单上下文保持回根。
- **验证**:`go build ./... && go vet ./controller` 通过、GetProblems 无错。**含后端改动,需重启后端生效**。

### 2026-09-29 · 功能 · 所有子菜单页默认自带「返回上一级」按钮
- **背景**:用户认为返回不应靠后台手动加 `back` 菜单项,应默认所有菜单页自带。
- **改动**(`controller/tgBotRuntime.go` `menuPageResult`):遍历子项时新增 `manualBack` 标记(遇 `action_type=back` 置 true);渲染完若 `parentID>0 && !manualBack`,自动 append 一个返回按钮——`backTarget>0` 文案「⬅️ 返回上一级」回父页,`backTarget==0`(本容器为一级菜单)文案「⬅️ 返回主菜单」回 `m:0`。根页不加;已手动配 back 子项则不重复(兼容旧数据)。终端页(text/http)回根兑底仍由 onCallback 负责,两者不冲突(菜单页按钮非空不会重复触发)。
- **验证**:`go build ./... && go vet ./controller` 通过、GetProblems 无错。**含后端改动,需重启后端生效**。旧数据里手动加的 back 子项可删。

### 2026-09-29 · 规范 · 后台移除「返回(back)」手动配置入口
- **背景**:返回已改为菜单页自动生成,无需也不应再让运营手动选 back。用户要求把下拉里的「返回(back)」清掉。
- **改动**(`tgMenu/index.vue`):从 `actionTypeOptions` 删除 `{value:'back'}` 一项(编辑弹窗与搜索筛选共用此数组,一处删两处生效);同时删除已选不到的「返回到」配置 `<template>` 块与 `actionHint` 里的 `back` 说明。`actionLabel`/`configSummary` 里的 back 保留(万一有遗留数据仍可读展示)。后端 `menuPageResult` 的 `case "back"` 保留(兼容旧数据)。查库确认当前无 `action_type='back'` 行。
- **验证**:`npm run build` 通过、GetProblems 无错。纯前端,Ctrl+F5 生效。

### 2026-09-29 · 调整 · 菜单页移除多余的「查询」按钮
- **背景**:菜单列表为全量拉一次+前端实时过滤(computed `displayData` 依赖 `where`),`search()` 是空方法,「查询」按钮点了无意义。用户要求去掉。
- **改动**(`tgMenu/index.vue`):搜索栏删除 `<el-button @click="search">查询</el-button>`,仅留「重置」。(命令页 `tgCommand` 的查询是真请求后端 `getList()`,保留不动;`@keyup.enter`/`@change` 绑的空 `search()` 无害保留)
- **验证**:`npm run build` 通过、GetProblems 无错。纯前端,Ctrl+F5 生效。

### 2026-09-29 · 调整 · 菜单页移除「语言」功能点(列表列+编辑下拉)
- **背景**:用户默认全部菜单 `lang='all'`,语言列/下拉无实际作用,要求去掉该功能点。
- **改动**(`tgMenu/index.vue`):删除表格「语言」`el-table-column`;编辑弹窗删除「语言」`el-select`(原与「点击行为」同排两列,现「点击行为」改为独立整行 `el-form-item`);删除 `langOptions` 数据与 `langLabel`/`langText` 两个仅服务于 UI 的方法。**保留**:表列 `lang` 字段本身、`emptyForm()` 的 `lang:'all'`(新增仍写 all)、`edit()` 回填 `lang: row.lang`(改存量行不篡改原值)、http 说明里的 `{lang}` 运行时变量。后端 `loadMenuChildren` 按 lang 过滤的逻辑保留(数据全为 all,恒匹配)。
- **验证**:`npm run build` 通过、GetProblems 无错。纯前端,Ctrl+F5 生效。

### 2026-09-29 · 调整 · 菜单弹窗移除动作说明与接口取数快速示例
- **背景**:用户觉得菜单编辑弹窗里「点击行为」下方的动作用途说明、以及接口取数的「快速示例(GET·查余额 / POST·查资料)」按钮行冗余,要求去掉。
- **改动**(`tgMenu/index.vue`):删除模板中 `{{ actionHint }}` 的 `form-tip` 行与 `.http-examples` 整块;同步删除已无引用的 computed `actionHint`、method `fillHttpExample`、样式 `.http-examples`/`.ex-cap`。保留:各字段 placeholder 提示、富文本开关旁的内联说明、cancel 配置块内的整行说明(不依赖 actionHint)。**命令页 `tgCommand/index.vue` 的 `actionHint` 本轮未动**。
- **验证**:`npm run build` 通过、GetProblems 无错。纯前端,Ctrl+F5 生效。

### 2026-09-29 · 调整 · 请求体 placeholder 改为标准 JSON 示例 + 修正 POST 演示数据
- **背景**:接口取数「请求体」空着时只提示“POST 正文,可用 {uid} 等变量”,没有可直接照抄的格式;且演示菜单(id=52 测试·发帖回显)里存的是不标准示例 `{"title":"张申然","age":23,"name":"费丽莎"}`(字段与 jsonplaceholder 的 posts 模型无关,age 也不是该接口字段)。用户要求给一个标准 JSON 例子。
- **改动**:①`tgMenu/index.vue` 请求体 placeholder 改为一行极简标准 JSON 示例 `{"name":"alice"}`(用户反馈不要多行/变量说明那种复杂的;且实测 `&#10;` 在 el-input placeholder 属性里不会被解析为换行、会直显乱码,故用单行);②数据修正(临时脚本 `db.Exec` + `json.Marshal`,避免 MySQL 字面量转义问题):id=52 的 body 改为 `{"title": "{first_name}", "body": "hello", "userId": {uid}}`,text 同步改为只引用接口实际回显字段 `✅ 提交成功：ID={id}，标题={title}，用户={userId}`(旧文案里的 `{age}`/`{name}` 接口不返回,永远为空)。
- **验证**:`npm run build` 通过、GetProblems 无错;脚本输出 `UPDATED rows affected = 1`并 AFTER 回查确认;临时 `_test.go`/`.exe` 已删并复查无残留。纯前端+数据,无需重启后端;Ctrl+F5 生效。

### 2026-09-29 · 功能 · 启动时按 debugPolling 自动注册 webhook(上线免手动 curl)
- **背景**:之前代码只有 `DeleteWebhook`(轮询前清一下),无 `SetWebhook`;`publicBase` 仅存于配置/文档、无任何 Go 代码读取。用户问上线是否要把 debugPolling改 false、publicBase 改域名——确认方向后要求改成启动时自动注册。
- **关键库限制**:v5.5.1 的 `tg.WebhookConfig` 结构体**无 `SecretToken` 字段**(只有 URL/Certificate/IPAddress/MaxConnections/AllowedUpdates/DropPendingUpdates)。若用它注册不带密钥,Telegram 回调不会回传 `X-Telegram-Bot-Api-Secret-Token`,会被 `Webhook()` 校验拦成 403。→ 改走**原生 HTTP POST** `api.telegram.org/bot<token>/setWebhook`,表单带 `url`/`secret_token`/`drop_pending_updates=true`。
- **改动**(`controller/tgBotRuntime.go`):`NewTgBotRuntime` 启动分派改为 `if debugPolling { go startPolling() } else { go registerWebhook() }`;新增 `registerWebhook()` 方法(放在 startPolling 后):publicBase 为空或含“你的域名”占位则跳过并告警;复用带 `ProxyFromEnvironment` 的短超时 `http.Client`(15s) PostForm;打印 setWebhook 响应。新增 import `io`/`net/url`。同步更新 `config.yaml` 中 publicBase/secretToken/debugPolling 注释、文档 §9 阶段3。
- **验证**:`go build ./...`+`go vet ./controller` 通过、GetProblems 无错。含后端改动,上线需重新编译部署。**本地仍为 debugPolling=true 不受影响**(走轮询分支)。
- **上线清单**:`debugPolling:false` + `publicBase:https://真实域名` + `secretToken:随机长串` + HTTPS 反代到 8200;重启后看日志 `tg setWebhook(...) 响应: ..."ok":true` 即成功。

### 2026-09-30 · 功能 · 接口取数(http)支持从响应里取图片链接内嵌显示(image_path)
- **背景**:用户问接口返回数据里有图片链接怎么处理,要求直接内嵌显示图片而非给链接点击。原 http 引擎只输出文案+按钮,不读响应里的图片。
- **改动**:①后端 `controller/tgHTTP.go` `execHTTPFetch`:新增配置键 `image_path`(指向响应里图片直链的 JSON 路径,如 `data.pic`),用现成 `jsonPath` 取值+`validTgURL` 校验后赋 `res.Image`;发送/编辑端(send/editMessage/safeImage)本就支持图文形态与文本↔图片互转,无需改。②前端 `tgMenu/index.vue`:http 配置块「显示文案」下新增「配图路径」输入框;`cfg` 四处初始化、`edit()` 回填、`buildActionConfig` http 序列化同步加 `image_path`。取不到/非法 URL 自动退化纯文本不报错。②b 追加(同日):用户反馈光有 placeholder 不够直观,「配图路径」下补 `form-tip` 对照示例:返回 `{"data":{"pic":"https://..."}}`→填 `data.pic`;数组第一项 `{"list":[{"url":...}]}`→填 `list[0].url`,并注明需公网 http(s) 直链、取不到自动退回纯文案。②c 追加(同日):用户要求兼容「直接填图片地址」——后端改为两种写法自适应:`image_path` 以 `http(s)://` 开头则直接当图片地址(先过 `httpRender`,支持 `{uid}` 等变量占位,每用户不同图),否则仍按响应 JSON 路径取值;前端 placeholder/说明同步改为①固定地址②接口返回字段路径两种写法对照。验证:go build/vet + npm build 通过、GetProblems 无错。**含后端改动,需重启后端**。②d 追加(同日):用户又要求删掉「配图路径」下的 form-tip 大段说明(嫌占地方),现仅保留输入框+一行 placeholder(两种写法信息已浓缩在 placeholder 里),功能逻辑不变。②e 追加(同日):用户把配图路径填成 `{pci}`(带花括号)导致图不显示——JSON 路径取值时把 `{pci` 当 key 查不到。后端兼容:路径分支先 `strings.Trim(ip, "{}")` 去掉外层花括号再查,`pci` 与 `{pci}` 两种写法均生效(go build/vet 通过,**需重启后端**)。
- **验证**:`go build ./...`+`go vet ./controller` 通过;临时脚本离线测 jsonPath+validTgURL 取值链路 PASS(已删净);`npm run build` 通过、GetProblems 无错。**含后端改动,需重启后端生效**;前端 Ctrl+F5。

### 2026-09-30 · 调整 · 菜单树默认全部展开
- **背景**:菜单树原默认全折叠,用户要求进页默认展开全部。
- **改动**(`tgMenu/index.vue`):抽出 `expandAll()`(收集所有含子节点的 id 填入 `expandedIds`),`loadMenus()` 成功后调用——进页/保存/刷新重拉均默认展开;`toggleExpand()` 改复用 `expandAll()`,手动「全部展开/折叠」与行首箭头折叠不受影响。
- **验证**:`npm run build` 通过、GetProblems 无错。纯前端,Ctrl+F5 生效。

### 2026-09-30 · 调整 · 后台富文本改为强制开启(先默认开、后删除开关)
- **背景**:用户认为填的都是模板,先要求所有填文本处默认开富文本;随即反馈开关也没意义,要求直接删除。
- **改动**(`tgMenu/index.vue` + `tgCommand/index.vue`,最终形态):①彻底删除 5 处「富文本」`el-switch` 表单项(菜单页 menu引导语/text回复文案/http显示文案 + 命令页 menu菜单引导语/text回复文案);②排版工具栏 `.rich-bar` 的 `v-if="cfg.format === 'html'"` 条件去掉,改为**常驻显示**;③`cfg.format` 数据字段保留且固定 `'html'`(data 初始/add/edit 预置/onActionTypeChange/edit 回填 `|| 'html'`),序列化仍写 `format:'html'`,**后端零改动**;④预览弹窗空内容提示去掉「开启富文本」字样。HTML 解析失败时后端 send/editMessage 已有回退纯文本兑底。
- **验证**:`npm run build` 通过、GetProblems 无错。纯前端,Ctrl+F5 生效。

### 2026-09-30 · 功能 · 接口取数(http)支持「先问用户要输入再调接口」(await-input)
- **背景**:用户希望点菜单后先让用户输入订单号之类,拿到输入再拿去调接口返回数据。原 http 引擎只会立即用内置变量调接口,且 onMessage 对普通文本直接 return 忽略,没有「等用户输入」这一步。
- **方案**(与用户确认默认):内存存等待态 + ForceReply 输入 + 5分钟超时 + `{input}` 变量。
- **后端改动**:
  - `tgBotRuntime.go`:`TgBotRuntime` 加 `awaitMu sync.Mutex` + `awaitInputs map[int64]*awaitInput`(key=chat_id);新增 `awaitInput{menuID,varKey,expire}` 与 `const awaitInputTTL=5*time.Minute`;`setAwait`/`consumeAwait`(取出即清、超时返回 nil)。`onMessage` 普通文本分支改为:若 `consumeAwait` 命中则用输入作为变量重新执行该菜单取数并 send。`onCallback` 新增:若 `res.ForceReply` 则保留原菜单不动、另发一条带 ForceReply 的新消息。`runHandlerByMenu` 对 http 行改走新 `menuHTTPResult`。
  - `tgHTTP.go`:`execHTTPFetch` 重命名为 `httpResult(cfg,u,extraCtx)`,extraCtx 最后覆盖上下文(用户输入优先);新增配置键 `input_prompt`/`input_key`;http 结果的 image/format 兑底从 execAction 内联到 httpResult 末尾。新函数 `menuHTTPResult`:配了 input_prompt 且本次无输入时→登记等待态+返回提示语(ForceReply),否则直接取数。`execAction` case "http" 改为直调 `httpResult(cfg,u,nil)`。
  - `tgBotHandler.go`:`TgResult` 加 `ForceReply bool` + `ReplyHint string`;`send` 中 ForceReply 时挂 `tg.ForceReply{Selective:true}`。
- **前端改动**(`tgMenu/index.vue`):新增「输入提示语」输入框 + 一行说明;cfg 四处初始化加 `input_prompt`、edit() 回填、buildActionConfig http 序列化同步。
- **使用**:菜单填 input_prompt=「请输入你的订单号」,在接口地址/请求体/显示文案里用 `{input}` 引用即可。
- **验证**:`go build ./...`+`go vet ./controller` 通过;`npm run build` 通过;GetProblems 无错。离线测 `TestZZAwaitFlow`(状态机登记/消费/超时)+`TestZZInputVarRender`(extraCtx 注入后渲染)均 PASS。含后端改动,**需重启后端**生效。
- 追加(同日、纯前端):用户要求①「点击行为」与「输入提示语」并排两列(将 input_prompt 上提入点击行为同一 el-row 的右侧 span12,v-if http);②去掉「输入变量名」输入框(固定用 `input`)——前端不再传 `input_key`,后端 `menuHTTPResult` 读空时本就默认 `input`,无需改后端/重编译。`npm run build` 通过、GetProblems 无错,Ctrl+F5 生效。③又要删掉「输入提示语」下方那段 form-tip 使用说明(placeholder 已够直观),仅留输入框。

#### 2026-09-30 · 修复 · await-input 查询结果无「返回上一级」按钮
- **现象**:用户配了输入提示语的 http 菜单,输入订单号后回的结果消息底部没有返回按钮(普通终端页都有)。
- **根因**:自动补返回按钮的逻辑只写在 `onCallback`;而 await-input 的结果是在 `onMessage` 消费等待态后直接 `r.send` 发出的,没经过那段逻辑。
- **修复**(`tgBotRuntime.go`):把补返回按钮抽成方法 `appendBackButton(res, menuID)`(内部判 `len(res.Buttons)>0` 则不补;按 menuID 的 parent_id 自适应文案/目标),`onCallback` 改调此方法,`onMessage` 的 await 恢复路径也补上 `r.appendBackButton(res, aw.menuID)` 再 send。go build/vet 通过,**需重启后端**。

### 2026-09-30 · 功能 · 后台新增「TG用户」列表页
- **背景**:用户要求把 `tg_user` 表里的机器人用户展示出来。
- **后端**:**无需改动**——`tgController.go` 的 `GetTgUserList`(支持 username 模糊/tg_user_id 精确/page+limit 分页、返回 count)、`SaveTgUser`、`DelTgUser` 及 `route.go` 对应路由早已存在。
- **前端新增**:`api/tgUser.js`(getTgUserList/saveTgUser/delTgUser);`views/tgUser/index.vue`(搜索栏+表格+服务端分页 el-pagination+编辑弹窗);`router/index.js` 注册 `/tgUser` 子路由;`layout/sidebar.vue` 菜单新增「TG用户」(el-icon-user-solid)。列展示 ID/TG用户ID/用户名(@前缀)/昵称/语言/绑定账号(0=未绑定)/状态开关/首次互动/最近活跃;编辑弹窗仅 tg_user_id/username/first_name 只读,可改 bind_user_id/lang/status。用户由互动自动创建,故无「添加」按钮。
- **验证**:`npm run build` 通过、GetProblems 无错。纯前端,Ctrl+F5 生效。

### 2026-09-30 · 功能 · 后台可主动给指定 TG 用户发送消息
- **背景**:用户要求在「TG用户」列表页能主动给某个用户推送消息。
- **后端**(`tgBotRuntime.go`):先把 `send(chatID, res)` 改为 **返回 error**(原调用方忽略返回值不影响编译),以便捕获发送失败(如用户未启动会话/拉黑 bot 的 403)。新增 `(*TgBotRuntime).SendUserMessage(c)` gin 处理器:入参 `{id: tg_user 行 id, text, image?, format?}`,按 id 从 `tg_user` 取 `chat_id`(为 0 回落 `tg_user_id`),构造 `TgResult` 调 `send`,失败回 `{code:502,message:"发送失败:..."}`。放运行时而非 TgController 是因为发送需 `r.bot`。`route.go` 注册 `boss.POST("/SendTgUserMessage", tgBot.SendUserMessage)`。go build/vet 通过,**需重启后端**。
- **前端**:`api/tgUser.js` 加 `sendTgUserMessage`;`views/tgUser/index.vue` 操作列加「发消息」按钮(width 90→150)+ 发送弹窗(接收用户只读、消息内容 textarea、配图地址选填、富文本开关默认开),`openSend/doSend` 提交。`npm run build` 通过、GetProblems 无错。
- **限制**:Telegram 硬性要求用户先发起过会话 bot 才能回推;从未互动/已拉黑的用户会返回发送失败。
- **追加(同日)·改为群发**:用户要求①发送支持群发;②「发消息」按钮从操作列移到工具栏「删除」右边;③必须勾选用户才能发。后端 `SendUserMessage` 入参改为 `ids`(逗号串),`tools.SplitIds` 解析 + 批量查 chat_id + 逐条 `send`,汇总 `{ok, failed:[用户名(原因)...]}` 返回(新增 `tgbot/tools` 导入)。前端删行内按钮(操作列宽回 90px),工具栏新增「发消息」(`openSend` 无参、先校验 `multipleSelection` 非空),弹窗「发送对象」改显「已勾选 N 位用户」,`doSend` 传 `ids.join(',')`,有失败项用 `$notify` 列失败名单。go build/vet + npm build 均通过、GetProblems 无错。**含后端改动,需重启后端**。
- **追加(同日)·发送框复用菜单页富文本编辑器(纯前端)**`views/tgUser/index.vue` 将普通 textarea 换为与 `tgMenu` 同一套 `.rich-bar` 工具栏(B/I/U/S/代码/剧透/链接 + 👁预览)+ `ref="richSend"` textarea;新增 `_richEl/insertTag/insertLink` 方法(操作 `sendForm.text`,与菜单页逐字对齐仅数据源不同)+ `previewVisible` 与富文本预览弹窗 + `.rich-bar/.rich-preview/.form-tip` 样式。与菜单页一致去掉「富文本」开关、固定 `format:'html'`(`sendForm` 移除 `html` 字段),弹窗宽度 560→680px 容纳工具栏。后续又把消息内容 textarea `rows` 5→10。`npm run build` 通过、GetProblems 无错,Ctrl+F5 生效。
- **追加(同日)·分页改用短剧后台 `.currentPage` 风格(纯前端)**:`views/tgUser/index.vue` 将内联样式的 `<el-pagination>` 改为包在 `<div class="currentPage">` 容器里(去掉 inline `margin-top/text-align`);`App.vue` 补上全局桌面规则 `.currentPage { margin-top:15px; text-align:center; }`(与短剧后台一致——分页整条居中;此前只有手机媒体查询的 `.currentPage .el-pagination` 居中换行规则、无页面命中,现在 TG用户页接入后桌面居中、≤768px 自动居中换行)。`npm run build` 通过、GetProblems 无错,Ctrl+F5 生效。
- **追加(同日)·分页风格照搬 shortplay(纯前端)**:用户指出本地就有 shortplay 项目(`D:\GoLand\shortplay\web\admin`),要求直接照搬其风格。读源码发现:shortplay 的 `App.vue` 与本项完全一致、**桌面端根没有 `.currentPage` 尺寸规则**(只有 ≤768px 媒体查询);其分页就是默认 Element 尺寸,容器用内联样式 `<div style="margin-top:10px; text-align:center;" class="currentPage">` + 无 size 的 `el-pagination background layout="total, sizes, prev, pager, next, jumper"`。据此:**删掉上一步刚加的 `.currentPage` 桌面居中规则与 32px/14px 尺寸放大规则**(恢复 App.vue 与 shortplay 一致),`views/tgUser/index.vue` 分页容器改为与 shortplay 逐字一致的内联样式写法。之前“TG 看着比短剧小”实为短剧有 36 页、按钮多而显饱满,TG 只 1 页显得稀疏,控件尺寸本就相同。`npm run build` 通过、GetProblems 无错。
- **追加(同日)·卡片满窗口对齐 shortplay(纯前端)**:用户要求 TG用户页卡片像 shortplay 那样铺满窗口。根因:`views/tgUser/index.vue` 的 `el-table` 高用 `calc(100vh - 250px)`,比 shortplay `drama/index.vue` 的 `calc(100vh - 182px)` 矮 68px,导致卡片底部留出灰色空隙。改为与 shortplay 一致的 `calc(100vh - 182px)`(两页结构相同:queryForm 头部 + toolbar + table + 分页,故同一偏移量适用)。`npm run build` 通过。注:本项其他列表页仍按统一规范 `calc(100vh - 210px)`,TG用户页按用户要求单独对齐 shortplay。
- **追加(同日)·卡片高度微修:182px 出现滚动条 → 200px(纯前端)**:用户反馈改 182px 后页面出现纵向滚动条。原因:shortplay 的 182px 不适配 TG——TG 工具栏比 shortplay 多一个「发消息」按钮 + 行尾提示 span,非表格区高度实测≈187px(而 shortplay≈175px),所以 182px 溢出~5px 触发滚动条、250px 又留~63px 空隙。改 `el-table` 高为 `calc(100vh - 200px)`(略大于实际 chrome≈187px,既消滚动条又接近满窗)。`npm run build` 通过。【最终用户定值 `calc(100vh - 192px)`】
- **追加(同日)·注入 100 条演示 TG 用户数据(纯数据,无代码变更)**:用户要看分页/群发效果。因本机无 mysql 客户端,沿用项目既有模式:写临时 `controller/zz_seedusers_test.go`(用 `config.Mysql` 自动初始化连接),`go test -c` 编译为 exe 后从项目根目录执行(PowerShell 下参数用等号形式并整体加引号 `& '.\xxx.exe' '-test.run=TestSeedTgUsers' '-test.v'` 避免被拆分)。向 `tg_user` 插入 100 行演示数据(tg_user_id 基址 7000000000+i 避开真实用户 7569435732;username/first_name/lang/bind_user_id/status 均有变化,含部分空用户名/未绑定/停用行),脚本开头先按 id 段删除旧演示行保证幂等。执行 PASS,tg_user 现 101 行。临时 test 文件与 exe 已删除并复查无残留。纯数据改动,后台刷新即可看到(无需重启后端)。
- **追加(同日)·列表页卡片高度统一为 `calc(100vh - 192px)`(纯前端)**:用户要求以后全部菜单卡片统一用此高度。把 `views/tgCommand/index.vue`、`views/tgMenu/index.vue` 的 `el-table` 高从旧的 `calc(100vh - 210px)` 改为 `calc(100vh - 192px)`(tgUser 已是 192px),作为全项目新标准(已同步更新记忆)。注:tgMenu 为树形表、无分页,192px 会在原分页位置留少量空白,属预期(统一数值优先)。`npm run build` 通过。
- **追加(同日)·TG用户页工具栏高度对齐机器人菜单(纯前端)**:用户发现 TG用户页与机器人菜单页的工具栏区域高度不一致,要求按机器人菜单风格。根因:`views/tgUser/index.vue` 的 scoped 样式多了一条 `.toolbar { margin-bottom: 10px }`,叠加全局 `.toolbar > .el-button { margin-bottom: 10px }` 后工具栏下方共 20px;而 `tgMenu` 无这条 scoped 规则(只靠全局 10px)。删除 tgUser 的 scoped `.toolbar` 规则,两页工具栏间距一致。`npm run build` 通过、Ctrl+F5 生效。
- **规范固化·后台页面风格必须一致**:用户明确要求以后新增的任何菜单/表格/卡片页面风格都与现有页统一。要点:①结构=queryForm 搜索栏 header + .toolbar 按钮行 + el-table(统一 `calc(100vh - 192px)`) + 可选 .currentPage 分页;②**间距只靠 App.vue 全局规则,禁止在单页 scoped 重定义 .toolbar/.queryForm/.el-card**(本次不一致即因 tgUser 多一条 scoped `.toolbar{margin-bottom:10px}`);③每页带齐 添加/编辑/删除。已同步写入长期规范记忆。
- **追加(同日)·富文本工具栏新增「⏎分段」按钮(纯前端)**:用户反馈 TG 里收到的一长段文字行距太紧。根因:Telegram 的 HTML 不支持 `line-height`/CSS,行距由客户端固定无法调;唯一能拉开段落间距的办法是插空行(`\n\n`)。于是在富文本工具栏「🔗链接」与「👁预览」之间加一个「⏎分段」按钮:新方法 `insertBreak(refName)` 在光标处插入 `\n\n`(不删除选区、不弹框)。tgMenu 三处工具栏(richMenu/richText/richHttp,写回 `cfg.text`) + tgUser 发消息工具栏(richSend,写回 `sendForm.text`)均已加,保持两页一致。`npm run build` 通过、GetProblems 无错、Ctrl+F5 生效。
- **追加(同日)·卡片高度回调至与短剧一致的 `calc(100vh - 182px)`(纯前端)**:上一步删掉 tgUser 多余的 scoped `.toolbar{margin-bottom:10px}` 后,其非表格区从≈187px 回落到≈175px,与短剧 `drama` 页一致。于是把 tgUser/tgCommand/tgMenu 三页 `el-table` 高从 192px 统一改为短剧同款 `calc(100vh - 182px)`,作为新的全项目标准(已更新记忆:标准 182px;并修正旧 pitfall 记忆——当初 182px 溢出的真因是那条多余 scoped 边距,非按钮数量)。`npm run build` 通过、Ctrl+F5 生效。
- **追加(同日)·TG用户分页每页条数持久化(纯前端)**:用户反馈选了 100/200 条一页后刷新又变回 20。在 `views/tgUser/index.vue` 用 localStorage 持久化 limit:data 初始化 `limit: Number(localStorage.getItem('pageSize_tgUser')) || 20`,`onSizeChange` 里 `localStorage.setItem('pageSize_tgUser', size)`。约定 key 命名 `pageSize_<路由名>`。(tgCommand 无分页、一次全量,无需处理)。`npm run build` 通过、Ctrl+F5 生效。
- **追加(同日)·发送完清空勾选(纯前端)**:用户要求群发发送完后把勾选的用户清掉、重新来。在 `views/tgUser/index.vue` 的 `doSend` 成功分支(`code===0`,含部分失败)关闭弹窗后补:若 `this.$refs.table` 存在则 `clearSelection()`、`multipleSelection = []`、重置 `sendForm`。(失败分支 `code!==0` 保留勾选供重试)。`npm run build` 通过、Ctrl+F5 生效。
- **追加(同日)·发图改为后端下载+上传,修复国内 CDN 图发不出(后端)**:用户反馈某张 `static-web.stcn.com`(证券时报阿里云 OSS、纯 `cn` 边缘节点)的图发不出去。日志实错:`tg send photo 失败,回退文本: Bad Request: failed to get HTTP URL content`——本机 curl 直测该 URL 正常(200/image/png/141KB),但 `sendPhoto` 是把 URL 交给 **Telegram 海外服务器去拉**,它访问不到国内 CDN → 失败后回退纯文本(所以文字发出去了、图没了)。方案:后端先把图下载成字节、再用 `tg.FileBytes{Name,Bytes}` 以 multipart 上传给 Telegram,不再依赖 TG 抓 URL。改动(`controller/`):①`TgResult` 加 `ImageData []byte/ImageName string`;②新增 `imgHTTPClient`(强制直连 `Proxy:nil`——国内图不能走 Telegram 的海外代理、带浏览器 UA、15s 超时)与 `downloadImage`(限 10MB、校验 2xx/非空)与 `imageNameFromURL`;③`send` 发图分支改 `tg.NewPhoto(chatID, r.photoFile(res,img))`,`photoFile` 优先用预下载字节、否则现下、都失败回退 `FileURL`(保留原行为兑海外可达图);④`SendUserMessage` 群发在循环外 `downloadImage` 一次、把字节复用到每个 `res`(避免 N 用户下 N 次)。验证:临时 `controller/zz_imgtest_test.go` 实跑 `downloadImage` 该 URL → PASS(144855 字节、文件名正确),临时文件与 imgtest.exe 已删并复查无残留;`go build ./...` 通过。**需重编译后端生效**(GOOS=windows)。
- **追加(同日)·补盖 editMessage 原地换图路径(后端)**:用户问接口取数等其他地方图是否也走新方法。查发现发图共两条路径:①`send`(新发消息,含菜单页/横幅/命令触发/等待输入结果/群发、以及 editMessage 形态不一致时的删旧发新)已走 `photoFile`✓;②但 `editMessage` 的「当前已是图片→再换成图片」分支(L445)仍用 `tg.FileURL`✗。已改为 `tg.NewInputMediaPhoto(r.photoFile(res, img))`(核实:v5.5.1 的 `prepareInputMediaFile` 对 `InputMediaPhoto` 且 `NeedsUpload` 会按 `file-{idx}` 上传、param 指向 `attach://`,即 editMessageMedia 支持上传字节)。至此所有发图路径统一走「后端下载+上传、失败回退 URL」。`go build ./...` 通过。接口取数/http 配图、菜单横幅、text 回复等只要带图都受益。**需重编译后端生效**。
- **新增(同日)·图片库(file_id 复用机制)上线**:用户想提前上传一批图片、后续与文本一起发送。核心决策——库不存图片文件/URL,而是存 Telegram 的 **`file_id`**:一张图成功上传过一次即得到 file_id,之后任意 chat 用 `tg.FileID(id)` 可直接复用同一张图(秒发、不占带宽、彻底不受“海外拉不到国内图”限制)。全链路实现:
  - **建表**(项目无 AutoMigrate,用临时 Go 程序 `CREATE TABLE IF NOT EXISTS` 对远程库执行、跑完即删):`tg_image(id,name,tag,tg_file_id,mime,size,source_url,status,created_at,updated_at)`。
  - **配置**:`config.yaml` 新增 `telegram.imageRelayChatID`(中转 chat,入库时把图发给它、读回 file_id 再删该消息;默认填了开发者本人 chat_id)。
  - **后端** `controller/tgImageController.go`(方法挂 `TgBotRuntime`,需 bot+db):`GetTgImageList`(分页/名称模糊/分类精确)、`AddTgImage`(本地 base64 或 URL 二选一→ `uploadToRelay` 换 file_id→ 落库)、`SaveTgImage`(改名/分类/状态)、`DelTgImage`、`GetTgImagePreview`(bot.getFile→ 下 Telegram 文件服务器→ 转 base64 供后台缩略图)。路由均挂 `/api/boss`(见 `route/route.go`)。
  - **发送端**(`tgBotRuntime.go`):`safeImage` 放行 `file:` 前缀引用;`photoFile` 遇 `file:<id>` 直接 `tg.FileID`。由此 菜单横幅/text回复/命令/群发/editMessage换图 全线自动支持库图(群发因 `validTgURL(file:)` 为 false 天然跳过下载,零成本)。
  - **前端**:新页 `views/tgImage/index.vue`(套用统一列表页规范:queryForm+toolbar+el-table 182px+分页 pageSize_tgImage 持久化;表格缩略图懒加载);可复用选择器 `components/ImagePicker.vue`(网格选图→ emit `file:<id>`);接入 tgMenu(menu横幅+text回复两处 cfg.image)、tgCommand(两处 cfg.image)、tgUser(群发 sendForm.image)各加「图片库」append 按钮 + `<ImagePicker>`。api 封装 `api/tgImage.js`。侧栏/路由加“图片库”入口。
  - **局限**:file_id 绑定当前 botToken,换 bot 需重新入库;`sendPhoto` 会被 TG 压缩(banner/配图够用)。**需重编译后端 + 重新构建前端生效**。`go vet`/`npm run build` 通过、GetProblems 无错。
- **新增(同日)·系统设置(运行时配置存库、后台改完即生效)**:背景——本地 config.yaml 改起来不方便、改了要重编重启。做法——新增通用 key-value 表 `sys_setting`(skey 唯一/svalue/name/remark/updated_at),把**运行时才读取的配置**下沉到数据库:
  - **后端**:`controller/common.go` 新增 `settingGet(db,key)`(取字符串值)与 `settingInt64(db,key,cfgKey)`(优先 sys_setting→无值回落 config.yaml);`controller/sysSettingController.go`(挂 TgController) GetSysSettingList/AddSysSetting(skey 唯一体检)/SaveSysSetting(按 id,skey 不得与他项重复)/DelSysSetting;`route/route.go` boss 组注册 4 路由;入参走 `onlyFields` 白名单。首条迁入:图片库中转 chat_id——`uploadToRelay` 改为 `settingInt64(r.db,"imageRelayChatID","telegram.imageRelayChatID")`,后台改了立即生效;config.yaml 仍保留作兵底默认。
  - **前端**:`api/sysSetting.js` + `views/sysSetting/index.vue`(**固定参数表单页**,非可增删的 CRUD 列表——因配置项是程序预定义的固定参数,不让运营手动加键;仿“站点设置”风格:一个 el-card + 若干固定字段 + 底部单个「保存」按钮)。页面由 `fields` 注册表驱动(input/textarea/number/radio 四种控件),以后加固定参数=往 `fields` 数组加一条(与后端 skey 对齐即可)。后端配套 `GetSysSettingMap`(一次返回 key→value 映射回填)与 `SaveSysSettingBatch`(按 skey upsert、一次提交多项;已存则只更新 svalue)。`router/index.js` 加 /sysSetting 路由、`layout/sidebar.vue` 加「系统设置」(el-icon-setting)。
  - **边界**:适合存库的是“发送/运行时才读”的参数;启动引导阶段就要用的(MySQL 连接/botToken/webhook 路径与密钥)仍留 config.yaml(搬这些需做“运行时重建 bot”,复杂且易错)。
  - **验证**:`go build ./...` BUILD OK;`go vet ./...` VET OK;`npm run build` DONE;GetProblems 无错;临时建表程序已删、无残留。**需重编译后端 + 重新构建前端生效**。
- **修复/规范(同日)·图片库列表页 UI 精简与复制修复**(`views/tgImage/index.vue`,纯前端):
  - 删除“分类/tag”相关 UI(搜索表单项/表格列/添加弹窗字段/编辑弹窗字段)——实际无用;后端 tag 列保留不动(兼容)。
  - 引用值列加宽(min-width 200→280px),改为 el-tooltip 鼠标移入展示完整 `file:<id>`(popper 内容 word-break 换行、max-width 520px,悬停可停留);行内截断阈值 20→40 字。
  - 操作列“复制引用”由 text 链接改为与“编辑”并排的 mini 按钮(列宽 120→180px,之前因窄导致换行堆叠)。
  - **修复复制失效**:原 `navigator.clipboard.writeText` 在 HTTP 非安全上下文(如 192.168.x.x:8080 局域网访问)下不可用。新增 `doCopy`/`fallbackCopy`:安全上下文用 clipboard,否则回退隐藏 textarea + `document.execCommand('copy')`,再失败则 $prompt 手动复制。
  - **美化“添加图片”弹窗**:上传拖拽区整宽加高(172px、圆角、悬停高亮、大图标+副标题提示格式/大小);选中后展示带缩略图+文件名+“重新拖入可更换”提示的预览卡片;URL 栏加 link 前缀图标与用途说明;名称字段 label-width 70→60px 对齐;按钮文案“确 定 入 库”→“确定入库”。新增 scoped 样式(上传区用 `.uploader >>> .el-upload-dragger` 穿透)。
  - **添加弹窗支持多图**:宽度 560→720px。本地上传加 `multiple`,选中图片存 `localFiles:[{name,base64}]`,下方缩略图网格(单张右上角 × 可删、顶部“已选 N 张/清空”);URL 导入改 textarea 支持每行一个批量。`doAdd` 改为循环逐张调 `addTgImage`(本地传 name=文件名+image_base64,URL 传 url),汇总成功/失败计数、失败项用 `$notify` 列出。**后端零改动**(AddTgImage 本就单张处理、name 为空时取传入文件名);去掉不适配多图的“名称”单输入框(名称自动取文件名,入库后可逐行编辑改名)。
  - **本地上传前端压缩到 3MB(保持清晰度)**:`onLocalFile` 改为对每张图调 `compressImage`(canvas):最大边先降到 2048px(与 TG 相册压缩上限一致),质量从 0.9 保守递减(不低于 0.8),仍超限则优先按比例降尺寸而非猛压质量,避免失真;统一输出 JPEG(白底铺透明区,与 Telegram `sendPhoto` 行为一致不额外损失)。压缩后 `dataUrlBytes` 估算仍 >3MB 则跳过并提示。上传副标题同步改为“自动压缩到 3MB 以内”。**后端零改动**(`AddTgImage` 已会剥 `data:...;base64,` 前缀)。URL 导入走后端下载暂不压。验证:`npm run build` DONE、GetProblems 无错。
  - 验证:`npm run build` DONE、GetProblems 无错。
  - **缩略图加载前用占位块代替文字**:列表页 `tgImage/index.vue` 将 `_thumb` 为空时的“加载中…”文字改为 `.thumb-ph` 占位块(72×72 灰底圆角 + `el-icon-picture-outline` 居中图标,尺寸与真图一致避免布局跳动);`components/ImagePicker.vue` 选择器网格内 `…` 同步改为同款图标(复用已有 `.ip-thumb` 灰底)。纯前端,Ctrl+F5 生效。
  - **修复引用值列“不满列”（大片空白）**:根因是 el-table 仅 `min-width` 列为弹性列会吸收剩余宽度,而该列内容是截断到 44 字的短串撑不满。将“引用值”列由 `min-width=280px` 改为固定 `width=420px`,让内容更长的“名称”列(唯一弹性列)去吸收多余宽度,表格分布均衡。
  - **本地上传自动重命名为简短名**:原名称直取本地文件名(多为长哈希/UUID)。新增 `genShortName()`,`onLocalFile` 改用它——丢弃原始名,生成 `img_月日_时分秒_序号.jpg`(序号由 `uploadSeq` 按会话自增、`openAdd` 重置为 0),同批多张不撞名。网格顶部提示同步改为“已自动重命名为简短名称”。仅影响新入库本地上传;URL 导入名仍由后端从链接派生(要改需动后端),旧记录可在列表逐条“编辑”改名。验证:`npm run build` DONE、GetProblems 无错。
  - **修复图片选择器搜索需失焦才生效**:`components/ImagePicker.vue` 搜索框原绑 `@change="load"`,而 el-input 的 change 是失焦才触发。改为 `@input`(防抖 400ms 自动搜)+ `@keyup.native.enter`/`@clear`(立即搜),新增 `onKwInput`/`doSearch`(搜索时重置 page=1)。纯前端,Ctrl+F5 生效。
  - **图片选择器改为每行 5 个**:原 `.ip-cell` 固定 `width:150px`,在 820px 弹窗里 5 个(5×150+4×12=798)超宽被挤成 4 列、右侧空一条。改 `width: calc((100% - 48px)/5)` + `box-sizing:border-box` 自适应正好 5 列(gap 12px×4=48px);`limit` 12→15(3 行铺满)。
  - **收紧“添加图片”弹窗顶部留白**:根因是全局 `App.vue` 的 `.el-dialog__body{padding:30px 20px 10px !important}` 给内容区加了 30px 顶边距,而此弹窗首行就是 tabs→顶部空一条。按用户要求**不改全局**,仅在 `tgImage/index.vue` scoped 样式用 `.add-dialog >>> .el-dialog__body{padding-top:14px !important}` 高特异性覆盖(只影响这一个框)。`class="add-dialog"` 落在 el-dialog 的 wrapper 上(与全局 `.responsive-dialog .el-dialog` 选择器一致)。

### 2026-09-30 · 功能 · 机器人被拉进群组后往群组发消息(自动捕获群组 + 后台选群群发)
- **背景**:用户新需求——bot 被拉进 TG 群组后要能往该群组发消息。经确认方案:后台手动选群推送 + 复用富文本配图 + 自动捕获群组(无需手动录入)。
- **关键发现**:发送层 `send(chatID, res)` 本就对任意 chat_id 通用(私聊/群组/频道同一条路),真正缺的是①拿到并记住群组 chat_id、②后台选群入口。且长轮询/webhook 均未限制 `allowed_updates`,默认订阅**已含 `my_chat_member`**——bot 被拉进群/被踢的事件其实早已推达服务,只是在 `handleUpdate` 里只处理了 Message/CallbackQuery 被丢弃。
- **建表**(临时 Go 程序 `tmpcmd` 跑 `CREATE TABLE IF NOT EXISTS` 对远程库执行、跑完即删,无 AutoMigrate):`tg_chat(id,chat_id UNIQUE,title,username,type,status,created_at,updated_at)`;`status` 1=bot 在群可推送、0=已离开/失效。
- **后端捕获**(`controller/tgBotRuntime.go`):`handleUpdate` 加 `case up.MyChatMember != nil` 分支→ 新增 `onMyChatMember(cmu *tg.ChatMemberUpdated)`:仅记录 group/supergroup/channel(私聊跳过),按 `cmu.NewChatMember.Status`(left/kicked/restricted→0,否则→1)判定在群状态,调 `ensureChat(chatID,title,username,type,status)` upsert 落库(存在则刷新群名/状态)。
- **后端接口**:仿 TG用户——`controller/tgController.go` 加 `tgChatCols` 白名单 + `GetTgChatList`(按 title/username 模糊、type 精确,order by status desc,id desc)/`SaveTgChat`(改 status)/`DelTgChat`;`controller/tgBotRuntime.go` 加 `SendChatMessage`(仿 `SendUserMessage`,入参 `{ids,text,image?,format?}`,批量查 `tg_chat` 的 chat_id 逐个经 `send` 推送,配图循环外 `downloadImage` 一次复用,汇总 `{ok,failed:[群名(原因)]}`;失败常见于 bot 已被移出群/无发言权限 403/429)。
- **路由**(`route/route.go`):boss 组注册 `GET /GetTgChatList`、`POST /SaveTgChat`、`GET /DelTgChat`(挂 tg)+ `POST /SendTgChatMessage`(挂 tgBot,需 bot)。
- **前端**:新页 `views/tgChat/index.vue`(仿 tgUser:queryForm[群名+类型下拉]+toolbar[删除/发消息/刷新]+el-table 182px[selection/ID/ChatID/群名/类型tag/状态tag/首次捕获/最近更新]+分页 pageSize_tgChat;发消息弹窗复用菜单页同一套 `.rich-bar` 富文本工具栏 + ImagePicker 选图 + 预览);`api/tgChat.js`(getTgChatList/saveTgChat/delTgChat/sendTgChatMessage);`router/index.js` 加 /tgChat(TG群组)、`layout/sidebar.vue` 加菜单项(el-icon-s-custom)。群组列表页无逐行编辑弹窗(群组信息随事件自动同步),status 以标签只读展示。
- **验证**:`go build ./...` + `go vet ./controller ./route` 通过、`npm run build` DONE、GetProblems 无错。**含后端改动,需重编译重启后端生效**;前端 Ctrl+F5。**已实机验证通过**:真机把 bot 拉进群「海外短剧群组」→ my_chat_member 事件自动落库(chat_id=-5361658306、类型群组、状态在群可推送),后台选群→富文本+图片库配图群发成功(图在上文在中)。

### 2026-09-30 · 优化 · 图片库预览加本地磁盘缓存(避免每次刷列表都全量回源 Telegram)
- **背景**:图片库只存 `file_id` 不存图,预览接口 `GetTgImagePreview` 每次都要 `bot.GetFile` + 从 `api.telegram.org/file/bot...` 现下载原图字节转 base64,列表一刷就全量重拉,又慢又耗带宽。经讨论确定用**本地磁盘缓存**(而非 Redis:缓存的是几百 KB~3MB 的图片 blob,Redis 纯内存不划算,且当前是单实例 Go 进程用不上共享缓存的价值)。
- **关键前提**:`file_id` 内容永久不变 → 天然无「图更新了缓存还是旧的」脏缓存问题;预览又是「先按 id 查库拿 file_id、查不到即 404」,故删图后请求在查库这步就被挡掉、根本走不到缓存,正确性不受残留影响,缓存只是省带宽的性能层。
- **新增** `controller/tgImageCache.go`(均属 package controller、未导出):`imageCacheDir()`(优先 `config telegram.imageCacheDir`,默认 `./data/imgcache`)、`imageCacheName()`(file_id→sha256 hex 作文件名,规避非法字符/超长)、`imageCachePath()`、`imageCacheGet()`(读失败/空→未命中)、`imageCachePut()`(尽力而为,失败只记日志不影响主流程)、`imageCacheEvict()`(按 file_id 批量删缓存文件)、`imageCacheSweep(db)`(启动清扫:先把库里所有 file_id 算成哈希集合,再遍历目录删「库里已无」的孤儿缓存,防磁盘无限累积)。
- **接线**:`GetTgImagePreview` 命中缓存直接返回 base64(无需 bot 在线)、未命中才回源下载并 `imageCachePut`;`DelTgImage` 删库前先 `Pluck tg_file_id`、删库后 `imageCacheEvict(fileIDs)` 同步清缓存;`NewTgBotRuntime` 里加 `go imageCacheSweep(r.db)`(带 recover、非阻塞)启动清扫。`config.yaml` telegram 段加可选项 `imageCacheDir: "data/imgcache"`(留空走默认)。
- **验证**:`go build ./...` + `go vet ./controller` 通过、GetProblems 无错(IDE 那几条「无法解析表」是 GoLand SQL 方言缓存误报、非编译错)。**含后端改动,需重编译重启后端生效**;首次刷列表仍回源下载并落盘,之后刷新即命中缓存不再打 Telegram。

### 2026-09-30 · 优化 · 图片库入库期间锁死弹窗(禁止中途关窗/取消/重复提交)
- **背景**:之前点「确定入库」后只有确定按钮转圈,用户仍可点取消/右上角 X/按 ESC 关窗,或在弹窗里继续切 tab、加图——多图逐张入库耗时较长,中途关掉会留下半截状态。
- **改动**(`web/admin/src/views/tgImage/index.vue`):入库中(`adding=true`)时——弹窗 `:show-close="!adding"` 隐右上角 X、`:close-on-press-escape="!adding"` 禁 ESC(`:close-on-click-modal="false"` 本就禁点遮罩);内容区包一层 `<div class="add-body" v-loading="adding">` 遮罩阻断对 tab/上传区/缩略图的交互并显示 `正在入库 i/N` 进度;取消按钮 `:disabled="adding"`;`doAdd` 首行加 `if (this.adding) return` 防重提交,新增 `addTotal/addDone` 计数驱动进度文案。
- **验证**:`npm run build` DONE、GetProblems 无错。纯前端改动,浏览器 Ctrl+F5 即生效,无需重启后端。

### 2026-09-30 · 优化 · 图片库上传缩略图网格固定高度(选图多时内部滚动,不撑高弹窗/页面)
- **背景**:`.up-multi-grid` 是无高度限制的 flex-wrap 网格,选几十张图就一直往下堆,把弹窗/浏览器撑出页面级滚动条。
- **改动**(`web/admin/src/views/tgImage/index.vue` scoped 样式):`.up-multi-grid` 加 `max-height:320px; overflow-y:auto; align-content:flex-start`,并 `padding-top:8px` 防止每张图左上角红色 ✕ 角标(`top:-7px`)被容器顶部裁掉;附 `::-webkit-scrollbar` 细滚动条样式。弹窗自身高度不再随选数变化,多出的图在网格内滚。
- **验证**:`npm run build` DONE、GetProblems 无错。纯前端改动,Ctrl+F5 即生效。
- **后续修正(右侧空白)**:固定 88px 格子 + flex-wrap 会在行尾留一条 ~80px 空白(6 个 88px 铺不满 660px 容器)。改为 CSS Grid `grid-template-columns: repeat(auto-fill, minmax(88px, 1fr))` + `align-content:start`,`.up-mini` 宽 100%、el-image `width:100%`,每列 1fr 拉伸铺满整行,消除右侧空白(列数仍按容器宽自适应)。
- **后续修正(横向滚动条)**:改 1fr 拉伸后底部冒出横向滚动条。根因:el-image 有 `border:1px` 但未设 `box-sizing:border-box`,`width:100%` 再加左右边框 = 比格子宽 2px;且 `overflow-y:auto` 会使另一轴 `visible` 计算为 `auto`→ 触发横滚。修:el-image 加 `box-sizing:border-box`;网格显式 `overflow-x:hidden` 并把 `padding-right` 6px→12px(兼顾最右列 ✕ 角标 overhang 不被裁)。

### 2026-09-30 · 规范 · 数据库全表逐列 + 表本身加中文注释
- **背景**:用户要求给数据库所有表加中文注释(便于 GoLand/IDE 与运维阅读 schema)。共 8 张表:admin/sys_setting/tg_chat/tg_command/tg_handler/tg_image/tg_menu/tg_user。
- **做法**:MySQL 加列注释必须用 `ALTER TABLE ... MODIFY COLUMN`(需原样带上类型/NULL/默认值,写错会改坏列)。故先用临时 Go 程序 `show create table` dump 出真实列定义,再逐表生成一条合并 ALTER(多个 MODIFY + 末尾 `COMMENT='表注释'`),临时程序跑完即删。未用 AutoMigrate。
- **验证**:8 条 ALTER 均 `ok=8 err=0`;再查 `information_schema.columns/tables` 确认 68 列全有注释、8 表全有表注释。同步把附录 A 建表 SQL 逐列逐表补上 COMMENT并新增缺失的 tg_chat 建表段,保证新装库与线上一致。
- **注**:纯 DDL 元数据变更(只改注释、不动数据/类型),无需重编译重启后端。

### 2026-09-30 · 规范 · 索引补中文注释(命名索引 8 个)
- **背景**:上轮只加了列注释 + 表注释,索引注释未动。用户追问后查 `information_schema.statistics`,发现 16 个索引中 8 个命名索引全缺注释(另有 8 个 PRIMARY 主键)。
- **范围**:6 个唯一索引(uk_account/uk_skey/uk_chat/uk_cmd/uk_key/uk_tg)+ 2 个普通索引(idx_tag/idx_parent)全部补注释;8 个主键跳过——MySQL 不支持给 PRIMARY 设 COMMENT,且 `id 主键`本不言自明。
- **做法**:MySQL 无「只改索引注释」语法,只能同一条 ALTER 里 `DROP INDEX x, ADD [UNIQUE] INDEX x (cols) COMMENT '...'` 先删后建。表都很小(最大百余行),重建毫秒级、同语句内完成,安全。临时 Go 程序执行、跑完即删。
- **验证**:8 条 ALTER 均成功;再查 `information_schema.statistics` 确认 `NAMED_IDX=8 missing=0`。附录 A 建表 SQL 的索引也同步逐条加上 `COMMENT`(用各表独有的结尾 ENGINE 行做上下文,避开 §3 同名示意 DDL)。

### 2026-09-30 · UI · 图片库「引用值(file:)」列改自适应
- **背景**:用户反馈图片库列表「引用值(file:)」列写死 `width=420px` 太宽。
- **改动**:`web/admin/src/views/tgImage/index.vue` 该列 `width="420px"` → `min-width="180px"`,使其与「名称」列一样参与剩余空间自适应分配。注:该列单元格已内置 `el-tooltip`(悬停显示完整 file_id),故不加 `show-overflow-tooltip`避免两个 tooltip 冲突。
- **验证**:`npm run build` DONE + GetProblems 无错;纯前端,浏览器 Ctrl+F5 生效,无需重启后端。

### 2026-09-30 · 修复 · 图片库刷新时页面/滚动条上下抖动
- **现象**:刷新(或切页)时整页抖动、右侧滚动条上下跳。
- **根因**:`loadThumbs()` 一次性发 limit(最多100/200)个预览请求,每个响应回来都 `this.$set(this.tableData, idx, {...整行})` 替换行对象。el-table 见行引用变化→当新行重渲染 + 触发 `doLayout` 重算表体高度与滚动条槽宽。~100 个响应陆续到达 = 上百次重排 → 表格内滚动条 + 页面滚动条被反复重算,表现为抖动。
- **修法**:`_thumb` 在 getList 建表时已初始化为 `''`(响应式),改为**原地赋值** `this.tableData[idx]._thumb = ...`,只重渲染该单元格;占位块与真图同为 72x72 不改变行高→不触发 doLayout。另加 `_thumbGen` 代次标记 + 行 id 校验,防止快速刷新时上一批在途响应写错行。
- **验证**:`npm run build` DONE + GetProblems 无错;纯前端,Ctrl+F5 生效。
- **通用教训**:el-table 固定高度下,高频异步回写行数据时**绝不要替换整行对象**(会带 doLayout 风暴),字段建表时就初始化好、后续原地改属性。

### 2026-09-30 · 修复 · 命令菜单页富文本工具栏补「⏎分段」按钮
- **背景**:用户发现命令菜单(tgCommand)编辑弹窗的富文本工具栏少了分段按钮。根因:2026-09-30 新增「⏎分段」时只加了 tgMenu 三处工具栏 + tgUser/tgChat 发消息工具栏,命令页 tgCommand 两处(richCmd 菜单引导语 / richText 回复文案)漏加。
- **改动**:`web/admin/src/views/tgCommand/index.vue` — 两处工具栏均在「🔗链接」与「👁 预览」之间插 `⏎分段` 按钮;新增 `insertBreak(refName)` 方法(与 tgUser 一致,写回 `cfg.text`,光标处插 `\n\n`)。
- **验证**:`npm run build` DONE + GetProblems 无错;纯前端,Ctrl+F5 生效。现在 5 处文案字段(菜单 3 + 用户/群发消息 + 命令 2)分段能力全对齐。

### 2026-09-30 · 功能 · 系统设置页新增「机器人资料」(名称/简介同步)
- **背景**:用户问后台能否改机器人名称/头像/简介。结论:名称、简介可通过 Bot API 改;头像无接口只能 @BotFather 手动。用户要求加到系统设置页。
- **后端**:新增 `controller/tgBotProfile.go`。因 `go-telegram-bot-api v5.5.1` 未封装 `setMyName/setMyShortDescription/setMyDescription`,且其 `Config` 接口的 `method()/params()` 非导出、外部包无法实现(走不了 `bot.Request`),故用 `tgCall()` 直接对 `https://api.telegram.org/bot<token>/<method>` 发原生 HTTP POST(带环境代理 + 20s 超时,版本无关)。`SaveBotProfile`:每个非空字段先 upsert 到 sys_setting(botName/botShortDescription/botDescription)回显,再推 Telegram;**留空 = 不改动该项**(不写库也不推,防误清空),返回 ok/skipped/failed 汇总。`GetBotProfile`:getMyName/getMyShortDescription/getMyDescription 实时拉取,单项失败不影响其它。路由 `POST /SaveBotProfile`、`GET /GetBotProfile`(挂 tgBot)。
- **前端**:`views/sysSetting/index.vue` 在「中转 ChatID」卡片下方新增「机器人资料」卡片(名称≤64/短简介≤120/简介≤512 + 头像提示),回显复用同一次 `GetSysSettingMap`(无额外网络);「保存并同步到 Telegram」+「拉取当前」两按钮。`api/sysSetting.js` 加 `getBotProfile/saveBotProfile`。
- **验证**:`go build ./...` + `go vet ./controller` 通过;`npm run build` DONE。含后端改动,需重编译重启后端生效。GoLand 报的 `无法解析表 sys_setting` 是 SQL 方言检查误报(同写法早已存在于 sysSettingController),非编译错。
- **遗留**:头像无法后台修改(Telegram 硬限制),页面上已提示去 @BotFather。后续微调:头像提示文字 12px→14px(用户反馈太小)。
- **提示语修正**:用户反馈「简介」填了却没在资料弹窗显示。根因非 bug,是 Telegram 两字段显示位置不同——`setMyShortDescription`(短简介)=资料弹窗那行「简介」/拉群前说明;`setMyDescription`(简介)=仅在「与机器人尚无历史消息」的空聊天窗口作开场白,有消息后即不再出现、也不在资料弹窗。原 placeholder 误写「About 长简介」致误解,已改为准确措辞并在两字段下各加一行 tip 说明显示位置。后续:用户要求将该字段 label 从「简介」改为「欢迎语」(更贴合其开场白语义;底层仍对应 `setMyDescription`/skey `botDescription` 不变)。
- **更正 + 短简介改多行**:用户出示一个 bot 资料页截图(「简介」区为多行 @用户名列表)问怎么加。据官方文档核实:`setMyShortDescription` 明确「shown on the bot's profile page」——**截图那段多行文字就是短简介**,且**支持换行**(此前误称短简介单行不能换行,已纠正)。但后台短简介原本用单行 `el-input` 根本敲不进回车→改为 `type="textarea" :rows="3"`,placeholder/tip 说明可回车换行、@用户名会被 TG 自动识别为链接。后端 `tgCall` 用 `PostForm`+`url.Values` 传参,`\n` 会 url-encode 为 `%0A` 送达 Telegram,`TrimSpace` 仅去首尾不伤内部换行→多行可正常落地。同步修正 `tgBotProfile.go` 两条存库 remark 措辞(短简介=资料页简介可多行;欢迎语=空聊天窗口开场白可多行)。含后端改动,需重编译重启。
- **再微调**:用户要求欢迎语改回单行(去掉 textarea rows=4→普通 `el-input`,maxlength 仍 512),并删除短简介/欢迎语下方的两行使用说明 tip(页面保持简洁)。短简介保留 textarea rows=3(需多行)。纯前端,`Ctrl+F5` 生效。后续:又删除了「中转 ChatID」下方的说明 tip(去掉 `fields[].tip`,`v-if="f.tip"` 即不再渲染;remark 保留存库不显示)。
- **踩坑更正·误判为缓存**:用户反馈提示文字仍在,我却因只 grep 了已删的短简介/欢迎语/ChatID 三条 tip 文本(确实为 0 匹配)就断言「源码干净、是浏览器/dev server 缓存」,误导用户换浏览器、重启 serve。真相:用户指的是「保存并同步」按钮下方另一条 tip「留空表示不修改该项;Telegram 有缓存,通常几秒内生效」(L67),我从头到尾没删过它。教训:**用户说“删提示文字/使用说明”时,必须先 grep 页内全部 `setting-tip`  occurrence 逐条确认,不能只盯自己记得改过的那几条**;排查“改了没生效”前,先确认要删的目标到底在不在源码里,再怀疑缓存。已删除 L67 该 tip。页面尚余两条可见提示(用户未要求删、待定):L39「修改后立即生效,无需重启服务」、L62「头像无法通过接口修改…」。
- **样式微调**:短简介 textarea `:rows` 3→4(更高好编辑多行);头像提示文字由默认灰 `#909399` 改为醒目橙 `#E6A23C`(内联 style 加 color)。纯前端,`Ctrl+F5` 生效。后续:欢迎语由单行 `el-input` 改为 `type="textarea" :rows="2"`(可回车换行)。
- **分发层按 chat_id 分片并发(性能)**:背景——原 `consume` 单 goroutine 串行消费全局 `queue`,某条慢的 Telegram 网络调用会堵住后面所有消息。改动 `tgBotRuntime.go`:结构体 `queue chan` → `shards []chan *tg.Update`;新增 `enqueue(up)`/`shardIndex(chatID,n)`/`updateChatID(up)` 三个函数;`NewTgBotRuntime` 按 `server.shardCount`(默认 8)建 N 个分片队列并各起一个 `consume(shard)` goroutine;Webhook 与 startPolling 均改调 `r.enqueue`。**同一会话恒定路由到同一分片(片内串行保序)、不同会话跨片并行**;`shardIndex` 用 `uint64(chatID)%n` 归一化使负数 chat_id(群/频道)也稳定映射。分片共享状态仅 `awaitInputs`(已 mutex 保护)与 db/bot(并发安全)。`config.yaml` 新增 `server.shardCount: 8`。验证:`GOOS=windows go build ./...` 通过。
- **补分发层纯逻辑单测**:新增 `controller/tgBotRuntime_test.go`(package controller),表驱动覆盖 10 个不依赖 DB 行的纯函数:`splitCb`/`parseConfig`(string与[]byte、空/null/非法均回空 map)/`parseArgs`/`validTgURL`/`toInt64`/`buildKeyboard`(cols 行布局、空→nil、非法外链跳过)/`messageHasMedia`/`imageNameFromURL`/`shardIndex`(稳定性与越界)/`updateChatID`(各类 Update 取 chat_id)。**跑法**(因 config.init() 会连库且 viper 从当前工作目录读 config.yaml,不能直接 `go test ./controller`):`GOOS=windows go test -c -o controllertest.exe ./controller` 只编译 → 切项目根 `& '.\controllertest.exe' '-test.v'`(此时 ./ 即根能读到 config.yaml,DB 用现有远程库)→ 10/10 PASS → 删除二进制。
- **登录限流(安全)**:背景——`/api/boss/login` 无任何频率限制,config 里 `suddenCapacity` 是未接线的死配置。新增 `middleware/rateLimit.go`:零依赖内存**固定窗口限流器** `fixedWindowLimiter`(按 key=客户端 IP 计数,mutex 保护 + janitor  goroutine 周期清理过期条目防 map 无界增长);`LoginRateLimit()` 中间件超阈直返 429。阈值走 config:`security.loginMaxAttempts`(默认 10)/`security.loginWindowSec`(默认 60)。`route.go` 将 `/login` 改为 `boss.POST("/login", middleware.LoginRateLimit(), bossController.AdminLogin)`。`config.yaml` 新增 `security` 段。单机内存态,多实例部署各限各的(需全局一致可后续换 Redis)。附 `middleware/rateLimit_test.go` 验证计数/超限/独立 key/窗口重置,同根目录跑法 PASS。
- **优雅停机(运维)**:背景——原 `web.go` 直用 `router.Run`,进程被 kill 时在途请求断、DB 池不关。改为显式 `http.Server`(带 ReadHeaderTimeout/Read/Write/Idle 超时防 Slowloris),后台 goroutine `ListenAndServe`;`signal.NotifyContext(os.Interrupt, SIGTERM)` 监听退出信号,收到后 `srv.Shutdown(10s 超时 ctx)` 让在途请求收尾 → 关 DB 连接池 `config.Mysql.DB().Close()` → `LoggerClose()`。`ErrServerClosed` 视为正常关闭不 Fatal。bot 分片消费为守护 goroutine,随进程自然退出(遗留:如需排空分片队列可给 TgBotRuntime 加 Stop/Drain)。验证:`GOOS=windows go build ./...` exit=0,`go vet` 干净。
- **限流提示文案修正(前端)**:登录触发 429 时页面弹的是 axios 默认英文 "Request failed with status code 429",而非后端中文 message。根因:非 2xx 走 axios 错误拦截器,而 `login()` 是 async `await request.post` 直接透传 axios error(其 `.message` 为英文),各页面 `catch(e){ $message.error(e.message) }` 就显了英文。修法(集中、全页面受益):`api/request.js` 响应错误拦截器新增 `else` 分支——有 `error.response` 时用 `error.response.data.message` 覆盖 `error.message`。这样 429 会显示后端「登录尝试过于频繁，请稍后再试」(业务错 HTTP 200/code!=0 本就走 `new Error(res.data.message)` 不受影响)。纯前端,`Ctrl+F5` 生效。
- **测试文件处置说明**:上述 `controller/tgBotRuntime_test.go` 与 `middleware/rateLimit_test.go` 均为本次**临时验证用**(分别跑出 10/10 、限流 PASS 的结论),按项目所有者习惯——**一次性验证产物用完即删**,两个 `_test.go` 已删除(不影响 `go build`,仅保留已落地的业务代码)。以后如需回归保护可重新补回。
- **首页横幅引导语支持接口取数(功能)**:背景——用户希望 `/start` 首页横幅下方那段引导语不再写死,能调外部接口动态生成(图保持静态)。方案:引导语来源可选「静态文本/接口取数」,复用现有 http 引擎。后端——`tgHTTP.go` 抽出 `httpFetch(rawURL,method,headers,body)(interface{},bool)` 供 `httpResult` 与新 `bannerAPIText(cfg,u)(string,bool)` 共用(后者只取渲染后的文本,不涉 list_path 按钮/图);`tgBotRuntime.go` 新增 `resolveRootBanner(cfg,u)(text,image,html)` 统一处理根横幅两处渲染(`/start` 命令的 execAction menu 分支与回根 m:0 的 menuPageResult else 分支),`menuPageResult` 签名由 `(parentID,lang)` 改为 `(parentID,u *TgUser)` 以拿到会话上下文。**缓存**:接口结果按 `bannerCache map[string]bannerEntry`+`bannerMu` 缓存,ttl 走 `server.bannerCacheSec`(默认 30s);**key 含 chat_id + 接口签名**(模板可能含 {uid}/{first_name} 按用户变量,必须分用户缓存防串号;多命令配不同接口也不串、改配置即时失效)。接口失败回退静态 `cfg["text"]` 且不写缓存(下次重试)。前端 `tgCommand/index.vue`:根主菜单块新增「引导语来源」el-select + 接口模式下的 方法/地址/请求头/请求体/引导语模板/兜底引导语 字段(`text_source`+`api_*` 写入 action_config 扁平 map,静态模式不写保持旧数据干净),配 data/add/edit/onActionTypeChange/buildActionConfig/validate。config.yaml 新增 `server.bannerCacheSec: 30`。验证:`GOOS=windows go build ./...` exit=0、`go vet ./controller` 干净、`npm run build` 通过、GetProblems 无错。生效需重启后端 + 后台 Ctrl+F5。
- **首页横幅接口取数 UI 精简(前端)**:背景——用户反馈上条接口模式字段太长、且「引导语模板 + 兜底引导语」两个文本框冗余,要求复用统一模板只留一个框、部分字段做两列。改动(`tgCommand/index.vue`,纯前端):①删除「兜底引导语」textarea,接口模式仅保留单个「引导语模板」(`api_text`)框,与「机器人菜单」页 http 引擎的模板字段命名一致;接口失败由后端回退默认「请选择：」(tip 同步改写)。②「引导语来源」与「请求方法」并入 `el-row`(`:span=12`,静态模式来源占满 24);「请求头」与「请求体」并入 `el-row`(GET 时请求头占满 24,POST 时各 12)。`buildActionConfig`/`validate` 逻辑不变(`text` 仍序列化作隐性兜底)。验证:`npm run build` 通过、GetProblems 无错。Ctrl+F5 生效。
- **横幅取数改为实时 + 内置时间变量(功能增强)**:背景——用户参考同行钱包 bot 首页(昵称/ID/USDT·TRX·CNY 余额/中国时间)确认这就是接口取数用例,并要求“实时”与“时间”。改动:①**实时**——`config.yaml` `server.bannerCacheSec` 由 30 改为 **0**(不缓存,每次 /start 都现拉接口);`NewTgBotRuntime` 原 `if bannerSec<=0 {bannerSec=30}` 会把显式 0 兵底回 30,改为用 `config.Conf.IsSet` 区分“未配置(默认 30)”与“显式配 0(不缓存)”(<0 归 0);`resolveRootBanner` 加 `bannerTTL>0` 守卫,TTL=0 时完全绕过 `bannerCacheGet/Set`。②**时间**——`tgHTTP.go` `httpCtx` 注入内置北京时间变量 `ctx["time"]`(2006-01-02 15:04:05)/`{date}`/`{clock}`,用 `time.FixedZone("CST", 8*3600)` 不依赖服务器时区也不依赖 tzdata;菜单 http 与横幅共用同一 ctx,两处都可用。若接口自身返回时间字段,用路径写法 `{data.time}` 取(ctx 优先于 resp,但 `{time}` 与 `{data.time}` 键不同不冲突)。前端 `tgCommand/index.vue` 引导语模板 placeholder 与 tip 同步点出 `{time}` 与实时说明。验证:`GOOS=windows go build ./...` exit=0、`go vet ./controller` 干净、`npm run build` 通过。含后端改动,需重编译+重启生效;GetProblems 仅剩两条与本次无关的 IDE 对 `tg_chat` 表未 introspect 的 SQL 告警。
- **引导语模板接富文本工具栏 + 去提示(前端)**:背景——用户要求首页横幅的「引导语模板」也像其他文案框一样上富文本工具栏,并去掉它下方那行 tip。改动(`tgCommand/index.vue`):①因横幅有两个文本模型(静态 `cfg.text` / 接口 `cfg.api_text`),把 `insertTag/insertLink/insertBreak` 泛化为多一个 `field='text'` 参(默认 text 不影响旧调用),api_text 工具栏传 `'api_text'`;新增 `openPreview(field)` + `previewField` data,预览弹窗改渲染 `cfg[previewField]`(三处预览按钮均改调 openPreview)。②引导语模板 textarea 加 ref=`richTpl` 工具栏(rows 3→4 按文案框规范),删除下方 `form-tip` 那行。横幅 `format` 默认 html,富文本标签会随引导语一起生效(后端 resolveRootBanner 读 cfg["format"])。验证:`npm run build` 通过、GetProblems 无错。纯前端,Ctrl+F5 生效。
- **命令弹窗去 actionHint 说明行(前端)**:背景——用户要求去掉「触发行为/跳转目标」下方那行“命令触发后打开主菜单（九宫格）…”提示。改动(`tgCommand/index.vue`):删除模板中 `{{ actionHint }}` 的 `form-tip` 行,并连带删除仅服务于它的 computed `actionHint`(整个 computed 块)——与 tgMenu 之前处理一致,两页现均无动作说明行。验证:`npm run build` 通过、GetProblems 无错。纯前端,Ctrl+F5 生效。
- **横幅接口配置块布局调整(前端)**:背景——用户要求「接口地址＋请求头」并列一行、「请求体」单独整行(不再与请求头同排)。改动(`tgCommand/index.vue` 纯前端):接口地址与请求头各 `el-col :span=12` 入 `el-row`;请求体改为独立整行 `el-form-item`(`v-if` POST 才显)。后续又按用户要求把接口地址由单行 `el-input` 改为 `type=textarea :rows=2`(与请求头对称)。验证:`npm run build` 通过、GetProblems 无错。Ctrl+F5 生效。
- **修复横幅两 bug:切换来源丢接口配置 + 静态文本占位符不生效(功能修复)**:背景——用户发现①引导语来源切到「静态文本」保存后,再切回「接口取数」接口配置全丢;②静态文本里写的 {time}/{first_name} 等占位符发到 TG 原样输出不替换。根因:①前端 `buildActionConfig` 仅在 text_source==='http' 时才写 api_* 字段,静态模式一保存就把接口配置从 action_config 抹掉;②后端 `resolveRootBanner` 静态分支直接返回 cfg["text"],从不走模板渲染。改动:①`tgCommand/index.vue`——menu 类型**始终写入** text_source + 全部 api_*(后端只在 text_source==='http' 才读,静态下存着无害);②`tgBotRuntime.go` resolveRootBanner——静态 text 非空且有用户上下文时过 `httpRender(text, httpCtx(u,nil), nil)`,支持内置变量({uid}/{first_name}/{time} 等);resp 传 nil 故 {data.xxx} 响应路径取不到(变空串),占位取数仍须接口模式(用户截图静态文本里的 {data.time} 属误用,应改 {time})。验证:`GOOS=windows go build ./...` exit=0、`go vet ./controller` exit=0、`npm run build` 通过;GetProblems 仅剩两条与本次无关的 IDE tg_chat 告警。含后端,需重启 + Ctrl+F5。遗留:上轮冗余排查清单中的后端死接口(handler CRUD 四接口/logout/auth-user)与 function.go 死函数仍在,待用户定清理范围。
- **全量文案出口统一支持内置变量占位(功能增强)**:背景——上条只修了横幅静态文本,用户发现菜单页「显示文字(text)」回复文案里 {first_name}/{uid} 等仍原样输出,要求“全部都要处理”。改动(`tgBotRuntime.go`,均在 u!=nil 时过 `httpRender(文本, httpCtx(u,nil), nil)`,与横幅/接口模式同一套内置变量):①execAction case "text" 的文案与配图直链(http 前缀才渲染,file: 的 file_id 不受影响);②execAction case "url" 的外链(支持带 {uid} 的落地页);③menuPageResult 容器菜单页引导语 scfg["text"]。未覆盖:菜单按钮 title(短标签,暂不渲染)、手动 back 的 cfg["text"](无消费方)。**提醒:{data.xxx} 响应路径仅接口模式有效**,静态文案里写 {data.time} 会变空串,应用内置 {time}。验证:`GOOS=windows go build ./...` exit=0、`go vet ./controller` exit=0;GetProblems 仅剩两条无关的 IDE tg_chat 告警。纯后端,需重启生效。
- **按钮排版新增:整行按钮 + 根页可配列数(功能增强)**:背景——用户想把首页九宫格里「热门动态」单独排一行,现有排版只有容器级 cols(按列数顺序填满),无断行能力,后台满足不了需改代码。改动:①后端 `tgBotHandler.go`——TgButton 加 `FullRow bool`,`buildKeyboard` 遇 FullRow 先 flush 当前行、该按钮独占一行(对任意来源按钮通用);②`tgBotRuntime.go` menuPageResult——子项行 cols==1 → FullRow(复用 tg_menu 既有 cols 列,无表变更;语义:容器 cols=每行几个,叶子项 1=整行、>1=跟随容器),根页 cols 改从 start 命令 action_config.cols 读(缺省 2);③前端 `tgMenu/index.vue`——非文件夹项弹窗也显示「按钮列数」(提示设 1=单独占一行),列表「每行」列非文件夹 cols=1 显示「整行」;④`tgCommand/index.vue`——根主菜单时新增「每行按钮数」(cfg.root_cols,存 action_config.cols 字符串,仅显式配置才写)。验证:`GOOS=windows go build ./...` exit=0、`go vet ./controller` exit=0、`npm run build` 通过、GetProblems 无错。含后端,需重启 + Ctrl+F5。用户操作:编辑「热门动态」把按钮列数设 1 即独占一行。后续按用户要求删掉两处新增字段下方的 form-tip 说明行(「按钮列数」与命令页「每行按钮数」),语义靠字段名自明;`npm run build` 复验通过。又把 http 弹窗的「配图路径」与「按钮列数」并成一排(el-row/两 el-col :span=12,挪进 http template 块内;注:首次替换误把 el-row 落在块外丢了闭合标签,二次修正结构后 build 复验通过)。
- **http 接口配图路径支持三种写法(功能增强)**:背景——用户要求接口取数菜单的配图“能用图库、能自填链接、能用占位符”。改动:①后端 `tgHTTP.go`——image_path 新增 file: 前缀分支(直接当 FileID 透传,发送端 photoFile 已识别 file:),与既有 http(s) 直链(过 httpRender 支持 {uid}/{data.xxx} 占位)、响应字段路径({pci}/data.pic)三态自适应;②前端 `tgMenu/index.vue`——http 弹窗配图路径加「图片库」append 按钮,openPicker(target)+pickerTarget 区分回填目标(http → cfg.image_path,其余 → cfg.image)。验证:`GOOS=windows go build ./...` exit=0、`go vet ./controller` exit=0、`npm run build` 通过、GetProblems 无错。含后端,需重启 + Ctrl+F5。

#### 2026-10-01
- **命令弹窗根页布局微调(前端)**:按用户要求把根主菜单(cfg.page==='root')的「配图」与「每行按钮数」两个 form-item 由上下堆叠改为并排一行(el-row + 两 el-col :span=12, `tgCommand/index.vue`)。验证:`npm run build` 通过、GetProblems 无错。纯前端,Ctrl+F5 生效。
- **新增动作类型 `pick_user`(选择收款人)——第一步:原生选人器 + 回显**:参考「皇冠担保钱包」点「转账」→弹原生「选择对话…」选人的流程。**核心障碍**:依赖的 go-telegram-bot-api v5.5.1 两头都不支持——`KeyboardButton` 无 `request_chat` 字段(发送端)、`Message` 无 `chat_shared` 字段(接收端)。故走**裸调 Bot API**(复用 `tgBotProfile.go` 的 `tgCall` 原生 HTTP 模式)。
  - **发送端**(`tgBotRuntime.go` `sendPickUserPage`):手拼带 `request_chat`(`request_id:1, chat_is_channel:false, user_is_bot:false` → 选真人)的 `reply_markup` JSON,走 `tgCall('sendPhoto'/'sendMessage')`。菜单项 `pick_user` 走新回调域 `p:`(`menuPageResult` 编 btn.Cb、`onCallback` case "p" → `menuPickUser`),点击时登记 `awaitInput{kind:"chat"}` 再发选人页。页配置字段:`page_text`/`page_image`(横幅 file: 走 sendPhoto)/`pick_btn`/`cancel_btn`/`input_key`(默认 payee)/`format`。
  - **接收端**(`Webhook`):`ShouldBindJSON` 会丢弃未知字段,改为先 `io.ReadAll(c.Request.Body)` 拿原始 body → `json.Unmarshal` 到 `tg.Update` **再** `captureChatShared` 双解析一层 `message.chat_shared`,命中则按 `UpdateID` 暂存到 `pickedChat map[int]pickedChat`(带 `pickMu` 锁 + 4096 容量兜底)。`onMessage` 顶部 `consumePicked(up.UpdateID)` 命中 → `handlePickedChat` **回显选中的人**(账户ID/名称/@username)并 `sendRemoveKeyboard` 收起键盘;点「取消」(回复键盘发来同名文本)则 `clearAwait` + 收键盘。
  - **前端**(`tgMenu/index.vue`):`actionTypeOptions` 加「选择收款人(pick_user)」;新增配置块(页面文案/横幅配图[图库 openPicker('page')]/选择按钮/取消按钮/收款人变量名);`cfg` 加 5 字段(4 处字面量同步)、`buildActionConfig`/`edit` 回填/`onPickImage`/`configSummary`/`actionLabel`/`actionTagType` 各加分支;`按钮列数` 通用项 v-if 排除 pick_user。
  - **本步范围**:仅「弹原生选人器 + 回显选中的人」跑通;`handlePickedChat` 已预留 aw.menuID 回查配置 → 后续「带 `{payee}` 调 http 接口」在此扩展(`httpResult(cfg,u,extraCtx)` 的 extraCtx 注入 payee/payee_name/payee_username)。验证:`go build ./...`、`go vet ./controller`、`npm run build` 全过。**需重启后端**方生效(改了 Go 代码)。
  - **修复(同日)**:首版用 `request_chat`,实测客户端弹的是「选择群组/会话」而非选真人。改用 `request_users`(Bot API 7.0,`sendPickUserPage` 里 `request_users:{request_id,user_is_bot:false,max_quantity:1,request_username:true}`),`captureChatShared` 新增解析 `message.users_shared`(取 users[0].user_id/first_name/last_name/username),chat_shared 保留兼容。验证:go build/vet 过。仍需重启后端。
  - **修复2(同日 UX):选完不再另发消息、而是原地改选人页**:旧版选完收款人后**另发一条**回显消息,那张「请选择一个收款人」页仍杵在原处很怪。改为**原地编辑**那条选人页:`awaitInput` 加 `pageChatID/pageMsgID/pageHasMedia`,`sendPickUserPage` 返回 `(msgID,hasMedia)`(经 `parseTgMsgID` 从 `tgCall` 的 Message JSON 取 message_id),`menuPickUser` 先发消息拿到 id 再 `setAwait`。新增 `editPickUserPage(chatID,msgID,hasMedia,text)`:带图页走 `editMessageCaption`(保留横幅、只换 caption)、纯文本走 `editMessageText`,`msgID<=0`(重启丢页)退化为另发新消息。`handlePickedChat` 与「取消」分支都改调 `editPickUserPage`(文案「✅ 你已选择了收款人：X / 🆔 账户ID」)。删掉 `sendRemoveKeyboard`——回复键盘 `one_time_keyboard:true` 按下即自动收起,无需再发移除消息。`onMessage` 选人等待中收到无关文本改为**静默忽略**(不消费等待态、不动键盘,避免误触把选择器收掉导致卡住)。验证:go build/vet 过,仍需重启后端。
  - **修复3(同日 UX、推翻修复2):选完直接删掉整条选人页**:修复2 的「原地改文案」仍保留那张大图横幅,用户反馈选完后横幅杵在那儿依旧很怪、应该「消失」。改为**直接 `deleteMessage` 删掉那条选人页**、再**另发一条干净确认**(纯文本、无横幅)。实现:`editPickUserPage` 改写为 `finishPickUserPage(aw,text)`——`aw.pageMsgID>0` 则 `deleteMessage(pageChatID,pageMsgID)`,`text` 非空则 `send` 一条新消息;`awaitInput` 去掉 `pageHasMedia`(不再需区分图文)、`sendPickUserPage` 返回值由 `(int,bool)` 改回 `int`。`handlePickedChat`(选完)与「取消」分支都改调 `finishPickUserPage`。验证:go build/vet 过,仍需重启后端。
  - **修复4(同日 根因):本地长轮询模式未捕获 users_shared、选人回传丢失**:用户反馈选完人后选人页未删、也无确认——即 `handlePickedChat` 根本没触发。查因:当前 `config.yaml` 为 `debugPolling: true`(本地长轮询调试),而 `captureChatShared` 双解析**只挂在 `Webhook` 里**;`startPolling` 用的是库的 `GetUpdatesChan`——v5.5.1 的 `tg.Update/tg.Message` 无 `users_shared` 字段,库解码时直接把选人回传的服务消息丢弃了。修法:重写 `startPolling`,不再用 `GetUpdatesChan`,而是用带 `ResponseHeaderTimeout>50s` 的 `http.Client` 直接 POST `getUpdates?timeout=50&offset=...` 拿 `result: []json.RawMessage`(保留每条 Update 原始 JSON),逐条 `json.Unmarshal` 到 `tg.Update` **再** `captureChatShared(up.UpdateID, raw)` 后 `enqueue`——与 webhook 走同一套双解析,两种模式行为一致。offset 手动递进(最大 update_id+1),请求/解析失败 3s 后重试。验证:go build/vet 过,仍需重启后端。
  - **第二步(同日 简化):pick_user 改为「固定默认选人页 + 反馈信息 + 复用 http 取数」三合一**:用户明确「页面文案/横幅/选择按钮/取消按钮/收款人变量名这些都用不上」,真正链路是**选完收款人 → 拿对方用户ID → 调预先配好的接口 → 把响应填模板回给用户**。据此把 pick_user 的 `action_config` 从 5 个页配置字段收敛为:**一个 `feedback_text`(选完先发的自定义提示)+ 完整复用 http 那套字段(`method`/`url`/`headers`/`body`/`args`/`text`/`image_path`/`format`/`cols`)**。后端:`sendPickUserPage(chatID)` 去掉 cfg/u 参数、选人页文案与按钮改常量 `pickUserPageText`/`pickUserPickBtn`/`pickUserCancelText`(全固定默认);`menuPickUser` 不再读 `input_key`/`cancel_btn`;`handlePickedChat` 重写为——先 `finishPickUserPage(aw,"")` 删选人页,回查 `aw.menuID` 配置,把 `{payee}`(对方ID)/`{payee_name}`/`{payee_username}` 作 `extraCtx` 最高优先级注入:① 有 `feedback_text` 先 `send` 一条(经 `httpRender`);② 有 `url` 则 `httpResult(cfg,u,payeeCtx)` 调接口并 `appendBackButton` 后 `send`。前端:`pick_user` 表单块删除 5 个旧字段、只留「反馈信息」,并让 **http 配置块 `v-if` 扩为 `action_type==='http' || action_type==='pick_user'`**(复用整套接口字段);`cfg` 初始化去 5 旧字段加 `feedback_text`(data/add/edit/onActionTypeChange 四处),`buildActionConfig`/`edit` 回填/`configSummary` 同步。验证:go build/vet 过,仍需重启后端。
  - **第三步(同日):pick_user 选完插入「等用户输入信息」环节再调接口**:用户反馈选完收款人不应直接调接口,而应先让用户输一条信息(如转账金额),拿到输入再连同 `{payee}` 去调接口。**复用现有 http 的「等待输入」机制**(`input_prompt` + `awaitInput{kind:"text"}`):`awaitInput` 新增 `payeeCtx map[string]string` 字段携带收款人上下文;`handlePickedChat` 在发完 `feedback_text` 后——若配了 `input_prompt` 则不再直接调接口,而是 `setAwait(kind:"text", varKey=input_key||"input", payeeCtx)` + 发一条带 `ForceReply` 的提示语(提示语经 `httpRender`、支持 `{payee}` 等变量)并 return;未配则保持原行为直接 `httpResult`。`onMessage` 普通文本分支的等待消费处,把 `aw.payeeCtx` 合并进 `extra`(与 `{varKey:文本}` 一起),再走 `menuHTTPResult`——因 extraCtx 非 nil 会跳过提示语分支直接 `httpResult`,接口的 url/body/text 同时可用 `{payee}` 与 `{input}`。前端:pick_user 表单块新增「输入提示语」(`cfg.input_prompt`,与 http 共用模型),`buildActionConfig` 的 pick_user 加 `input_prompt`。验证:go build/vet 过,仍需重启后端。
  - **调整(同日):反馈信息合并作提示语、去掉单独的输入提示语字段**:用户反馈两个文本框冗余,「反馈信息作为提示语就行了」。合并:`handlePickedChat` 不再读 `input_prompt`,而是把 `feedback_text` 直接当「选完确认 + 等待输入提示语」——配了 `feedback_text` 就 `setAwait(kind:"text", payeeCtx)` + 发带 `ForceReply` 的反馈并 return(用户输入作 `{input}`);留空则跳过输入环节直接 `httpResult`(仅 `{payee}`)。前端 pick_user 表单块删「输入提示语」字段、只留「反馈信息」(placeholder 说明它兼作输入引导),`buildActionConfig` 去 `input_prompt`。`onMessage` 等待消费处不变(payeeCtx 已合并进 extra)。验证:go build/vet 过,仍需重启后端。
  - **调整(同日):删除低频的「参数」(args) 表单字段**:用户反馈 args 作用不大。确认 args 仅是在 `httpCtx` 里把 `k=v,k2=v2` 当自定义静态变量注入模板(优先级低于内置变量),完全可直接写死在 url/body/text 里——低频可省。前端 `tgMenu/index.vue` 彻底删「参数」:表单 el-form-item、`cfg` 初始化四处、`edit` 回填、`buildActionConfig`(http+pick_user 两处);为避开删后留的空档,把**请求方法上移到顶部行(http||pick_user 均显)**、**输入提示语从顶行挪进接口块占满整行(v-if http)**。后端 `parseArgs(cfg["args"])` **保留不动**(存量行兼容、无 args 回空 map 无害,handler 仍用)。验证:`npm run build` 过;纯前端无需重启后端。
  - **调整(同日):类别名由「选择收款人」改为通用的「选择用户」**:用户要求不把场景定死(以后可适应更多用途)。action_type key 仍为 `pick_user` 不变(后端路由/存量数据不受影响),只改显示文案:前端 `tgMenu/index.vue`——`actionTypeOptions` 下拉 label 、`actionLabel` 中文映射、`configSummary` 的 `👥 选用户` 前缀、pick_user 块注释与 placeholder(“选完用户后…请输入金额”);后端 `tgBotRuntime.go` 选人页常量 `pickUserPageText`=“请使用以下方法选择一个用户：”/`pickUserPickBtn`=“🔍 选择用户”、取消回显“已取消，未选择用户”。验证:go build/vet + `npm run build` 均过。**含后端常量改动,需重启后端 + 刷新前端**。
  - **调整(同日):接口地址留空时不发请求、直接返回模板**:用户场景——如「我要发红包」选用户+输金额后无需真接口,只想把「显示文案」模板(如 `红包发送成功：{input}`)直接回给用户。改 `tgHTTP.go` `httpResult`:把“URL 缺失即回 fail”改为——`cfg["url"]` 非空才渲染+校验+`httpFetch`(失败仍回 fail);为空则跳过整个请求环节,`parsed` 保持 nil 继续往下走,用 ctx(含 extraCtx 的 `{input}`/`{payee}` + 内置变量)渲染 `text`/`image_path` 直接返回(响应路径变量 `{data.xxx}` 因无 resp 取空)。连带:`handlePickedChat` 反馈为空的直调分支守卫由 `url!=""` 改为 `url!="" || text!=""`;前端 `save` 对 http 的校验改为“接口地址与显示文案至少填一个”。此行为 http 与 pick_user 共用(都走 httpResult)。验证:go build/vet + `npm run build` 均过。**含后端改动,需重启后端 + 刷新前端**。
  - **调整(同日):接口取数(http)与 pick_user 提示语行为对齐**:用户要求“接口取数那边也一致”。排查发现无-url→模板行为本就共用 `httpResult`(http 已一致);但有一处真不一致:pick_user 的 feedback_text 发送前过 `httpRender`(支持变量),而 http 的 `input_prompt` 在 `menuHTTPResult` 提示语分支里是**原样发送不做变量替换**。改:`menuHTTPResult` 的 prompt 分支返回前过 `httpRender(prompt, httpCtx(u, parseArgs(cfg["args"])), nil)`,使 http 输入提示语也支持 `{uid}/{first_name}/{time}` 等内置变量(pick_user 额外有 `{payee}`)。验证:go build/vet 过,需重启后端。
  - **调整(同日):菜单树连接线颜色调深**:用户反馈树形连接线太浅。`tgMenu/index.vue` 里 `.tree-guide--vline::before`/`.tree-guide--elbow::before`/`.tree-guide--elbow::after` 三处 `background` 由 `#dcdfe6` 改为次级边框色 `#c0c4cc`(仅深一档、不刺眼)。纯前端,`npm run build` 过,Ctrl+F5 生效,无需重启后端。
  - **调整(同日):tgMenu 卡片满窗口(无分页页高度偏移例外)**:用户反馈菜单页底部空一截——因 tgMenu 是树形表、无分页,但仍用带分页页的统一偏移 `calc(100vh - 182px)`,底部正好空出分页条位置。带分页页表格下多一个 `.currentPage`(margin-top:10px + 约 40px 高的 el-pagination 条 ≈ 50px),tgMenu 无此块。改:`tgMenu/index.vue` 表格高度由 `calc(100vh - 182px)` 先改 `132px`——但 132 减过头出现垂直滚动条(实际可填充量小于 50px),回调为 `calc(100vh - 150px)`。因 `calc` 定值靠猜像素反复调不精确(对比 tgUser 带分页页才看清差异),**最终改为动态算高**:模板 `:height="tableHeight"`,新增 `setTableHeight()`——`this.$nextTick` 里用 `this.$refs.table.$el.getBoundingClientRect().top` 实量表格顶部到视口顶距离,`tableHeight = Math.max(240, window.innerHeight - top - 24)`(24=el-card body 下内边距10+卡片下外边距10+余量4,与带分页页卡片底到视口底间距对齐);`mounted` 调用并挂 `window resize` 监听,`beforeDestroy` 摘除。**tgMenu 不再用 182px 定值,改自适应,任何窗口高度都铺满且不出页面滚动条**(表格行多时仍保留其内部滚动)。纯前端,`npm run build` 过,Ctrl+F5 生效。
  - **调整(同日):全局统一——抽出 `tableAutoHeight` mixin 应用到所有列表页**:用户要求“整个后台表格高度都改成运行时动态计算”。新建 `web/admin/src/mixins/tableAutoHeight.js`——`data.tableHeight`(初值400) + `setTableHeight()`(`$nextTick` 里 `getBoundingClientRect().top` 量表格顶部位置,再**累加表格后续同父兄弟元素(分页条)高度+margin**,`tableHeight = Math.max(240, innerHeight - top - below - 24)`),`mounted` 挂 `resize` 监听、`beforeDestroy` 摘除。因同时量 top 与 below(分页条),**带分页(tgUser/tgChat/tgImage)与无分页(tgMenu/tgCommand)均自适应**,不再需为无分页页单独记偏移。接入方式:每页 `mixins: [tableAutoHeight]` + 表格 `ref="table" :height="tableHeight"`(前提:表格 ref 必为 table、且与分页条同为 el-card body 直接子元素)。tgMenu 删内联的动态逻辑改用它。注:sysSetting 无表格不受影响。验证:`npm run build` 过、GetProblems 无错;纯前端,Ctrl+F5 生效。
  - **新增(同日):自建两个 Mock 调试接口,免依赖第三方**:用户调试 pick_user/http 时发现第三方 mock 站 `jsonplaceholder.typicode.com` 的 **POST 写接口返回 404/500**(GET 仍 200),导致“转账”类 POST 流程一直报“暂时取不到数据”。为稳定联调,在自家后端挂两个不鉴权的测试接口。新建 `controller/mockController.go`:`MockGet`(GET `/api/test/get`,**把收到的查询参数原样放进 `data` 回显**)、`MockPost`(POST `/api/test/post`,**把收到的 JSON body 原样放进 `data` 回显、不额外加任何字段**,`code`/`message` 固定为 0/ok,**恒返回 200**,非法 JSON 则把原文字符串当 `data` 原样回传)。`route/route.go` 加公开组 `test := router.Group("/api/test")` 挂这两条(不挂 BossAuth)。`validTgURL` 只校验 scheme(http/https)+ASCII 主机名,故 `http://127.0.0.1:8200/api/test/xxx` 可通过、bot 自调本机端口无碍。验证:`go build ./...` + `go vet` 过、GetProblems 无错。**含后端改动,需重启后端生效**。

### 运维备忘
- **端口占用**:调试残留的 `tgbot.exe` 会占 8200,报 `bind ... Only one usage of each socket address`;`Stop-Process -Name tgbot` 释放。
- **端口被系统保留(与上条不同、易混淆)**:若报 `listen tcp :8200: bind: An attempt was made to access a socket in a way forbidden by its access permissions`(Windows 错误码 **10013 / WSAEACCES**),**不是**进程占用(10048 才是),而是 **Hyper-V/WinNAT 开机时随机划走的 TCP 保留段**把 8200 圈进去了(查:`netsh interface ipv4 show excludedportrange protocol=tcp`,看 8200 是否落在某段内;`netstat -ano | findstr :8200` 无结果是佐证)。保留段每次开机随机变,故时好时坏。修法(**管理员** PowerShell,与现有 `8100 *` 同款做法):`net stop winnat` → `netsh int ipv4 add excludedportrange protocol=tcp startport=8200 numberofports=1 store=persistent` → `net start winnat`。管理员持久排除(*)不禁止本机监听,只是阻止 WinNAT 以后再划走它;中间会短暂断 Docker/WSL2/Hyper-V 网络几秒。
- **单实例**:getUpdates 长轮询同一 bot **同时只能跑一个进程**,否则抢更新 + 撞端口;调试固定用 GoLand 的 Run。
- **环境**:本机全局 `go env GOOS=linux`,本地 build/run 前需临时 `Set-Item -Path env:GOOS -Value windows`(且与命令同行,env 不跨 shell 保留)。

---

## 附:新增一类「返回数据」的标准动作

1. `controller/tgBotHandler.go` 里写一个 `TgHandlerFunc`,`init()` 里 `RegisterTg("my_key", fn)`。
2. `tg_handler` 表加一行 `handler_key=my_key, name=中文名`。
3. 后台「机器人菜单」里新建一条 `action_type=handler`,数据类选 `my_key`,填参数,挂到目标父菜单下。
4. 纯配置改动无需重启;只有新增了 Go 函数才需重新编译部署。

---

## 附:新增列表页(表格)标准模板——高度自适应“卡片满窗口”

后台所有列表页的 `el-table` 高度统一由 mixin `web/admin/src/mixins/tableAutoHeight.js` **运行时动态计算**(量表格顶部到视口顶距离 + 累加表格后面分页条等兄弟元素高度),精确铺满窗口底部且不出页面级滚动条。**新增一个表格页只需按下面两步接入,不要再写 `calc(100vh - 182px)` 之类定值。**

### 两步接入(必做)

1. 组件里引入并挂上 mixin:
   ```js
   import tableAutoHeight from "@/mixins/tableAutoHeight";
   export default {
     name: "XxxList",
     mixins: [tableAutoHeight],   // ← 提供 data.tableHeight + setTableHeight + resize 监听
     // ...data/computed/methods/mounted 照常写,与 mixin 合并(生命周期两边都跑)
   }
   ```
2. 表格用 `ref="table"` 且高度绑定 `:height="tableHeight"`:
   ```html
   <el-card>
     <!-- 搜索头 / 工具栏(可选) -->
     <el-table ref="table" class="tableData" :data="tableData" :height="tableHeight" border>
       <!-- 列... -->
     </el-table>
     <!-- 分页(可选):必须与表格同为 el-card body 的直接子元素 -->
     <div class="currentPage" style="margin-top:10px;text-align:center;">
       <el-pagination ... />
     </div>
   </el-card>
   ```

### mixin 依赖的两个前提(违反则不生效/算错)

- **表格 `ref` 必须叫 `table`**(mixin 写死取 `this.$refs.table`)。
- **表格与分页条必须是 `el-card` body 的同级直接子元素**(mixin 靠“累加表格 `nextElementSibling` 链的高度”扣掉分页条;无分页则累加为 0,自动适配)。
- 结构类间距(`.toolbar` / `.el-card` margin / `.el-card__body` padding)**只走 App.vue 全局规则,单页不得用 scoped 重定义**——历史上页面级 scoped `.toolbar{margin-bottom}` 叠加全局导致过溢出。底部预留常量 `24`(=body 下内边距10 + 卡片下外边距10 + 余量4)已内置,若个别页有 1~2px 缝/滚动条,微调 mixin 里这一个数即可(全局生效)。

### 已接入页面

`tgMenu`(树形无分页)、`tgCommand`(无分页)、`tgUser`、`tgChat`、`tgImage`(后三者带分页)。`sysSetting` 是表单无表格,不涉及。

---

## 附录 A:完整建表 + 示例数据 SQL(可直接执行)

> 在 GoLand 连到 `tgbot` 库的 Database 控制台里,整段选中执行一次即可。重复执行会因主键冲突报错,属正常。表结构说明见 §3,这里是可直接跑的完整脚本(含演示四种动作类型 + 两级子菜单 + 三条命令的示例数据)。

```sql
-- ---------- 1. 菜单树 ----------
CREATE TABLE IF NOT EXISTS tg_menu (
  id            BIGINT PRIMARY KEY AUTO_INCREMENT COMMENT '主键ID',
  parent_id     BIGINT      NOT NULL DEFAULT 0     COMMENT '父菜单ID(0=根菜单)',
  lang          VARCHAR(8)  NOT NULL DEFAULT 'all'  COMMENT '语言(all=全部语言)',
  title         VARCHAR(64) NOT NULL                COMMENT '菜单按钮文案',
  action_type   VARCHAR(16) NOT NULL DEFAULT 'menu' COMMENT '动作类型:menu/text/url/handler/http',
  action_config JSON        NULL                    COMMENT '动作配置(JSON,随 action_type 而异)',
  cols          TINYINT     NOT NULL DEFAULT 2      COMMENT '每行按钮列数',
  sort          INT         NOT NULL DEFAULT 0      COMMENT '排序(升序)',
  status        TINYINT     NOT NULL DEFAULT 1      COMMENT '状态:1=启用,0=禁用',
  created_at    DATETIME    NULL                    COMMENT '创建时间',
  updated_at    DATETIME    NULL                    COMMENT '更新时间',
  KEY idx_parent (parent_id, status, sort) COMMENT '按父菜单查子级(父ID+状态+排序)'
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='机器人菜单树(可视化九宫格菜单)';

-- ---------- 2. 动态数据类白名单 ----------
CREATE TABLE IF NOT EXISTS tg_handler (
  id          BIGINT PRIMARY KEY AUTO_INCREMENT COMMENT '主键ID',
  handler_key VARCHAR(64)  NOT NULL              COMMENT '数据类标识(对应代码 RegisterTg 的 key)',
  name        VARCHAR(64)  NOT NULL              COMMENT '中文名',
  param_hint  VARCHAR(255) NOT NULL DEFAULT ''   COMMENT '参数示例提示(如 limit=5)',
  remark      VARCHAR(255) NOT NULL DEFAULT ''   COMMENT '备注说明',
  status      TINYINT      NOT NULL DEFAULT 1    COMMENT '状态:1=启用,0=禁用',
  created_at  DATETIME     NULL                  COMMENT '创建时间',
  updated_at  DATETIME     NULL                  COMMENT '更新时间',
  UNIQUE KEY uk_key (handler_key) COMMENT '数据类标识唯一'
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='动态数据类白名单(供菜单 action_type=handler 选用)';

-- ---------- 3. Telegram 用户 ----------
CREATE TABLE IF NOT EXISTS tg_user (
  id           BIGINT PRIMARY KEY AUTO_INCREMENT COMMENT '主键ID',
  tg_user_id   BIGINT      NOT NULL              COMMENT 'Telegram 用户ID',
  chat_id      BIGINT      NOT NULL              COMMENT '私聊 chat_id(发消息用)',
  username     VARCHAR(64) NOT NULL DEFAULT ''   COMMENT 'Telegram 用户名',
  first_name   VARCHAR(64) NOT NULL DEFAULT ''   COMMENT '名字',
  lang         VARCHAR(8)  NOT NULL DEFAULT 'en' COMMENT '语言代码',
  bind_user_id BIGINT      NOT NULL DEFAULT 0    COMMENT '绑定的业务系统账号ID(预留,0=未绑定)',
  status       TINYINT     NOT NULL DEFAULT 1    COMMENT '状态:1=正常,0=已屏蔽bot不可推送',
  created_at   DATETIME    NULL                  COMMENT '首次交互时间',
  updated_at   DATETIME    NULL                  COMMENT '最近更新时间',
  UNIQUE KEY uk_tg (tg_user_id) COMMENT 'Telegram用户ID唯一'
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='Telegram 用户(bot 交互过的用户)';

-- ---------- 4. 后台管理员 ----------
CREATE TABLE IF NOT EXISTS admin (
  id         BIGINT PRIMARY KEY AUTO_INCREMENT COMMENT '主键ID',
  account    VARCHAR(64) NOT NULL              COMMENT '登录账号',
  password   VARCHAR(64) NOT NULL              COMMENT '登录密码(明文存储)',
  status     TINYINT     NOT NULL DEFAULT 1    COMMENT '状态:1=启用,0=禁用',
  created_at DATETIME    NULL                  COMMENT '创建时间',
  updated_at DATETIME    NULL                  COMMENT '更新时间',
  UNIQUE KEY uk_account (account) COMMENT '登录账号唯一'
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='后台管理员账号';

-- ---------- 5. 命令菜单(原生「菜单」按钮) ----------
CREATE TABLE IF NOT EXISTS tg_command (
  id            BIGINT PRIMARY KEY AUTO_INCREMENT COMMENT '主键ID',
  command       VARCHAR(32) NOT NULL                COMMENT '命令名(不含斜杠,如 start)',
  description   VARCHAR(64) NOT NULL                COMMENT '命令描述(显示在 TG 命令列表)',
  action_type   VARCHAR(16) NOT NULL DEFAULT 'menu' COMMENT '动作类型:menu/text/url/handler/http',
  action_config JSON        NULL                    COMMENT '动作配置(JSON,随 action_type 而异)',
  sort          INT         NOT NULL DEFAULT 0      COMMENT '排序(升序)',
  status        TINYINT     NOT NULL DEFAULT 1      COMMENT '状态:1=启用,0=禁用',
  created_at    DATETIME    NULL                    COMMENT '创建时间',
  updated_at    DATETIME    NULL                    COMMENT '更新时间',
  UNIQUE KEY uk_cmd (command) COMMENT '命令名唯一'
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='命令菜单(原生菜单按钮里的斜杠命令)';

-- ---------- 6. 图片库(存 Telegram file_id,供各处配图复用) ----------
CREATE TABLE IF NOT EXISTS tg_image (
  id         BIGINT PRIMARY KEY AUTO_INCREMENT COMMENT '主键ID',
  name       VARCHAR(191)  NOT NULL DEFAULT '' COMMENT '图片名称(便于识别与搜索)',
  tag        VARCHAR(64)   NOT NULL DEFAULT '' COMMENT '标签/分类(可空)',
  tg_file_id VARCHAR(512)  NOT NULL DEFAULT '' COMMENT 'Telegram file_id(发送时直接引用,免重复上传)',
  mime       VARCHAR(64)   NOT NULL DEFAULT '' COMMENT '图片 MIME 类型',
  size       INT           NOT NULL DEFAULT 0  COMMENT '图片字节大小',
  source_url VARCHAR(1024) NOT NULL DEFAULT '' COMMENT '来源直链(URL 导入时记录,可空)',
  status     TINYINT       NOT NULL DEFAULT 1  COMMENT '状态:1=启用,0=禁用',
  created_at DATETIME      NULL                COMMENT '入库时间',
  updated_at DATETIME      NULL                COMMENT '更新时间',
  KEY idx_tag (tag) COMMENT '按标签查图'
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='图片库(存 Telegram file_id,供各处配图复用)';

-- ---------- 7. 系统设置(运行时配置 key-value,后台可改、改完即生效) ----------
CREATE TABLE IF NOT EXISTS sys_setting (
  id         BIGINT PRIMARY KEY AUTO_INCREMENT COMMENT '主键ID',
  skey       VARCHAR(64)  NOT NULL DEFAULT ''  COMMENT '设置键(程序读取用的唯一标识)',
  svalue     TEXT         NULL                 COMMENT '设置值',
  name       VARCHAR(191) NOT NULL DEFAULT ''  COMMENT '设置名称(后台展示)',
  remark     VARCHAR(255) NOT NULL DEFAULT ''  COMMENT '备注说明',
  updated_at DATETIME     NULL                 COMMENT '更新时间',
  UNIQUE KEY uk_skey (skey) COMMENT '设置键唯一'
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='系统设置(运行时配置 key-value,后台可改、改完即生效)';

-- ---------- 8. Telegram 群组/频道(bot 被拉进群时自动落库) ----------
CREATE TABLE IF NOT EXISTS tg_chat (
  id         BIGINT PRIMARY KEY AUTO_INCREMENT COMMENT '主键ID',
  chat_id    BIGINT       NOT NULL              COMMENT 'Telegram 群组/频道ID(通常为负数)',
  title      VARCHAR(255) NOT NULL DEFAULT ''   COMMENT '群组/频道标题',
  username   VARCHAR(64)  NOT NULL DEFAULT ''   COMMENT '群组公开用户名(可空)',
  type       VARCHAR(16)  NOT NULL DEFAULT ''   COMMENT '类型:group/supergroup/channel',
  status     TINYINT      NOT NULL DEFAULT 1    COMMENT '状态:1=bot在群可推送,0=已离开或失效',
  created_at DATETIME     NULL                  COMMENT '首次捕获时间',
  updated_at DATETIME     NULL                  COMMENT '最近更新时间',
  UNIQUE KEY uk_chat (chat_id) COMMENT '群组chat_id唯一(每群一条)'
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='Telegram 群组/频道(bot 被拉进群时自动落库,供后台选群群发)';

-- 首条:图片库中转 ChatID(须是 bot 能主动发消息的对象;为空则回落 config.yaml.telegram.imageRelayChatID)
INSERT INTO sys_setting (skey, svalue, name, remark, updated_at) VALUES
 ('imageRelayChatID', '7569435732', '图片库中转 ChatID', '入库时把图发给它换取 Telegram file_id,再删除该消息', NOW())
 ON DUPLICATE KEY UPDATE name=VALUES(name), remark=VALUES(remark);

-- 动态数据类:与代码 RegisterTg 的 key 对应
INSERT INTO tg_handler (id, handler_key, name, param_hint, remark, status, created_at, updated_at) VALUES
 (1, 'demo_list',   '示例动态列表', 'limit=5', '占位,替换成真实查询', 1, NOW(), NOW()),
 (2, 'item_detail', '示例条目详情', 'id=条目ID', '二级动作演示',       1, NOW(), NOW());

-- 首页根菜单(parent_id=0),演示四种 action_type
INSERT INTO tg_menu (id, parent_id, lang, title, action_type, action_config, cols, sort, status, created_at, updated_at) VALUES
 (1, 0, 'all', '🔥 热门动态', 'handler', '{"handler":"demo_list","args":"limit=5"}', 2, 1, 1, NOW(), NOW()),
 (2, 0, 'all', '🛒 分类浏览', 'menu',    '{}',                                        2, 2, 1, NOW(), NOW()),
 (3, 0, 'all', '💬 联系客服', 'text',    '{"text":"📮 客服邮箱 support@你的域名\n工作时间 9:00-21:00"}', 2, 3, 1, NOW(), NOW()),
 (4, 0, 'all', '🌐 访问官网', 'url',     '{"url":"https://你的域名"}',                 2, 4, 1, NOW(), NOW()),
 (5, 0, 'all', '⚙️ 更多服务', 'menu',    '{}',                                        2, 5, 1, NOW(), NOW());

-- 「分类浏览」的子菜单(parent_id=2)
INSERT INTO tg_menu (id, parent_id, lang, title, action_type, action_config, cols, sort, status, created_at, updated_at) VALUES
 (10, 2, 'all', '📱 手机数码', 'text',    '{"text":"📱 手机数码分类占位"}',            2, 1, 1, NOW(), NOW()),
 (11, 2, 'all', '🛋️ 家居生活', 'text',    '{"text":"🛋️ 家居生活分类占位"}',            2, 2, 1, NOW(), NOW()),
 (12, 2, 'all', '🎮 电竞装备', 'handler', '{"handler":"demo_list","args":"limit=3"}', 2, 3, 1, NOW(), NOW());

-- 「更多服务」的子菜单(parent_id=5)
INSERT INTO tg_menu (id, parent_id, lang, title, action_type, action_config, cols, sort, status, created_at, updated_at) VALUES
 (20, 5, 'all', 'ℹ️ 关于我们', 'text', '{"text":"我们是做 XX 的团队,专注好物。"}', 2, 1, 1, NOW(), NOW()),
 (21, 5, 'all', '📜 使用帮助', 'text', '{"text":"点输入框旁「菜单」或消息里的按钮即可操作。"}', 2, 2, 1, NOW(), NOW());

-- 命令菜单:启动后 setMyCommands 会推到原生「菜单」按钮
-- start 用 action_type=menu + {"page":"root"} 表示渲染首页根菜单
INSERT INTO tg_command (id, command, description, action_type, action_config, sort, status, created_at, updated_at) VALUES
 (1, 'start',   '主菜单', 'menu', '{"page":"root"}', 1, 1, NOW(), NOW()),
 (2, 'help',    '帮助',   'text', '{"text":"❓ 帮助:点「菜单」按钮选功能,或直接点消息里的九宫格。"}', 2, 1, NOW(), NOW()),
 (3, 'support', '客服',   'text', '{"text":"📮 客服:support@你的域名\n工作时间 9:00-21:00"}', 3, 1, NOW(), NOW());

-- 后台管理员(务必登录后改密码!)
INSERT INTO admin (id, account, password, status, created_at, updated_at) VALUES
 (1, 'admin', 'qwer1324', 1, NOW(), NOW());

-- 校验:
-- SELECT id,parent_id,title,action_type,action_config FROM tg_menu ORDER BY parent_id,sort;
-- SELECT command,description,action_type FROM tg_command ORDER BY sort;
-- SELECT handler_key,name FROM tg_handler;
```

> 占位待替换:示例文案里的 `support@你的域名`、官网按钮的 `https://你的域名`。
> 决策:`admin.password` 采用**明文存储**(已与用户确认,不做 bcrypt 哈希);线上密码为 `qwer1324`,登录后请及时更换。

### 返回主菜单(建议加)

当前模型没有「返回」按钮,进二级菜单后只能靠 `/start` 回首页。要加就在每个二级菜单页插一条按钮:

```sql
INSERT INTO tg_menu (id, parent_id, lang, title, action_type, action_config, cols, sort, status, created_at, updated_at) VALUES
 (30, 2, 'all', '⬅️ 返回主菜单', 'menu', '{"page":"root"}', 2, 99, 1, NOW(), NOW()),
 (31, 5, 'all', '⬅️ 返回主菜单', 'menu', '{"page":"root"}', 2, 99, 1, NOW(), NOW());
```

`execAction` 对 `action_type=menu` 需支持 `action_config.page=="root"` → 渲染 `parent_id=0` 的首页;否则用该按钮自身 id 当 `parent_id` 渲染其子菜单。
