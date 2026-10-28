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
	"path/filepath"
	"syscall"
	"time"

	"github.com/Sizolity/archview/internal/assets"
	"github.com/Sizolity/archview/internal/cache"
	"github.com/Sizolity/archview/internal/config"
	"github.com/Sizolity/archview/internal/crap"
	"github.com/Sizolity/archview/internal/diff"
	"github.com/Sizolity/archview/internal/export"
	"github.com/Sizolity/archview/internal/model"
	"github.com/Sizolity/archview/internal/web"
)

// version is injected by goreleaser via -ldflags "-X main.version=...".
var version = "dev"

// loadModel scans dir and attaches the editor mode plus a fresh CRAP snapshot.
//
// The snapshot is taken here, not once at startup: go-crap keys its report by
// file:line, so a reload that shifts a single line would leave most scores
// unmatched if the snapshot were older than the model. collect is nil when the
// scores are switched off.
func loadModel(ctx context.Context, dir string, cfg *config.Config, mode string,
	collect func(context.Context, string) *crap.Bundle, diffRef string) (*model.Module, error) {
	m, err := model.LoadWith(ctx, dir, cfg)
	if err != nil {
		return nil, err
	}
	m.Editor = mode

	// The diff goes on before the scores: a failure to read git must not cost
	// the metrics, and it only costs the change markers when it fails.
	markDiff(ctx, m, diffRef)

	if collect == nil {
		return m, nil
	}

	started := time.Now()
	b := collect(ctx, dir)
	b.Thresholds = cfg.Metrics.Thresholds
	matched := b.Apply(m)
	switch {
	case len(b.Warnings) > 0:
		slog.Warn("CRAP 指标不可用,页面不显示分数", "reason", b.Warnings[0])
	case matched == 0:
		slog.Warn("CRAP 报告没有对上任何函数,页面不显示分数", "functions", len(b.ByKey))
	default:
		slog.Info("CRAP 指标已采集", "matched", matched, "functions", len(b.ByKey),
			"cached", b.Cached, "took", time.Since(started).Round(time.Millisecond))
	}
	return m, nil
}

// markDiff tags the declarations the working tree changed since ref.
func markDiff(ctx context.Context, m *model.Module, ref string) {
	if ref == "" || m.Root == "" {
		return
	}
	changes, err := diff.Run(ctx, m.Root, ref)
	if err != nil {
		slog.Warn("改动范围不可用", "ref", ref, "err", model.OneLine(err.Error()))
		m.Warnings = append(m.Warnings, "改动范围不可用: "+model.OneLine(err.Error()))
		return
	}
	if !diff.Apply(m, changes) {
		slog.Info("改动范围为空", "ref", ref)
		return
	}
	slog.Info("改动范围已标记", "ref", ref, "files", m.Diff.Files,
		"functions", m.Diff.Functions, "types", m.Diff.Types)
}

// defaultCacheDir is where the CRAP report is cached: the user's cache
// directory, never the inspected module.
func defaultCacheDir() string {
	if d := os.Getenv("ARCHVIEW_CACHE_DIR"); d != "" {
		return d
	}
	d, err := os.UserCacheDir()
	if err != nil {
		return ""
	}
	return filepath.Join(d, "archview")
}

// cliFlags is the parsed command line. Grouping it here keeps run readable and
// lets parseFlags be tested on its own.
type cliFlags struct {
	addr, editor, out, ednOut, importPath, crapCmd, crapProfile string
	cacheDir, configPath, dir, diffRef                          string
	noCrapCache, noCrap, watch, showVersion                     bool
}

