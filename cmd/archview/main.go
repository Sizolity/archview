// Command archview serves a read-only, browser-based view of a Go module's
// architecture: packages, types, fields, methods and their relations.
//
// Run it from the module root. It is a development tool: it reads the module
// and never writes to it, and it adds no third-party dependency.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Sizolity/archview/internal/assets"
	"github.com/Sizolity/archview/internal/cache"
	"github.com/Sizolity/archview/internal/config"
	"github.com/Sizolity/archview/internal/crap"
	"github.com/Sizolity/archview/internal/export"
	"github.com/Sizolity/archview/internal/model"
	"github.com/Sizolity/archview/internal/web"
)

// version is injected by goreleaser via -ldflags "-X main.version=...".
var version = "dev"

func main() {
	addr := flag.String("addr", "127.0.0.1:8788", "监听地址(默认只绑回环)")
	editor := flag.String("editor", "source", "源码跳转方式: source|vscode|goland|file|none")
	out := flag.String("out", "", "把自包含页面写到该文件后退出(单文件导出)")
	ednOut := flag.String("edn", "", "把 uml-viewer 拓扑 IR 写到该文件后退出")
	importPath := flag.String("import-edn", "", "读取 uml-viewer IR 并直接渲染(不扫描代码)")
	crapCmd := flag.String("crap-cmd", "go-crap", "CRAP 报告命令(调用 scan -f json)")
	noCrap := flag.Bool("no-crap", false, "跳过 CRAP 指标")
	configPath := flag.String("config", "", "配置文件路径(默认读当前目录的 "+config.DefaultFile+")")
	dir := flag.String("dir", "", "扫描目录(默认当前目录;目录里有 go.work 时按工作区扫描)")
	watch := flag.Bool("watch", false, "轮询源码变化,通过 SSE 通知页面自动重载")
	showVersion := flag.Bool("version", false, "打印版本后退出")
	flag.Parse()

	if *showVersion {
		fmt.Println("archview " + version)
		return
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	set := map[string]bool{}
	flag.Visit(func(f *flag.Flag) { set[f.Name] = true })

	cfg, err := config.Load(*configPath, set["config"])
	if err != nil {
		logger.Error("配置无效", "err", err)
		os.Exit(2)
	}
	if cfg.Path != "" {
		logger.Info("已读取配置", "path", cfg.Path)
	}

	mode := config.Editor(cfg, *editor, set["editor"])
	if set["editor"] && mode != *editor {
		logger.Warn("未知的 -editor,回退到内置源码页", "editor", *editor)
	}

	noMetrics := *noCrap
	if !set["no-crap"] && cfg.Metrics.Enabled != nil {
		noMetrics = !*cfg.Metrics.Enabled
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	metrics := &crap.Bundle{Thresholds: cfg.Metrics.Thresholds}
	if !noMetrics && *importPath == "" {
		started := time.Now()
		metrics = crap.Collect(ctx, *dir, *crapCmd)
		metrics.Thresholds = cfg.Metrics.Thresholds
		if len(metrics.Warnings) > 0 {
			logger.Warn("CRAP 指标不可用,页面不显示分数", "reason", metrics.Warnings[0])
		} else {
			logger.Info("CRAP 指标已采集",
				"functions", len(metrics.ByKey), "took", time.Since(started).Round(time.Millisecond))
		}
	}

	// The loader applies the editor mode and the metrics snapshot, so the cache
	// can hand the same model to every request without anyone mutating it.
	loadFn := func(c context.Context, dir string) (*model.Module, error) {
		m, err := model.LoadWith(c, dir, cfg)
		if err != nil {
			return nil, err
		}
		m.Editor = mode
		metrics.Apply(m)
		return m, nil
	}

	var m *model.Module
	var cached *cache.Cache
	var load func(context.Context) (*model.Module, error)
	if *importPath != "" {
		// An imported IR replaces the scan entirely: there is no module to read.
		data, err := os.ReadFile(*importPath)
		if err != nil {
			logger.Error("读取 EDN 失败", "path", *importPath, "err", err)
			os.Exit(1)
		}
		imported, ignored, err := export.Import(data)
		if err != nil {
			logger.Error("EDN 无法解析", "path", *importPath, "err", err)
			os.Exit(1)
		}
		imported.Editor = mode
		m = imported
		load = func(context.Context) (*model.Module, error) { return imported, nil }
		logger.Info("已导入 EDN", "path", *importPath, "packages", len(m.Packages),
			"edges", len(m.Edges), "ignored", ignored)
	} else {
		cached = cache.New(*dir, loadFn)
		m, err = cached.Load(ctx)
		if err != nil {
			logger.Error("扫描模块失败", "err", err)
			os.Exit(1)
		}
		load = cached.Load
		logger.Info("索引完成", "packages", len(m.Packages), "root", m.Root, "layers", len(cfg.Layers))
	}

	if *out != "" || *ednOut != "" {
		if *out != "" {
			// Metrics and editor mode are already on the model; a static page
			// has no /source endpoint, so "source" degrades to file:// links.
			m.Editor = export.StaticEditor(mode)
			page, err := export.HTML(assets.UI, assets.JS, m)
			if err != nil {
				logger.Error("导出失败", "err", err)
				os.Exit(1)
			}
			if err := os.WriteFile(*out, page, 0o644); err != nil {
				logger.Error("写文件失败", "path", *out, "err", err)
				os.Exit(1)
			}
			logger.Info("已导出单文件页面", "path", *out, "bytes", len(page))
		}
		if *ednOut != "" {
			ir := export.EDN(m)
			if err := os.WriteFile(*ednOut, ir, 0o644); err != nil {
				logger.Error("写文件失败", "path", *ednOut, "err", err)
				os.Exit(1)
			}
			logger.Info("已导出 EDN", "path", *ednOut, "bytes", len(ir))
		}
		return
	}

	srv := &web.Server{Root: m.Root, UI: assets.UI, JS: assets.JS, CSS: assets.CSS, Load: load}
	if *watch && cached != nil {
		srv.Watch = cache.NewWatcher()
		go cache.WatchLoop(ctx, cached, srv.Watch, 500*time.Millisecond)
	}
	if *watch && cached == nil {
		logger.Warn("-watch 对导入的 EDN 无效,已忽略")
	}

	httpSrv := &http.Server{
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}
	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		logger.Error("监听失败", "addr", *addr, "err", err)
		os.Exit(1)
	}
	logger.Info("服务已启动", "url", "http://"+ln.Addr().String())

	errCh := make(chan error, 1)
	go func() { errCh <- httpSrv.Serve(ln) }()

	select {
	case <-ctx.Done():
	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("服务异常退出", "err", err)
			os.Exit(1)
		}
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := httpSrv.Shutdown(shutdownCtx); err != nil {
		logger.Error("优雅关闭失败", "err", err)
	}
	logger.Info("服务已停止")
}
