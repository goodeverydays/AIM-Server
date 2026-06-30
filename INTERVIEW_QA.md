# 简历面试深挖问答（面试官视角）

> 本文档以**面试官视角**对该简历逐项追问，给出参考答案，并标注答案对应的**代码位置**（`文件:行号`）。
> 路径基准：IMServer 源码在 `IMServer/IMServer/`，Go 服务在各自目录；分布式项目在 `distributed-system` 仓库。
> 🔴 标记为「诚实性红线」的题，是简历措辞容易被戳穿的地方，**必须提前按下面的诚实版准备**，否则一深挖就露馅、扣信任分。

---

## 一、专业技能区（会被挑着深挖）

### C++

**Q：虚函数怎么实现的？虚表在什么时候建立？**
A：每个有虚函数的类有一张虚函数表(vtable)，对象头部有个 vptr 指向它；调用虚函数时通过 vptr→vtable→函数地址做动态分派。vtable 在编译期为每个类生成，vptr 在对象构造时由编译器插入代码赋值（基类构造完→派生类构造时改写）。
📍 项目里多态用得不重，老实说"项目主要用 muduo 的回调而非继承多态"，别硬编。

**Q：`vector` 扩容机制？`map` 底层？**
A：`vector` 连续内存，满了按 1.5x/2x 扩容、搬迁元素（迭代器失效）；`std::map` 是红黑树（有序、O(logN)），`unordered_map` 是哈希表（均摊 O(1)）。
📍 项目中 `UserManager` 用 `std::map<int32_t, UserPtr>` 存用户、`std::set` 存好友关系：`UserManager.cpp`。

**Q：智能指针怎么用的？shared_ptr 线程安全吗？**
A：`shared_ptr` 引用计数管理生命周期；**控制块的引用计数是原子的（线程安全），但指向的对象本身不是**。项目里 UserManager 读用户返回 `make_shared<User>(*it)` 深拷贝，正是因为多线程共享同一对象不安全。
📍 `UserManager.cpp:213-221`（`GetUserByID` 返回深拷贝快照）。

### Go

**Q：Goroutine 和线程区别？GMP 模型？**
A：goroutine 是用户态轻量协程（初始 2KB 栈、可增长），由 Go runtime 调度到少量 OS 线程上。GMP：G=goroutine，M=OS 线程，P=逻辑处理器（持有可运行 G 队列）；M 必须绑定 P 才能跑 G，P 的数量默认=CPU 核数。
📍 项目里每个微服务用 goroutine 跑后台任务，如验证码清理协程 `GoMailServer/internal/store/codestore.go:110`（`StartCleanup`）。

**Q：channel 用在哪？怎么避免 goroutine 泄漏？**
A：用 `context.Context` + `select{ case <-ctx.Done() }` 控制后台 goroutine 生命周期，主程序退出 cancel，协程收到信号退出。
📍 验证码清理协程靠 ctx 退出：`GoMailServer/internal/store/codestore.go:110-128`；各服务 main 的优雅退出：`goagent/cmd/agent/main.go`。

### 网络（简历写了"IO多路复用/三握四挥/拥塞控制"——⚠️ 这里措辞混了概念，见红线题）

**Q：epoll 和 select/poll 区别？**
A：select/poll 每次调用要把全部 fd 拷到内核并轮询，O(n)；epoll 用红黑树管理 fd、就绪事件用回调放进就绪链表，`epoll_wait` 只返回就绪的，O(1) 拿就绪集，适合海量连接。muduo 底层就是 epoll。
📍 项目用 muduo 的 Reactor（epoll）：`IMSer.cpp:9-50`（`setThreadNum` 多 Reactor）。

**Q：TCP 三次握手为什么不是两次？四次挥手为什么 TIME_WAIT？**
A：两次握手无法确认"客户端的接收能力"和防止旧连接请求；TIME_WAIT(2MSL) 保证最后的 ACK 能重传、让旧报文在网络中消亡。压测 connect 场景成功率低就是 TIME_WAIT 堆积耗尽端口造成的（实测踩过）。

### 操作系统

