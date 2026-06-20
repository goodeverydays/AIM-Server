#include "MailGrpcClient.h"

#include <grpcpp/grpcpp.h>
#include "mail.pb.h"
#include "mail.grpc.pb.h"

#include <chrono>
#include <iostream>

using grpc::Channel;
using grpc::ClientContext;
using grpc::Status;

struct MailGrpcClient::Impl {
    std::shared_ptr<Channel> channel;
    std::unique_ptr<mail::MailService::Stub> stub;

    Impl(const std::string& target)
        : channel(grpc::CreateChannel(target, grpc::InsecureChannelCredentials()))
        , stub(mail::MailService::NewStub(channel))
    {}
};

MailGrpcClient::MailGrpcClient(const std::string& target)
    : m_impl(new Impl(target))
{
    std::cout << "[MailGrpcClient] connected to " << target << std::endl;
}

MailGrpcClient::~MailGrpcClient() = default;

MailGrpcClient::SendCodeResult
MailGrpcClient::SendCode(const std::string& email) {
    SendCodeResult result;

    mail::SendCodeReq req;
    req.set_email(email);

    ClientContext ctx;
    ctx.set_deadline(std::chrono::system_clock::now() + std::chrono::seconds(10));

    mail::SendCodeRsp rsp;
    Status status = m_impl->stub->SendCode(&ctx, req, &rsp);

    if (!status.ok()) {
        result.code = 3;
        result.msg = "gRPC error: " + status.error_message();
        std::cerr << "[MailGrpcClient] " << result.msg << std::endl;
        return result;
    }

    result.code = rsp.code();
    result.msg = rsp.msg();
    result.cooldownSeconds = rsp.cooldown_seconds();
    result.ok = (rsp.code() == 0);
    return result;
}

MailGrpcClient::VerifyResult
MailGrpcClient::VerifyCode(const std::string& email, const std::string& code) {
    VerifyResult result;

    mail::VerifyCodeReq req;
    req.set_email(email);
    req.set_code(code);

    ClientContext ctx;
    ctx.set_deadline(std::chrono::system_clock::now() + std::chrono::seconds(10));

    mail::VerifyCodeRsp rsp;
    Status status = m_impl->stub->VerifyCode(&ctx, req, &rsp);

    if (!status.ok()) {
        result.msg = "gRPC error: " + status.error_message();
        std::cerr << "[MailGrpcClient] " << result.msg << std::endl;
        return result;
    }

    result.ok = (rsp.code() == 0);
    result.msg = rsp.msg();
    return result;
}
