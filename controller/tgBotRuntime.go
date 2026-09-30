package controller

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	tg "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"gorm.io/gorm"

	"tgbot/config"
	"tgbot/tools"
)

// tgTokenPattern 粗校验 bot token 形如 "数字:长串";占位/未配置时不联网,避免拖慢启动。
var tgTokenPattern = regexp.MustCompile(`^\d+:[A-Za-z0-9_-]{20,}$`)

// ==================== 通道层 + 分发层(§6.3) ====================
//
// 通道层 Webhook 收到 Telegram 的 Update → 入队 → 秒回 200(否则会被重推);
// 分发层 consume(goroutine)消费队列,按 callback_data 的域派活,导航一律用
// EditMessageText 原地编辑(不刷新高消息),每个 callback 必须 AnswerCallbackQuery。
// 硬性要求:耗时逻辑(查库/调 Telegram)全部在 consume goroutine 里做,绝不阻塞通道层。

type TgBotRuntime struct {
	db  *gorm.DB
	bot *tg.BotAPI
	// shards 按 chat_id 分片的更新队列:同会话恒定落同一分片(片内串行保序),跨分片并行消费
	shards []chan *tg.Update
	// awaitMu 保护 awaitInput:用户「正在等待输入」状态(内存态,重启丢失用户重新点一次即可)
	awaitMu     sync.Mutex
	awaitInputs map[int64]*awaitInput
	// bannerMu 保护 bannerCache:根横幅「接口取数」引导语的按会话(chat_id)短时缓存,
	// 避免每次 /start、每次回根都打外部接口拖慢首屏。必须按 chat 分key缓存:模板可能含
	// {uid}/{first_name} 等按用户变量,全局缓存会串号。bannerTTL 为缓存存活时长;为 0 则不缓存、每次实时拉接口。
	bannerMu    sync.Mutex
	bannerCache map[string]bannerEntry
	bannerTTL   time.Duration
	// pickMu 保护 pickedChat:原生「选择收款人」选人器(request_chat)回传的 chat_shared 暂存,按 UpdateID 存。
	// 因 go-telegram-bot-api v5.5.1 的 tg.Message 无 ChatShared 字段,webhook 里另解析原始 body 存这里,分发到 onMessage 时按 UpdateID 取用。
	pickMu     sync.Mutex
	pickedChat map[int]pickedChat
}

// pickedChat 一次原生选人器(request_chat)回传的被选会话信息。私聊选择时 chatID 即对方 user_id。
type pickedChat struct {
	chatID   int64
	title    string
	username string
}

// bannerEntry 根横幅引导语缓存条目(按 chat_id 存,带过期时刻)
type bannerEntry struct {
	text   string
	expire time.Time
}

// awaitInput 一次「等待用户输入」的会话状态:由配了 input_prompt 的 http 菜单点击时登记,
// 用户发来的下一条普通文本会被消费为模板变量 {input_key},再去执行该菜单的取数动作。
const awaitInputTTL = 5 * time.Minute

type awaitInput struct {
	menuID     int64     // 等待中的菜单行 id(恢复执行时回查配置)
	varKey     string    // 输入绑定的模板变量名(默认 input)
	expire     time.Time // 超时时刻
	kind       string    // ""/"text"=等待文本输入(原 input_prompt 流程);"chat"=等待原生选人器回传
	cancelText string    // kind=chat 时「取消」按钮文案,用户点它(发来同文本)则清状态收键盘
	// 选人页定位:选完/取消后删掉那条「请选择一个收款人」页(回复键盘 one_time_keyboard 按下即自动收起)
	pageChatID int64             // 选人页所在会话
	pageMsgID  int               // 选人页消息 ID(0=没记住,退化为只发确认不删)
	payeeCtx   map[string]string // pick_user:选完人后若进入「等待输入」,携带的收款人上下文({payee}/{payee_name}/{payee_username}),文本到达时合并进模板变量
}

// NewTgBotRuntime 构造 Bot 运行时:建非校验连接(不阻塞启动)→ 启动分发循环
// → 注册原生命令菜单 → 按配置开关决定是否启动本地 getUpdates 长轮询调试入口。
func NewTgBotRuntime() *TgBotRuntime {
	var bot *tg.BotAPI
	token := config.Conf.GetString("telegram.botToken")
	if tgTokenPattern.MatchString(token) {
		// 用带超时控制的 http.Client 构造 bot:连不上 Telegram 时快速失败、只打 warning、
		// 不 panic、不阻断后台接口启动(否则境内无代理时整个服务都起不来)。
		// 只在「连接建立/TLS 握手」阶段设短超时(无出网时 GetMe 约 5s 快速失败),
		// 但不设整体 http.Client.Timeout——否则会把 getUpdates 长轮询(cfg.Timeout=50s)每次提前掐断。
		httpClient := &http.Client{
			Transport: &http.Transport{
				Proxy:                 http.ProxyFromEnvironment,
				DialContext:           (&net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
				TLSHandshakeTimeout:   5 * time.Second,
				ResponseHeaderTimeout: 60 * time.Second, // 覆盖长轮询 50s + 余量
				ExpectContinueTimeout: 1 * time.Second,
			},
		}
		b, err := tg.NewBotAPIWithClient(token, tg.APIEndpoint, httpClient) // 会调 GetMe 校验 token(需能出站访问 api.telegram.org)
		if err != nil {
			config.LogWarning("telegram bot 初始化失败,发送/编辑将被跳过:%v", err)
		} else {
			bot = b
		}
	} else {
		config.LogInfo("telegram botToken 未配置(占位),跳过联网初始化;收发消息请在 config.yaml 填真实 token")
	}
	queueCap := config.Conf.GetInt("server.queueCapacity")
	if queueCap <= 0 {
		queueCap = 1000
	}
	// 按 chat_id 分片并发:同一会话(私聊/群)恒定路由到同一分片、片内串行保证消息顺序;
	// 不同会话分散到不同分片并行消费,单条耗时的 Telegram 调用不再堵住整条队列。
	shardCount := config.Conf.GetInt("server.shardCount")
	if shardCount <= 0 {
		shardCount = 8
	}
	perShardCap := queueCap / shardCount
	if perShardCap < 1 {
		perShardCap = 1
	}
	shards := make([]chan *tg.Update, shardCount)
	for i := range shards {
		shards[i] = make(chan *tg.Update, perShardCap)
	}
	// 根横幅接口取数缓存时长(秒):未配置默认 30s;显式配 0 = 不缓存(每次 /start 都实时拉接口)。
	bannerSec := 30
	if config.Conf.IsSet("server.bannerCacheSec") {
		bannerSec = config.Conf.GetInt("server.bannerCacheSec")
		if bannerSec < 0 {
			bannerSec = 0
		}
	}
	r := &TgBotRuntime{db: config.Mysql, bot: bot, shards: shards, awaitInputs: map[int64]*awaitInput{},
		pickedChat:  map[int]pickedChat{},
		bannerCache: map[string]bannerEntry{}, bannerTTL: time.Duration(bannerSec) * time.Second}
	for _, sh := range shards {
		go r.consume(sh) // 每个分片一个消费 goroutine(长驻)
	}
	// 图片库预览缓存启动清扫:删掉库里已无对应的孤儿缓存文件,防磁盘泄漏(打库+读目录,放 goroutine 不阻塞启动)
	go func() {
		defer func() {
			if e := recover(); e != nil {
				config.LogWarning("tg imgcache sweep panic: %v", e)
			}
		}()
		imageCacheSweep(r.db)
	}()
	// setMyCommands 会打网络,放到 goroutine 里,避免拖慢/阻塞服务启动
	go func() {
		defer func() {
			if e := recover(); e != nil {
				config.LogWarning("tg syncCommands panic: %v", e)
			}
		}()
		r.syncCommands()
	}()
	if config.Conf.GetBool("telegram.debugPolling") {
		go r.startPolling() // 本地长轮询调试入口(webhook 与 getUpdates 互斥)
	} else {
		go r.registerWebhook() // 上线模式:启动时自动向 Telegram 注册 webhook(打网络,放 goroutine 避免阻塞启动)
	}
	return r
}

// Webhook 通道层:校验来源 secret → 入队 → 恒 200
func (r *TgBotRuntime) Webhook(c *gin.Context) {
	if c.GetHeader("X-Telegram-Bot-Api-Secret-Token") !=
		config.Conf.GetString("telegram.secretToken") {
		c.AbortWithStatus(403)
		return
	}
	var up tg.Update
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(200, gin.H{"code": 400, "message": err.Error()})
		return
	}
	if err := json.Unmarshal(body, &up); err != nil {
		c.JSON(200, gin.H{"code": 400, "message": err.Error()})
		return
	}
	// 库版本 tg.Message 无 chat_shared 字段,另解析一层原始 body:命中原生选人器回传则按 UpdateID 暂存,供 onMessage 取用
	r.captureChatShared(up.UpdateID, body)
	r.enqueue(&up)                                 // 按 chat_id 路由到对应分片(片满则丢弃,绝不阻塞通道层)
	c.JSON(200, gin.H{"code": 0, "message": "ok"}) // 恒 200
}