**Q：死锁四个必要条件？项目里怎么避免的？**
A：互斥、占有且等待、不可抢占、循环等待。项目里 MySQL 连接池用 `mutex+condition_variable`，加锁顺序统一、临界区短、acquire 拿不到就 wait 让出锁，避免循环等待。
📍 `MySqlManager.cpp:153-170`（`acquire`/`release`）。

---

## 二、IM 项目深挖（★重头戏）

### 2.1 网络与协议

**Q：你的多 Reactor 是怎么做的？同一连接会跨线程吗？**
A：muduo one-loop-per-thread，`setThreadNum(N)` 开 N 个 IO 线程各一个 EventLoop；同一 `TcpConnection` 的所有回调固定在它所属线程串行执行，**不跨线程**，所以 ClientSession 内部状态无需加锁，只有跨连接共享的全局数据才要锁。
📍 `IMSer.cpp:9-50`（`init`，第 20-22 行 `setThreadNum`）；`ClientSession.cpp:80-163`（`OnMessageReceived` 分发）。

**🔴 Q：你说"自定义二进制协议解决粘包/拆包"——粘包切包逻辑是你自己写的吗？**
A（诚实版）：**传输层的粘包处理和帧校验用的是 muduo 的 `ProtobufCodecLite`**（陈硕的实现），不是我手写的。**我自己设计的是应用层协议**——用 `MessageContainer` 信封统一承载 cmd/seq/target_id/payload，seq 做请求-响应配对。**Qt 客户端用不了 muduo，所以我按同一帧格式（IM01+Protobuf+Adler32）自己实现了一遍编解码**，保证两端一致。
📍 服务端封装：`IMCodec.h:2`（注释"封装 muduo::net::ProtobufCodecLite"）、`IMCodec.cpp:15`（tag="IM01"）；muduo 原件：`net/protobuf/ProtobufCodecLite.h`（`Copyright Shuo Chen`）；**客户端自实现**：`IMClient/src/network/ImCodec.cpp`（`encode` 手写帧）。
> ⚠️ 千万别说"粘包是我写的"，会和"基于 muduo"矛盾。

**Q：seq 请求-响应怎么配对？一条连接上能并发多个请求吗？**
A：每个请求带自增 seq，服务端响应回填同一 seq，客户端用 seq 匹配回调。这样单条长连接上能并发多个在途请求（多路复用）。
📍 客户端 `IMClient/src/network/ImClient.cpp:257`（`sendRequest`，注册 `m_pendingRequests[seq]`）。

**Q：消息怎么从 A 转发到 B？B 不在线呢？**
A：A 发 chat(1100)→服务端 `OnChatResponse` 取 target_id→`GetSessionByID` 查 B 的在线会话→在线则直接 `SendContainer` 推送；不在线则落库，B 上线后拉历史。
📍 `ClientSession.cpp:896-1043`（`OnChatResponse`）；`IMSer.cpp:82-93`（`GetSessionByID`）。

### 2.2 并发安全与连接池

**Q：多 IO 线程访问同一份用户数据，怎么保证安全？**
A：读方法返回 `make_shared<User>(*it)` 深拷贝快照（读到副本不怕被改），写方法（如改在线状态）在 mutex 锁内原地改，会话表增删查加独立锁。竞争点消除后才敢默认开 4 线程。
📍 `UserManager.cpp:213-221`（`GetUserByID` 快照）、`222-230`（`SetUserStatus` 锁内写）。

**Q：为什么要 MySQL 连接池？Query 完为什么能立刻还连接？**
A：单连接非线程安全，多 IO 线程并发会串话；连接池给每个并发分配独立连接，池满则 `condition_variable` 阻塞等待。能立刻还是因为用 `mysql_store_result` 把结果集全量拷到客户端内存了，后续遍历不依赖连接。
📍 `MySqlManager.cpp:100-152`（`Init` 建池）、`153-170`（acquire/release）、`171-180`（Query）。

### 2.3 RabbitMQ

**Q：为什么聊天要过 MQ？MQ 挂了消息会丢吗？**
A：把落库从转发热路径剥离，削峰解耦；IMServer 发完 MQ 就返回，由 GoChatPersister 异步落库。**MQ 发布失败会回退同步写库**，所以即使 MQ 全挂消息也不丢，只是失去异步好处。
📍 发布+回退：`ClientSession.cpp:896-1043`（`OnChatResponse` 内 `#ifdef HAVE_RABBITMQ`，失败回退 `SaveChatMsgToDb`）；生产者：`RabbitMqPublisher.cpp:76-100`（`Publish`）；回退落库：`UserManager.cpp:449-471`。

