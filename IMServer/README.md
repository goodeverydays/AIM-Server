# IMServer

基于 **muduo** 网络库的 C++ 即时通讯服务器。承担 IM 系统的核心职责：长连接管理、消息路由、好友/群组关系、离线消息缓存，并作为 gRPC 客户端对接周边 Go 微服务（智能助手、头像存储、邮箱验证码）。

配套客户端为 Qt6/QML 编写的 **IMClient**，压测工具为 Go 编写的 **IMStressTest**。

---

## 系统架构

整体采用 **「单体 IM 核心 + 旁路 gRPC 微服务」** 模式：核心连接与会话逻辑集中在 IMServer 单进程内，易变/独立的能力（AI 助手、图床、邮件）拆分为独立 Go 微服务，IMServer 以 gRPC 客户端身份调用。

```
┌──────────────────┐   TCP 9527 (protobuf 帧)   ┌─────────────────────────────┐
│  IMClient        │ ◄────────────────────────► │  IMServer (C++ / muduo)      │
│  (Qt6 / QML)     │   [len][IM01][pb][adler32] │  ClientSession (每连接)      │
└──────────────────┘                            │  UserManager / IMSer(单例)   │
                                                 │  MsgCacheManager(离线消息)   │
                                                 └───┬───────┬───────┬──────────┘
                                          gRPC 19527 │  19529 │ 19531 │
                                          ┌──────────▼┐ ┌─────▼────┐ ┌▼────────────┐
                                          │ goagent   │ │GoImage   │ │GoMailServer │
                                          │ 智能助手  │ │Server    │ │ 验证码/SMTP │
                                          │ (LLM)     │ │ 头像存储 │ │             │
                                          └───────────┘ └──────────┘ └─────────────┘
                                                 │ MySQL 3306 (myim)
                                          ┌──────▼──────────────────┐
                                          │ t_user / t_user_relation │
                                          └──────────────────────────┘
```

| 节点 | 技术栈 | 地址 | 职责 |
|------|--------|------|------|
| **IMServer** | C++ / muduo | `0.0.0.0:9527` | 核心 IM 逻辑、消息路由、关系管理 |
| **goagent** | Go / gRPC | `127.0.0.1:19527` | 智能助手（接入 LLM 后生成 AI 回复） |
| **GoImageServer** | Go / gRPC + HTTP | `127.0.0.1:19529`(gRPC) / `:8080`(HTTP) | 头像上传、存储、静态分发 |
| **GoMailServer** | Go / gRPC | `127.0.0.1:19531` | 邮箱验证码生成 + SMTP 发信 |
| **MySQL** | 8.0 | `127.0.0.1:3306` | 用户/关系持久化（库 `myim`） |

> 三个 Go 微服务与 IMServer 部署在同一台机器，gRPC 地址硬编码为 `127.0.0.1:<port>`，仅在 IMServer 以 `HAVE_AGENT_GRPC` 宏编译时启用。

---

## 项目结构

```
IMServer/
├── base/                   # 线程/同步/日志基础库（muduo fork）
├── net/                    # 事件驱动 TCP 网络库（muduo fork）
│   └── protobuf/           # ProtobufCodecLite —— protobuf 帧编解码
├── IMServer/               # 应用层：IM 业务逻辑
│   ├── IMServer.cpp        # 程序入口（初始化 MySQL、启动 IMSer）
│   ├── IMSer.h/cpp         # 聊天服务器（TcpServer 封装 + 持有 gRPC 客户端）
│   ├── ClientSession.h/cpp # 客户端会话：消息分发与各 cmd 处理
│   ├── UserManager.h/cpp   # 用户与群组管理（内存索引 + DB 读写）
│   ├── MsgCacheManager.h/cpp # 离线消息缓存
│   ├── IMCodec.h/cpp        # protobuf 编解码适配层（tag="IM01" + Adler32）
│   ├── MySqlManager.h/cpp   # MySQL 连接与建表
│   ├── AgentGrpcClient.*    # gRPC 客户端 → goagent（智能助手）
│   ├── AvatarGrpcClient.*   # gRPC 客户端 → GoImageServer（头像）
│   ├── MailGrpcClient.*     # gRPC 客户端 → GoMailServer（验证码）
│   ├── im.proto             # IM 主协议（C↔S）
│   └── agent/avatar/mail.proto # 三个微服务的 gRPC 协议
└── out/                    # CMake 构建输出
```

