package main

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

// BenchConfig 压测配置
type BenchConfig struct {
	ServerAddr  string
	Concurrency int
	Duration    time.Duration
}

// ─── 场景 A：连接压测 ───
// 大量短连接快速建立→关闭，测 IMServer 的 TCP accept 能力（协议无关，纯 TCP）
func benchConnect(cfg BenchConfig) *BenchReport {
	log.Printf("[连接压测] 启动: 并发=%d, 时长=%v", cfg.Concurrency, cfg.Duration)
	report := &BenchReport{Scenario: "连接压测", Latency: &LatencyStats{}}

	var wg sync.WaitGroup
	var connOK, connFail atomic.Int64
	stopCh := make(chan struct{})
	startTime := time.Now()

	for i := 0; i < cfg.Concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stopCh:
					return
				default:
				}
				start := time.Now()
				client := &IMClient{}
				if err := client.Connect(cfg.ServerAddr, 3*time.Second); err != nil {
					connFail.Add(1)
					report.Latency.Record(time.Since(start))
					continue
				}
				connOK.Add(1)
				report.Latency.Record(time.Since(start))
				client.Close()
			}
		}()
	}

	time.Sleep(cfg.Duration)
	close(stopCh)
	wg.Wait()

	elapsed := time.Since(startTime)
	report.Duration = elapsed
	report.TotalReqs = connOK.Load() + connFail.Load()
	report.SuccessReqs = connOK.Load()
	report.FailedReqs = connFail.Load()
	report.ReqsPerSec = float64(report.TotalReqs) / elapsed.Seconds()
	report.Connections = connOK.Load()
	report.ConnFailures = connFail.Load()
	return report
}

// ─── 场景 B：登录压测 ───
// 需预置账号 bench_user_0..N（明文密码 123456）；在已登录连接上反复登录测登录处理吞吐
func benchLogin(cfg BenchConfig) *BenchReport {
	log.Printf("[登录压测] 启动: 并发=%d, 时长=%v", cfg.Concurrency, cfg.Duration)
	log.Println("[登录压测] 需预置账号 bench_user_0.. （明文密码 123456）；未预置会全部失败")
	report := &BenchReport{Scenario: "登录压测", Latency: &LatencyStats{}}

	var success, failed atomic.Int64
	stopCh := make(chan struct{})
	startTime := time.Now()

	var wg sync.WaitGroup
	for i := 0; i < cfg.Concurrency; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			client := &IMClient{}
			if err := client.Connect(cfg.ServerAddr, 3*time.Second); err != nil {
				failed.Add(1)
				return
			}
			defer client.Close()
			payload := encodeLoginReq(fmt.Sprintf("bench_user_%d", idx), "123456")
			for {
				select {
				case <-stopCh:
					return
				default:
				}
				rsp, elapsed, err := client.SendRecv(MsgLogin, 0, payload)
				report.Latency.Record(elapsed)
				if err != nil || decodeCommonRspCode(rsp) != 0 {
					failed.Add(1)
				} else {
					success.Add(1)
				}
			}
		}(i)
	}

	time.Sleep(cfg.Duration)
	close(stopCh)
	wg.Wait()

	elapsed := time.Since(startTime)
	report.Duration = elapsed
	report.SuccessReqs = success.Load()
	report.FailedReqs = failed.Load()
	report.TotalReqs = report.SuccessReqs + report.FailedReqs
	report.ReqsPerSec = float64(report.TotalReqs) / elapsed.Seconds()
	return report
}

