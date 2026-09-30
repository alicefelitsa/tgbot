package main

import (
	"log"

	"github.com/gin-gonic/gin"

	"tgbot/config"
	"tgbot/route"
)

func main() {
	gin.SetMode(gin.ReleaseMode)  // 设置为生产模式
	router := route.SetupRouter() // 设置路由地址
	// 启动服务
	if err := router.Run(":" + config.Conf.GetString("server.webPort")); err != nil {
		log.Fatal("服务器启动失败：", err)
	}
}
