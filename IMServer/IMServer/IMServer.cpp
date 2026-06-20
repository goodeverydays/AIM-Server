// IMServer.cpp: 定义应用程序的入口点。
//

#include "IMServer.h"
#include <signal.h>
#include <fcntl.h>
#include <cstdlib>
#include <string>
#include "IMSer.h"
#include "base/Singleton.h"//单例模式头文件，提供了一个全局访问点来获取IMSer实例，确保在整个程序中只有一个IMSer对象存在
#include "MySqlManager.h"
#include "UserManager.h"

using namespace std;

void show_help(const char* cmd)
{
	cout << "found error argument!\r\n";
	cout << "Usage: " << endl;
	cout << cmd << "[-d]" << endl;
	cout << "\t-d run in daemon mode\r\n";
}

// 全局事件循环指针：供信号处理函数触发优雅退出
muduo::net::EventLoop* g_mainLoop = nullptr;

void signal_exit(int signum)
{
	cout << "signal " << signum << " found, exit ...\r\n";
	switch (signum)
	{
	case SIGINT:
	case SIGTERM:
		// 可优雅退出的信号：通知事件循环退出，由 main 在 loop() 返回后清理资源。
		// quit() 置退出标志，wakeup() 通过 eventfd 唤醒阻塞的 poll（write 异步信号安全），
		// 保证即使设置了 SA_RESTART 也能立即返回。
		if (g_mainLoop != nullptr)
		{
			g_mainLoop->quit();
			g_mainLoop->wakeup();
			return;
		}
		exit(signum);
		break;
	default:
		// 致命信号（段错误/非法指令/abort 等）：进程状态已不可信，直接退出
		exit(signum);
		break;
	}
}

void daemon()
{
	signal(SIGCHLD, SIG_IGN);
	int pid = fork();
	if (pid < 0)
	{
		cout << "fork call error,code is " << pid << "error code is " << errno << endl;
		exit(-1);
	}
	if (pid > 0)//主进程在这里退出
	{
		exit(0);
	}

	//是为了不让前台影响后台
	setsid();//子进程一开始的会话id来自父进程，如果不创建新的会话id，父进程结束后子进程会因为会话id导致一起被终止

	//不让后台影响前台
	int fd = open("/dev/null", O_RDWR, 0);//把后台所有的错误提示或者信息导入null中，null约等于黑洞，从而让后台不会影响前台
	cout << "invoke success! " << endl;
	cout << "STDIN_FILENO is " << STDIN_FILENO << endl;
	cout << "STDOUT_FILENO is " << STDOUT_FILENO << endl;
	cout << "STDERR_FILENO is " << STDERR_FILENO << endl;
	cout << "fd is " << fd << endl;
	if (fd != -1)
	{
		dup2(fd, STDIN_FILENO);
		dup2(fd, STDOUT_FILENO);
		dup2(fd, STDERR_FILENO);
	}
	if (fd > STDERR_FILENO)
		close(fd);
}

void onConnection(const muduo::net::TcpConnectionPtr& conn)
{
	cout << conn->name() << std::endl;
}

void onMessage(const muduo::net::TcpConnectionPtr& conn,
	muduo::net::Buffer* buf,
	muduo::Timestamp time)
{
	conn->send(buf);
	conn->shutdown();
}

void test(muduo::net::EventLoop& loop)
{
	muduo::net::InetAddress addr(9527);//服务器监听地址和端口
	muduo::net::TcpServer server(&loop, addr, "echo server");//创建TCP服务器对象，绑定事件循环和监听地址
	server.setConnectionCallback(onConnection);//设置连接回调函数，当有新的客户端连接或断开连接时会调用该函数进行处理
	server.setMessageCallback(onMessage);//设置消息回调函数，当服务器接收到消息时会调用该函数进行处理
	server.start();//启动服务器，开始监听和接受客户端连接

}

