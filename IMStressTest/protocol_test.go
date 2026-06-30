package main

import (
	"encoding/hex"
	"testing"
)

// 验证帧格式与 MessageContainer 往返
func TestFrameRoundTrip(t *testing.T) {
	payload := encodeLoginReq("alice", "pw")
	frame := packFrame(1002, 7, 42, payload)

	if string(frame[4:8]) != "IM01" {
		t.Fatalf("tag 错误: %x", frame[4:8])
	}
	body := frame[8 : len(frame)-4] // 去掉 total_len(4)+tag(4) 和尾部 adler32(4)
	cmd, seq, pl := decodeContainer(body)
	if cmd != 1002 || seq != 7 {
		t.Fatalf("cmd/seq 错误: cmd=%d seq=%d", cmd, seq)
	}
	if len(pl) == 0 {
		t.Fatal("payload 丢失")
	}
}

// 验证从 LoginRsp 解 userid：LoginRsp{user=field3{userid=field1=123}} = 1a 02 08 7b
func TestLoginUserID(t *testing.T) {
	rsp := []byte{0x1a, 0x02, 0x08, 0x7b}
	if id := decodeLoginUserID(rsp); id != 123 {
		t.Fatalf("userid 解析错误: %d (期望 123)", id)
	}
}

// 打印 heartbeat 帧 hex，人工核对 IM01 格式
func TestHeartbeatHex(t *testing.T) {
	f := packFrame(1000, 1, 0, nil)
	// 预期: 0000000d 494d3031 08e807 1001 <adler32:4B>
	//  total_len=0x0d(13)  tag="IM01"  cmd=1000(08e807) seq=1(1001)
	t.Logf("heartbeat 帧: %s", hex.EncodeToString(f))
	if hex.EncodeToString(f[:13]) != "0000000d494d303108e8071001" {
		t.Fatalf("帧前缀不符: %s", hex.EncodeToString(f[:13]))
	}
}
