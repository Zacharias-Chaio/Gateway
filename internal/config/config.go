// Package config 负责加载应用配置文件（configs/app.yaml）。
package config

import (
	"os"

	"gopkg.in/yaml.v3"
)

// App 应用配置根节点。目前仅包含日志配置，后续可平级追加其他模块。
type App struct {
	Log Log `yaml:"log"`
}

// Log 日志相关配置参数。
type Log struct {
	Level       string `yaml:"level"`       // debug | info | warn | error
	Console     bool   `yaml:"console"`     // 终端窗口输出
	File        string `yaml:"file"`        // 文件输出路径（空则不落文件）
	MaxSizeMB   int    `yaml:"maxSizeMB"`   // 单文件大小上限（MB），大小轮转阈值
	MaxBackups  int    `yaml:"maxBackups"`  // 保留历史文件份数
	MaxAgeDays  int    `yaml:"maxAgeDays"`  // 历史文件保留天数
	Compress    bool   `yaml:"compress"`    // 历史文件 gzip 压缩
	DailyRotate bool   `yaml:"dailyRotate"` // 每日 00:00 轮转
	BufferSize  int    `yaml:"bufferSize"`  // 前端出口环形缓冲条数
}

// Default 返回内置默认配置，作为配置文件缺失或解析失败时的回退。
func Default() App {
	return App{Log: Log{
		Level:       "info",
		Console:     true,
		File:        "logs/gateway.log",
		MaxSizeMB:   20,
		MaxBackups:  30,
		MaxAgeDays:  30,
		Compress:    true,
		DailyRotate: true,
		BufferSize:  500,
	}}
}

// Load 读取并解析 app.yaml；文件缺失或解析失败时回退到 Default。
// 第二个返回值为读取/解析过程中遇到的错误（供调用方决定是否记录），
// 即便出错也总会返回一份可用的配置。
func Load(path string) (App, error) {
	cfg := Default()
	if path == "" {
		return cfg, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, err
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Default(), err
	}
	return cfg, nil
}
