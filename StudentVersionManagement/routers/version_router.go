package routers

import (
	"StudentVersionManagement/controller"
	"github.com/gin-gonic/gin"
)

func SetUpVersionRouter(versionController *controller.VersionController) *gin.Engine {
	r := gin.Default()

	// 定义数据库操作相关路由
	r.POST("/add_versions", versionController.AddVersion)
	r.DELETE("/delete_versions/:id", versionController.DeleteVersion)
	r.PUT("/update_versions/:id", versionController.UpdateVersion)
	r.GET("/get_versions/:id", versionController.GetVersionByID)

	// raft相关路由
	r.GET("/join_raft_cluster", versionController.JoinRaftCluster)
	r.GET("/leader_handle_command", versionController.LeaderHandleCommand)
	r.GET("/get_leader_port_address", versionController.GetLeaderPortAddress)
	r.GET("/delete_fatal_peer", versionController.DeleteFatalPeer)
	// 启动服务器
	return r
}
