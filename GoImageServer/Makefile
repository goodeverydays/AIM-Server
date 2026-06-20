.PHONY: all build clean proto run

BINARY_NAME=avatarserver
BUILD_DIR=build

all: proto build

proto:
	@echo ">>> 生成 protobuf 代码..."
	D:/protobuf/bin/protoc.exe --proto_path=proto \
		--go_out=. --go_opt=module=GoImageServer \
		--go-grpc_out=. --go-grpc_opt=module=GoImageServer \
		proto/avatar.proto

build:
	@echo ">>> 编译..."
	@if not exist $(BUILD_DIR) mkdir $(BUILD_DIR)
	go build -o $(BUILD_DIR)/$(BINARY_NAME).exe ./cmd/avatarserver

run:
	go run ./cmd/avatarserver

clean:
	@if exist $(BUILD_DIR) rmdir /s /q $(BUILD_DIR)
