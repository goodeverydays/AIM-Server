# CMakeList.txt: IMServer 的 CMake 项目，在此处包括源代码并定义
# 项目特定的逻辑。
#

# 将源代码添加到此项目的可执行文件。

#if (CMAKE_VERSION VERSION_GREATER 3.12)
 # set_property(TARGET IMServer PROPERTY CXX_STANDARD 20)
#endif()
cmake_minimum_required (VERSION 3.16)

project(IMServer)

#这个宏会影响编译参数
set(CMAKE_CXX_FLAGS "${CMAKE_CXX_FLAGS} -std=c++0x -g -Wall -O0 -Wno-unused-variable -pthread")

Link_directories(#这个是针对64位的
	${PROJECT_SOURCE_DIR}/lib
	/usr/lib64/mysql/
)

find_library(MYSQL_LIB libmysqlclient.so /usr/lib64/mysql/)
if(NOT MYSQL_LIB)
	message(FATAL_ERROR "not found mysqlclient.so at /usr/lib64/mysql/")
endif(NOT MYSQL_LIB)#必须进行匹配

# --- Protobuf 依赖（IM协议编解码）---
find_package(Protobuf REQUIRED)

# 编译 .proto 文件生成 C++ 代码
protobuf_generate_cpp(PROTO_SRCS PROTO_HDRS ${CMAKE_CURRENT_SOURCE_DIR}/im.proto)

# --- gRPC 依赖（Agent/Avatar/Mail 客户端）---
# 优先 CMake CONFIG 模式(源码安装的 gRPC)；失败则回退 pkg-config + 系统插件。
# Ubuntu 的 libgrpc++-dev 不带 CMake config，必须靠此回退，否则 gRPC 客户端被禁用。
set(HAVE_GRPC FALSE)
find_package(gRPC CONFIG QUIET)
if(gRPC_FOUND)
    get_target_property(GRPC_CPP_PLUGIN gRPC::grpc_cpp_plugin LOCATION)
    set(GRPC_LINK_LIBS gRPC::grpc++ gRPC::grpc++_reflection)
    set(HAVE_GRPC TRUE)
    message(STATUS "gRPC found via CMake CONFIG")
else()
    find_package(PkgConfig QUIET)
    find_program(GRPC_CPP_PLUGIN grpc_cpp_plugin)
    if(PkgConfig_FOUND AND GRPC_CPP_PLUGIN)
        pkg_check_modules(GRPCPP grpc++)
        if(GRPCPP_FOUND)
            link_directories(${GRPCPP_LIBRARY_DIRS})
            set(GRPC_LINK_LIBS ${GRPCPP_LIBRARIES})
            set(HAVE_GRPC TRUE)
            message(STATUS "gRPC found via pkg-config (plugin: ${GRPC_CPP_PLUGIN})")
        endif()
    endif()
endif()

if(HAVE_GRPC)
    message(STATUS "gRPC enabled, building Agent/Avatar/Mail clients")

    # ── 自定义函数：生成 gRPC C++ 代码 ──
    # protobuf_generate_grpc_cpp 不是 CMake 内置函数，需手动定义
    function(protobuf_generate_grpc_cpp SRCS HDRS PROTO)
        get_filename_component(PROTO_DIR ${PROTO} DIRECTORY)
        get_filename_component(PROTO_NAME ${PROTO} NAME_WE)
        set(GRPC_SRC ${CMAKE_CURRENT_BINARY_DIR}/${PROTO_NAME}.grpc.pb.cc)
        set(GRPC_HDR ${CMAKE_CURRENT_BINARY_DIR}/${PROTO_NAME}.grpc.pb.h)
        add_custom_command(
            OUTPUT ${GRPC_SRC} ${GRPC_HDR}
            COMMAND ${Protobuf_PROTOC_EXECUTABLE}
                --proto_path=${PROTO_DIR}
                --grpc_out=${CMAKE_CURRENT_BINARY_DIR}
                --plugin=protoc-gen-grpc=${GRPC_CPP_PLUGIN}
                ${PROTO}
            DEPENDS ${PROTO}
            COMMENT "Generating gRPC code for ${PROTO_NAME}"
        )
        set(${SRCS} ${GRPC_SRC} PARENT_SCOPE)
        set(${HDRS} ${GRPC_HDR} PARENT_SCOPE)
    endfunction()

    # agent.proto
    protobuf_generate_cpp(AGENT_PROTO_SRCS AGENT_PROTO_HDRS ${CMAKE_CURRENT_SOURCE_DIR}/agent.proto)
    protobuf_generate_grpc_cpp(AGENT_GRPC_SRCS AGENT_GRPC_HDRS ${CMAKE_CURRENT_SOURCE_DIR}/agent.proto)

    # avatar.proto
    protobuf_generate_cpp(AVATAR_PROTO_SRCS AVATAR_PROTO_HDRS ${CMAKE_CURRENT_SOURCE_DIR}/avatar.proto)
    protobuf_generate_grpc_cpp(AVATAR_GRPC_SRCS AVATAR_GRPC_HDRS ${CMAKE_CURRENT_SOURCE_DIR}/avatar.proto)

    # mail.proto
    protobuf_generate_cpp(MAIL_PROTO_SRCS MAIL_PROTO_HDRS ${CMAKE_CURRENT_SOURCE_DIR}/mail.proto)
    protobuf_generate_grpc_cpp(MAIL_GRPC_SRCS MAIL_GRPC_HDRS ${CMAKE_CURRENT_SOURCE_DIR}/mail.proto)

    set(AGENT_SRCS
        AgentGrpcClient.cpp
        AvatarGrpcClient.cpp
        MailGrpcClient.cpp
        ${AGENT_PROTO_SRCS}
        ${AGENT_GRPC_SRCS}
        ${AVATAR_PROTO_SRCS}
        ${AVATAR_GRPC_SRCS}
        ${MAIL_PROTO_SRCS}
        ${MAIL_GRPC_SRCS}
    )
    add_definitions(-DHAVE_AGENT_GRPC)
