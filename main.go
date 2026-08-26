package main

import (
	"context"
	"flag"
	"os"

	"github.com/example/toutiaohao-mcp-server/cookies"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	log "github.com/sirupsen/logrus"
)

func startStdioServer(ctx context.Context, run func(context.Context) error) error {
	return run(ctx)
}

func main() {
	port := flag.String("port", "8080", "HTTP server port")
	loginMode := flag.Bool("login", false, "Start interactive QR code login to save cookies and then exit")
	stdioMode := flag.Bool("stdio", false, "Start in MCP Stdio mode (for Cursor, Claude Desktop etc.)")
	flag.Parse()

	log.SetFormatter(&log.TextFormatter{FullTimestamp: true})

	cookieStore := cookies.NewFileCookieStore(cookies.GetDefaultCookiePath())
	service := NewToutiaoService(cookieStore)

	if *loginMode {
		log.Info("Starting Toutiao MCP Server in Login Mode...")
		ctx := context.Background()
		if err := service.QrCodeLogin(ctx); err != nil {
			log.Fatalf("QR code login failed: %v", err)
		}
		log.Info("Login complete, cookies saved to cookies.json. Exiting...")
		return
	}

	if *stdioMode {
		// Stdio 模式通过 stdin/stdout 交互。必须在启动时通过 log.SetOutput(os.Stderr) 隔离日志，防止日志内容污染 stdout 导致 AI 客户端 JSON-RPC 解析崩溃。
		log.SetOutput(os.Stderr)
		log.Info("Starting Toutiao MCP Server in Stdio Mode...")

		ctx := context.Background()
		appServer := NewAppServer(service)
		server := InitMCPServer(appServer)

		if err := startStdioServer(ctx, func(runCtx context.Context) error {
			return server.Run(runCtx, &mcp.StdioTransport{})
		}); err != nil {
			log.Fatalf("Stdio server error: %v", err)
		}
		return
	}

	log.Info("Starting Toutiao MCP Server...")
	server := NewAppServer(service)

	if err := server.Start(*port); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}