// captureChatShared 从原始 webhook body 里探测选人回传(v5.5.1 的 tg.Message 无这些字段):
// request_users 回 message.users_shared(选真人)、request_chat 回 message.chat_shared(选会话),
// 两者都兼容。命中则按 UpdateID 暂存被选用户。解析失败/无相关字段一律静默忽略,不影响正常分发。
func (r *TgBotRuntime) captureChatShared(updateID int, body []byte) {
	var shadow struct {
		Message *struct {
			ChatShared *struct {
				ChatID   int64  `json:"chat_id"`
				Title    string `json:"title"`
				UserName string `json:"username"`
			} `json:"chat_shared"`
			UsersShared *struct {
				Users []struct {
					UserID    int64  `json:"user_id"`
					UserName  string `json:"username"`
					FirstName string `json:"first_name"`
					LastName  string `json:"last_name"`
				} `json:"users"`
			} `json:"users_shared"`
		} `json:"message"`
	}
	if json.Unmarshal(body, &shadow) != nil || shadow.Message == nil {
		return
	}
	var pc pickedChat
	switch {
	case shadow.Message.UsersShared != nil && len(shadow.Message.UsersShared.Users) > 0: // request_users 选真人
		us := shadow.Message.UsersShared.Users[0]
		pc = pickedChat{chatID: us.UserID, title: strings.TrimSpace(us.FirstName + " " + us.LastName), username: us.UserName}
	case shadow.Message.ChatShared != nil: // request_chat 选会话(兼容保留)
		cs := shadow.Message.ChatShared
		pc = pickedChat{chatID: cs.ChatID, title: cs.Title, username: cs.UserName}
	default:
		return
	}
	r.pickMu.Lock()
	defer r.pickMu.Unlock()
	r.pickedChat[updateID] = pc
	if len(r.pickedChat) > 4096 { // 容量兜底:正常都在 onMessage 即时消费,异常累积时整体清空防泄漏
		for k := range r.pickedChat {
			delete(r.pickedChat, k)
		}
	}
}

// consumePicked 按 UpdateID 取出并清除暂存的选人结果;无则返回 nil。
func (r *TgBotRuntime) consumePicked(updateID int) *pickedChat {
	r.pickMu.Lock()
	defer r.pickMu.Unlock()
	pc, ok := r.pickedChat[updateID]
	if !ok {
		return nil
	}
	delete(r.pickedChat, updateID)
	return &pc
}

// consume 分发层:消费单个分片队列,把每条 Update 交给对应处理器。
// 每个分片各起一个 goroutine:片内串行(保证同一会话消息顺序),跨片并行(互不阻塞)。
// handleUpdate 内部逐条 recover,避免某条 panic 拖垮该分片循环(否则该分片后续消息都不再处理)。
func (r *TgBotRuntime) consume(shard chan *tg.Update) {
	for up := range shard {
		r.handleUpdate(up)
	}
}

// enqueue 按 chat_id 把 Update 路由到固定分片;分片满则丢弃并告警(绝不阻塞通道层)。
func (r *TgBotRuntime) enqueue(up *tg.Update) {
	idx := shardIndex(updateChatID(up), len(r.shards))
	select {
	case r.shards[idx] <- up:
	default:
		config.LogWarning("tg shard %d full, drop %d", idx, up.UpdateID)
	}
}

// shardIndex 计算某 chat_id 应落到的分片下标。用 uint64 归一化(群/频道 chat_id 为负数也能稳定映射),
// 保证同一 chat_id 恒定命中同一分片 → 会话内消息严格有序。
func shardIndex(chatID int64, n int) int {
	if n <= 1 {
		return 0
	}
	return int(uint64(chatID) % uint64(n))
}

// updateChatID 从 Update 中尽力取出会话 chat_id 用于分片路由;取不到返回 0(落 0 号分片,不影响正确性)。
func updateChatID(up *tg.Update) int64 {
	switch {
	case up.Message != nil && up.Message.Chat != nil:
		return up.Message.Chat.ID
	case up.EditedMessage != nil && up.EditedMessage.Chat != nil:
		return up.EditedMessage.Chat.ID
	case up.ChannelPost != nil && up.ChannelPost.Chat != nil:
		return up.ChannelPost.Chat.ID
	case up.EditedChannelPost != nil && up.EditedChannelPost.Chat != nil:
		return up.EditedChannelPost.Chat.ID
	case up.CallbackQuery != nil && up.CallbackQuery.Message != nil && up.CallbackQuery.Message.Chat != nil:
		return up.CallbackQuery.Message.Chat.ID
	case up.MyChatMember != nil:
		return up.MyChatMember.Chat.ID
	case up.ChatMember != nil:
		return up.ChatMember.Chat.ID
	}
	return 0
}

// handleUpdate 处理单条 Update,带 panic 兜底
func (r *TgBotRuntime) handleUpdate(up *tg.Update) {
	defer func() {
		if e := recover(); e != nil {
			config.LogError("tg handle update %d panic: %v\n%s", up.UpdateID, e, debug.Stack())
		}
	}()
	switch {
	case up.Message != nil:
		r.onMessage(up)
	case up.CallbackQuery != nil:
		r.onCallback(up.CallbackQuery)
	case up.MyChatMember != nil:
		r.onMyChatMember(up.MyChatMember)
	}
}

// onMyChatMember bot 自身在群/频道中的成员状态变化(被拉进群、被踢、被设管理员、离群等)。
// 默认订阅已含 my_chat_member,故被拉进群的事件本就已推达服务,这里只需落库供后台选群推送。
// 只记录群组/频道(私聊不入 tg_chat);据 NewChatMember.Status 判定 bot 当前是否在群:在群=可推送(status 1),已离开/被禁言(status 0)。
func (r *TgBotRuntime) onMyChatMember(cmu *tg.ChatMemberUpdated) {
	if cmu == nil {
		return
	}
	chat := cmu.Chat
	if chat.Type != "group" && chat.Type != "supergroup" && chat.Type != "channel" {
		return
	}
	status := int8(1)
	switch cmu.NewChatMember.Status {
	case "left", "kicked", "restricted":
		status = 0
	}
	r.ensureChat(chat.ID, chat.Title, chat.UserName, chat.Type, status)
}

// ensureChat upsert 群组到 tg_chat:不存在则插入,存在则刷新标题/用户名/类型/状态(群名可改、bot 可被踢出或拉回)。
func (r *TgBotRuntime) ensureChat(chatID int64, title, username, ctype string, status int8) {
	now := time.Now()
	list := make([]map[string]interface{}, 0)
	r.db.Raw("select id from tg_chat where chat_id = ? limit 1", chatID).Scan(&list)
	if len(list) == 0 {
		if err := r.db.Table("tg_chat").Create(map[string]interface{}{
			"chat_id":    chatID,
			"title":      title,
			"username":   username,
			"type":       ctype,
			"status":     status,
			"created_at": now,
			"updated_at": now,
		}).Error; err != nil {
			config.LogWarning("tg ensureChat create %d: %v", chatID, err)
		}
		return
	}
	if err := r.db.Table("tg_chat").Where("chat_id = ?", chatID).Updates(map[string]interface{}{
		"title":      title,
		"username":   username,
		"type":       ctype,
		"status":     status,
		"updated_at": now,
	}).Error; err != nil {
		config.LogWarning("tg ensureChat update %d: %v", chatID, err)
	}
}

