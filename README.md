# AIM-Chat

全栈即时通讯系统：C++/muduo 长连接服务 + Go 微服务集群 + Qt 客户端。

## 架构

| 服务 | 语言 | 职责 |
|---|---|---|
| **IMServer** | C++/muduo | TCP 长连接、多 Reactor、MySQL 连接池、protobuf 编解码 |
| **goagent** | Go | AI 助手：LLM 对话 + RAG 记忆问答（Qdrant 向量库）+ Redis 缓存 + 配额限流 |
| **GoImageServer** | Go | 头像上传/存储（本地 / S3 对象存储） |
| **GoMediaServer** | Go | 语音/视频媒体存储 |
| **GoMailServer** | Go | 邮箱验证码（SMTP） |
| **GoChatPersister** | Go | RabbitMQ 消费者，聊天消息异步落库 |

- **中间件**：MySQL / Redis / Qdrant / RabbitMQ
- **服务间通信**：gRPC（IMServer → 各 Go 服务）
- **异步解耦**：RabbitMQ（聊天消息削峰落库）
- **日志**：标准库 `log/slog` 结构化日志

## 一键部署（阿里云 ECS / 任意 Docker 主机）

前提：已安装 Docker + Docker Compose（host 网络模式，单机部署）。

```bash
git clone git@github.com:goodeverydays/AIM-Chat.git
cd AIM-Chat
cp .env.example .env
vim .env          # 填 PUBLIC_HOST(公网IP) / MYSQL_PASSWORD / DEEPSEEK_API_KEY / EMBED_* ；SMTP 可留空
docker compose up -d --build    # 首次 IMServer 构建较慢（10min+）
docker compose ps               # 全部 Up 即部署成功
```

## 安全须知

- `.env` 含密钥，已 gitignore，**切勿提交**。
- 阿里云安全组只放行 `9527`（IM）+ 头像/媒体 HTTP 端口；`3306 / 6379 / 5672 / 6333 / 15672` **不开公网**（代码已绑 127.0.0.1）。
- Qt 客户端 `qml/main.qml` 的 `serverHost` 改为 ECS 公网 IP。
