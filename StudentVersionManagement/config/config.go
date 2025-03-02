package config

import (
	"time"
)

// MySQLConfig 定义 MySQL 数据库的配置信息
type MySQLConfig struct {
	DSN string // 数据库连接字符串
}

// RedisConfig 定义 Redis 缓存的配置信息
type RedisConfig struct {
	Addr     string // Redis 服务器地址
	Password string // Redis 密码
	DB       int    // Redis 数据库编号
}

// DBConfig 定义内存数据库配置结构体
type DBConfig struct {
	MaxCapacity   int     //内存容量
	EvictionRatio float64 //触发内存淘汰时淘汰的键的比例
}

// ServerConfig 定义服务器的运行配置
type ServerConfig struct {
	ReloadInterval         time.Duration // 缓存重载的时间间隔
	PeriodicDeleteInterval time.Duration // 过期键删除的时间间隔
	ExamineSize            int           // 每次删除时检查的键的数量
}

// Node 定义当前节点的信息
type Node struct {
	ID      string // 节点唯一标识
	Address string // 节点地址
	Port    string // 节点端口
}

// Peer 定义集群中其他节点的信息
type Peer struct {
	ID      string // 节点唯一标识
	Address string // 节点地址
	Port    string // 节点端口
}

// Config 定义整个应用的配置信息
type Config struct {
	MySQL  MySQLConfig // MySQL 配置
	Redis  RedisConfig // Redis 配置
	DB     DBConfig
	Server ServerConfig // 服务器配置
	Node   Node         // 当前节点配置
	Peers  []*Peer      // 集群中其他节点配置
}

// GetConfig 返回应用的配置实例
func GetConfig() Config {
	return Config{
		// 配置 MySQL 数据库连接
		MySQL: MySQLConfig{
			DSN: "root:wsm665881@tcp(127.0.0.1:3306)/student_version_db?charset=utf8mb4&parseTime=True&loc=Local",
		},
		// 配置 Redis 连接
		Redis: RedisConfig{
			Addr:     "127.0.0.1:6379",
			Password: "wsm665881",
			DB:       0,
		},
		DB: DBConfig{
			MaxCapacity:   10,
			EvictionRatio: 0.2,
		},
		// 配置服务器运行参数
		Server: ServerConfig{
			ReloadInterval:         time.Minute,
			PeriodicDeleteInterval: time.Hour,
			ExamineSize:            10,
		},
		// 配置当前节点信息
		Node: Node{
			ID:      "节点1",
			Address: "127.0.0.1:8080",
			Port:    "8080",
		},
		// 初始化集群中其他节点为空
		Peers: []*Peer{},
	}
}