**Q：消费端怎么保证不丢不重？什么是毒丸消息？**
A：手动 ack——落库成功才 Ack，失败 Nack 重入队，崩溃未 ack 的 broker 重投（至少一次）。毒丸=永远处理不了的坏消息（如坏 JSON），对它 `Nack(requeue=false)` 丢弃避免无限重投；只对 DB 临时失败 `Nack(requeue=true)` 重试。配合落库幂等去重实现"有效一次"。
📍 `GoChatPersister/internal/consumer/consumer.go:81-137`（`consumeOnce`），ack 策略在 `126/131/134` 行。

### 2.4 gRPC 微服务

**Q：为什么内部用 gRPC，对客户端用 HTTP？**
A：gRPC=HTTP/2+protobuf+强类型 IDL，跨语言（C++↔Go）契约清晰、性能高，适合内部服务间；对 Qt 客户端用 HTTP/JSON 更通用轻量。
📍 IMServer 作 gRPC 客户端：`IMServer/IMServer/AgentGrpcClient.cpp`；服务端 RPC：`goagent/internal/service/agent_service.go:38-118`（`ProcessMessage`）；HTTP 入口：`goagent/internal/server/http.go`。

### 2.5 RAG 记忆问答

**Q：RAG 是什么？为什么需要它？embedding 怎么找"相关"？**
A：检索增强生成——LLM 不知道用户私有聊天记录，先从历史消息里向量检索相关片段作上下文再喂 LLM。embedding 把文本转向量，用余弦相似度取 top-k 最相关的。
📍 入口：`agent_service.go:79-101`（RAG 分支）；检索：`goagent/internal/rag/retriever.go:116-150`（`Retrieve`）；向量化：`rag/embedder.go:63-108`。

**Q：每次都把全部历史重新 embedding 吗？多用户怎么隔离？**
A：增量索引——先批量查向量库哪些已索引，只对新消息 embedding；多用户靠 Qdrant 的 payload `owner` 字段过滤，只检索自己的记录。
📍 增量：`retriever.go:44-113`（`EnsureIndexed`，`HasIDs` 查重）；隔离：`rag/qdrant_store.go`（`TopK` 带 owner 过滤、`pointID` 确定性 UUID）。

### 2.6 Redis 限流与降级

**Q：限流怎么做的？Redis 挂了服务还能用吗？**
A：基于 Redis 的按日计数（`IncrWithTTL`），普通/VIP 不同额度；Redis 连接失败时降级到进程内 MemoryCache，核心 AI 回复不中断（优雅降级）。
📍 降级：`goagent/cmd/agent/main.go:36-50`；限流：`goagent/internal/ratelimit/limiter.go:76`（`Allow`）；计数：`cache/redis.go:133`（`IncrWithTTL`）。

### 2.7 性能与压测

**Q：你这 5 万 QPS 怎么测的？测试环境？**
A：自研 Go 压测工具，还原 IM01+Protobuf 二进制协议建大量长连接发心跳测往返。环境是**单机 2核2G，压测器和全部服务共部署**，200 并发吞吐峰值 5.4 万 QPS、P99<20ms、100% 成功；1000 并发仍 100% 成功（吞吐在 200 就被 2 核 CPU 饱和，再加并发只升延迟不升吞吐）。
📍 压测工具：`IMStressTest/`（`protocol.go` 复刻协议、`scenarios.go` 心跳/单聊场景、`client.go` 连接）。

---

## 三、分布式系统框架项目深挖

**Q：你的服务注册发现是 Push 还是 Pull？区别？**
A：Push——注册中心在服务上下线时**主动通知**依赖方更新，而不是客户端定时轮询(Pull)，省去无效轮询、变更更及时。
📍 `distributed-system`：`registry/server.go`（注册中心 + 通知）、`registry/client.go:14`（`RegisterService`）。

