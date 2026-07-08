package main

import (
	"context"
	"errors"
	"flag"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"gateway/internal/config"
	"gateway/internal/engine"
	"gateway/internal/logx"
	"gateway/internal/store"
	"gateway/internal/web"
)

func main() {
	addr := flag.String("addr", ":8080", "HTTP 监听地址")
	dbPath := flag.String("db", "data/config.db", "SQLite 配置数据库路径")
	hwPath := flag.String("hardware", "configs/hardware.yaml", "硬件接口配置文件路径")
	cfgPath := flag.String("config", "configs/app.yaml", "应用配置文件路径")
	flag.Parse()

	// 加载应用配置并初始化统一日志管理器（终端 + 文件 + 前端出口）。
	cfg, cfgErr := config.Load(*cfgPath)
	logx.Init(logx.Options{
		Level:       cfg.Log.Level,
		Console:     cfg.Log.Console,
		File:        cfg.Log.File,
		MaxSizeMB:   cfg.Log.MaxSizeMB,
		MaxBackups:  cfg.Log.MaxBackups,
		MaxAgeDays:  cfg.Log.MaxAgeDays,
		Compress:    cfg.Log.Compress,
		DailyRotate: cfg.Log.DailyRotate,
		BufferSize:  cfg.Log.BufferSize,
	})
	logger := logx.Module("main")
	if cfgErr != nil {
		logger.Warn("加载应用配置失败，已回退默认日志配置", "path", *cfgPath, "err", cfgErr)
	}

	db, err := store.Open(*dbPath)
	if err != nil {
		logger.Error("打开数据库失败", "err", err)
		os.Exit(1)
	}

	// 监听中断/终止信号，实现优雅退出；该 ctx 同时作为链路引擎的父上下文。
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// 创建链路引擎，加载已保存的链路并启动（每条链路一个 goroutine）。
	eng := engine.New(ctx)
	var channels []store.Channel
	if err := db.Order("id asc").Find(&channels).Error; err != nil {
		logger.Warn("加载链路配置失败，引擎以空配置启动", "err", err)
	}
	eng.Apply(channels)

	srv := &http.Server{Addr: *addr, Handler: web.Router(db, *hwPath, eng)}

	go func() {
		logger.Info("网关微服务启动", "addr", *addr, "url", "http://localhost"+*addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("服务异常退出", "err", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	stop() // 恢复默认信号处理：再次 Ctrl+C 可强制退出
	logger.Info("正在关闭服务…")

	eng.Stop() // 关闭所有链路 goroutine

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("优雅关闭失败", "err", err)
		os.Exit(1)
	}
	logger.Info("服务已关闭")
}