// ─── 场景 C：心跳压测 ───
// 心跳无需登录（OnHeartbeatResponse 不校验会话），连上即全速 send-recv，
// 测 IMServer 单条消息的完整往返处理吞吐与延迟。
func benchHeartbeat(cfg BenchConfig) *BenchReport {
	log.Printf("[心跳压测] 启动: 并发=%d, 时长=%v", cfg.Concurrency, cfg.Duration)
	report := &BenchReport{Scenario: "心跳往返", Latency: &LatencyStats{}}

	log.Println("[心跳压测] 建立连接...")
	clients := make([]*IMClient, 0, cfg.Concurrency)
	var mu sync.Mutex
	var wg sync.WaitGroup
	for i := 0; i < cfg.Concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c := &IMClient{}
			if err := c.Connect(cfg.ServerAddr, 3*time.Second); err != nil {
				return
			}
			mu.Lock()
			clients = append(clients, c)
			mu.Unlock()
		}()
	}
	wg.Wait()
	log.Printf("[心跳压测] 建立 %d 个连接", len(clients))

	var success, failed atomic.Int64
	stopCh := make(chan struct{})
	startTime := time.Now()
	for _, c := range clients {
		wg.Add(1)
		go func(cl *IMClient) {
			defer wg.Done()
			for {
				select {
				case <-stopCh:
					cl.Close()
					return
				default:
				}
				_, elapsed, err := cl.SendRecv(MsgHeartbeat, 0, nil)
				report.Latency.Record(elapsed)
				if err != nil {
					failed.Add(1)
					return // 连接出错，退出该 goroutine
				}
				success.Add(1)
			}
		}(c)
	}

	time.Sleep(cfg.Duration)
	close(stopCh)
	wg.Wait()

	elapsed := time.Since(startTime)
	report.Duration = elapsed
	report.SuccessReqs = success.Load()
	report.FailedReqs = failed.Load()
	report.TotalReqs = report.SuccessReqs + report.FailedReqs
	report.ReqsPerSec = float64(report.TotalReqs) / elapsed.Seconds()
	report.Connections = int64(len(clients))
	return report
}

// ─── 场景 D：单聊转发压测 ───
// 需预置账号 bench_user_0..(2*c-1)。每对 A→B：A 持续 send-only 发消息，
// B 端接收服务端转发的消息计数 = 端到端转发吞吐；content 内嵌纳秒时间戳测端到端延迟。
func benchChat(cfg BenchConfig) *BenchReport {
	log.Printf("[单聊压测] 启动: 并发=%d 对, 时长=%v", cfg.Concurrency, cfg.Duration)
	log.Println("[单聊压测] 需预置账号 bench_user_0..(2*c-1)（明文密码 123456）")
	report := &BenchReport{Scenario: "单聊转发", Latency: &LatencyStats{}}

	type pair struct {
		a, b     *IMClient
		aID, bID int32
	}
	pairs := make([]*pair, 0, cfg.Concurrency)
	var mu sync.Mutex
	var wg sync.WaitGroup

	log.Println("[单聊压测] 建立连接对并登录...")
	for i := 0; i < cfg.Concurrency; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			a, b := &IMClient{}, &IMClient{}
			if a.Connect(cfg.ServerAddr, 3*time.Second) != nil {
				return
			}
			if b.Connect(cfg.ServerAddr, 3*time.Second) != nil {
				a.Close()
				return
			}
			ra, _, ea := a.SendRecv(MsgLogin, 0, encodeLoginReq(fmt.Sprintf("bench_user_%d", idx*2), "123456"))
			rb, _, eb := b.SendRecv(MsgLogin, 0, encodeLoginReq(fmt.Sprintf("bench_user_%d", idx*2+1), "123456"))
			if ea != nil || eb != nil {
				a.Close()
				b.Close()
				return
			}
			aID, bID := decodeLoginUserID(ra), decodeLoginUserID(rb)
			if aID == 0 || bID == 0 {
				a.Close()
				b.Close()
				return
			}
			mu.Lock()
			pairs = append(pairs, &pair{a, b, aID, bID})
			mu.Unlock()
		}(i)
	}
	wg.Wait()
	log.Printf("[单聊压测] 建立 %d 对会话", len(pairs))

	var sent, recv, failed atomic.Int64
	stopCh := make(chan struct{})
	startTime := time.Now()

	for _, p := range pairs {
		wg.Add(2)
		// A 持续发给 B
		go func(p *pair) {
			defer wg.Done()
			for {
				select {
				case <-stopCh:
					return
				default:
				}
				content := strconv.FormatInt(time.Now().UnixNano(), 10)
				if err := p.a.SendOnly(MsgChat, p.bID, encodeChatMsg(p.aID, p.bID, content)); err != nil {
					failed.Add(1)
					return
				}
				sent.Add(1)
				time.Sleep(time.Millisecond) // 控速，避免压测端 CPU 打满使数据失真
			}
		}(p)
		// B 接收服务端转发并计端到端延迟
		go func(p *pair) {
			defer wg.Done()
			for {
				select {
				case <-stopCh:
					p.b.Close()
					return
				default:
				}
				p.b.SetReadDeadline(time.Now().Add(2 * time.Second))
				_, _, payload, err := p.b.ReadFrame()
				if err != nil {
					continue
				}
				recv.Add(1)
				if c := pbScanBytes(payload, 3); c != nil { // ChatMsg.content
					if ns, e := strconv.ParseInt(string(c), 10, 64); e == nil {
						report.Latency.Record(time.Duration(time.Now().UnixNano() - ns))
					}
				}
			}
		}(p)
	}

	time.Sleep(cfg.Duration)
	close(stopCh)
	for _, p := range pairs {
		p.a.Close() // 关发送端，促使接收端 ReadFrame 退出
	}
	wg.Wait()

	elapsed := time.Since(startTime)
	report.Duration = elapsed
	report.SuccessReqs = recv.Load() // 端到端成功转发条数
	report.FailedReqs = failed.Load()
	report.TotalReqs = sent.Load()
	report.ReqsPerSec = float64(recv.Load()) / elapsed.Seconds() // 端到端转发吞吐
	report.Connections = int64(len(pairs) * 2)
	return report
}

