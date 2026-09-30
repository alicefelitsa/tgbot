package route

import (
	"net/http"
	"tgbot/config"
	"tgbot/controller"
	"tgbot/middleware"

	"github.com/gin-gonic/gin"
)

// SetupRouter 设置路由地址。本轮落地通道层 Webhook + 分发层;
// 后台 CRUD(tg_menu/tg_handler/tg_user/tg_command)后续接入。
func SetupRouter() *gin.Engine {
	router := gin.New()
	router.Use(gin.Logger())
	router.Use(gin.Recovery())
	router.Use(middleware.Cors())
	router.NoRoute(func(c *gin.Context) {
		c.JSON(http.StatusNotFound, gin.H{
			"code":    404,
			"message": "页面不存在",
		})
	})

	// Bot 运行时先建(启动即拉起分发循环、注册原生命令菜单,并按配置开启本地轮询)
	tgBot := controller.NewTgBotRuntime()

	// 管理后台接口（挂 BossAuth，login 等在其白名单内放行）
	boss := router.Group("/api/boss", middleware.BossAuth)
	{
		bossController := controller.NewBossController()
		boss.POST("/login", middleware.LoginRateLimit(), bossController.AdminLogin) // login 在 BossAuth 白名单里放行;额外挂 IP 限流挡爆破
		boss.GET("/logout", bossController.AdminLogout)                             // logout 在 BossAuth 白名单里放行
		boss.GET("/auth/user", bossController.AuthUser)

		tg := controller.NewTgController()
		// 菜单树
		boss.GET("/GetTgMenuList", tg.GetTgMenuList)
		boss.POST("/AddTgMenu", tg.AddTgMenu)
		boss.POST("/SaveTgMenu", tg.SaveTgMenu)
		boss.GET("/DelTgMenu", tg.DelTgMenu)
		// 动态数据类白名单
		boss.GET("/GetTgHandlerList", tg.GetTgHandlerList)
		boss.POST("/AddTgHandler", tg.AddTgHandler)
		boss.POST("/SaveTgHandler", tg.SaveTgHandler)
		boss.GET("/DelTgHandler", tg.DelTgHandler)
		// Telegram 用户(以查/改/删为主)
		boss.GET("/GetTgUserList", tg.GetTgUserList)
		boss.POST("/SaveTgUser", tg.SaveTgUser)
		boss.GET("/DelTgUser", tg.DelTgUser)
		// 主动给指定用户发消息(需 bot 运行时,故挂在 tgBot 上)
		boss.POST("/SendTgUserMessage", tgBot.SendUserMessage)
		// Telegram 群组/频道(bot 被拉进群时自动落库;以查/改/删 + 选群群发为主)
		boss.GET("/GetTgChatList", tg.GetTgChatList)
		boss.POST("/SaveTgChat", tg.SaveTgChat)
		boss.GET("/DelTgChat", tg.DelTgChat)
		boss.POST("/SendTgChatMessage", tgBot.SendChatMessage)
		// 图片库(存 Telegram file_id,供各处配图复用;入库/预览需 bot,故挂在 tgBot 上)
		boss.GET("/GetTgImageList", tgBot.GetTgImageList)
		boss.POST("/AddTgImage", tgBot.AddTgImage)
		boss.POST("/SaveTgImage", tgBot.SaveTgImage)
		boss.GET("/DelTgImage", tgBot.DelTgImage)
		boss.GET("/GetTgImagePreview", tgBot.GetTgImagePreview)
		// 系统设置(运行时配置存库,后台改完即生效)
		boss.GET("/GetSysSettingList", tg.GetSysSettingList)
		boss.GET("/GetSysSettingMap", tg.GetSysSettingMap)
		boss.POST("/SaveSysSettingBatch", tg.SaveSysSettingBatch)
		boss.POST("/AddSysSetting", tg.AddSysSetting)
		boss.POST("/SaveSysSetting", tg.SaveSysSetting)
		boss.GET("/DelSysSetting", tg.DelSysSetting)
		// 命令菜单(原生「菜单」按钮)
		boss.GET("/GetTgCommandList", tg.GetTgCommandList)
		boss.POST("/AddTgCommand", tg.AddTgCommand)
		boss.POST("/SaveTgCommand", tg.SaveTgCommand)
		boss.GET("/DelTgCommand", tg.DelTgCommand)
		// 改完命令后刷新原生「菜单」按钮
		boss.POST("/SyncTgCommand", tgBot.SyncCommands)
		// 机器人资料(名称/简介):存库回显 + 通过 Bot API setMyName/setMyShortDescription/setMyDescription 同步(头像无接口,只能 @BotFather)
		boss.POST("/SaveBotProfile", tgBot.SaveBotProfile)
		boss.GET("/GetBotProfile", tgBot.GetBotProfile)
	}

	// Mock 调试接口:自建两个稳定测试接口(不鉴权),供后台 http/pick_user 菜单填地址联调,免依赖第三方
	test := router.Group("/api/test")
	{
		test.GET("/get", controller.MockGet)    // GET:把收到的查询参数原样回显进 data
		test.POST("/post", controller.MockPost) // POST:把收到的 JSON body 原样回显进 data
	}

	// Telegram 回调：公开组，不挂 BossAuth，用 secret token 头校验
	router.POST(config.Conf.GetString("telegram.webhookPath"), tgBot.Webhook)

	return router
}