else()
    message(WARNING "gRPC not found, gRPC clients disabled. "
                    "Install: apt install libgrpc++-dev protobuf-compiler-grpc")
    set(AGENT_SRCS "")
endif()  # HAVE_GRPC

include_directories(. ./base ./net ./third_party/jsoncpp/include ${CMAKE_CURRENT_BINARY_DIR})

set(muduo_srcs
base/Timestamp.cc
base/TimeZone.cc
base/Date.cc
base/CountDownLatch.cc
base/FileUtil.cc
base/Logging.cc
base/LogFile.cc
base/LogStream.cc
base/AsyncLogging.cc
base/Thread.cc
base/CurrentThread.cc
base/ProcessInfo.cc
"base/Condition.cc"


net/Acceptor.cc
net/Buffer.cc
net/Channel.cc
net/Connector.cc
net/Poller.cc
net/poller/DefaultPoller.cc
net/poller/EPollPoller.cc
net/poller/PollPoller.cc
net/InetAddress.cc
net/Socket.cc
net/SocketsOps.cc
net/TcpClient.cc
net/TcpConnection.cc
net/TcpServer.cc
net/EventLoop.cc
net/EventLoopThread.cc
net/EventLoopThreadPool.cc
net/Timer.cc
net/TimerQueue.cc
)

set(mysql_srcs
mysqltool.cpp
Field.cpp
QueryResult.cpp
MySqlManager.cpp

)

# --- RabbitMQ 依赖(聊天消息异步落库,可选)---
find_library(RABBITMQ_LIB rabbitmq)
find_path(RABBITMQ_INC amqp.h)
if(RABBITMQ_LIB AND RABBITMQ_INC)
    message(STATUS "RabbitMQ (rabbitmq-c) found, enabling MQ publisher")
    add_definitions(-DHAVE_RABBITMQ)
    include_directories(${RABBITMQ_INC})
    set(RABBITMQ_SRCS RabbitMqPublisher.cpp)
    set(RABBITMQ_LINK ${RABBITMQ_LIB})
else()
    message(WARNING "rabbitmq-c not found, MQ publisher disabled. Install: apt install librabbitmq-dev")
    set(RABBITMQ_SRCS "")
    set(RABBITMQ_LINK "")
endif()

set(chat_srcs
IMServer.cpp
IMSer.cpp
ClientSession.cpp
UserManager.cpp
MsgCacheManager.cpp
IMCodec.cpp                # protobuf编解码适配层
net/protobuf/ProtobufCodecLite.cc
${PROTO_SRCS}              # protoc生成的 im.pb.cc
${AGENT_SRCS}              # Agent gRPC 客户端
${RABBITMQ_SRCS}          # RabbitMQ 发布者(可选)
)

set(file_srcs
    IMTestClient.cpp
)

# jsoncpp 第三方库
add_subdirectory(third_party/jsoncpp)

add_executable (imchatserver ${chat_srcs} ${muduo_srcs} ${mysql_srcs})
#只是确认库存在或者设置库目录，是没有用的，需要链接进来才能起效果

# 链接 gRPC 库 + RabbitMQ(可选)
if(HAVE_GRPC)
    target_link_libraries(imchatserver mysqlclient jsoncpp ${Protobuf_LIBRARIES} z ${GRPC_LINK_LIBS} ${RABBITMQ_LINK})
else()
    target_link_libraries(imchatserver mysqlclient jsoncpp ${Protobuf_LIBRARIES} z ${RABBITMQ_LINK})
endif()

add_executable (imfileserver ${file_srcs} ${muduo_srcs})
#只是确认库存在或者设置库目录，是没有用的，需要链接进来才能起效果
target_link_libraries(imfileserver)
# TODO: 如有需要，请添加测试并安装目标。