---

## 通信协议

### 客户端 ↔ 服务器（TCP, protobuf）

帧格式（由 `IMCodec` / `ProtobufCodecLite` 实现）：

```
┌──────────────┬────────────┬────────────────────┬──────────────────┐
│ total_len:4B │ tag:"IM01" │ protobuf body : N  │ Adler32 校验:4B  │
│   (大端)     │   (4B)     │ (MessageContainer) │    (大端)        │
└──────────────┴────────────┴────────────────────┴──────────────────┘
```

所有消息封装在 `MessageContainer{ cmd, seq, data }` 中，`cmd` 决定 `data` 的具体 protobuf 类型，`seq` 用于请求-响应匹配。

### 协议命令（`im.proto`）

| cmd | 方向 | 载荷 | 说明 |
|-----|------|------|------|
| 1000 | C→S | — | 心跳 |
| 1001 | C→S | RegisterReq / RegisterRsp | 注册（含邮箱+验证码校验） |
| 1002 | C→S | LoginReq / LoginRsp | 登录 |
| 1003 | C→S | FriendListRsp | 获取好友/群组列表 |
| 1004 | C→S | FindUserReq / FindUserRsp | 查找用户 |
| 1005 | C→S | OperateFriendReq / OperateFriendRsp | 加好友 / 删好友 / 邀请入群 |
| 1006 | S→C | UserStatusChangeNotify | 上线/离线/被加/被删/**头像更新**推送 |
| 1007 | C→S | UpdateUserInfoReq / CommonRsp | 更新个人信息 |
| 1008 | C→S | ModifyPasswordReq / CommonRsp | 修改密码 |
| 1009 | C→S | CreateGroupReq / CreateGroupRsp | 创建群组 |
| 1010 | C→S | GetGroupMembersReq / GetGroupMembersRsp | 获取群成员 |
| 1011 | C→S | GetChatHistory* | 获取聊天历史 |
| 1012 | C→S | 头像上传 | 经 AvatarGrpcClient 转发至 GoImageServer |
| 1013 | C→S | SendEmailCodeReq / SendEmailCodeRsp | 发送邮箱验证码（经 MailGrpcClient） |
| 1100 | C↔S | ChatMsg | 单聊消息 |
| 1101 | C→S | MultiChatTargets | 群发消息 |

---

## 主要功能

- **账号体系**：注册（邮箱验证码校验，经 GoMailServer）、登录、修改密码、更新个人资料
- **好友关系**：查找用户、加好友/删好友、双向状态变更通知（上线/离线/被加/被删）
- **群组**：创建群组、获取群成员、邀请入群、群消息转发（跳过发送者避免重复）
- **消息**：单聊、群聊、聊天历史查询、离线消息缓存与登录后推送
- **头像**：上传至 GoImageServer，并向在线好友广播头像更新（`UserStatusChangeNotify type=5`）
- **智能助手**：用户与特殊 ID（`-1`）对话，IMServer 经 gRPC 转发至 goagent

> goagent 接入真实 LLM（如 OpenAI 兼容接口 / DeepSeek / 通义千问）后即可提供 AI 问答与多轮对话；当前默认 EmptyProvider 仅返回占位文本。

---

## 数据库

MySQL 8.0，库名 `myim`，主要表：

- **`t_user`** — 用户与群组（群组复用本表，以 `f_owner_id` 区分）。字段含 `f_user_id, f_username, f_nickname, f_password, f_mail, f_register_time, f_gender, f_birthday, f_signature, f_address, f_phonenumber` 等。
- **`t_user_relationship`** — 好友 / 群成员关系（双向，按 small/great id 排序存储）。

UserManager 启动时将用户加载进内存双索引（`m_cachedUsers` 按名遍历 / `m_mapUsers` 按 ID 查找，二者共享同一 `shared_ptr<User>` 对象保证一致性），并从 `t_user_relationship` 还原好友集合。

---

## 构建

依赖：**MySQL 8.0**、**Protobuf**、**gRPC**（`libgrpc++-dev protobuf-compiler-grpc`，缺失则自动禁用微服务功能）、**zlib**、**jsoncpp**。

```bash
# Linux (x64 Debug)
cmake --preset linux-debug
cmake --build out/build/linux-debug

# Windows (x64 Debug, 需 Visual Studio 2022 + MySQL 8.0)
cmake --preset x64-debug
cmake --build out/build/x64-debug
```

> 修改任一 `.proto` 后需重新执行 cmake configure 以重新生成 pb/grpc 代码。

---

## 运行

```bash
./imchatserver        # 前台运行
./imchatserver -d     # 守护进程模式
```

- 监听 `0.0.0.0:9527`，连接 MySQL `127.0.0.1:3306`（库 `myim`，用户 `root`）。
- 启用 AI/头像/邮件功能需另行启动对应 Go 微服务（19527 / 19529 / 19531），并为 GoMailServer 配置 SMTP 环境变量。

---

## 压测（IMStressTest）

压测工具为独立 Go 程序（独立仓库 `IMStressTest`），利用 goroutine 轻量级并发产生压力。

> **注意**：压测工具内置 mock server 使用的是早期自定义二进制协议（`packagesize + bodySize + checksum + cmd/seq/data`，7bit 变长编码），与当前 IMServer 实际采用的 protobuf `IM01` 帧协议**不一致**。下表数据采集自 Go mock server，仅反映工具本身的吞吐上限；针对真实 C++ 服务器的压测需先对齐协议后重新执行。

### 命令行参数

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `-mode` | `bench` | `bench`(压测) / `server`(模拟服务器) |
| `-host` | `127.0.0.1` | 服务器地址 |
| `-port` | `9527` | 服务器端口 |
| `-c` | `100` | 并发连接数 |
| `-d` | `10` | 持续时间（秒） |
| `-s` | `all` | 场景：`connect`/`login`/`heartbeat`/`chat`/`mixed`/`all` |
| `-md` | `false` | 输出 Markdown 表格 |

```bash
cd IMStressTest && go build -o imstress.exe .
./imstress.exe -host 127.0.0.1 -port 9527 -c 100 -d 30 -s all -md
```

### 压测场景

| 场景 | 说明 | 测试目标 |
|------|------|----------|
| connect | 快速建立→关闭 TCP 连接 | accept 吞吐量 |
| login | 批量注册后并发登录 | 登录业务处理能力 |
| heartbeat | 长连接持续收发心跳 | 心跳处理吞吐量 |
| chat | 已登录用户互发消息 | 消息转发延迟/吞吐 |
| mixed | 40%心跳+30%登录+20%连接+10%聊天 | 混合真实负载 |

### 压测结果（Go mock server, 100 并发 × 10s）

| 场景 | 吞吐(req/s) | 平均延迟(ms) | P95(ms) | P99(ms) | 成功率 |
|------|-------------|-------------|---------|---------|--------|
| 连接 | 19134 | 5.17 | 8.18 | 10.25 | 2.36% |
| 登录 | 86935 | 0.47 | 1.42 | 1.77 | 99.93% |
| 心跳 | 321 | 0.02 | 0.00 | 0.58 | 100.00% |
| 单聊 | 19129 | 0.06 | 0.53 | 0.82 | 100.00% |
| 混合 | 9818 | 3.33 | 4.87 | 6.84 | 6.70% |

500 并发下登录仍达 **85,000+ req/s**（99.66% 成功率，P99 14.5ms）。连接场景在 mock 的单线程 accept 循环下成为瓶颈；真实服务器基于 muduo EventLoopThreadPool 多线程 accept，预期显著优于该数据。

### 压测环境

| 项目 | 配置 |
|------|------|
| CPU | Intel Core i7-13620H (16 核) |
| 内存 | 32 GB |
| OS | Windows 11（工具）+ WSL Ubuntu 24.04（服务器） |
| Go | 1.26.3 |
| MySQL | 8.0.44 |

---

## 已知限制与待增强

- **goagent 未接入真实 LLM**：当前返回占位文本，需实现并配置 LLM Provider。
- **已读回执 / 文件传输**：尚未实现。
- **群成员管理**：仅支持创建时设定 + 邀请入群，缺少踢人/退群/权限。
- **优雅退出与缓存定时落盘**：`IMServer` 信号处理、`MsgCacheManager` 定时持久化标记为 TODO。
