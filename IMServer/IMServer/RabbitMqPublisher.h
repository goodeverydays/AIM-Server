#pragma once
#include <string>
#include <mutex>

// RabbitMQ 发布者(rabbitmq-c 封装)。线程安全:单连接 + 互斥,供多 IO 线程并发发布。
// 仅在 CMake 检测到 librabbitmq 时编译(HAVE_RABBITMQ)。
class RabbitMqPublisher {
public:
	// url 形如 amqp://user:pass@host:5672/ ；exchange 为 direct 交换机名。
	RabbitMqPublisher(const std::string& url, const std::string& exchange);
	~RabbitMqPublisher();

	// 线程安全发布。成功返回 true；失败(含未连接/断线)返回 false，调用方应回退到同步落库。
	bool Publish(const std::string& routingKey, const std::string& body);

private:
	bool ensureConnected();   // 需持锁调用
	void closeConn();

	std::string m_url;
	std::string m_exchange;
	std::mutex  m_mutex;
	void*       m_conn = nullptr;   // amqp_connection_state_t(用 void* 避免在头文件暴露 amqp.h)
	bool        m_connected = false;
};
