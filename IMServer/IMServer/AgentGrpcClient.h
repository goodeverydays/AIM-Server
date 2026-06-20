#pragma once

#include <memory>
#include <string>
#include <vector>
#include <functional>

// 前向声明 gRPC 生成代码
namespace agent {
class AgentService;
class ProcessMessageReq;
class ProcessMessageRsp;
}

namespace grpc {
class Channel;
class ClientContext;
class Status;
}

/// AgentGrpcClient — gRPC 客户端，连接 goagent 微服务
///
/// 将 IM 消息转发给 AI Agent，获取智能回复。
/// 使用 gRPC + Protobuf 通信，与客户端完全解耦 ——
/// 客户端不知道 Agent 是独立服务，IMServer 作为网关统一路由。
class AgentGrpcClient {
public:
    /// @param target  goagent gRPC 地址，如 "127.0.0.1:19527"
    explicit AgentGrpcClient(const std::string& target);
    ~AgentGrpcClient();

    AgentGrpcClient(const AgentGrpcClient&) = delete;
    AgentGrpcClient& operator=(const AgentGrpcClient&) = delete;

    /// 发送消息到 Agent 并异步等待回复
    /// @param userId    发送者用户ID
    /// @param targetId  目标ID（单聊为对方ID，群聊为群ID）
    /// @param content   消息文本
    /// @param chatType  1=单聊, 2=群聊
    /// @param callback  回调 (success: bool, reply_content: string)
    /// @param skill     技能标识（空=普通对话，"summarize"/"suggest_reply" 等）
    /// @param context   技能所需上下文（如待总结的会话历史）
    using ReplyCallback = std::function<void(bool, const std::string&)>;
    void sendMessage(int32_t userId, int32_t targetId,
                     const std::string& content, int32_t chatType,
                     ReplyCallback callback,
                     const std::string& skill = "",
                     const std::vector<std::string>& context = {});

    /// 健康检查
    bool isHealthy();

    /// 升级 VIP（同步调用 goagent；阻塞当前线程，调用方需在非事件循环线程使用或可接受短暂阻塞）
    /// @return 是否成功；msgOut 提示信息，isVipOut 升级后是否为 VIP
    bool upgradeVip(int32_t userId, const std::string& paymentToken,
                    int32_t amountCents, std::string& msgOut, bool& isVipOut);

    /// 查询 VIP 状态（登录时调用，用于恢复客户端会员身份）
    /// @return gRPC 是否成功；isVipOut 为当前是否 VIP（失败时置 false）
    bool queryVipStatus(int32_t userId, bool& isVipOut);

    /// 同步调用（阻塞当前线程，仅用于简单场景）
    bool processMessageSync(int32_t userId, int32_t targetId,
                            const std::string& content, int32_t chatType,
                            std::string& replyOut,
                            const std::string& skill = "",
                            const std::vector<std::string>& context = {});

private:
    std::string m_target;
    // gRPC 对象通过 impl 模式隐藏，避免头文件暴露 gRPC 依赖
    struct Impl;
    std::unique_ptr<Impl> m_impl;
};