// onMessage 收到消息:先落库用户;是命令就按 tg_command 分发,否则普通文本初稿忽略(§5.5)
func (r *TgBotRuntime) onMessage(up *tg.Update) {
	m := up.Message
	if m == nil || m.From == nil || m.Chat == nil {
		return
	}
	u := r.ensureUser(m.From, m.Chat.ID)
	// 原生选人器回传:命中本条 Update 的 chat_shared → 走「选人」后续(本步仅回显选中的人)
	if pc := r.consumePicked(up.UpdateID); pc != nil {
		r.handlePickedChat(m.Chat.ID, u, pc)
		return
	}
	if !strings.HasPrefix(m.Text, "/") {
		// 选人等待中:只认「取消」按钮文本(删掉选人页 + 发一句已取消),其余普通文本/其它服务消息一律忽略,
		// 且不消费等待态、不动键盘(one_time_keyboard 按下已自动收起)
		if aw := r.peekAwait(m.Chat.ID); aw != nil && aw.kind == "chat" {
			if strings.TrimSpace(m.Text) == aw.cancelText {
				r.clearAwait(m.Chat.ID)
				r.finishPickUserPage(aw, "已取消，未选择用户")
			}
			return
		}
		// 普通文本:若该用户正被某个 http/pick_user 菜单「等待输入」,消费这条文本并继续执行取数动作;否则忽略
		if aw := r.consumeAwait(m.Chat.ID); aw != nil {
			// 用户输入绑到 varKey;pick_user 流程还携带收款人上下文,一并合并进模板变量(供接口 url/body/text 用 {payee}+{input})
			extra := map[string]string{aw.varKey: strings.TrimSpace(m.Text)}
			for k, v := range aw.payeeCtx {
				extra[k] = v
			}
			res := r.menuHTTPResult(aw.menuID, u, extra)
			if res != nil {
				r.appendBackButton(res, aw.menuID) // 结果是新发消息,同样补返回按钮
				r.send(u.ChatID, res)
			}
		}
		return
	}
	cmd := strings.Fields(m.Text)[0]          // "/start 额外参数" → "/start"
	if i := strings.Index(cmd, "@"); i >= 0 { // 群里常见 /start@yourbot
		cmd = cmd[:i]
	}
	row := r.loadCommand(cmd)
	if row == nil {
		if cmd == "/start" { // 兜底:没配 start 也渲染根菜单,保证能用
			r.send(u.ChatID, r.menuPageResult(0, u))
		}
		return
	}
	res := r.execAction(row, u) // 复用 menu/text/url/handler 执行引擎
	if res == nil {
		return
	}
	r.send(u.ChatID, res)
}

// onCallback 点按钮 → 按 callback_data 的域派活;导航用 EditMessageText 原地编辑,并应答 callback(§6.3/§8)
func (r *TgBotRuntime) onCallback(q *tg.CallbackQuery) {
	if q.Message == nil || q.Message.Chat == nil || q.From == nil {
		r.answer(q, "无法定位会话")
		return
	}
	u := r.ensureUser(q.From, q.Message.Chat.ID)
	domain, arg := splitCb(q.Data)
	var res *TgResult

	switch domain {
	case "m": // 打开该菜单作为父页的子菜单(arg=0 即首页根)
		res = r.menuPageResult(toInt64(arg), u)
	case "h": // 触发该菜单配置的接口取数/处理器
		res = r.runHandlerByMenu(arg, u)
	case "t": // 该菜单配置的固定文案
		if row := r.loadMenuByID(arg); row != nil {
			res = r.execAction(row, u)
		}
	case "item": // handler 的二级动作
		res = r.run("item_detail", u, map[string]string{"id": arg})
	case "c": // 取消:删除本条菜单消息(会话里直接消失)
		r.deleteMessage(q.Message.Chat.ID, q.Message.MessageID)
		r.answer(q, "已取消")
		return
	case "p": // 选择收款人:另发「选人页 + 回复键盘」并登记等待;不做原地编辑(回复键盘不属于消息)
		r.menuPickUser(toInt64(arg), u)
		r.answer(q, "")
		return
	default:
		r.answer(q, "未知操作")
		return
	}
	if res == nil {
		r.answer(q, "暂无数据")
		return
	}
	// 等待输入提示:保留原菜单消息不动,另发一条带 ForceReply 的新消息(高亮输入框),
	// 用户下一条文本将被 onMessage 捕获并代入变量后继续执行。
	if res.ForceReply {
		r.answer(q, "")
		r.send(u.ChatID, res)
		return
	}
	// 终端页(无按钮)补一个返回按钮,否则原地编辑会保留上一屏陈旧按钮、用户被困住。
	var backMenuID int64
	if domain == "t" || domain == "h" || domain == "m" {
		backMenuID = toInt64(arg) // t/h 的 arg=被点菜单 id、m 的 arg=当前展示的文件夹 id
	}
	r.appendBackButton(res, backMenuID)
	// 原地编辑消息实现「页面跳转」,不刷新高消息;支持图文互转
	if r.bot != nil {
		r.editMessage(q, res)
	}
	r.answer(q, "") // 必须应答 callback,否则按钮转圈
}

// appendBackButton 终端页(结果无任何按钮)自动补一个返回按钮,防止用户被困在纯内容页。
// 目标/文案按「当前菜单」的 parent_id 自适应:父级>0 → 「返回上一级」回父菜单;父级=0(一级菜单) → 「返回主菜单」回根。
// menuID 传 0 表示无菜单上下文(如命令触发),统一回根主菜单。
func (r *TgBotRuntime) appendBackButton(res *TgResult, menuID int64) {
	if res == nil || len(res.Buttons) > 0 {
		return
	}
	target, label := int64(0), "⬅️ 返回主菜单"
	if menuID > 0 {
		if self := r.loadMenuByID(fmt.Sprintf("%d", menuID)); self != nil {
			if p := toInt64(self["parent_id"]); p > 0 {
				target, label = p, "⬅️ 返回上一级"
			}
		}
	}
	res.Buttons = append(res.Buttons, TgButton{Text: label, Cb: fmt.Sprintf("m:%d", target)})
}

// run 按 key 调注册表里的数据处理器
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

// answer 应答 CallbackQuery(硬性要求:每个 callback 都要应答,否则按钮转圈)
func (r *TgBotRuntime) answer(q *tg.CallbackQuery, text string) {
	if r.bot == nil {
		return
	}
	a := tg.NewCallback(q.ID, text)
	_, _ = r.bot.Request(a)
}

// send 发送新消息(命令首屏 / 后台主动推送用)。res.Image 非空且合法 → 发「图片+caption+按钮」,否则纯文本+按钮。
// 开 HTML 若因非法标签报错,自动去掉 parse_mode 回退纯文本重发;配图发送失败则回退成文本消息。
// 返回最终发送错误(成功返回 nil),供后台主动推送接口捕获失败(如用户拉黑 bot 的 403)。
func (r *TgBotRuntime) send(chatID int64, res *TgResult) error {
	if r.bot == nil {
		return fmt.Errorf("bot 未初始化(token 未配或校验失败)")
	}
	kb := buildKeyboard(res.Buttons, res.Cols)
	if img := r.safeImage(res); img != "" {
		photo := tg.NewPhoto(chatID, r.photoFile(res, img))
		photo.Caption = res.Text
		if res.HTML {
			photo.ParseMode = tg.ModeHTML
		}
		if kb != nil {
			photo.ReplyMarkup = kb
		}
		if _, err := r.bot.Send(photo); err != nil {
			if res.HTML { // 可能 HTML 解析失败:去富文本再试
				photo.ParseMode = ""
				if _, e2 := r.bot.Send(photo); e2 == nil {
					return nil
				}
			}
			config.LogWarning("tg send photo 失败,回退文本: %v", err)
			return r.send(chatID, &TgResult{Text: res.Text, HTML: res.HTML, Buttons: res.Buttons, Cols: res.Cols})
		}
		return nil
	}
	msg := tg.NewMessage(chatID, res.Text)
	if res.HTML {
		msg.ParseMode = tg.ModeHTML
	}
	if res.ForceReply { // 等待输入:点提示语下方直接就是输入框,意图明确不易串到别的对话
		fr := tg.ForceReply{Selective: true}
		if res.ReplyHint != "" {
			fr.InputFieldPlaceholder = res.ReplyHint
		}
		msg.ReplyMarkup = fr
	}
	if kb != nil { // 无按钮时不带 reply_markup(空键盘会被 Telegram 拒发)
		msg.ReplyMarkup = kb
	}
	if _, err := r.bot.Send(msg); err != nil {
		if res.HTML {
			msg.ParseMode = ""
			if _, e2 := r.bot.Send(msg); e2 == nil {
				return nil
			}
		}
		config.LogError("tg send msg: %v", err)
		return err
	}
	return nil
}

// safeImage 返回可用的配图引用(经校验);非法则告警并返回空(退化为纯文本)。
// 支持两种形态:①图片库引用 "file:<file_id>"(直接放行,发送端走 tg.FileID);②http/https 直链 URL。
func (r *TgBotRuntime) safeImage(res *TgResult) string {
	if res.Image == "" {
		return ""
	}
	if strings.HasPrefix(res.Image, "file:") {
		return res.Image // 图片库 file_id 引用,无需下载/抓 URL
	}
	if !validTgURL(res.Image) {
		config.LogWarning("tg 跳过非法配图 URL:%q", res.Image)
		return ""
	}
	return res.Image
}

