# IMServer —— C++/muduo IM 核心服务(muduo 源码内置于仓库)
# 用 Ubuntu 24.04:自带较新的 gRPC 1.51/protobuf(无 22.04 gRPC 1.30 的 grpc_impl 头冲突)。
# 单阶段构建:运行所需的 protobuf/grpc/mysql/ssl 等库已在镜像内,免去多阶段运行库包名匹配的坑。
FROM ubuntu:24.04
ENV DEBIAN_FRONTEND=noninteractive TZ=Asia/Shanghai
RUN apt-get update && apt-get install -y --no-install-recommends \
        g++ cmake make git pkg-config \
        libboost-dev \
        libprotobuf-dev protobuf-compiler protobuf-compiler-grpc \
        libgrpc++-dev \
        librabbitmq-dev \
        default-libmysqlclient-dev \
        zlib1g-dev libssl-dev ca-certificates tzdata \
    && rm -rf /var/lib/apt/lists/*

# CMakeLists 把 MySQL 库路径硬编码成 CentOS 风格 /usr/lib64/mysql/，
# Ubuntu 实际是 libmysqlclient.so 或 libmariadb.so，软链兼容(mariadb 与 mysql 客户端 ABI 互通)。
RUN mkdir -p /usr/lib64/mysql && \
    L="$(find /usr/lib -name 'libmysqlclient.so' -o -name 'libmariadb.so' 2>/dev/null | head -1)" && \
    ln -sf "$L" /usr/lib64/mysql/libmysqlclient.so

WORKDIR /src
COPY . .
# 仅在容器内把 C++ 标准提到 c++17(适配新版 gRPC/abseil);不动仓库 CMakeLists，VM 构建不受影响。
RUN sed -i 's/-std=c++0x/-std=c++17/' CMakeLists.txt && \
    cmake -S . -B build -DCMAKE_BUILD_TYPE=Release && \
    cmake --build build --target imchatserver -j"$(nproc)"

EXPOSE 9527
ENTRYPOINT ["/src/build/imchatserver"]
