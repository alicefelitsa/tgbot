package config

import (
	"github.com/spf13/viper"
	"log"
)

var Conf *viper.Viper

func init() {
	InitConfigYaml("config")
}

// InitConfigYaml 初始化项目根目录config.yaml配置文件
func InitConfigYaml(fileName string) {
	Conf = viper.New()
	Conf.AddConfigPath("./")     // 项目根目录
	Conf.SetConfigName(fileName) // 文件名
	Conf.SetConfigType("yaml")   // 文件类型
	if err := Conf.ReadInConfig(); err != nil {
		log.Fatal("找不到config.yaml文件：", err)
	}
}