// imgHTTPClient 专用于后端下载配图再上传给 Telegram:强制直连(不走 Telegram 的海外代理,
// 国内 CDN 图直连才能拉到)、带整体超时。拉不到时调用方会回退 FileURL 交给 Telegram 自己抓。
var imgHTTPClient = &http.Client{
	Timeout: 15 * time.Second,
	Transport: &http.Transport{
		Proxy:               nil, // 直连:图片下载不经代理
		DialContext:         (&net.Dialer{Timeout: 8 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		TLSHandshakeTimeout: 8 * time.Second,
	},
}

const maxPhotoBytes = 10 << 20 // Telegram 照片上限约 10MB

// photoFile 决定 sendPhoto 用哪种文件源:
// "file:<id>" → 直接用图片库的 Telegram file_id(秒发、不下载不抓 URL);
// 否则有预下载字节(群发一次下载复用)直接用;再否则尝试后端下载;都失败才回退 FileURL。
func (r *TgBotRuntime) photoFile(res *TgResult, imgURL string) tg.RequestFileData {
	if strings.HasPrefix(imgURL, "file:") {
		return tg.FileID(strings.TrimSpace(imgURL[len("file:"):]))
	}
	if len(res.ImageData) > 0 {
		return tg.FileBytes{Name: res.ImageName, Bytes: res.ImageData}
	}
	if data, name, err := r.downloadImage(imgURL); err == nil {
		return tg.FileBytes{Name: name, Bytes: data}
	} else {
		config.LogWarning("tg 配图下载失败,回退 URL 直发: %v", err)
	}
	return tg.FileURL(imgURL)
}

// downloadImage 由后端把图片 URL 下载成字节,以便以 multipart 上传给 Telegram。
// 用于规避 Telegram 海外服务器拉不到国内 CDN/需 Referer 的图片直链(否则 sendPhoto 报
// "failed to get HTTP URL content")。失败返回 error,调用方回退 FileURL 或纯文本。
func (r *TgBotRuntime) downloadImage(rawURL string) ([]byte, string, error) {
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, "", err
	}
	// 部分 CDN 会拦截无 UA 的默认 Go 客户端,伪装浏览器 UA 更稳
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0 Safari/537.36")
	req.Header.Set("Accept", "image/*,*/*")
	resp, err := imgHTTPClient.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxPhotoBytes+1))
	if err != nil {
		return nil, "", err
	}
	if len(data) == 0 {
		return nil, "", fmt.Errorf("图片内容为空")
	}
	if len(data) > maxPhotoBytes {
		return nil, "", fmt.Errorf("图片超过 %dMB,不适合按照片上传", maxPhotoBytes>>20)
	}
	return data, imageNameFromURL(rawURL, resp.Header.Get("Content-Type")), nil
}

// imageNameFromURL 从 URL/Content-Type 推一个带扩展名的文件名(仅供 multipart 用,Telegram 主要看内容)。
func imageNameFromURL(rawURL, contentType string) string {
	s := rawURL
	if i := strings.IndexAny(s, "?#"); i >= 0 {
		s = s[:i]
	}
	name := "photo"
	if i := strings.LastIndex(s, "/"); i >= 0 && i+1 < len(s) {
		name = s[i+1:]
	}
	if !strings.Contains(name, ".") {
		ext := ".jpg"
		switch {
		case strings.Contains(contentType, "png"):
			ext = ".png"
		case strings.Contains(contentType, "gif"):
			ext = ".gif"
		case strings.Contains(contentType, "web"):
			ext = ".webp"
		}
		name += ext
	}
	return name
}

// editMessage 原地编辑回调消息。Telegram 限制:editMessageMedia 只能改「本就是媒体」的消息,
// editMessageText 改文本消息会把图片残留。故按「目标形态 vs 当前形态」是否一致分派:
//   - 形态不一致(文本↔图片)→ 删旧消息再发新消息(视觉正确,位置会变到底部)。
//   - 两边都是图片 → EditMessageMedia 原地换图+caption+键盘。
//   - 两边都是文本 → EditMessageText 原地改文案+键盘。
//
// 开 HTML 时若因非法标签失败,回退纯文本重试。
func (r *TgBotRuntime) editMessage(q *tg.CallbackQuery, res *TgResult) {
	kb := buildKeyboard(res.Buttons, res.Cols)
	chatID, msgID := q.Message.Chat.ID, q.Message.MessageID
	img := r.safeImage(res)
	if (img != "") != messageHasMedia(q.Message) { // 形态不一致:删旧发新
		r.deleteMessage(chatID, msgID)
		r.send(chatID, res)
		return
	}
	if img != "" { // 当前也是图片:原地换媒体
		media := tg.NewInputMediaPhoto(r.photoFile(res, img)) // 同 send:优先后端下载上传,失败回退 FileURL
		media.Caption = res.Text
		if res.HTML {
			media.ParseMode = tg.ModeHTML
		}
		edit := tg.EditMessageMediaConfig{BaseEdit: tg.BaseEdit{ChatID: chatID, MessageID: msgID, ReplyMarkup: kb}, Media: media}
		if _, err := r.bot.Request(edit); err != nil {
			if res.HTML {
				media.ParseMode = ""
				edit.Media = media
				if _, e2 := r.bot.Request(edit); e2 == nil {
					return
				}
			}
			config.LogWarning("tg edit media: %v", err)
		}
		return
	}
	edit := tg.NewEditMessageText(chatID, msgID, res.Text)
	if res.HTML {
		edit.ParseMode = tg.ModeHTML
	}
	edit.ReplyMarkup = kb
	if _, err := r.bot.Request(edit); err != nil {
		if res.HTML {
			edit.ParseMode = ""
			if _, e2 := r.bot.Request(edit); e2 == nil {
				return
			}
		}
		config.LogWarning("tg edit msg: %v", err) // "Message is not modified" 属良性错误
	}
}

// deleteMessage 删除一条消息(取消按钮 / 图文互转时清旧消息用)。
func (r *TgBotRuntime) deleteMessage(chatID int64, msgID int) {
	if r.bot == nil {
		return
	}
	if _, err := r.bot.Request(tg.DeleteMessageConfig{ChatID: chatID, MessageID: msgID}); err != nil {
		config.LogWarning("tg delete msg: %v", err)
	}
}

// messageHasMedia 判断消息是否带媒体(图/视频/动图/文档/音频/语音/视频笔记/贴纸)。
func messageHasMedia(m *tg.Message) bool {
	if m == nil {
		return false
	}
	return len(m.Photo) > 0 || m.Video != nil || m.Animation != nil ||
		m.Document != nil || m.Audio != nil || m.Voice != nil ||
		m.VideoNote != nil || m.Sticker != nil
}

// ==================== 动作执行引擎(命令与按钮共用) ====================

