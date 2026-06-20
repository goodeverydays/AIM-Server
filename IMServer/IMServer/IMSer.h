#pragma once

#include "net/EventLoop.h"
#include "net/EventLoopThreadPool.h"
#include "net/EventLoopThread.h"
#include "net/TcpServer.h"
#include "base/Logging.h"
#include <list>
#include <mutex>
#include <memory>

using namespace muduo;
using namespace muduo::net;

class ClientSession;//前向声明，避免与ClientSession.h的循环依赖
#ifdef HAVE_AGENT_GRPC
#include "AgentGrpcClient.h"
#include "AvatarGrpcClient.h"
#include "MailGrpcClient.h"
#endif
#ifdef HAVE_RABBITMQ
#include "RabbitMqPublisher.h"
#endif

class IMSer final//final关键字表示这个类不能被继承，确保IMSer类的设计和实现不会被修改或扩展，保持其稳定性和安全性
{
public:
	IMSer() = default;
	IMSer(const IMSer&) = delete;
	IMSer& operator=(const IMSer&) = delete;
	~IMSer() = default;
	bool init(const std::string& ip, short port, EventLoop* loop);
	std::shared_ptr<ClientSession> GetSessionByID(int32_t userid);//根据用户ID查找对应的会话
#ifdef HAVE_AGENT_GRPC
	AgentGrpcClient* GetAgentClient();    // 获取 Agent gRPC 客户端
	AvatarGrpcClient* GetAvatarClient();  // 获取 Avatar gRPC 客户端
	MailGrpcClient* GetMailClient();      // 获取 Mail gRPC 客户端
#endif
#ifdef HAVE_RABBITMQ
	// 聊天消息异步落库的 MQ 发布者；未启用(RABBITMQ_ENABLED!=1)时返回 nullptr。
	RabbitMqPublisher* GetMqPublisher() { return m_mqPublisher.get(); }
#endif

protected:
	void OnConnection(const TcpConnectionPtr& conn);
	void OnClose(const TcpConnectionPtr& conn);
private:
	std::shared_ptr<TcpServer> m_server;//TCP服务器对象，负责监听和接受客户端连接
	std::map<std::string, std::shared_ptr<ClientSession>> m_mapclient;//连接ID和连接对象的映射，方便根据连接ID查找和管理连接
	std::mutex m_sessionlock;//保护m_mapclient的操作
#ifdef HAVE_AGENT_GRPC
	std::unique_ptr<AgentGrpcClient> m_agentClient;    // Agent gRPC 客户端
	std::unique_ptr<AvatarGrpcClient> m_avatarClient;   // Avatar gRPC 客户端
	std::unique_ptr<MailGrpcClient> m_mailClient;       // Mail gRPC 客户端
#endif
#ifdef HAVE_RABBITMQ
	std::unique_ptr<RabbitMqPublisher> m_mqPublisher;   // 聊天消息异步落库发布者(可空)
#endif
};
typedef std::pair<std::string, std::shared_ptr<ClientSession>> ConnPair;//定义一个连接对，包含连接ID和连接对象
typedef std::map<std::string, std::shared_ptr<ClientSession>>::iterator ConnIter;//定义一个连接迭代器，方便遍历连接映射
