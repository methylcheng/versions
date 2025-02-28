package main

import (
	"StudentVersionManagement/config"
	"StudentVersionManagement/controller"
	"StudentVersionManagement/dao"
	"StudentVersionManagement/mysql"
	"StudentVersionManagement/redis"
	"StudentVersionManagement/routers"
	"StudentVersionManagement/service"
	"log"
	"time"
)

func main() {
	// 初始化数据库和缓存
	cfg := config.GetConfig()

	if err := mysql.InitDB(cfg.MySQL.DSN); err != nil {
		log.Fatalf("节点：%s 初始化数据库失败: %v", cfg.Node.ID, err)
	}
	redis.InitRedis(cfg.Redis.Addr, cfg.Redis.Password, cfg.Redis.DB)

	// 初始化 DAO
	versionCacheDao := dao.NewVersionRedisDao(redis.RedisClient)
	versionMysqlDao, err := dao.NewVersionMysqlDao(mysql.DB)
	if err != nil {
		log.Fatalf("节点：%s 初始化 MySQL DAO 失败: %v", cfg.Node.ID, err)
	}

	// 初始化服务
	versionCacheService := service.NewVersionRedisService(versionCacheDao)
	versionMysqlService, err := service.NewVersionMysqlService(versionMysqlDao) // 修改这里
	if err != nil {
		log.Fatalf("节点：%s 初始化 MySQL 服务层失败: %v", cfg.Node.ID, err)
	}
	versionService, err := service.NewVersionService(versionMysqlService, versionCacheService, cfg.Node, cfg.Peers)
	if err != nil {
		log.Fatalf("节点：%s 初始化学生服务层失败：%v", cfg.Node.ID, err)
	}

	// 初始化控制器
	versionController := controller.NewVersionController(versionService)

	//启动时等待10秒 第一个节点要等待领导者选举完成再获得地址
	if cfg.Node.Port == "8080" {
		time.Sleep(10 * time.Second)
		log.Printf("节点：%s 等待领导者选举完成", cfg.Node.ID)
	}
	leaderPortAddr, err, fatalNode := versionService.GetLeaderPortAddr()
	if fatalNode != nil {
		if err = versionService.DeleteWrongPeer(fatalNode); err != nil {
			log.Printf("删除节点：%s失败：%v", fatalNode.ID, err)
			return
		}
	}
	//定期清空缓存 定期清除内存中的过期键 让领导者节点提交命令给所有节点
	if err != nil {
		log.Fatalf("节点 %s 获取领导者端口地址失败：%v", cfg.Node.ID, err)
	}
	go func() {
		if cfg.Node.Port == leaderPortAddr {
			versionService.ReLoadCacheData(cfg.Server.ReloadInterval)
		}
	}()

	//初始化路由
	versionRouter := routers.SetUpVersionRouter(versionController)
	serverAddress := ":" + cfg.Node.Port
	if err = versionRouter.Run(serverAddress); err != nil {
		log.Fatalf("节点：%s 初始化学生路由时出错：%v", cfg.Node.ID, err)
	}

	//启动定期同步任务
	go startPeriodicSync(versionService, cfg.Server.ReloadInterval)
}

// startPeriodicSync 启动定期同步任务
func startPeriodicSync(versionService *service.VersionService, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for range ticker.C {
		log.Println("开始定期同步 MySQL 数据到 Redis")
		if err := versionService.ReLoadCacheDataInternal(); err != nil {
			log.Printf("定期同步 MySQL 数据到 Redis 失败: %v", err)
		} else {
			log.Println("定期同步 MySQL 数据到 Redis 完成")
		}
	}
}