// execAction 是命令和按钮共用的动作执行器:按 action_type 分派。
//   - menu:渲染子菜单;action_config.page=="root" → parent_id=0(回首页),否则用本条 id 当父页
//   - text:返回固定文案
//   - handler:调注册表拿动态数据
//   - url:作为命令触发时回一条带外链按钮的消息(菜单里的 url 按钮由 Telegram 直开、不走此处)
func (r *TgBotRuntime) execAction(row map[string]interface{}, u *TgUser) *TgResult {
	actionType := fmt.Sprintf("%v", row["action_type"])
	cfg := parseConfig(row["action_config"])
	switch actionType {
	case "menu":
		// 命令跳转:page=root → 根主菜单(parentID=0);否则 cfg.page 存的是目标一级菜单 id,直接展开它
		// (execAction 的 menu 分支仅命令路径会走到;菜单树里的文件夹按钮走 m: 回调不经过这里)
		parentID := int64(0)
		if p := cfg["page"]; p != "" && p != "root" {
			parentID = toInt64(p)
		}
		res := r.menuPageResult(parentID, u)
		// 命令触发打开根主菜单:横幅以「当前命令自身」配置为准(而非固定读 start);未配则回落默认「请选择：」。
		// 导航回根(m:0)无命令上下文,仍由 menuPageResult 兜底读 start,不受这里影响。
		if parentID == 0 {
			res.Text, res.Image, res.HTML = r.resolveRootBanner(cfg, u)
		}
		return res
	case "back":
		// 菜单里的「返回」按钮在 menuPageResult 已直接编成 m: 回调,不走这里;
		// 命令路径无“当前页”上下文,统一回根主菜单。
		return r.menuPageResult(0, u)
	case "cancel":
		// 命令路径无法删除消息,回一句提示文案。
		return &TgResult{Text: "已取消"}
	case "text":
		// 固定文案也接模板变量:{uid}/{first_name}/{time} 等内置占位发送前替换(与横幅/接口模式同一套上下文)
		txt := cfg["text"]
		if u != nil {
			txt = httpRender(txt, httpCtx(u, nil), nil)
		}
		img := cfg["image"]
		if u != nil && strings.HasPrefix(img, "http") { // 配图直链同样支持 {uid} 等变量(file: 前缀的 file_id 不受影响)
			img = httpRender(img, httpCtx(u, nil), nil)
		}
		return &TgResult{Text: txt, Image: img, HTML: cfg["format"] == "html"}
	case "handler":
		return r.run(cfg["handler"], u, parseArgs(cfg["args"]))
	case "http":
		return r.httpResult(cfg, u, nil)
	case "url":
		// 作为命令触发时:URL 无法直接“打开”,回一条带外链按钮的消息让用户点击跳转。
		// (作为菜单里的 url 按钮不会走到这里——外链由 Telegram 直接打开,不产生回调)
		link := cfg["url"]
		if link == "" {
			return nil
		}
		if u != nil {
			link = httpRender(link, httpCtx(u, nil), nil) // 外链也可带用户变量(如带 {uid} 的落地页)
		}
		return &TgResult{
			Text:    "🔗 " + link,
			Buttons: []TgButton{{Text: "🔗 打开链接", Url: link}},
			Cols:    1,
		}
	}
	return nil
}

// menuPageResult 构建某父菜单下的子菜单页:查启用子项,逐条转成按钮
func (r *TgBotRuntime) menuPageResult(parentID int64, u *TgUser) *TgResult {
	res := &TgResult{Text: "请选择：", Cols: 2}
	// 页面每行按钮数取「本容器菜单自身」的 cols(旧实现误读子项 cols 且最后一个覆盖,语义颠倒;根页无容器行用默认 2)
	// backTarget:本容器的父级 id,供「返回上一级」按钮当目标(根页无容器行,保持 0)
	var backTarget int64
	if parentID > 0 {
		if self := r.loadMenuByID(fmt.Sprintf("%d", parentID)); self != nil {
			if c := toInt(self["cols"]); c > 0 {
				res.Cols = c
			}
			backTarget = toInt64(self["parent_id"])
			// 容器自身可配「引导语/横幅配图/富文本」,让菜单页也能做成图+文+按钮形态
			scfg := parseConfig(self["action_config"])
			if t := strings.TrimSpace(scfg["text"]); t != "" {
				if u != nil { // 菜单页引导语同样支持内置变量占位
					t = httpRender(t, httpCtx(u, nil), nil)
				}
				res.Text = t
			}
			res.Image = scfg["image"]
			res.HTML = scfg["format"] == "html"
		}
	} else {
		// 根菜单页(/start 或回根 m:0):根页无对应 tg_menu 行,横幅图/引导语/富文本配在 start 命令上
		if cmd := r.loadCommand("start"); cmd != nil {
			ccfg := parseConfig(cmd["action_config"])
			res.Text, res.Image, res.HTML = r.resolveRootBanner(ccfg, u)
			if c := toInt(ccfg["cols"]); c > 0 { // 根页每行按钮数也可在 start 命令上配(缺省 2)
				res.Cols = c
			}
		}
	}
	manualBack := false // 本容器是否已手动配了「返回」子项,配了就不再自动补
	for _, m := range r.loadMenuChildren(parentID, u.Lang) {
		cfg := parseConfig(m["action_config"])
		btn := TgButton{Text: fmt.Sprintf("%v", m["title"])}
		if toInt(m["cols"]) == 1 {
			btn.FullRow = true // 子项 cols=1 → 该按钮独占一行(同行其余按钮自动断行)
		}
		switch fmt.Sprintf("%v", m["action_type"]) {
		case "menu":
			if cfg["page"] == "root" {
				btn.Cb = "m:0" // 回首页根菜单
			} else {
				btn.Cb = fmt.Sprintf("m:%d", toInt64(m["id"]))
			}
		case "back": // 返回按钮:回根 or 回上一级(渲染期已算好本容器父级)
			manualBack = true
			if cfg["target"] == "root" {
				btn.Cb = "m:0"
			} else {
				btn.Cb = fmt.Sprintf("m:%d", backTarget)
			}
		case "cancel": // 取消按钮:点击删除本条消息(c: 域)
			btn.Cb = "c:0"
		case "handler":
			btn.Cb = fmt.Sprintf("h:%d", toInt64(m["id"]))
		case "http": // 复用 h: 域:点击→runHandlerByMenu 回查该行→execAction 按 action_type 路由到 http 分支
			btn.Cb = fmt.Sprintf("h:%d", toInt64(m["id"]))
		case "pick_user": // 选择收款人:点击走 p: 回调 → menuPickUser 发选人页(横幅+原生选人器回复键盘)
			btn.Cb = fmt.Sprintf("p:%d", toInt64(m["id"]))
		case "url":
			btn.Url = cfg["url"]
		case "text":
			btn.Cb = fmt.Sprintf("t:%d", toInt64(m["id"]))
		}
		res.Buttons = append(res.Buttons, btn)
	}
	// 所有子菜单页默认自带「返回上一级」按钮(无需后台手动加 back 项);根页不加,已手动配 back 子项则不重复(兼容旧数据)
	if parentID > 0 && !manualBack {
		label := "⬅️ 返回上一级"
		if backTarget == 0 {
			label = "⬅️ 返回主菜单" // 本容器就是一级菜单,上一级即根主菜单
		}
		res.Buttons = append(res.Buttons, TgButton{Text: label, Cb: fmt.Sprintf("m:%d", backTarget)})
	}
	return res
}

// runHandlerByMenu 菜单里 action_type=handler/http 的按钮:按 id 回查该行配置再执行
func (r *TgBotRuntime) runHandlerByMenu(id string, u *TgUser) *TgResult {
	row := r.loadMenuByID(id)
	if row == nil {
		return nil
	}
	if fmt.Sprintf("%v", row["action_type"]) == "http" {
		return r.menuHTTPResult(toInt64(id), u, nil)
	}
	return r.execAction(row, u)
}

// menuHTTPResult 菜单 http 按钮的统一入口:配了 input_prompt 且本次没有用户输入时,
// 不立即调接口,而是登记「等待输入」状态并回提示语(ForceReply 高亮输入框);
// 其余情况直接执行取数。extraCtx 为恢复执行时注入的用户输入变量。
func (r *TgBotRuntime) menuHTTPResult(menuID int64, u *TgUser, extraCtx map[string]string) *TgResult {
	row := r.loadMenuByID(fmt.Sprintf("%d", menuID))
	if row == nil {
		return nil
	}
	cfg := parseConfig(row["action_config"])
	if prompt := strings.TrimSpace(cfg["input_prompt"]); prompt != "" && extraCtx == nil {
		key := strings.TrimSpace(cfg["input_key"])
		if key == "" {
			key = "input"
		}
		r.setAwait(u.ChatID, &awaitInput{menuID: menuID, varKey: key, expire: time.Now().Add(awaitInputTTL)})
		// 与 pick_user 的反馈提示语一致:发送前过 httpRender,支持 {uid}/{first_name}/{time} 等内置变量
		return &TgResult{Text: httpRender(prompt, httpCtx(u, parseArgs(cfg["args"])), nil), ForceReply: true, HTML: cfg["format"] == "html"}
	}
	return r.httpResult(cfg, u, extraCtx)
}

// setAwait 登记某会话的等待输入状态(覆盖旧值:重新点了别的等待菜单就以新的为准)
func (r *TgBotRuntime) setAwait(chatID int64, aw *awaitInput) {
	r.awaitMu.Lock()
	defer r.awaitMu.Unlock()
	r.awaitInputs[chatID] = aw
}

// consumeAwait 取出并清除该会话的等待输入状态;未登记或已超时返回 nil(超时顺带清掉)
func (r *TgBotRuntime) consumeAwait(chatID int64) *awaitInput {
	r.awaitMu.Lock()
	defer r.awaitMu.Unlock()
	aw := r.awaitInputs[chatID]
	if aw == nil {
		return nil
	}
	delete(r.awaitInputs, chatID)
	if time.Now().After(aw.expire) {
		return nil
	}
	return aw
}

