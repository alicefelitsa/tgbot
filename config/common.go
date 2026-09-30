package config

import (
	"time"
	// 嵌入 IANA 时区数据库，保证交叉编译出的静态二进制在没有 tzdata 的 Linux 服务器上，
	// 也能通过 time.LoadLocation / DSN 的 loc=Asia/Shanghai 正确加载东八区
	_ "time/tzdata"
)

func init() {
	// 将进程默认时区统一为东八区（北京时间）。
	// 服务器系统时区可能为 UTC，若不设置，time.Now() 及写入 MySQL 的时间会偏差 8 小时。
	if loc, err := time.LoadLocation("Asia/Shanghai"); err == nil {
		time.Local = loc
	}
}
