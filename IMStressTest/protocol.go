package main

// 真 IMServer 线格式（与 Qt 客户端 / IMCodec 一致）：
//
//	[total_len:4B BE] ["IM01":4B] [protobuf MessageContainer] [adler32:4B BE]
//
//	total_len = 4(tag) + len(body) + 4(adler32)
//	adler32 对 [tag + body] 计算（zlib adler32(1,...) == Go hash/adler32.Checksum）
//
// MessageContainer { cmd=1, seq=2, target_id=3, payload=5(bytes) }
// payload = 业务消息（LoginReq / ChatMsg ...）的 protobuf 序列化
//
// 这里手写最小 protobuf 编解码（只覆盖压测所需字段），避免依赖 protoc-gen-go。

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"hash/adler32"
	"io"
	"net"
	"time"
)

const imTag = "IM01"

// ───────── 手写 protobuf 编码 ─────────

func pbKey(buf *bytes.Buffer, field, wire int) {
	buf.Write(binary.AppendUvarint(nil, uint64(field)<<3|uint64(wire)))
}

// pbInt 编码 varint 字段（proto3 默认值 0 不编码）
func pbInt(buf *bytes.Buffer, field int, v int64) {
	if v == 0 {
		return
	}
	pbKey(buf, field, 0)
	buf.Write(binary.AppendUvarint(nil, uint64(v)))
}

// pbStr 编码 length-delimited 字符串字段
func pbStr(buf *bytes.Buffer, field int, s string) {
	if s == "" {
		return
	}
	pbKey(buf, field, 2)
	buf.Write(binary.AppendUvarint(nil, uint64(len(s))))
	buf.WriteString(s)
}

// pbBytes 编码 length-delimited bytes 字段
func pbBytes(buf *bytes.Buffer, field int, p []byte) {
	if len(p) == 0 {
		return
	}
	pbKey(buf, field, 2)
	buf.Write(binary.AppendUvarint(nil, uint64(len(p))))
	buf.Write(p)
}

// encodeContainer 编码外层信封 MessageContainer
func encodeContainer(cmd, seq, targetID int32, payload []byte) []byte {
	var b bytes.Buffer
	pbInt(&b, 1, int64(cmd))      // cmd
	pbInt(&b, 2, int64(seq))      // seq
	pbInt(&b, 3, int64(targetID)) // target_id
	pbBytes(&b, 5, payload)       // payload
	return b.Bytes()
}

// encodeLoginReq 编码 LoginReq{username,password,clienttype=1,status=1}
func encodeLoginReq(username, password string) []byte {
	var b bytes.Buffer
	pbStr(&b, 1, username)
	pbStr(&b, 2, password)
	pbInt(&b, 3, 1) // clienttype
	pbInt(&b, 4, 1) // status
	return b.Bytes()
}

// encodeChatMsg 编码 ChatMsg{senderid,targetid,content,timestamp}
func encodeChatMsg(senderid, targetid int32, content string) []byte {
	var b bytes.Buffer
	pbInt(&b, 1, int64(senderid))
	pbInt(&b, 2, int64(targetid))
	pbStr(&b, 3, content)
	pbInt(&b, 4, time.Now().Unix())
	return b.Bytes()
}

// ───────── 帧编码 ─────────

// packFrame 组装完整 IM 协议帧
func packFrame(cmd, seq, targetID int32, payload []byte) []byte {
	body := encodeContainer(cmd, seq, targetID, payload)

	tagBody := make([]byte, 0, len(imTag)+len(body))
	tagBody = append(tagBody, imTag...)
	tagBody = append(tagBody, body...)

	cs := adler32.Checksum(tagBody) // 对 [tag+body] 算校验和
	totalLen := uint32(len(tagBody) + 4)

	frame := make([]byte, 0, 4+len(tagBody)+4)
	frame = binary.BigEndian.AppendUint32(frame, totalLen)
	frame = append(frame, tagBody...)
	frame = binary.BigEndian.AppendUint32(frame, cs)
	return frame
}

// ───────── 帧解码 ─────────

