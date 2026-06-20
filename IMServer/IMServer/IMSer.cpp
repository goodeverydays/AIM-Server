#include "IMSer.h"
#include "ClientSession.h"
#include <sstream>
#include <cstdlib>
#include <iostream>

using namespace std::placeholders;

bool IMSer::init(const std::string& ip, short port, EventLoop* loop)
{
	InetAddress addr(ip, port);
	m_server.reset(new TcpServer(loop, addr, "chatserver", TcpServer::kReusePort));
	m_server->setConnectionCallback(std::bind(&IMSer::OnConnection, this, std::placeholders::_1));

	// 多 Reactor：N 个 IO 线程(one-loop-per-thread)，每个连接固定到一个线程，
	// 配合 MySQL 连接池(每次查询借/还一条独立连接)避免多线程共用单条连接。
	// 并发安全：UserManager 读方法返回深拷贝快照、写方法均在锁内更新缓存、在线状态经
	// SetUserStatus 加锁写入，已消除 User 字段(customface/nickname/friends 等)的数据竞争，
	// 故默认开 4 线程。可由 IMSERVER_THREADS 覆盖。
	int ioThreads = 4;
	if (const char* v = std::getenv("IMSERVER_THREADS")) { int n = atoi(v); if (n > 0) ioThreads = n; }
	m_server->setThreadNum(ioThreads);
	std::cout << "IMServer IO threads = " << ioThreads << std::endl;

	m_server->start();

#ifdef HAVE_AGENT_GRPC
	// 初始化 gRPC 客户端（连接微服务）
	m_agentClient.reset(new AgentGrpcClient("127.0.0.1:19527"));
	m_avatarClient.reset(new AvatarGrpcClient("127.0.0.1:19529"));
	m_mailClient.reset(new MailGrpcClient("127.0.0.1:19531"));
#endif

#ifdef HAVE_RABBITMQ
	// RABBITMQ_ENABLED=1 时启用：聊天消息改发 MQ、由 GoChatPersister 异步落库。
	// 用纯 C++11 写法(VM 工程为 -std=c++0x)，避免 if 初始化语句的 C++17 扩展警告。
	const char* mqEn = std::getenv("RABBITMQ_ENABLED");
	if (mqEn && std::string(mqEn) == "1") {
		const char* urlEnv = std::getenv("RABBITMQ_URL");
		const char* exEnv  = std::getenv("RABBITMQ_EXCHANGE");
		std::string url = urlEnv ? urlEnv : "amqp://guest:guest@127.0.0.1:5672/";
		std::string ex  = exEnv  ? exEnv  : "im.events";
		m_mqPublisher.reset(new RabbitMqPublisher(url, ex));
		std::cout << "RabbitMQ enabled: chat persistence offloaded to MQ (" << url << ")" << std::endl;
	}
#endif

	return true;
}

void IMSer::OnConnection(const TcpConnectionPtr& conn)
{
	if (conn->connected())
	{
		ClientSessionPtr client(new ClientSession(conn));
		{
			std::lock_guard<std::mutex> guard(m_sessionlock);
			m_mapclient.insert(ConnPair((std::string)*client, client));
		}
	}
	else {
		OnClose(conn);
	}

}

void IMSer::OnClose(const TcpConnectionPtr& conn)
{
	stringstream ss;
	ss << (void*)conn.get();
	ConnIter iter = m_mapclient.find(ss.str());
	if(iter != m_mapclient.end())
	{
		m_mapclient.erase(iter);
	}
	else
	{
		std::cout << conn->name() << std::endl;
	}
}

std::shared_ptr<ClientSession> IMSer::GetSessionByID(int32_t userid)
{
	std::lock_guard<std::mutex> guard(m_sessionlock);
	for (const auto& pair : m_mapclient)
	{
		if (pair.second->UserID() == userid)
			return pair.second;
	}
	return ClientSessionPtr();
}

#ifdef HAVE_AGENT_GRPC
AgentGrpcClient* IMSer::GetAgentClient()
{
	return m_agentClient.get();
}

AvatarGrpcClient* IMSer::GetAvatarClient()
{
	return m_avatarClient.get();
}

MailGrpcClient* IMSer::GetMailClient()
{
	return m_mailClient.get();
}
#endif
