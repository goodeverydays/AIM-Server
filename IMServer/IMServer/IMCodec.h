// IMCodec —— IM服务器protobuf编解码适配层
// 封装muduo::net::ProtobufCodecLite，提供IM消息的统一序列化/反序列化接口
// 帧格式: [total_len:4B][tag:"IM01":4B][protobuf_body:N bytes][checksum:4B Adler32]
#pragma once

#include "net/protobuf/ProtobufCodecLite.h"
#include "net/Callbacks.h"
#include "net/TcpConnection.h"
#include "base/Timestamp.h"

#include <functional>
#include <memory>

// 前向声明 proto 生成的类
namespace im {
class MessageContainer;
}

namespace muduo {
namespace net {

// IM协议编解码器
// 基于ProtobufCodecLite的帧封装（含Adler32校验），tag="IM01"作为协议魔数
class IMCodec : noncopyable {
public:
    // 消息回调类型：连接、反序列化后的MessageContainer、接收时间
    using MessageCallback = std::function<void(
        const TcpConnectionPtr&,
        const im::MessageContainer&,
        Timestamp)>;

    // 错误回调类型
    using ErrorCallback = ProtobufCodecLite::ErrorCallback;

    /// 构造函数
    /// @param messageCb 消息到达回调，当完整解码一条MessageContainer后触发
    /// @param errorCb   解码错误回调（可选）
    IMCodec(const MessageCallback& messageCb,
            const ErrorCallback& errorCb = ProtobufCodecLite::defaultErrorCallback);

    ~IMCodec() = default;

    /// 发送一条IM消息给指定连接
    /// 自动完成：MessageContainer序列化 → 帧封装 → Adler32校验 → TCP发送
    /// @param conn 目标连接
    /// @param msg  要发送的消息容器
    void send(const TcpConnectionPtr& conn, const im::MessageContainer& msg);

    /// 发送预序列化的MessageContainer数据
    /// 用于发送缓存消息（离线消息推送场景），内部解析后走标准send流程
    /// @param conn 目标连接
    /// @param data MessageContainer序列化后的字节串
    void sendRaw(const TcpConnectionPtr& conn, const std::string& data);

    /// 处理从TCP缓冲区接收到的数据
    /// 自动完成：帧解析 → 校验验证 → MessageContainer反序列化 → 触发messageCallback
    /// @param conn 来源连接
    /// @param buf  接收缓冲区
    /// @param time 接收时间戳
    void onMessage(const TcpConnectionPtr& conn, Buffer* buf, Timestamp time);

    /// 获取协议标识tag
    const std::string& tag() const;

private:
    /// ProtobufCodecLite内部回调，将MessagePtr转型为具体类型后转发给用户回调
    void onProtobufMessage(const TcpConnectionPtr& conn,
                           const MessagePtr& msg,
                           Timestamp time);

    ProtobufCodecLite codec_;       // 底层protobuf编解码器
    MessageCallback   messageCallback_; // 用户消息回调
};

}  // namespace net
}  // namespace muduo
