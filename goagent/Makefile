.PHONY: all build clean proto run test

# 变量定义
PROTOC = D:/protobuf/bin/protoc.exe
GO_BIN = D:/Go/bin
PROTO_PATH = proto
BINARY_NAME = goagent
BUILD_DIR = build

# 默认目标
all: proto build

# 生成protobuf代码
proto:
	@echo ">>> 生成 protobuf 代码..."
	$(PROTOC) --proto_path=$(PROTO_PATH) \
		--go_out=. --go_opt=module=goagent \
		--go-grpc_out=. --go-grpc_opt=module=goagent \
		$(PROTO_PATH)/agent.proto
	@echo ">>> protobuf 代码生成完成"

# 编译
build:
	@echo ">>> 编译 goagent..."
	@if not exist $(BUILD_DIR) mkdir $(BUILD_DIR)
	go build -ldflags "-s -w -X main.Version=0.1.0" -o $(BUILD_DIR)/$(BINARY_NAME).exe ./cmd/agent
	@echo ">>> 编译完成: $(BUILD_DIR)/$(BINARY_NAME).exe"

# 运行
run:
	go run ./cmd/agent

# 清理
clean:
	@if exist $(BUILD_DIR) rmdir /s /q $(BUILD_DIR)
	@echo ">>> 清理完成"

# 测试
test:
	go test ./...
