package middleware

import (
	"net/http"
	"sync"
	"tgbot/config"
	"time"

	"github.com/gin-gonic/gin"
)

// fixedWindowLimiter 简单内存限流器:按 key(通常是客户端 IP)在固定时间窗内计数,
// 超过 max 即拒绝。适合登录这类低频、无需精确令牌桶的防护场景(单机内存态,重启清零;
// 多实例部署时各限各的,如需全局一致可换 Redis 计数——本项目暂未引入 Redis)。
type fixedWindowLimiter struct {
	mu     sync.Mutex
	counts map[string]*winEntry
	max    int
	window time.Duration
}

type winEntry struct {
	count int
	reset time.Time // 该 key 当前窗口的重置时刻
}

func newFixedWindowLimiter(max int, window time.Duration) *fixedWindowLimiter {
	l := &fixedWindowLimiter{counts: map[string]*winEntry{}, max: max, window: window}
	go l.janitor() // 定期清理过期条目,防 map 无界增长
	return l
}

// allow 判断 key 在当前窗口内是否仍被允许;允许则计数 +1,超限返回 false。
func (l *fixedWindowLimiter) allow(key string) bool {
	now := time.Now()
	l.mu.Lock()
	defer l.mu.Unlock()
	e, ok := l.counts[key]
	if !ok || now.After(e.reset) { // 新窗口
		l.counts[key] = &winEntry{count: 1, reset: now.Add(l.window)}
		return true
	}
	if e.count >= l.max {
		return false
	}
	e.count++
	return true
}

// janitor 周期性清理已过期窗口的 key(避免不同 IP 长期累积占内存)。
func (l *fixedWindowLimiter) janitor() {
	defer func() {
		if r := recover(); r != nil {
			config.LogWarning("limiter janitor panic: %v", r)
		}
	}()
	ticker := time.NewTicker(l.window)
	defer ticker.Stop()
	for range ticker.C {
		now := time.Now()
		l.mu.Lock()
		for k, e := range l.counts {
			if now.After(e.reset) {
				delete(l.counts, k)
			}
		}
		l.mu.Unlock()
	}
}

// LoginRateLimit 登录接口限流:按客户端 IP 固定窗口计数,超限直接返回 429,挡住暴力猜密。
// 阈值走 config.yaml:security.loginMaxAttempts(每窗口最大次数,默认 10)/ security.loginWindowSec(窗口秒数,默认 60)。
func LoginRateLimit() gin.HandlerFunc {
	max := config.Conf.GetInt("security.loginMaxAttempts")
	if max <= 0 {
		max = 10
	}
	sec := config.Conf.GetInt("security.loginWindowSec")
	if sec <= 0 {
		sec = 60
	}
	limiter := newFixedWindowLimiter(max, time.Duration(sec)*time.Second)
	return func(c *gin.Context) {
		if !limiter.allow(c.ClientIP()) {
			c.JSON(http.StatusTooManyRequests, gin.H{
				"code":    429,
				"message": "登录尝试过于频繁，请稍后再试",
			})
			c.Abort()
			return
		}
		c.Next()
	}
}