func parseFlags(args []string) (*cliFlags, map[string]bool, error) {
	fs := flag.NewFlagSet("archview", flag.ContinueOnError)
	f := &cliFlags{}
	fs.StringVar(&f.addr, "addr", "127.0.0.1:8788", "监听地址(默认只绑回环)")
	fs.StringVar(&f.editor, "editor", "source", "源码跳转方式: source|vscode|goland|file|none")
	fs.StringVar(&f.out, "out", "", "把自包含页面写到该文件后退出(单文件导出)")
	fs.StringVar(&f.ednOut, "edn", "", "把 uml-viewer 拓扑 IR 写到该文件后退出")
	fs.StringVar(&f.importPath, "import-edn", "", "读取 uml-viewer IR 并直接渲染(不扫描代码)")
	fs.StringVar(&f.crapCmd, "crap-cmd", "go-crap", "CRAP 报告命令(调用 scan -f json)")
	fs.StringVar(&f.crapProfile, "crap-profile", "", "已有覆盖率文件(go test -coverprofile 产出),给出则不跑测试")
	fs.BoolVar(&f.noCrapCache, "no-crap-cache", false, "不缓存 CRAP 报告(默认缓存在用户缓存目录)")
	fs.StringVar(&f.cacheDir, "cache-dir", "", "缓存目录(默认用户缓存目录;空表示默认)")
	fs.BoolVar(&f.noCrap, "no-crap", false, "跳过 CRAP 指标")
	fs.StringVar(&f.configPath, "config", "", "配置文件路径(默认读当前目录的 "+config.DefaultFile+")")
	fs.StringVar(&f.dir, "dir", "", "扫描目录(默认当前目录;目录里有 go.work 时按工作区扫描)")
	fs.StringVar(&f.diffRef, "diff", "", "把工作区相对该 git 引用的改动标在函数/类型上(如 HEAD、HEAD~1)")
	fs.BoolVar(&f.watch, "watch", false, "轮询源码变化,通过 SSE 通知页面自动重载")
	fs.BoolVar(&f.showVersion, "version", false, "打印版本后退出")
	if err := fs.Parse(args); err != nil {
		return nil, nil, err
	}
	set := map[string]bool{}
	fs.Visit(func(x *flag.Flag) { set[x.Name] = true })
	return f, set, nil
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := run(ctx, os.Args[1:]); err != nil {
		var exit *exitError
		if errors.As(err, &exit) {
			slog.Error("archview 退出", "err", exit.err)
			os.Exit(exit.code)
		}
		slog.Error("archview 退出", "err", err)
		os.Exit(1)
	}
}

// exitError carries the exit status the CLI documents (2 for a bad config).
type exitError struct {
	code int
	err  error
}

func (e *exitError) Error() string { return e.err.Error() }

