package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"

	"tgbot/config"
	"tgbot/route"
)

func main() {
	gin.SetMode(gin.ReleaseMode)  // 设置为生产模式
	router := route.SetupRouter() // 设置路由地址

	webPort := config.Conf.GetString("server.webPort")
	// 用显式 http.Server 而非 router.Run:以便拿到 Shutdown 能力,并给读/写设超时防慢连接(Slowloris)。
	srv := &http.Server{
		Addr:              ":" + webPort,
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	// 监听退出信号(Ctrl+C / systemd stop / docker stop),触发优雅关停
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		config.LogInfo("服务器已启动,监听端口 :%s", webPort)
		// ErrServerClosed 是 Shutdown 主动关闭的正常返回,不算启动失败
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal("服务器启动失败：", err)
		}
	}()

	<-ctx.Done() // 阻塞直到收到退出信号
	config.LogInfo("收到退出信号,开始优雅停机(最多等待 10s 让在途请求处理完)...")

	// 给在途请求最多 10s 收尾:停止接受新连接,等待活跃连接处理完;超时则强制关闭
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		config.LogWarning("HTTP 优雅关闭超时/失败: %v", err)
	}

	// 关闭数据库连接池,释放到远程 MySQL 的连接(bot 分片消费 goroutine 为守护协程,随进程退出自然结束)
	if sqlDB, err := config.Mysql.DB(); err == nil {
		_ = sqlDB.Close()
	}
	config.LogInfo("服务已安全停止")
	config.LoggerClose()
}
