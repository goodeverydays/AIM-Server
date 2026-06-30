package main

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"time"
)

// 延迟统计器：记录每次操作的延迟，计算 P50/P90/P95/P99/Avg/Min/Max
type LatencyStats struct {
	mu      sync.Mutex
	samples []float64 // 单位：毫秒
}

func (s *LatencyStats) Record(d time.Duration) {
	s.mu.Lock()
	s.samples = append(s.samples, float64(d.Microseconds())/1000.0)
	s.mu.Unlock()
}

func (s *LatencyStats) Count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.samples)
}

func (s *LatencyStats) Percentile(p float64) float64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.samples) == 0 {
		return 0
	}
	sorted := make([]float64, len(s.samples))
	copy(sorted, s.samples)
	sort.Float64s(sorted)
	idx := int(math.Ceil(p/100.0*float64(len(sorted)))) - 1
	if idx < 0 {
		idx = 0
	}
	return sorted[idx]
}

func (s *LatencyStats) Avg() float64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.samples) == 0 {
		return 0
	}
	sum := 0.0
	for _, v := range s.samples {
		sum += v
	}
	return sum / float64(len(s.samples))
}

func (s *LatencyStats) Min() float64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.samples) == 0 {
		return 0
	}
	min := s.samples[0]
	for _, v := range s.samples {
		if v < min {
			min = v
		}
	}
	return min
}

func (s *LatencyStats) Max() float64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.samples) == 0 {
		return 0
	}
	max := s.samples[0]
	for _, v := range s.samples {
		if v > max {
			max = v
		}
	}
	return max
}

// 压测报告
type BenchReport struct {
	Scenario     string
	Duration     time.Duration
	TotalReqs    int64
	SuccessReqs  int64
	FailedReqs   int64
	ReqsPerSec   float64
	Latency      *LatencyStats
	Connections  int64
	ConnFailures int64
}

// 打印控制台报告
func (r *BenchReport) Print() {
	fmt.Println()
	fmt.Println(strings.Repeat("=", 60))
	fmt.Printf("  压测场景: %s\n", r.Scenario)
	fmt.Printf("  持续时间: %v\n", r.Duration.Round(time.Second))
	fmt.Printf("  总请求数: %d\n", r.TotalReqs)
	fmt.Printf("  成功请求: %d\n", r.SuccessReqs)
	fmt.Printf("  失败请求: %d\n", r.FailedReqs)
	if r.TotalReqs > 0 {
		fmt.Printf("  成功率:   %.2f%%\n", float64(r.SuccessReqs)/float64(r.TotalReqs)*100)
	}
	fmt.Printf("  吞吐量:   %.0f req/s\n", r.ReqsPerSec)
	if r.Connections > 0 {
		fmt.Printf("  建立连接: %d (失败: %d)\n", r.Connections, r.ConnFailures)
	}
	fmt.Println("  --- 延迟统计 (ms) ---")
	fmt.Printf("  平均:  %8.3f\n", r.Latency.Avg())
	fmt.Printf("  最小:  %8.3f\n", r.Latency.Min())
	fmt.Printf("  最大:  %8.3f\n", r.Latency.Max())
	fmt.Printf("  P50:   %8.3f\n", r.Latency.Percentile(50))
	fmt.Printf("  P90:   %8.3f\n", r.Latency.Percentile(90))
	fmt.Printf("  P95:   %8.3f\n", r.Latency.Percentile(95))
	fmt.Printf("  P99:   %8.3f\n", r.Latency.Percentile(99))
	fmt.Println(strings.Repeat("=", 60))
	fmt.Println()
}

// Markdown 表格行，方便写入 README
func (r *BenchReport) Markdown() string {
	sRate := "0.00"
	if r.TotalReqs > 0 {
		sRate = fmt.Sprintf("%.2f", float64(r.SuccessReqs)/float64(r.TotalReqs)*100)
	}
	return fmt.Sprintf("| %s | %v | %.0f | %.2f | %.2f | %.2f | %.2f | %s%% |",
		r.Scenario,
		r.Duration.Round(time.Second),
		r.ReqsPerSec,
		r.Latency.Avg(),
		r.Latency.Percentile(50),
		r.Latency.Percentile(95),
		r.Latency.Percentile(99),
		sRate,
	)
}