// readFrame 从连接读取一个完整帧，返回 cmd/seq/payload
func readFrame(conn net.Conn) (cmd, seq int32, payload []byte, err error) {
	var lenBuf [4]byte
	if _, err = io.ReadFull(conn, lenBuf[:]); err != nil {
		return 0, 0, nil, err
	}
	totalLen := binary.BigEndian.Uint32(lenBuf[:])
	if totalLen < 8 || totalLen > 10*1024*1024 {
		return 0, 0, nil, fmt.Errorf("invalid total_len: %d", totalLen)
	}
	buf := make([]byte, totalLen)
	if _, err = io.ReadFull(conn, buf); err != nil {
		return 0, 0, nil, err
	}
	if string(buf[:4]) != imTag {
		return 0, 0, nil, fmt.Errorf("bad tag: %q", buf[:4])
	}
	body := buf[4 : totalLen-4] // 去掉 tag 与尾部 4B adler32
	cmd, seq, payload = decodeContainer(body)
	return cmd, seq, payload, nil
}

// decodeContainer 解析 MessageContainer，取 cmd/seq/payload（忽略其它字段）
func decodeContainer(body []byte) (cmd, seq int32, payload []byte) {
	i := 0
	for i < len(body) {
		key, n := binary.Uvarint(body[i:])
		if n <= 0 {
			break
		}
		i += n
		field, wire := int(key>>3), int(key&7)
		switch wire {
		case 0: // varint
			v, n := binary.Uvarint(body[i:])
			if n <= 0 {
				return
			}
			i += n
			switch field {
			case 1:
				cmd = int32(v)
			case 2:
				seq = int32(v)
			}
		case 2: // length-delimited
			l, n := binary.Uvarint(body[i:])
			if n <= 0 || i+n+int(l) > len(body) {
				return
			}
			i += n
			if field == 5 {
				payload = body[i : i+int(l)]
			}
			i += int(l)
		default:
			return // 不支持的 wire type，停止
		}
	}
	return
}

// decodeCommonRspCode 从 payload(CommonRsp/LoginRsp 等)解出 code(field 1)，
// 用于判断业务是否成功（0=成功）。解不出按成功(0)处理。
func decodeCommonRspCode(payload []byte) int32 {
	i := 0
	for i < len(payload) {
		key, n := binary.Uvarint(payload[i:])
		if n <= 0 {
			break
		}
		i += n
		field, wire := int(key>>3), int(key&7)
		if field == 1 && wire == 0 {
			v, n := binary.Uvarint(payload[i:])
			if n <= 0 {
				return 0
			}
			return int32(v)
		}
		// 跳过其它字段
		switch wire {
		case 0:
			_, n := binary.Uvarint(payload[i:])
			i += n
		case 2:
			l, n := binary.Uvarint(payload[i:])
			i += n + int(l)
		default:
			return 0
		}
	}
	return 0
}

// pbScanBytes 取 buf 中字段号 field 的 length-delimited 值（取不到返回 nil）
func pbScanBytes(buf []byte, field int) []byte {
	i := 0
	for i < len(buf) {
		key, n := binary.Uvarint(buf[i:])
		if n <= 0 {
			break
		}
		i += n
		f, w := int(key>>3), int(key&7)
		switch w {
		case 0:
			_, n := binary.Uvarint(buf[i:])
			if n <= 0 {
				return nil
			}
			i += n
		case 2:
			l, n := binary.Uvarint(buf[i:])
			if n <= 0 || i+n+int(l) > len(buf) {
				return nil
			}
			i += n
			if f == field {
				return buf[i : i+int(l)]
			}
			i += int(l)
		default:
			return nil
		}
	}
	return nil
}

// decodeLoginUserID 从 LoginRsp 解出自己的 userid
// （LoginRsp.user = field 3(UserInfo) → UserInfo.userid = field 1）
func decodeLoginUserID(loginRsp []byte) int32 {
	user := pbScanBytes(loginRsp, 3)
	if user == nil {
		return 0
	}
	// 复用 decodeCommonRspCode 的逻辑取 field 1 varint（UserInfo.userid）
	return decodeCommonRspCode(user)
}
