package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"time"
)

func main() {
	// 模式选择
	mode := flag.String("mode", "bench", "运行模式: bench(压测) / server(模拟服务器)")
	// 压测参数
	host := flag.String("host", "127.0.0.1", "IM 服务器地址")
	port := flag.Int("port", 9527, "IM 服务器端口")
	concurrency := flag.Int("c", 100, "并发连接数/goroutine 数")
	duration := flag.Int("d", 10, "压测持续时间（秒）")
	scenario := flag.String("s", "all", "压测场景: connect/login/heartbeat/chat/mixed/all")
	outputMD := flag.Bool("md", false, "以 Markdown 表格格式输出结果")
	flag.Parse()

	if *mode == "server" {
		log.Fatal("server(mock) 模式已移除：本工具现按真 IMServer 的 protobuf+IM01 协议压测，请用 bench 模式直接压真服务器")
	}

	cfg := BenchConfig{
		ServerAddr:  fmt.Sprintf("%s:%d", *host, *port),
		Concurrency: *concurrency,
		Duration:    time.Duration(*duration) * time.Second,
	}

	log.SetFlags(log.Ltime)
	log.Printf("IM 服务器压测工具 v1.0")
	log.Printf("目标: %s, 并发: %d, 时长: %v", cfg.ServerAddr, cfg.Concurrency, cfg.Duration)

	scenarios := parseScenarios(*scenario)
	reports := make([]*BenchReport, 0, len(scenarios))

	for _, s := range scenarios {
		var r *BenchReport
		switch s {
		case "connect":
			r = benchConnect(cfg)
		case "login":
			r = benchLogin(cfg)
		case "heartbeat":
			r = benchHeartbeat(cfg)
		case "chat":
			r = benchChat(cfg)
		case "mixed":
			r = benchMixed(cfg)
		}
		if r != nil {
			reports = append(reports, r)
		}
	}

	// 输出 Markdown 汇总表
	if *outputMD {
		fmt.Println()
		fmt.Println("| 场景 | 持续时间 | 吞吐量(req/s) | 平均延迟(ms) | P50(ms) | P95(ms) | P99(ms) | 成功率 |")
		fmt.Println("|------|----------|---------------|-------------|---------|---------|---------|--------|")
		for _, r := range reports {
			fmt.Println(r.Markdown())
		}
	}

	os.Exit(0)
}

func parseScenarios(s string) []string {
	if s == "all" {
		return []string{"connect", "login", "heartbeat", "chat", "mixed"}
	}
	parts := splitAndTrim(s, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		switch p {
		case "connect", "login", "heartbeat", "chat", "mixed":
			result = append(result, p)
		default:
			log.Printf("未知场景: %s，已跳过", p)
		}
	}
	return result
}

func splitAndTrim(s, sep string) []string {
	result := make([]string, 0)
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == sep[0] {
			if trimmed := trim(s[start:i]); trimmed != "" {
				result = append(result, trimmed)
			}
			start = i + 1
		}
	}
	if trimmed := trim(s[start:]); trimmed != "" {
		result = append(result, trimmed)
	}
	return result
}

func trim(s string) string {
	start, end := 0, len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t') {
		end--
	}
	return s[start:end]
}
