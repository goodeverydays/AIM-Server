# AIM-Chat

> 一套**分布式即时通讯系统**：C++/muduo 高性能长连接核心 + Go 微服务集群 + AI 记忆问答 + Qt 桌面客户端，一键 Docker 容器化部署。

![C++17](https://img.shields.io/badge/C%2B%2B-17-00599C?logo=cplusplus)
![Go](https://img.shields.io/badge/Go-1.25-00ADD8?logo=go)
![muduo](https://img.shields.io/badge/net-muduo%20Reactor-orange)
![gRPC](https://img.shields.io/badge/RPC-gRPC%2Fprotobuf-244c5a)
![RabbitMQ](https://img.shields.io/badge/MQ-RabbitMQ-FF6600?logo=rabbitmq)
![Qdrant](https://img.shields.io/badge/VectorDB-Qdrant-DC244C)
![Docker](https://img.shields.io/badge/deploy-Docker%20Compose-2496ED?logo=docker)

---

## ✨ 核心特性

- **高性能长连接**：C++/muduo 多 Reactor（one-loop-per-thread）TCP 服务器，自定义二进制协议（长度前缀 + Protobuf + Adler32 校验）解决粘包/拆包。
- **并发安全**：MySQL 连接池（`condition_variable` 借还）+ "读返回深拷贝快照、写加锁"，安全开启多 IO 线程吃满多核。
- **异步削峰**：聊天消息经 RabbitMQ（持久化 + 手动 ack + 幂等消费）异步落库，从转发热路径剥离 DB 写；发布失败自动回退同步写库，消息不丢。
- **微服务化**：用 gRPC 把 AI、头像、媒体、邮箱验证码拆成 4 个 Go 服务，标准 `cmd/internal` 分层 + 接口抽象 + 依赖注入。
- **AI 记忆问答（RAG）**：Qdrant 向量库 + Embedding，对用户历史消息增量索引 + 余弦 Top-K 检索，结合 LLM 实现"基于聊天记录"的问答。
- **优雅降级**：Redis 做缓存/限流/会员，不可用时自动降级进程内内存，核心功能不中断。
- **一键上云**：Docker Compose 编排全栈（6 服务 + 4 中间件），单机一条命令部署到阿里云。

## 🏗 系统架构

```
                         ┌─────────────────────────────┐
        TCP长连接          │         IMServer (C++)       │
  Qt客户端 ───────────────▶│  muduo 多Reactor + protobuf  │
   (9527)                 │  会话/业务/数据 三层          │
                         └───┬─────────┬────────┬───────┘
                  gRPC       │         │        │   AMQP(发布)
              ┌──────────────┘         │        └──────────────┐
              ▼                        ▼                       ▼
       ┌────────────┐          ┌────────────┐          ┌──────────────┐
       │  goagent   │          │GoImageServer│         │  RabbitMQ     │
       │ AI/RAG     │          │ 头像 (8080) │          │  im.events    │
       │(19527/19528)│         └────────────┘          └──────┬───────┘
       └─────┬──────┘          ┌────────────┐                 │ 消费
             │                 │GoMediaServer│         ┌──────▼───────┐
       ┌─────▼─────┐           │语音/视频(8090)│        │GoChatPersister│
       │Redis/Qdrant│          └────────────┘          │ 异步落库      │
       └───────────┘           ┌────────────┐          └──────┬───────┘
                               │GoMailServer │                 ▼
                               │验证码(gRPC) │            MySQL t_chatmsg
                               └────────────┘
```

| 服务 | 语言/框架 | 职责 |
|---|---|---|
| **IMServer** | C++ / muduo | TCP 长连接、多 Reactor、自定义协议、MySQL 连接池、消息转发与落库 |
| **goagent** | Go / gRPC+HTTP | AI 助手：LLM 对话 + RAG 记忆问答（Qdrant）+ Redis 缓存 + 配额限流 + VIP |
| **GoImageServer** | Go / gRPC+HTTP | 头像上传/存储（本地 / S3 对象存储）、图片缩放压缩 |
| **GoMediaServer** | Go / HTTP | 语音/视频媒体上传与存储 |
| **GoMailServer** | Go / gRPC | 邮箱验证码（SMTP，内存态 + TTL + 防爆破） |
| **GoChatPersister** | Go / amqp091 | RabbitMQ 消费者，聊天消息异步落库 |

## 🧰 技术栈

| 分层 | 技术 |
|---|---|
| **通讯核心** | C++17、muduo（Reactor）、Protocol Buffers、MySQL（连接池）、zlib(Adler32) |
| **微服务** | Go 1.25、gRPC、net/http、`log/slog`、依赖注入 |
| **AI / RAG** | LLM（OpenAI 兼容，DeepSeek）、Embedding（bge-m3）、Qdrant 向量库 |
| **中间件** | MySQL 8、Redis 7、RabbitMQ 3、Qdrant |
| **客户端** | Qt 6 / QML |
| **运维** | Docker、Docker Compose、阿里云 ECS / 轻量应用服务器 |

## 📦 功能清单

- **账号**：邮箱验证码注册、登录鉴权、改昵称/资料/密码、头像上传
- **社交**：加好友（申请-同意制）、好友列表、实时在线状态、删好友
- **群组**：建群、群成员、群聊、踢人、群改名、设/撤管理员
- **聊天**：单聊、群聊、历史记录、消息异步落库（MQ）
- **媒体**：语音、视频消息上传/播放
- **AI**：LLM 多轮对话、RAG 记忆问答、按日配额限流（普通/VIP）、模拟支付升级 VIP

## 🚀 快速开始

> 前提：目标主机已安装 **Docker + Docker Compose**（采用 host 网络，单机部署）。

```bash
git clone https://github.com/goodeverydays/AIM-Chat.git
cd AIM-Chat
cp .env.example .env
# 编辑 .env：填 PUBLIC_HOST(公网IP) / MYSQL_PASSWORD / DEEPSEEK_API_KEY / EMBED_* ；SMTP 可留空
docker compose up -d --build     # 首次 IMServer(C++) 镜像构建较慢，约 10min+
docker compose ps                # 全部 Up、mysql 显示 healthy 即部署成功
```

**验证服务就绪**：

```bash
ss -tln | grep -E ':9527|:8080|:8090'        # 三个对外端口在监听
curl -s localhost:8080/api/health            # 头像服务 {"healthy":true}
curl -s localhost:8090/api/health            # 媒体服务 {"healthy":true}
docker compose logs imserver | grep "IO threads"   # IMServer 多线程就绪
```

## 🔌 端口与客户端连接

| 端口 | 服务 | 谁连 | 是否对公网 |
|---|---|---|---|
| **9527**/tcp | IMServer | 客户端（登录/聊天/好友/头像上传） | ✅ 开放 |
| **8080**/tcp | 头像 HTTP | 客户端（头像显示下载） | ✅ 开放 |
| **8090**/tcp | 媒体 HTTP | 客户端（语音/视频上传下载） | ✅ 开放 |
| 19527/19528 | goagent gRPC/HTTP | IMServer 内部 | ❌ |
| 19529 / 19531 | 头像/邮件 gRPC | IMServer 内部 | ❌ |
| 3306 / 6379 / 5672 / 6333 / 15672 | MySQL/Redis/RabbitMQ/Qdrant | 仅本机 | ❌ |

**客户端配置**：Qt 客户端 `qml/main.qml` 的 `serverHost` 改为服务器公网 IP。
⚠️ 三处必须一致：`.env` 的 `PUBLIC_HOST`、客户端 `serverHost`、防火墙/安全组放行的目标 IP。

## 📁 项目结构

```
AIM-Chat/
├── docker-compose.yml          # 全栈编排（6 服务 + 4 中间件）
├── .env.example                # 配置模板（复制为 .env 填真实值）
├── IMServer/IMServer/          # C++ 核心（muduo + protobuf）
├── goagent/                    # AI 微服务（gRPC+HTTP / RAG / 缓存 / 限流）
├── GoImageServer/              # 头像服务
├── GoMediaServer/              # 媒体服务
├── GoMailServer/               # 邮箱验证码服务
└── GoChatPersister/            # 聊天异步落库消费者
```

各 Go 服务统一采用 `cmd/<svc>/main.go`（装配入口）+ `internal/`（config/server/service/storage 分层）。

## 🔒 安全须知

- 安全组/防火墙**只放行 `9527 / 8080 / 8090`**；数据库与中间件端口（`3306/6379/5672/6333/15672`）不暴露公网（代码已绑 `127.0.0.1`）。
- 云主机除云端安全组外，注意**系统内 `firewalld`** 也需放行上述端口（host 网络的 Docker 端口不会自动加入 firewalld）。

---

> 本项目为全栈技术实践，覆盖高性能网络编程、并发、消息队列、微服务、RAG/向量检索与容器化部署。
