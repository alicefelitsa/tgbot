package config

import (
	"fmt"
	"io"
	"log"
	"os"
	"path"
	"runtime"
	"strings"
	"sync"
	"time"
)

// LogLevel 日志级别类型
type LogLevel int

const (
	DEBUG LogLevel = iota
	INFO
	WARNING
	ERROR
	FATAL
)

var (
	levelFlags = []string{"DEBUG", "INFO", "WARNING", "ERROR", "FATAL"}
	logger     *log.Logger
	logFile    *os.File
	mu         sync.Mutex
	config     Settings
)

// Settings 日志配置结构
type Settings struct {
	Path       string   // 日志文件路径
	Name       string   // 日志文件名
	Ext        string   // 文件扩展名
	TimeFormat string   // 时间格式
	MaxSize    int      // 文件最大大小(MB)
	MaxBackups int      // 最大备份文件数
	MaxAge     int      // 最大保存天数
	Compress   bool     // 是否压缩
	LogLevel   LogLevel // 日志级别
}

// 初始化默认配置
var defaultSettings = Settings{
	Path:       "./logs",
	Name:       "app",
	Ext:        "log",
	TimeFormat: "2006-01-02",
	MaxSize:    1,  // 1MB
	MaxBackups: 5,  // 保留5个备份
	MaxAge:     30, // 保存30天
	Compress:   true,
	LogLevel:   INFO,
}

func init() {
	InitLoggerSetup()
}

// InitLoggerSetup 初始化日志配置
func InitLoggerSetup(settings ...Settings) {
	mu.Lock()
	defer mu.Unlock()

	if len(settings) > 0 {
		config = settings[0]
	} else {
		config = defaultSettings
	}

	var err error

	// 创建日志目录
	err = createLogDir()

	// 打开日志文件
	err = openLogFile()

	if err != nil {
		log.Fatal("初始化日志失败：", err)
	}

	// 设置日志输出
	writers := []io.Writer{logFile}
	if config.LogLevel == DEBUG {
		writers = append(writers, os.Stdout) // 调试级别时同时输出到控制台
	}

	multiWriter := io.MultiWriter(writers...)
	logger = log.New(multiWriter, "", 0)
}

// createLogDir 创建日志目录
func createLogDir() error {
	if _, err := os.Stat(config.Path); os.IsNotExist(err) {
		if err := os.MkdirAll(config.Path, 0755); err != nil {
			return fmt.Errorf("创建日志目录失败: %v", err)
		}
	}
	return nil
}

// openLogFile 打开日志文件
func openLogFile() error {
	filename := fmt.Sprintf("%s.%s", config.Name, config.Ext)
	fullPath := path.Join(config.Path, filename)

	file, err := os.OpenFile(fullPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("打开日志文件失败: %v", err)
	}
	logFile = file
	return nil
}

// checkLogFile 检查并处理日志文件大小
func checkLogFile() {
	if logFile == nil {
		return
	}

	info, err := logFile.Stat()
	if err != nil {
		return
	}

	// 检查文件大小是否超过限制
	if info.Size() > int64(config.MaxSize)*1024*1024 {
		rotateLogFile()
	}
}

// rotateLogFile 日志文件轮转
func rotateLogFile() {
	if logFile == nil {
		return
	}

	// 关闭当前文件
	_ = logFile.Close()

	// 重命名当前日志文件（添加时间戳）
	timestamp := time.Now().Format("20060102150405")
	oldFilename := fmt.Sprintf("%s.%s", config.Name, config.Ext)
	newFilename := fmt.Sprintf("%s.%s.%s", config.Name, timestamp, config.Ext)

	oldPath := path.Join(config.Path, oldFilename)
	newPath := path.Join(config.Path, newFilename)

	_ = os.Rename(oldPath, newPath)

	// 重新打开日志文件
	_ = openLogFile()

	// 清理过期日志文件
	cleanupOldLogs()
}

// cleanupOldLogs 清理过期日志
func cleanupOldLogs() {
	files, err := os.ReadDir(config.Path)
	if err != nil {
		return
	}
	for _, file := range files {
		if strings.HasPrefix(file.Name(), config.Name) && strings.HasSuffix(file.Name(), config.Ext) {
			if info, err := file.Info(); err == nil {
				if time.Since(info.ModTime()) > time.Duration(config.MaxAge)*24*time.Hour {
					_ = os.Remove(path.Join(config.Path, file.Name()))
				}
			}
		}
	}
}

// logMessage 记录日志的通用函数
func logMessage(level LogLevel, format string, args ...interface{}) {
	if level < config.LogLevel {
		return
	}
	checkLogFile()
	if logger == nil {
		InitLoggerSetup()
	}
	// 获取调用者信息
	_, file, line, ok := runtime.Caller(2)
	if !ok {
		file = "???"
		line = 0
	}
	shortFile := path.Base(file)
	// 构建完整的日志消息
	timestamp := time.Now().Format("2006-01-02 15:04:05.000")
	levelFlag := levelFlags[level]
	message := fmt.Sprintf(format, args...)
	fullMessage := fmt.Sprintf("[%s] [%s] %s:%d: %s", timestamp, levelFlag, shortFile, line, message)
	_ = logger.Output(3, fullMessage)
	if level == FATAL {
		os.Exit(1)
	}
}

// LogDebug 调试日志
func LogDebug(format string, args ...interface{}) {
	logMessage(DEBUG, format, args...)
}

// LogInfo 信息日志
func LogInfo(format string, args ...interface{}) {
	logMessage(INFO, format, args...)
}

// LogWarning 警告日志
func LogWarning(format string, args ...interface{}) {
	logMessage(WARNING, format, args...)
}

// LogError 错误日志
func LogError(format string, args ...interface{}) {
	logMessage(ERROR, format, args...)
}

// LogFatal 致命日志
func LogFatal(format string, args ...interface{}) {
	logMessage(FATAL, format, args...)
}

// LogWithFields 带字段的日志记录（简单结构化）
func LogWithFields(level LogLevel, fields map[string]interface{}, message string) {
	if level < config.LogLevel {
		return
	}
	var fieldStrs []string
	for key, value := range fields {
		fieldStrs = append(fieldStrs, fmt.Sprintf("%s=%v", key, value))
	}
	fieldsStr := strings.Join(fieldStrs, " ")
	logMessage(level, "%s %s", fieldsStr, message)
}

// LoggerClose 关闭日志文件
func LoggerClose() {
	if logFile != nil {
		_ = logFile.Close()
		logFile = nil
	}
}
