#include "AvatarGrpcClient.h"

#include <grpcpp/grpcpp.h>
#include "avatar.pb.h"
#include "avatar.grpc.pb.h"

#include <chrono>
#include <iostream>

using grpc::Channel;
using grpc::ClientContext;
using grpc::Status;

struct AvatarGrpcClient::Impl {
    std::shared_ptr<Channel> channel;
    std::unique_ptr<avatar::AvatarService::Stub> stub;

    Impl(const std::string& target)
        : channel(grpc::CreateChannel(target, grpc::InsecureChannelCredentials()))
        , stub(avatar::AvatarService::NewStub(channel))
    {}
};

AvatarGrpcClient::AvatarGrpcClient(const std::string& target)
    : m_impl(new Impl(target))
{
    std::cout << "[AvatarGrpcClient] connected to " << target << std::endl;
}

AvatarGrpcClient::~AvatarGrpcClient() = default;

AvatarGrpcClient::UploadResult
AvatarGrpcClient::Upload(int32_t userId, const std::string& imgData, const std::string& format) {
    UploadResult result;

    avatar::UploadReq req;
    req.set_user_id(userId);
    req.set_image_data(imgData);
    req.set_format(format);

    ClientContext ctx;
    ctx.set_deadline(std::chrono::system_clock::now() + std::chrono::seconds(10));

    avatar::UploadRsp rsp;
    Status status = m_impl->stub->Upload(&ctx, req, &rsp);

    if (!status.ok()) {
        result.errMsg = "gRPC error: " + status.error_message();
        std::cerr << "[AvatarGrpcClient] " << result.errMsg << std::endl;
        return result;
    }

    if (rsp.code() != 0) {
        result.errMsg = rsp.msg();
        return result;
    }

    result.ok = true;
    result.url = rsp.url();
    std::cout << "[AvatarGrpcClient] upload ok, url=" << result.url << std::endl;
    return result;
}