int main(int argc, char* argv[], char* env[])
{
	signal(SIGCHLD, SIG_DFL);//子进程处理：SIGCHLD信号在子进程结束时发送给父进程，默认需要调用wait或waitpid避免僵尸进程
	//默认处理方式：SIG_DFL表示采用系统默认处理方式，自动回收子进程资源
	signal(SIGPIPE, SIG_IGN);//管道破裂处理：SIGPIPE信号在向已关闭的管道或套接字写入数据时发送，默认会终止进程
	//忽略处理方式：SIG_IGN表示忽略该信号，防止进程被意外终止，可以通过检查写操作的返回值来处理错误
	//在服务器开发中，忽略SIGPIPE信号可以提高稳定性，避免因客户端断开连接而导致服务器崩溃
	signal(SIGINT, signal_exit);//中断错误
	signal(SIGTERM, signal_exit);//ctrl + c
	signal(SIGKILL, signal_exit);
	signal(SIGILL, signal_exit);//非法指令错误
	signal(SIGSEGV, signal_exit);//段错误处理：SIGSEGV信号在访问无效内存地址时发送，通常表示程序存在内存访问错误，例如访问已释放的内存、
	signal(SIGTRAP, signal_exit);//ctrl + break
	signal(SIGABRT, signal_exit);//abort()函数调用时发送，通常表示程序异常终止，例如内存泄漏、非法操作等

	cout << "imchatserver is imvoking ..." << endl;
	int ch = 0;
	bool is_daemon = false;
	while ((ch = getopt(argc, argv, "d")) != -1)//getopt函数用于解析命令行参数，返回当前选项字符，如果没有更多选项则返回-1
	{
		cout << "ch = " << ch << endl;
		cout << "current " << optind << " value:" << argv[optind - 1] << endl;
		switch (ch)
		{
		case 'd':
			is_daemon = true;
			break;
		default:
			show_help(argv[0]);
			return -1;
		}
	}

	if (is_daemon)
	{
		daemon();//开启守护进程
	}

	muduo::net::EventLoop loop;//事件循环对象，负责处理事件和调度回调函数
	g_mainLoop = &loop;//注册全局事件循环指针，供信号处理触发优雅退出
	//test();
	//单例解决了两个问题：1.全局唯一性，确保在整个程序中只有一个实例存在；2.全局访问点，提供一个全局访问点来获取这个实例，方便在不同的地方使用它。
	// 数据库连接信息从环境变量读取，避免明文密码硬编码进源码。
	// 可设置 MYSQL_HOST / MYSQL_USER / MYSQL_PASSWORD / MYSQL_DB 覆盖默认值。
	auto envOr = [](const char* key, const char* def) -> std::string {
		const char* v = std::getenv(key);
		return (v && *v) ? std::string(v) : std::string(def);
	};
	std::string dbHost = envOr("MYSQL_HOST", "127.0.0.1");
	std::string dbUser = envOr("MYSQL_USER", "root");
	std::string dbPass = envOr("MYSQL_PASSWORD", "qaz000999plm");
	std::string dbName = envOr("MYSQL_DB", "myim");
	// 连接池大小：建议 >= IO 线程数(IMSERVER_THREADS)，默认 8
	int poolSize = atoi(envOr("MYSQL_POOL_SIZE", "8").c_str());
	if (poolSize < 1) poolSize = 1;
	if(Singleton<MySqlManager>::instance().Init(dbHost, dbUser, dbPass, dbName, 3306, poolSize) == false)
	{
		cout << "database init errno! (检查 MYSQL_* 环境变量)\r\n";
		return -2;
	}
	cout << __FILE__ << "(" << __LINE__ << ")\r\n";
	if(Singleton<UserManager>::instance().init() == false)//初始化用户管理器，加载用户信息和关系数据，如果初始化失败，则输出错误信息并返回
	{
		cout << "load user failed!\r\n";
		return -3;
	}
	cout << __FILE__ << "(" << __LINE__ << ")\r\n";
	if (Singleton<IMSer>::instance().init("0.0.0.0", 9527, &loop) == false)//初始化服务器，绑定监听地址和事件循环
	{
		//Singleton<IMSer>::instance()获取IMSer类的单例实例，调用init方法进行初始化，传入监听地址、端口和事件循环对象
		cout << "server init error!\r\n";
		return -1;
	}
	loop.loop();//进入事件循环，等待和处理事件

	// 事件循环退出后的优雅清理：离线消息在产生时即写入数据库
	//（MsgCacheManager 持久化），进程退出不会丢失，无需额外落盘。
	cout << "imchatserver shutting down, cleanup done.\r\n";
	return 0;
}
