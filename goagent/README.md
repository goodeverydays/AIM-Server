# goagent — IM Agent 微服务

为 IM 系统提供智能 Agent 能力的微服务，通过 gRPC 暴露接口。

## 架构

```
IMServer (C++)  ──gRPC──►  goagent (Go)  ──API──►  LLM Provider (OpenAI/Anthropic/...)
```

## 依赖

- Go 1.22+
- protoc + protoc-gen-go + protoc-gen-go-grpc

## 构建 & 运行

```bash
# 生成 protobuf 代码
make proto

# 编译
make build

# 运行
make run

# 或直接运行编译产物
./build/goagent.exe
```

## 配置

通过环境变量配置：

| 环境变量 | 说明 | 默认值 |
|---|---|---|
| `AGENT_HOST` | 监听地址 | `0.0.0.0` |
| `AGENT_PORT` | 监听端口 | `19527` |
| `LLM_PROVIDER` | LLM提供商 | `empty` |
| `OPENAI_API_KEY` | OpenAI API Key | - |
| `OPENAI_BASE_URL` | OpenAI Base URL | - |
| `OPENAI_MODEL` | OpenAI 模型名 | - |
| `ANTHROPIC_API_KEY` | Anthropic API Key | - |
| `ANTHROPIC_MODEL` | Anthropic 模型名 | - |
| `LOG_LEVEL` | 日志级别 | `info` |
| `LOG_FORMAT` | 日志格式 | `text` |

## gRPC 接口

```protobuf
service AgentService {
  rpc ProcessMessage (ProcessMessageReq) returns (ProcessMessageRsp);
  rpc HealthCheck    (HealthCheckReq)    returns (HealthCheckRsp);
  rpc ListModels     (ListModelsReq)     returns (ListModelsRsp);
}
```

端口：`19527`

## 调试

```bash
# 使用 grpcurl 测试
grpcurl -plaintext localhost:19527 list
grpcurl -plaintext localhost:19527 agent.AgentService/HealthCheck
```

## 扩展 LLM Provider

1. 在 `internal/llm/` 下新建文件（如 `openai_provider.go`）
2. 实现 `Provider` 接口
3. 在 `cmd/agent/main.go` 的 `createProvider()` 中添加分支