// run is the whole command. It returns instead of exiting so the paths that
// used to be untestable -- a bad config, a failed import, a busy port -- can be
// asserted in a test.
func run(ctx context.Context, args []string) error {
	f, set, err := parseFlags(args)
	if err != nil {
		return err
	}
	if f.showVersion {
		fmt.Println("archview " + version)
		return nil
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	cfg, err := config.Load(f.configPath, set["config"])
	if err != nil {
		return &exitError{code: 2, err: fmt.Errorf("配置无效: %w", err)}
	}
	if cfg.Path != "" {
		logger.Info("已读取配置", "path", cfg.Path)
	}

	mode := config.Editor(cfg, f.editor, set["editor"])
	if set["editor"] && mode != f.editor {
		logger.Warn("未知的 -editor,回退到内置源码页", "editor", f.editor)
	}

	noMetrics := f.noCrap
	if !set["no-crap"] && cfg.Metrics.Enabled != nil {
		noMetrics = !*cfg.Metrics.Enabled
	}

	// collect runs go-crap once per model build, or not at all.
	var collect func(context.Context, string) *crap.Bundle
	if !noMetrics && f.importPath == "" {
		var crapArgs []string
		if f.crapProfile != "" {
			crapArgs = append(crapArgs, "--coverage-profile", f.crapProfile)
		}
		cacheDir := f.cacheDir
		if cacheDir == "" {
			cacheDir = defaultCacheDir()
		}
		if f.noCrapCache {
			cacheDir = ""
		}
		collect = func(c context.Context, dir string) *crap.Bundle {
			return crap.CollectCached(c, dir, f.crapCmd, crapArgs, cacheDir)
		}
	}

	// The loader applies the editor mode and a CRAP snapshot taken from the same
	// tree as the model, so the cache can hand the result to every request
	// without anyone mutating it.
	loadFn := func(c context.Context, dir string) (*model.Module, error) {
		return loadModel(c, dir, cfg, mode, collect, f.diffRef)
	}

	var m *model.Module
	var cached *cache.Cache
	var load func(context.Context) (*model.Module, error)
	if f.importPath != "" {
		// An imported IR replaces the scan entirely: there is no module to read.
		data, err := os.ReadFile(f.importPath)
		if err != nil {
			return fmt.Errorf("读取 EDN 失败: %w", err)
		}
		imported, ignored, err := export.Import(data)
		if err != nil {
			return fmt.Errorf("EDN 无法解析: %w", err)
		}
		imported.Editor = mode
		m = imported
		load = func(context.Context) (*model.Module, error) { return imported, nil }
		logger.Info("已导入 EDN", "path", f.importPath, "packages", len(m.Packages),
			"edges", len(m.Edges), "ignored", ignored)
	} else {
		cached = cache.New(f.dir, loadFn)
		m, err = cached.Load(ctx)
		if err != nil {
			return fmt.Errorf("扫描模块失败: %w", err)
		}
		load = cached.Load
		logger.Info("索引完成", "packages", len(m.Packages), "root", m.Root, "layers", len(cfg.Layers))
	}

	if f.out != "" || f.ednOut != "" {
		return writeOutputs(m, mode, f.out, f.ednOut, logger)
	}

	srv := &web.Server{Root: m.Root, UI: assets.UI, JS: assets.JS, CSS: assets.CSS, Ctx: ctx, Load: load}
	switch {
	case f.watch && cached != nil:
		srv.Watch = cache.NewWatcher()
		go cache.WatchLoop(ctx, cached, srv.Watch, 500*time.Millisecond)
	case f.watch:
		logger.Warn("-watch 对导入的 EDN 无效,已忽略")
	}
	return serve(ctx, srv, f.addr, logger)
}

// writeOutputs renders the model to the files -out and -edn asked for.
func writeOutputs(m *model.Module, mode, out, ednOut string, logger *slog.Logger) error {
	if out != "" {
		// Metrics and editor mode are already on the model; a static page has no
		// /source endpoint, so "source" degrades to file:// links.
		m.Editor = export.StaticEditor(mode)
		page, err := export.HTML(assets.UI, assets.JS, m)
		if err != nil {
			return fmt.Errorf("导出失败: %w", err)
		}
		if err := os.WriteFile(out, page, 0o644); err != nil {
			return fmt.Errorf("写文件失败: %w", err)
		}
		logger.Info("已导出单文件页面", "path", out, "bytes", len(page))
	}
	if ednOut != "" {
		ir := export.EDN(m)
		if err := os.WriteFile(ednOut, ir, 0o644); err != nil {
			return fmt.Errorf("写文件失败: %w", err)
		}
		logger.Info("已导出 EDN", "path", ednOut, "bytes", len(ir))
	}
	return nil
}

// serve runs the HTTP server until the context is cancelled.
func serve(ctx context.Context, srv *web.Server, addr string, logger *slog.Logger) error {
	httpSrv := &http.Server{Handler: srv.Handler(), ReadHeaderTimeout: 5 * time.Second}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("监听失败: %w", err)
	}
	logger.Info("服务已启动", "url", "http://"+ln.Addr().String())

	errCh := make(chan error, 1)
	go func() { errCh <- httpSrv.Serve(ln) }()

	select {
	case <-ctx.Done():
	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("服务异常退出: %w", err)
		}
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := httpSrv.Shutdown(shutdownCtx); err != nil {
		logger.Error("优雅关闭失败", "err", err)
	}
	logger.Info("服务已停止")
	return nil
}
