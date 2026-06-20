#include "AgentGrpcClient.h"

#include <grpcpp/grpcpp.h>
#include "agent.pb.h"
#include "agent.grpc.pb.h"

#include <thread>
#include <chrono>
#include <iostream>

using grpc::Channel;
using grpc::ClientContext;
using grpc::Status;

struct AgentGrpcClient::Impl {
    std::shared_ptr<Channel> channel;
    std::unique_ptr<agent::AgentService::Stub> stub;

    Impl(const std::string& target)
        : channel(grpc::CreateChannel(target, grpc::InsecureChannelCredentials()))
        , stub(agent::AgentService::NewStub(channel))
    {}
};

AgentGrpcClient::AgentGrpcClient(const std::string& target)
    : m_target(target)
    , m_impl(std::unique_ptr<Impl>(new Impl(target)))
{
    std::cout << "[AgentGrpcClient] created, target=" << target << std::endl;
}

AgentGrpcClient::~AgentGrpcClient() = default;

void AgentGrpcClient::sendMessage(int32_t userId, int32_t targetId,
                                   const std::string& content, int32_t chatType,
                                   ReplyCallback callback,
                                   const std::string& skill,
                                   const std::vector<std::string>& context) {
    // 在后台线程执行同步 gRPC 调用，避免阻塞 IMServer 事件循环
    std::thread([this, userId, targetId, content, chatType, callback, skill, context]() {
        std::string reply;
        bool ok = processMessageSync(userId, targetId, content, chatType, reply, skill, context);
        if (callback) {
            callback(ok, reply);
        }
    }).detach();
}

bool AgentGrpcClient::processMessageSync(int32_t userId, int32_t targetId,
                                          const std::string& content, int32_t chatType,
                                          std::string& replyOut,
                                          const std::string& skill,
                                          const std::vector<std::string>& context) {
    // 构造请求
    agent::ProcessMessageReq req;
    req.set_user_id(userId);
    req.set_target_id(targetId);
    req.set_content(content);
    req.set_chat_type(chatType);
    if (!skill.empty()) {
        req.set_skill(skill);
    }
    for (const auto& line : context) {
        req.add_context(line);
    }

    // 调用 gRPC
    ClientContext ctx;
    ctx.set_deadline(std::chrono::system_clock::now() + std::chrono::seconds(15));

    agent::ProcessMessageRsp rsp;
    Status status = m_impl->stub->ProcessMessage(&ctx, req, &rsp);

    if (!status.ok()) {
        std::cerr << "[AgentGrpcClient] gRPC error: " << status.error_message() << std::endl;
        replyOut = "Agent 服务不可用: " + status.error_message();
        return false;
    }

    if (rsp.code() != 0) {
        std::cerr << "[AgentGrpcClient] Agent returned error: " << rsp.msg() << std::endl;
        replyOut = rsp.msg();
        return false;
    }

    replyOut = rsp.reply();
    std::cout << "[AgentGrpcClient] reply received, length=" << replyOut.length() << std::endl;
    return true;
}

bool AgentGrpcClient::upgradeVip(int32_t userId, const std::string& paymentToken,
                                 int32_t amountCents, std::string& msgOut, bool& isVipOut) {
    agent::UpgradeVipReq req;
    req.set_user_id(userId);
    req.set_payment_token(paymentToken);
    req.set_amount_cents(amountCents);

    ClientContext ctx;
    ctx.set_deadline(std::chrono::system_clock::now() + std::chrono::seconds(10));

    agent::VipStatusRsp rsp;
    Status status = m_impl->stub->UpgradeVip(&ctx, req, &rsp);

    if (!status.ok()) {
        std::cerr << "[AgentGrpcClient] UpgradeVip gRPC error: " << status.error_message() << std::endl;
        msgOut = "会员服务不可用: " + status.error_message();
        isVipOut = false;
        return false;
    }

    msgOut = rsp.msg();
    isVipOut = rsp.is_vip();
    return rsp.code() == 0;
}

bool AgentGrpcClient::queryVipStatus(int32_t userId, bool& isVipOut) {
    isVipOut = false;

    agent::VipStatusReq req;
    req.set_user_id(userId);

    ClientContext ctx;
    // 登录路径上调用，超时设短一些，避免 goagent 异常时拖慢登录
    ctx.set_deadline(std::chrono::system_clock::now() + std::chrono::seconds(3));

    agent::VipStatusRsp rsp;
    Status status = m_impl->stub->GetVipStatus(&ctx, req, &rsp);

    if (!status.ok()) {
        std::cerr << "[AgentGrpcClient] GetVipStatus gRPC error: " << status.error_message() << std::endl;
        return false;
    }

    isVipOut = rsp.is_vip();
    return rsp.code() == 0;
}

bool AgentGrpcClient::isHealthy() {
    agent::HealthCheckReq req;
    ClientContext ctx;
    ctx.set_deadline(std::chrono::system_clock::now() + std::chrono::seconds(3));

    agent::HealthCheckRsp rsp;
    Status status = m_impl->stub->HealthCheck(&ctx, req, &rsp);

    if (!status.ok()) return false;
    return rsp.healthy();
}
