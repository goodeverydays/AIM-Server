#pragma once
#include <list>
#include <string>
#include <mutex>
#include <stdint.h>

// 离线消息缓存：接收方不在线时，将消息持久化到数据库表 t_offline_msg，
// 待其登录后补推。持久化保证服务进程重启/崩溃后离线消息不丢失。

struct NotifyMsgCache
{
	int32_t     userid;
	std::string notifymsg;   // 序列化后的 MessageContainer 字节串
};

struct ChatMsgCache
{
	int32_t     userid;
	std::string chatmsg;     // 序列化后的 MessageContainer 字节串
};

class MsgCacheManager final
{
public:
	MsgCacheManager();
	~MsgCacheManager();

	MsgCacheManager(const MsgCacheManager& rhs) = delete;
	MsgCacheManager& operator=(const MsgCacheManager& rhs) = delete;

	// 写入一条离线通知消息（如被加好友通知）
	bool AddNotifyMsgCache(int32_t userid, const std::string& cache);
	// 取出并清空某用户的离线通知消息（按入队顺序）
	void GetNotifyMsgCache(int32_t userid, std::list<NotifyMsgCache>& cached);

	// 写入一条离线聊天消息
	bool AddChatMsgCache(int32_t userid, const std::string& cache);
	// 取出并清空某用户的离线聊天消息（按入队顺序）
	void GetChatMsgCache(int32_t userid, std::list<ChatMsgCache>& cached);

private:
	std::mutex m_mutex;   // 串行化离线消息的数据库读写
};
