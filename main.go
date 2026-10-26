// Command archview serves a read-only, browser-based view of a Go module's
// architecture: packages, types, fields, methods and their references.
//
// Run it from the module root. It is a development tool: it reads the module
// and never writes to it, and it adds no third-party dependency.
package main

import (
	"context"
	_ "embed"
	"errors"
	"flag"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

//go:embed ui.html
var uiHTML []byte

func main() {
	addr := flag.String("addr", "127.0.0.1:8788", "监听地址(默认只绑回环)")
	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	m, err := Load(ctx)
	if err != nil {
		logger.Error("扫描模块失败", "err", err)
		os.Exit(1)
	}
	logger.Info("索引完成", "packages", len(m.Packages), "root", m.Root)

	srv := &Server{Root: m.Root, UI: uiHTML, Load: Load}
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
