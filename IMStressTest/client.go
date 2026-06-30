package main

import (
	"net"
	"sync/atomic"
	"time"
)

// IMClient 封装与真 IMServer 的单条 TCP 连接，按 protobuf + IM01 协议收发。
type IMClient struct {
	conn net.Conn
	seq  int32
}

// 消息类型常量（与 IMServer ImCmd 枚举一致）
const (
	MsgHeartbeat = 1000 // 心跳（服务端会回包）
	MsgRegister  = 1001 // 注册（需邮箱验证码，压测一般预置账号绕过）
	MsgLogin     = 1002 // 登录（请求-响应）
	MsgChat      = 1100 // 单聊（服务端转发给对方，不回发送方 ack）
)

// Connect 建立 TCP 连接
func (c *IMClient) Connect(addr string, timeout time.Duration) error {
	conn, err := net.DialTimeout("tcp", addr, timeout)
	if err != nil {
		return err
	}
	c.conn = conn
	return nil
}

// Close 关闭连接
func (c *IMClient) Close() {
	if c.conn != nil {
		c.conn.Close()
	}
}

// SendOnly 发送一帧但不等响应（用于 chat 等 fire-and-forget）
func (c *IMClient) SendOnly(cmd, targetID int32, payload []byte) error {
	seq := atomic.AddInt32(&c.seq, 1)
	_, err := c.conn.Write(packFrame(cmd, seq, targetID, payload))
	return err
}

// SendRecv 发送并同步等待一个响应帧，返回响应 payload 与往返延迟（用于 heartbeat/login）
func (c *IMClient) SendRecv(cmd, targetID int32, payload []byte) (rsp []byte, elapsed time.Duration, err error) {
	start := time.Now()
	if err = c.SendOnly(cmd, targetID, payload); err != nil {
		return nil, time.Since(start), err
	}
	_, _, rsp, err = readFrame(c.conn)
	return rsp, time.Since(start), err
}

// ReadFrame 读取一帧（用于 chat 接收端统计服务端转发过来的消息）
func (c *IMClient) ReadFrame() (cmd, seq int32, payload []byte, err error) {
	return readFrame(c.conn)
}

// SetReadDeadline 给接收设置超时（接收端循环用，避免永久阻塞）
func (c *IMClient) SetReadDeadline(t time.Time) {
	if c.conn != nil {
		c.conn.SetReadDeadline(t)
	}
}