// ─── 场景 E：混合压测 ───
// 模拟真实负载：50% 心跳(无需账号) + 30% 短连接 + 20% 登录(需预置账号)
func benchMixed(cfg BenchConfig) *BenchReport {
	log.Printf("[混合压测] 启动: 并发=%d, 时长=%v", cfg.Concurrency, cfg.Duration)
	report := &BenchReport{Scenario: "混合负载", Latency: &LatencyStats{}}

	var success, failed atomic.Int64
	ctx, cancel := context.WithTimeout(context.Background(), cfg.Duration)
	defer cancel()
	startTime := time.Now()

	hCount := cfg.Concurrency * 50 / 100
	cCount := cfg.Concurrency * 30 / 100
	lCount := cfg.Concurrency - hCount - cCount

	var wg sync.WaitGroup

	// 心跳长连接 worker（无需账号）
	for i := 0; i < hCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c := &IMClient{}
			if c.Connect(cfg.ServerAddr, 3*time.Second) != nil {
				return
			}
			defer c.Close()
			for ctx.Err() == nil {
				_, elapsed, err := c.SendRecv(MsgHeartbeat, 0, nil)
				report.Latency.Record(elapsed)
				if err != nil {
					failed.Add(1)
					return
				}
				success.Add(1)
				time.Sleep(20 * time.Millisecond)
			}
		}()
	}

	// 短连接 worker
	for i := 0; i < cCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for ctx.Err() == nil {
				c := &IMClient{}
				if c.Connect(cfg.ServerAddr, 2*time.Second) != nil {
					failed.Add(1)
					continue
				}
				success.Add(1)
				c.Close()
			}
		}()
	}

	// 登录 worker（需预置账号）
	for i := 0; i < lCount; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			payload := encodeLoginReq(fmt.Sprintf("bench_user_%d", idx), "123456")
			for ctx.Err() == nil {
				c := &IMClient{}
				if c.Connect(cfg.ServerAddr, 3*time.Second) != nil {
					failed.Add(1)
					continue
				}
				rsp, elapsed, err := c.SendRecv(MsgLogin, 0, payload)
				report.Latency.Record(elapsed)
				if err != nil || decodeCommonRspCode(rsp) != 0 {
					failed.Add(1)
				} else {
					success.Add(1)
				}
				c.Close()
				time.Sleep(50 * time.Millisecond)
			}
		}(i)
	}

	wg.Wait()
	elapsed := time.Since(startTime)
	report.Duration = elapsed
	report.SuccessReqs = success.Load()
	report.FailedReqs = failed.Load()
	report.TotalReqs = report.SuccessReqs + report.FailedReqs
	report.ReqsPerSec = float64(report.TotalReqs) / elapsed.Seconds()
	return report
}