// peekAwait 只看不取:返回该会话未超时的等待状态(超时顺带清掉),用于取消判定等不需消费的场景
func (r *TgBotRuntime) peekAwait(chatID int64) *awaitInput {
	r.awaitMu.Lock()
	defer r.awaitMu.Unlock()
	aw := r.awaitInputs[chatID]
	if aw == nil {
		return nil
	}
	if time.Now().After(aw.expire) {
		delete(r.awaitInputs, chatID)
		return nil
	}
	return aw
}

// clearAwait 清除某会话的等待状态(取消时调用)
func (r *TgBotRuntime) clearAwait(chatID int64) {
	r.awaitMu.Lock()
	defer r.awaitMu.Unlock()
	delete(r.awaitInputs, chatID)
}

// ==================== 原生「选择收款人」(action_type=pick_user) ====================
//
// 交互:点菜单项 → 弹固定默认的「选人页」(原生 request_users 回复键盘) → 用户选一个真人 →
// bot 收到对方账户ID → 先回一条自定义「反馈信息」(feedback_text) → 再用 {payee} 调配置的 http 接口
// → 把响应填进模板回给用户。选人页文案/按钮全用固定默认,不在后台配。
const (
	pickUserPageText   = "请使用以下方法选择一个用户："
	pickUserPickBtn    = "🔍 选择用户"
	pickUserCancelText = "❌ 取消"
)

// menuPickUser 用户点「选择收款人」类菜单项:发固定默认选人页(带 request_users 的回复键盘),
// 并登记「等待选人」状态。用户经原生选择器选完后,回传 users_shared → handlePickedChat 续跑。
func (r *TgBotRuntime) menuPickUser(menuID int64, u *TgUser) {
	if r.loadMenuByID(fmt.Sprintf("%d", menuID)) == nil {
		return
	}
	// 选人页固定默认:发页 → 记住消息 ID(选完/取消要删掉它) → 登记等待选人
	msgID := r.sendPickUserPage(u.ChatID)
	r.setAwait(u.ChatID, &awaitInput{menuID: menuID, expire: time.Now().Add(awaitInputTTL), kind: "chat", cancelText: pickUserCancelText, pageChatID: u.ChatID, pageMsgID: msgID})
}

// sendPickUserPage 用原生 HTTP 发「选人页」:固定文案 + 一个带 request_users 的回复键盘按钮 + 取消按钮。
// 因 v5.5.1 KeyboardButton 无 request_users 字段,回复键盘只能手拼 JSON 走 tgCall。
// 用 request_users(Bot API 7.0)而非 request_chat:前者弹「选择用户」选真人,后者弹「选择群组/会话」。
// 返回发出消息的 message_id(供选完后删页;失败返回 0)。
func (r *TgBotRuntime) sendPickUserPage(chatID int64) int {
	rb, _ := json.Marshal(map[string]interface{}{
		"keyboard": [][]map[string]interface{}{
			{{"text": pickUserPickBtn, "request_users": map[string]interface{}{
				"request_id": 1, "user_is_bot": false, "max_quantity": 1, "request_username": true,
			}}},
			{{"text": pickUserCancelText}},
		},
		"resize_keyboard":   true,
		"one_time_keyboard": true,
	})
	params := url.Values{
		"chat_id":      {strconv.FormatInt(chatID, 10)},
		"text":         {pickUserPageText},
		"reply_markup": {string(rb)},
	}
	res, err := r.tgCall("sendMessage", params)
	if err != nil {
		config.LogWarning("pick_user 发选人页失败: %v", err)
	}
	return parseTgMsgID(res)
}

// finishPickUserPage 结束选人流程:删掉那张「请选择一个收款人」页(选完后横幅留着很怪),
// 再按需另发一条干净的收尾消息(text 传空则只删不发)。回复键盘 one_time_keyboard 按下已自动收起,无需再处理。
func (r *TgBotRuntime) finishPickUserPage(aw *awaitInput, text string) {
	if aw.pageMsgID > 0 {
		r.deleteMessage(aw.pageChatID, aw.pageMsgID)
	}
	if strings.TrimSpace(text) != "" {
		r.send(aw.pageChatID, &TgResult{Text: text})
	}
}

// parseTgMsgID 从 tgCall 返回的 Message JSON 里取 message_id(失败返回 0)。
func parseTgMsgID(res json.RawMessage) int {
	var m struct {
		MessageID int `json:"message_id"`
	}
	_ = json.Unmarshal(res, &m)
	return m.MessageID
}

// handlePickedChat 原生选人器回传后:删掉选人页 → 把反馈信息(feedback_text)当提示语让用户输入(如金额) →
// 拿输入的 {input} 连同 {payee} 调该菜单配置的 http 接口回填模板;未配反馈信息则选完直接调接口。
func (r *TgBotRuntime) handlePickedChat(chatID int64, u *TgUser, pc *pickedChat) {
	aw := r.consumeAwait(chatID)
	if aw == nil || aw.kind != "chat" {
		return // 不在选人等待态(可能已超时/重启):忽略
	}
	name := pc.title
	if name == "" {
		name = pc.username
	}
	if name == "" {
		name = fmt.Sprintf("%d", pc.chatID)
	}
	r.finishPickUserPage(aw, "") // 先删掉那张选人页(选完横幅留着很怪),收尾消息下面按需另发
	row := r.loadMenuByID(fmt.Sprintf("%d", aw.menuID))
	if row == nil {
		return
	}
	cfg := parseConfig(row["action_config"])
	// 选中的收款人作为最高优先级模板变量注入(httpResult 内 extraCtx 覆盖内置变量)
	payeeCtx := map[string]string{
		"payee":          strconv.FormatInt(pc.chatID, 10),
		"payee_name":     name,
		"payee_username": pc.username,
	}
	// 反馈信息:既作选完的确认、又作「等待输入」的提示语(带 ForceReply 高亮输入框)。用户下一条文本作 {input},
	// 连同 {payee} 一起调接口。不配反馈信息则跳过输入环节、直接调接口。
	if fb := strings.TrimSpace(cfg["feedback_text"]); fb != "" {
		key := strings.TrimSpace(cfg["input_key"])
		if key == "" {
			key = "input"
		}
		r.setAwait(chatID, &awaitInput{menuID: aw.menuID, varKey: key, expire: time.Now().Add(awaitInputTTL), kind: "text", payeeCtx: payeeCtx})
		fbCtx := httpCtx(u, parseArgs(cfg["args"]))
		for k, v := range payeeCtx {
			fbCtx[k] = v
		}
		r.send(chatID, &TgResult{Text: httpRender(fb, fbCtx, nil), ForceReply: true, HTML: cfg["format"] == "html"})
		return
	}
	// 没配反馈信息:直接出结果——配了接口就调接口,没配接口则把「显示文案」模板直接渲染回给用户
	if strings.TrimSpace(cfg["url"]) != "" || strings.TrimSpace(cfg["text"]) != "" {
		if res := r.httpResult(cfg, u, payeeCtx); res != nil {
			r.appendBackButton(res, aw.menuID)
			r.send(chatID, res)
		}
	}
}

// resolveRootBanner 计算根横幅的引导语文本/配图/富文本标记(供 /start 命令与回根 m:0 两处共用)。
// 引导语来源由 cfg["text_source"] 决定:
//   - 缺省/static:用 cfg["text"] 静态文本(空则「请选择：」);文本同样过一遍 httpRender,
//     支持 {uid}/{first_name}/{time} 等内置变量(不传接口,故 {data.xxx} 响应路径取不到、会变空串)。
//   - http:调接口渲染,结果按 chat_id 缓存 bannerTTL 秒(见结构体注释);接口失败回退静态文本且
//     不写缓存(下次重试)。配图恒用静态 cfg["image"],不随接口变化。
func (r *TgBotRuntime) resolveRootBanner(cfg map[string]string, u *TgUser) (text, image string, html bool) {
	image = cfg["image"]
	html = cfg["format"] == "html"
	text = strings.TrimSpace(cfg["text"])
	if text == "" {
		text = "请选择："
	} else if u != nil {
		// 静态文本也接模板变量:复用 http 引擎的用户/时间上下文(resp 传 nil 即不查接口)
		text = httpRender(text, httpCtx(u, nil), nil)
	}
	if cfg["text_source"] != "http" || u == nil {
		return
	}
	key := fmt.Sprintf("%d|%s|%s", u.ChatID, cfg["api_url"], cfg["api_text"]) // 含接口签名:多命令开根菜单配不同接口不串,改配置也即时失效
	if r.bannerTTL > 0 {                                                      // bannerCacheSec=0 时不缓存,每次实时拉接口
		if cached, ok := r.bannerCacheGet(key); ok {
			return cached, image, html
		}
	}
	if apiText, ok := bannerAPIText(cfg, u); ok {
		if r.bannerTTL > 0 {
			r.bannerCacheSet(key, apiText)
		}
		return apiText, image, html
	}
	return text, image, html // 接口失败:静态兜底
}

