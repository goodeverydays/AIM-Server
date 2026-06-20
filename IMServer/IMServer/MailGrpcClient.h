#pragma once

#include <memory>
#include <string>

namespace mail {
class MailService;
class SendCodeReq;
class SendCodeRsp;
class VerifyCodeReq;
class VerifyCodeRsp;
}

namespace grpc {
class Channel;
}

/// MailGrpcClient — gRPC 客户端，连接 GoMailServer 邮箱验证码微服务
class MailGrpcClient {
public:
    explicit MailGrpcClient(const std::string& target);
    ~MailGrpcClient();

    /// 发送验证码
    struct SendCodeResult {
        bool   ok = false;
        std::string msg;
        int32_t cooldownSeconds = 0;
        int32_t code = 3;   // 0=成功, 1=冷却中, 2=邮箱格式错误, 3=发送失败
    };
    SendCodeResult SendCode(const std::string& email);

    /// 校验验证码
    struct VerifyResult {
        bool   ok = false;
        std::string msg;
    };
    VerifyResult VerifyCode(const std::string& email, const std::string& code);

private:
    struct Impl;
    std::unique_ptr<Impl> m_impl;
};
