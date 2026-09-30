package config

import (
	"fmt"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"log"
	"sync"
	"time"
)

const (
	maxOpenConns = 100 // 最大连接数
	maxIdleConns = 10  // 空闲连接数（预热数量对齐此值）
)

var Mysql *gorm.DB

func init() {
	InitMysql()
}

// InitMysql 初始化Mysql连接
func InitMysql() {
	dbAddress := Conf.GetString("mysql.address")
	dbName := Conf.GetString("mysql.database")
	dbUser := Conf.GetString("mysql.user")
	dbPasswd := Conf.GetString("mysql.password")
	dsn := fmt.Sprintf("%v:%v@tcp(%v)/%v?charset=utf8mb4&parseTime=true&loc=Asia%%2FShanghai&timeout=5s&readTimeout=30s&writeTimeout=30s&interpolateParams=true", dbUser, dbPasswd, dbAddress, dbName)
	var err error
	Mysql, err = gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatal("初始化Mysql时出错：", err)
	}
	// 设置连接池参数
	sqlDB, err := Mysql.DB()
	if err != nil {
		log.Fatal("获取Mysql连接池时出错：", err)
	}
	sqlDB.SetMaxOpenConns(maxOpenConns)        // 最大连接数
	sqlDB.SetMaxIdleConns(maxIdleConns)        // 空闲连接数
	sqlDB.SetConnMaxLifetime(30 * time.Minute) // 连接最大存活时间
	sqlDB.SetConnMaxIdleTime(10 * time.Minute) // 空闲连接最大存活时间
	if err = sqlDB.Ping(); err != nil {
		log.Fatal("连接到Mysql时出错：", err)
	}
	fmt.Println("Mysql连接成功！")
	// 预热连接池：启动时预建 maxIdleConns 条连接，避免首批请求承担建连开销
	warmupConnections(Mysql, maxIdleConns)
}

// warmupConnections 预热指定数量的连接：并发执行 SELECT 1 填满空闲池，
// 使首批业务请求无需等待 TCP 建连 + 握手，降低冷启动首请求延迟。
func warmupConnections(db *gorm.DB, count int) {
	start := time.Now()
	var wg sync.WaitGroup
	errChan := make(chan error, count)
	for i := 0; i < count; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := db.Exec("SELECT 1").Error; err != nil {
				errChan <- err
			}
		}()
	}
	wg.Wait()
	close(errChan)
	for err := range errChan {
		if err != nil {
			log.Fatalf("连接池预热失败：%v", err)
		}
	}
	fmt.Printf("Mysql已预热%v个连接，耗时：%v\n", count, time.Since(start))
}