// bannerCacheGet 读某会话的横幅引导语缓存;缺失或过期返回 false
func (r *TgBotRuntime) bannerCacheGet(key string) (string, bool) {
	r.bannerMu.Lock()
	defer r.bannerMu.Unlock()
	e, ok := r.bannerCache[key]
	if !ok || time.Now().After(e.expire) {
		return "", false
	}
	return e.text, true
}

// bannerCacheSet 写缓存;条目过多时顺带清理过期项,防长期累积的会话占内存
func (r *TgBotRuntime) bannerCacheSet(key, text string) {
	now := time.Now()
	r.bannerMu.Lock()
	defer r.bannerMu.Unlock()
	r.bannerCache[key] = bannerEntry{text: text, expire: now.Add(r.bannerTTL)}
	if len(r.bannerCache) > 4096 {
		for k, e := range r.bannerCache {
			if now.After(e.expire) {
				delete(r.bannerCache, k)
			}
		}
	}
}

// ==================== DB 读取(原生 SQL,照 shortplay 习惯) ====================

// loadMenuChildren 查某父菜单下启用的子项(all 或匹配语言的都能出)
func (r *TgBotRuntime) loadMenuChildren(parentID int64, lang string) []map[string]interface{} {
	list := make([]map[string]interface{}, 0)
	r.db.Raw("select id,title,action_type,action_config,cols from tg_menu"+
		" where parent_id = ? and status = 1 and (lang = 'all' or lang = ?) order by sort asc, id asc",
		parentID, lang).Scan(&list)
	return list
}

// loadMenuByID 按 id 读单条菜单(启用),供 handler/text 按钮回查配置
func (r *TgBotRuntime) loadMenuByID(id string) map[string]interface{} {
	list := make([]map[string]interface{}, 0)
	r.db.Raw("select id,parent_id,action_type,action_config,cols from tg_menu where id = ? and status = 1 limit 1", id).Scan(&list)
	if len(list) == 0 {
		return nil
	}
	return list[0]
}

// loadCommand 按命令名(去掉斜杠)读启用的命令行
func (r *TgBotRuntime) loadCommand(cmd string) map[string]interface{} {
	name := strings.TrimPrefix(cmd, "/")
	list := make([]map[string]interface{}, 0)
	r.db.Raw("select id,action_type,action_config from tg_command where command = ? and status = 1 limit 1", name).Scan(&list)
	if len(list) == 0 {
		return nil
	}
	return list[0]
}

// ensureUser 按 tg_user_id 查用户,查不到就落库;返回分发层用的 *TgUser
func (r *TgBotRuntime) ensureUser(from *tg.User, chatID int64) *TgUser {
	u := &TgUser{TgUserID: from.ID, ChatID: chatID, Lang: from.LanguageCode, Username: from.UserName, FirstName: from.FirstName}
	if u.Lang == "" {
		u.Lang = "en"
	}
	list := make([]map[string]interface{}, 0)
	r.db.Raw("select bind_user_id,lang from tg_user where tg_user_id = ? limit 1", from.ID).Scan(&list)
	if len(list) == 0 {
		now := time.Now()
		if err := r.db.Table("tg_user").Create(map[string]interface{}{
			"tg_user_id": from.ID,
			"chat_id":    chatID,
			"username":   from.UserName,
			"first_name": from.FirstName,
			"lang":       u.Lang,
			"created_at": now,
			"updated_at": now,
		}).Error; err != nil {
			config.LogWarning("tg ensureUser create %d: %v", from.ID, err)
		}
		return u
	}
	u.Lang = fmt.Sprintf("%v", list[0]["lang"])
	u.BindID = toInt64(list[0]["bind_user_id"])
	return u
}

// ==================== 原生命令菜单(§5.5) ====================

// syncCommands 把 tg_command 里启用命令注册成原生「菜单」按钮内容(启动时 + 后台改命令后调用)
func (r *TgBotRuntime) syncCommands() {
	if r.bot == nil {
		return
	}
	list := make([]map[string]interface{}, 0)
	r.db.Raw("select command, description from tg_command where status = 1 order by sort").Scan(&list)
	cmds := make([]tg.BotCommand, 0, len(list))
	for _, m := range list {
		cmds = append(cmds, tg.BotCommand{
			Command:     fmt.Sprintf("%v", m["command"]),     // 小写、不含斜杠
			Description: fmt.Sprintf("%v", m["description"]), // 「菜单」按钮里显示的说明
		})
	}
	if len(cmds) == 0 {
		return
	}
	if _, err := r.bot.Request(tg.NewSetMyCommands(cmds...)); err != nil {
		config.LogWarning("tg setMyCommands: %v", err)
	}
}

// SyncCommands 后台「同步命令」接口:改完 tg_command 后刷新原生「菜单」按钮
func (r *TgBotRuntime) SyncCommands(c *gin.Context) {
	r.syncCommands()
	c.JSON(200, gin.H{"code": 0, "message": "命令已同步"})
}

// SendUserMessage 后台主动给指定用户发消息(支持群发)。入参 {ids: "1,2,3"(tg_user 行 id 逗号串), text, image?, format?}。
// 批量查出 chat_id(为 0 时回落 tg_user_id,私聊两者相等),逐个经 send 推送;
// 捕获每个用户的发送错误(如用户从未启动会话 / 拉黑 bot 会返回 403),汇总成功/失败回传前端。
func (r *TgBotRuntime) SendUserMessage(c *gin.Context) {
	raw := make(map[string]interface{})
	_ = c.BindJSON(&raw)
	getStr := func(k string) string {
		if v, ok := raw[k]; ok {
			if s, ok := v.(string); ok {
				return strings.TrimSpace(s)
			}
			return strings.TrimSpace(fmt.Sprintf("%v", v))
		}
		return ""
	}
	ids := tools.SplitIds(getStr("ids"))
	text := getStr("text")
	image := getStr("image")
	if len(ids) == 0 || (text == "" && image == "") {
		c.JSON(200, gin.H{"code": 400, "message": "参数错误:未选择用户或消息内容为空"})
		return
	}
	if r.bot == nil {
		c.JSON(200, gin.H{"code": 500, "message": "bot 未初始化,无法发送(检查 botToken)"})
		return
	}
	// 一次性查出目标用户的 chat_id,按 id 建索引便于回填失败名单
	rows := make([]map[string]interface{}, 0)
	r.db.Raw("select id,chat_id,tg_user_id,username from tg_user where id in (?)", ids).Scan(&rows)
	byID := make(map[int64]map[string]interface{}, len(rows))
	for _, m := range rows {
		byID[toInt64(m["id"])] = m
	}
	html := getStr("format") == "html"
	// 配图若在:群发前在循环外下载一次,复用到每个用户(避免 N 个用户下载 N 次)。
	// 下载成功则 sendPhoto 走 multipart 上传(Telegram 不必再抓 URL);失败则留空,由 send 内回退 FileURL。
	var imgData []byte
	var imgName string
	if image != "" && validTgURL(image) {
		if d, n, e := r.downloadImage(image); e == nil {
			imgData, imgName = d, n
		} else {
			config.LogWarning("群发配图下载失败,回退 URL 直发: %v", e)
		}
	}
	var ok int
	var failed []string
	for _, idv := range ids {
		id := toInt64(idv)
		row, exist := byID[id]
		if !exist {
			failed = append(failed, fmt.Sprintf("%d(用户不存在)", id))
			continue
		}
		chatID := toInt64(row["chat_id"])
		if chatID == 0 {
			chatID = toInt64(row["tg_user_id"]) // 私聊 chat_id == 用户 id 的兜底
		}
		res := &TgResult{Text: text, Image: image, HTML: html, ImageData: imgData, ImageName: imgName}
		if err := r.send(chatID, res); err != nil {
			name := fmt.Sprintf("%v", row["username"])
			if name == "" || name == "<nil>" {
				name = fmt.Sprintf("%d", id)
			}
			failed = append(failed, fmt.Sprintf("%s(%v)", name, err))
			continue
		}
		ok++
	}
	msg := fmt.Sprintf("已发送 %d 条", ok)
	if len(failed) > 0 {
		msg += fmt.Sprintf(",%d 条失败", len(failed))
	}
	c.JSON(200, gin.H{"code": 0, "message": msg, "data": gin.H{"ok": ok, "failed": failed}})
}