**Q："io.Writer 透明转发日志"具体怎么实现的？为什么零侵入？**
A：让日志类型实现 `io.Writer` 接口（`Write([]byte)`），把它设为标准库 `log` 的输出目标，业务代码照常 `log.Print`，输出就被透明转发到远程日志服务——业务无感知，所以零侵入。
📍 `distributed-system`：`log/server.go:14`（`fileLog.Write` 实现 io.Writer）、`log/client.go`（转发）。

**Q：context 怎么做优雅关闭？**
A：服务启动接收 ctx，监听取消信号；收到关闭信号 cancel ctx，各组件（HTTP server、注册注销）随 ctx 链式退出。
📍 `distributed-system`：`service/service.go:11`（`Start(ctx,...)`）。

**Q（追问）：这个框架和你 IM 项目的微服务有什么区别？**
A：诚实答——这是个**学习性框架**（HTTP+JSON，偏教学），实现了注册发现/日志收集的核心骨架；IM 项目是更完整的生产形态（gRPC+protobuf+中间件）。两者体现我对"服务注册发现、可观测性"从原理到落地的理解。

---

## 四、🔴 诚实性红线题（必须提前准备，否则翻车）

| 简历写法 | 风险 | 诚实答法 |
|---|---|---|
| "自定义二进制协议解决粘包" | 粘包其实是 muduo ProtobufCodecLite 做的 | "粘包用 muduo 的 codec；我设计的是应用层信封+命令体系，客户端按同格式自实现编解码"（见 2.1） |
| "模拟支付升级 VIP" | 面试官问"对接了什么支付？" | **主动说是模拟**：校验金额+token 非空即视为成功，预留了对接真实网关的位置。📍 `agent_service.go:123-152`（`UpgradeVip` 注释"模拟支付"） |
| "熟悉 claude code 等 AI 编程工具" | 怀疑"项目是 AI 写的"，深挖你不懂的细节 | 别强调这条；被问就答"用 AI 提效，但架构设计/选型/调试是我主导"，然后用你能讲清的代码位置证明（本文档就是弹药） |
| "5 万 QPS" | "什么环境？" | 主动说**2核2G 同机共部署**的实测，别让人以为是独立高配机（见 2.7） |
| "个人开发者 / 工程师"角色 | 个人项目写职位像夸大 | 大方说是个人项目/独立开发 |
| 技能"IO多路复用，如三握四挥、拥塞控制" | **概念混了**：三握四挥/拥塞控制是 TCP，不是 IO 多路复用 | 简历改：分开写"熟悉 TCP/IP(三次握手/拥塞控制)"和"掌握 I/O 多路复用(epoll)" |

---

## 五、系统设计 / 扩展题（考判断力）

**Q：这套系统怎么扩展到百万连接 / 多机？**
A：① IMServer 前加接入网关，按用户哈希把连接固定到某节点；② 在线状态/路由从单机内存挪到 Redis 集群，跨节点投递走 MQ/RPC；③ Go 微服务无状态直接多副本+LB；④ MySQL 分库分表+读写分离；⑤ 加注册发现、链路追踪、Prometheus 监控。

**Q：你觉得这个项目还有什么不足 / 怎么改进？（高频，考反思）**
A：① 鉴权目前是密码登录+连接绑定会话，可加 JWT+Redis 黑名单支持多端互踢/强制下线；② goagent 的 HTTP 接口暂无鉴权，靠内网隔离，应补；③ 缺可观测性（Prometheus+Grafana+链路追踪）；④ 消息可加序列号做客户端去重和断线补偿；⑤ 单机部署，在线表外置 Redis 才能多机。
> 这些都是真实可落地的点，**不堆砌大词**，体现你清楚自己项目的边界。

**Q：如果让你优化 IMServer 的延迟，你会从哪查？**
A：先分层定位——网络层(抓包看 RTT)、解码层、业务层(查 UserManager 锁竞争/DB 慢查询)、回包层；用压测工具复现，看 P99 在哪个阶段堆积。我压测时就观察到 2 核 CPU 饱和后延迟从 3.6ms 升到 19.5ms，说明瓶颈在 CPU 而非逻辑。

---

> 用法：面试前通读本文档，对每个「📍代码位置」打开代码确认自己能讲出"这段干嘛、为什么、有什么坑"。红线题（第四章）反复演练，确保表述诚实一致——**诚实 + 能讲到代码行 = 面试官信任**。
