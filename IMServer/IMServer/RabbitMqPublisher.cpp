#ifdef HAVE_RABBITMQ
#include "RabbitMqPublisher.h"
#include <amqp.h>
#include <amqp_tcp_socket.h>
#include <vector>
#include <cstring>
#include <iostream>

RabbitMqPublisher::RabbitMqPublisher(const std::string& url, const std::string& exchange)
	: m_url(url), m_exchange(exchange) {}

RabbitMqPublisher::~RabbitMqPublisher() { closeConn(); }

bool RabbitMqPublisher::ensureConnected()
{
	if (m_connected && m_conn) return true;

	// amqp_parse_url 会改写传入缓冲区，复制一份可写副本
	struct amqp_connection_info ci;
	memset(&ci, 0, sizeof(ci));
	std::vector<char> buf(m_url.begin(), m_url.end());
	buf.push_back('\0');
	if (amqp_parse_url(buf.data(), &ci) != AMQP_STATUS_OK) {
		std::cout << "[RabbitMQ] parse url failed: " << m_url << std::endl;
		return false;
	}

	amqp_connection_state_t conn = amqp_new_connection();
	amqp_socket_t* sock = amqp_tcp_socket_new(conn);
	if (!sock) { amqp_destroy_connection(conn); return false; }
	if (amqp_socket_open(sock, ci.host, ci.port) != AMQP_STATUS_OK) {
		std::cout << "[RabbitMQ] socket open failed " << ci.host << ":" << ci.port << std::endl;
		amqp_destroy_connection(conn);
		return false;
	}
	// rabbitmq-c 把 URL 结尾的 "/" 解析成空 vhost，而 broker 默认 vhost 是 "/"。
	// 空则回退 "/"，使 URL 带不带结尾斜杠都能登录。
	const char* vhost = (ci.vhost && ci.vhost[0]) ? ci.vhost : "/";
	if (amqp_login(conn, vhost, 0, 131072, 0, AMQP_SASL_METHOD_PLAIN,
				   ci.user, ci.password).reply_type != AMQP_RESPONSE_NORMAL) {
		std::cout << "[RabbitMQ] login failed (vhost=" << vhost << ")" << std::endl;
		amqp_destroy_connection(conn);
		return false;
	}
	amqp_channel_open(conn, 1);
	if (amqp_get_rpc_reply(conn).reply_type != AMQP_RESPONSE_NORMAL) {
		amqp_destroy_connection(conn);
		return false;
	}
	// 声明 direct 持久化交换机(幂等)
	amqp_exchange_declare(conn, 1, amqp_cstring_bytes(m_exchange.c_str()),
						  amqp_cstring_bytes("direct"), 0, 1, 0, 0, amqp_empty_table);
	if (amqp_get_rpc_reply(conn).reply_type != AMQP_RESPONSE_NORMAL) {
		amqp_destroy_connection(conn);
		return false;
	}

	m_conn = conn;
	m_connected = true;
	std::cout << "[RabbitMQ] publisher connected, exchange=" << m_exchange << std::endl;
	return true;
}

void RabbitMqPublisher::closeConn()
{
	if (m_conn) {
		amqp_connection_state_t conn = static_cast<amqp_connection_state_t>(m_conn);
		amqp_channel_close(conn, 1, AMQP_REPLY_SUCCESS);
		amqp_connection_close(conn, AMQP_REPLY_SUCCESS);
		amqp_destroy_connection(conn);
		m_conn = nullptr;
	}
	m_connected = false;
}

bool RabbitMqPublisher::Publish(const std::string& routingKey, const std::string& body)
{
	std::lock_guard<std::mutex> lk(m_mutex);
	if (!ensureConnected()) return false;

	amqp_connection_state_t conn = static_cast<amqp_connection_state_t>(m_conn);
	amqp_basic_properties_t props;
	memset(&props, 0, sizeof(props));
	props._flags = AMQP_BASIC_CONTENT_TYPE_FLAG | AMQP_BASIC_DELIVERY_MODE_FLAG;
	props.content_type = amqp_cstring_bytes("application/json");
	props.delivery_mode = 2; // 持久化消息

	amqp_bytes_t bodyBytes;
	bodyBytes.len = body.size();
	bodyBytes.bytes = const_cast<char*>(body.data());

	int rc = amqp_basic_publish(conn, 1, amqp_cstring_bytes(m_exchange.c_str()),
								amqp_cstring_bytes(routingKey.c_str()), 0, 0, &props, bodyBytes);
	if (rc != AMQP_STATUS_OK) {
		std::cout << "[RabbitMQ] publish failed rc=" << rc << ", reconnect next time" << std::endl;
		closeConn();   // 断线，下次重连
		return false;
	}
	return true;
}
#endif // HAVE_RABBITMQ