// SendChatMessage 后台主动往指定群组/频道发消息(支持多选群发)。入参 {ids: "1,2,3"(tg_chat 行 id 逗号串), text, image?, format?}。
// 与 SendUserMessage 同构:批量查出 chat_id 后逐个经 send 推送(群组与私聊/频道走同一条发送链路);
// 失败常见于 bot 已被移出该群/无发言权限(403/429),逐群汇总成功/失败回传前端。群名优先用 title,空则回落 @username/chat_id。
func (r *TgBotRuntime) SendChatMessage(c *gin.Context) {
	raw := make(map[string]interface{})
	_ = c.BindJSON(&raw)
	getStr := func(k string) string {
		if v, ok := raw[k]; ok {
			if s, ok := v.(string); ok {
				return strings.TrimSpace(s)
			}
			return strings.TrimSpace(fmt.Sprintf("%v", v))
		}
		return ""
	}
	ids := tools.SplitIds(getStr("ids"))
	text := getStr("text")
	image := getStr("image")
	if len(ids) == 0 || (text == "" && image == "") {
		c.JSON(200, gin.H{"code": 400, "message": "参数错误:未选择群组或消息内容为空"})
		return
	}
	if r.bot == nil {
		c.JSON(200, gin.H{"code": 500, "message": "bot 未初始化,无法发送(检查 botToken)"})
		return
	}
	rows := make([]map[string]interface{}, 0)
	r.db.Raw("select id,chat_id,title,username from tg_chat where id in (?)", ids).Scan(&rows)
	byID := make(map[int64]map[string]interface{}, len(rows))
	for _, m := range rows {
		byID[toInt64(m["id"])] = m
	}
	html := getStr("format") == "html"
	// 配图若在:群发前在循环外下载一次,复用到每个群(避免 N 个群下载 N 次)。
	var imgData []byte
	var imgName string
	if image != "" && validTgURL(image) {
		if d, n, e := r.downloadImage(image); e == nil {
			imgData, imgName = d, n
		} else {
			config.LogWarning("群组配图下载失败,回退 URL 直发: %v", e)
		}
	}
	var ok int
	var failed []string
	for _, idv := range ids {
		id := toInt64(idv)
		row, exist := byID[id]
		if !exist {
			failed = append(failed, fmt.Sprintf("%d(群组不存在)", id))
			continue
		}
		chatID := toInt64(row["chat_id"])
		res := &TgResult{Text: text, Image: image, HTML: html, ImageData: imgData, ImageName: imgName}
		if err := r.send(chatID, res); err != nil {
			name := fmt.Sprintf("%v", row["title"])
			if name == "" || name == "<nil>" {
				if un := fmt.Sprintf("%v", row["username"]); un != "" && un != "<nil>" {
					name = "@" + un
				} else {
					name = fmt.Sprintf("%d", chatID)
				}
			}
			failed = append(failed, fmt.Sprintf("%s(%v)", name, err))
			continue
		}
		ok++
	}
	msg := fmt.Sprintf("已发送到 %d 个群组", ok)
	if len(failed) > 0 {
		msg += fmt.Sprintf(",%d 个失败", len(failed))
	}
	c.JSON(200, gin.H{"code": 0, "message": msg, "data": gin.H{"ok": ok, "failed": failed}})
}

// ==================== 本地 getUpdates 长轮询调试入口(配置开关) ====================

// startPolling 本地长轮询调试入口(telegram.debugPolling=true;webhook 与 getUpdates 互斥)。
// 用原生 getUpdates 而非库的 GetUpdatesChan:v5.5.1 的 tg.Update/tg.Message 无 users_shared/chat_shared 字段,
// 库解码会把选人器回传的服务消息直接丢弃,导致 request_users 选完人收不到。这里拿每条 Update 的原始 JSON,
// 走与 webhook 完全相同的双解析(captureChatShared)后再入队,保证两种模式行为一致。
func (r *TgBotRuntime) startPolling() {
	if r.bot == nil {
		config.LogWarning("tg 轮询跳过:bot 未初始化")
		return
	}
	// 轮询前删掉可能存在的 webhook,否则 Telegram 返回 409 conflict
	if _, err := r.bot.Request(tg.DeleteWebhookConfig{}); err != nil {
		config.LogWarning("tg deleteWebhook(轮询前): %v", err)
	}
	token := strings.TrimSpace(config.Conf.GetString("telegram.botToken"))
	endpoint := "https://api.telegram.org/bot" + token + "/getUpdates"
	// 长轮询 timeout=50s,故 ResponseHeaderTimeout 需 >50s;不设整体 Timeout 以免提前掐断长轮询。
	client := &http.Client{
		Transport: &http.Transport{
			Proxy:                 http.ProxyFromEnvironment,
			DialContext:           (&net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
			TLSHandshakeTimeout:   5 * time.Second,
			ResponseHeaderTimeout: 60 * time.Second,
		},
	}
	config.LogInfo("tg 本地长轮询已启动(getUpdates 调试模式)")
	var offset int64
	for {
		form := url.Values{"timeout": {"50"}}
		if offset > 0 {
			form.Set("offset", strconv.FormatInt(offset, 10)) // 从上次已处理位置之后拉取
		}
		resp, err := client.PostForm(endpoint, form)
		if err != nil {
			config.LogWarning("tg getUpdates 请求失败,3s 后重试: %v", err)
			time.Sleep(3 * time.Second)
			continue
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		var out struct {
			OK          bool              `json:"ok"`
			Result      []json.RawMessage `json:"result"` // 保留每条 Update 原始 JSON,供 captureChatShared 双解析
			Description string            `json:"description"`
		}
		if json.Unmarshal(body, &out) != nil || !out.OK {
			config.LogWarning("tg getUpdates 响应异常,3s 后重试: %s", out.Description)
			time.Sleep(3 * time.Second)
			continue
		}
		for _, raw := range out.Result {
			var up tg.Update
			if json.Unmarshal(raw, &up) != nil {
				continue
			}
			offset = int64(up.UpdateID) + 1
			r.captureChatShared(up.UpdateID, raw) // 与 webhook 同一套:捕获 request_users/request_chat 选人回传
			r.enqueue(&up)                        // 与 webhook 通道共用按 chat_id 的分片路由
		}
	}
}

// ==================== 上线 webhook 自动注册入口(配置开关) ====================

// registerWebhook 由 telegram.debugPolling=false 触发:启动时自动向 Telegram 注册回调地址。
// 用原生 HTTP POST 调 setWebhook 而非库的 WebhookConfig——因 v5.5.1 的 WebhookConfig 无 secret_token 字段,
// 若不带密钥注册,Telegram 回调时不会回传 X-Telegram-Bot-Api-Secret-Token,将被 Webhook() 校验拦成 403。
// url = publicBase(去尾斜杠) + webhookPath;secret_token 与 Webhook() 读取的同一配置项保持一致。
func (r *TgBotRuntime) registerWebhook() {
	if r.bot == nil {
		config.LogWarning("tg webhook 注册跳过:bot 未初始化(botToken 未配或校验失败)")
		return
	}
	token := config.Conf.GetString("telegram.botToken")
	base := strings.TrimRight(config.Conf.GetString("telegram.publicBase"), "/")
	path := config.Conf.GetString("telegram.webhookPath")
	secret := config.Conf.GetString("telegram.secretToken")
	if base == "" || strings.Contains(base, "你的域名") {
		config.LogWarning("tg webhook 注册跳过:telegram.publicBase 未配置为真实公网 HTTPS 域名(当前=%q)", base)
		return
	}
	hookURL := base + path

	form := url.Values{}
	form.Set("url", hookURL)
	if secret != "" {
		form.Set("secret_token", secret)
	}
	form.Set("drop_pending_updates", "true") // 切 webhook 时丢弃轮询遗留的旧更新,避免上线瞬间刷屏

	// 复用带代理的短超时客户端:境内无出网时快速失败、只打 warning、不阻断服务启动
	client := &http.Client{
		Transport: &http.Transport{Proxy: http.ProxyFromEnvironment, DialContext: (&net.Dialer{Timeout: 5 * time.Second}).DialContext, TLSHandshakeTimeout: 5 * time.Second},
		Timeout:   15 * time.Second,
	}
	resp, err := client.PostForm("https://api.telegram.org/bot"+token+"/setWebhook", form)
	if err != nil {
		config.LogWarning("tg setWebhook 请求失败: %v", err)
		return
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	config.LogInfo("tg setWebhook(%s) 响应: %s", hookURL, strings.TrimSpace(string(body)))
}
