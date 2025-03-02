package controller

import (
	"StudentVersionManagement/model"
	"StudentVersionManagement/service"
	"github.com/gin-gonic/gin"
	"log"
	"net/http"
)

// VersionController 版本号控制器
// 它包含一个版本服务的实例，用于执行与版本号相关的操作
type VersionController struct {
	// versionService 是 VersionController 的一个成员
	// 它是一个指向 service.VersionService 的指针，用于提供版本号相关的服务
	versionService *service.VersionService
}

// NewVersionController 创建版本号控制器实例
func NewVersionController(versionService *service.VersionService) *VersionController {
	return &VersionController{
		versionService: versionService,
	}
}

// AddVersion 添加版本号信息
// @Summary 添加版本号信息
// @Description 添加一个新的版本号信息
// @Tags 版本管理
// @Accept json
// @Produce json
// @Param version body model.Version true "版本号信息"
// @Success 200   "成功添加版本号"
// @Failure 400   "无效的请求体"
// @Failure 500   "服务器内部错误"
// @Router /add_versions [post]
func (vc *VersionController) AddVersion(c *gin.Context) {
	var version model.Version
	if err := c.BindJSON(&version); err != nil {
		log.Printf("绑定JSON数据失败: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"错误": err.Error()})
		return
	}
	err := vc.versionService.AddVersion(&version)
	if err != nil {
		log.Printf("添加版本信息失败: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"错误": err.Error()})
		return
	}
	log.Printf("成功添加版本号: %v", version)
	c.JSON(http.StatusOK, gin.H{"消息": "成功添加版本号"})
}

// DeleteVersion 删除版本号信息
// @Summary 删除版本号信息
// @Description 根据ID删除版本号信息
// @Tags 版本管理
// @Accept json
// @Produce json
// @Param id path string true "版本号ID"
// @Success 200   "版本号删除成功"
// @Failure 400   "无效的版本号ID"
// @Failure 500   "服务器内部错误"
// @Router /delete_versions/{id} [delete]
func (vc *VersionController) DeleteVersion(c *gin.Context) {
	id := c.Param("id")
	err := vc.versionService.DeleteVersion(id)
	if err != nil {
		log.Printf("删除版本信息失败: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"错误": err.Error()})
		return
	}
	log.Printf("成功删除版本号: %s", id)
	c.JSON(http.StatusOK, gin.H{"消息": "版本号删除成功"})
}

// UpdateVersion 更新版本号信息
// @Summary 更新版本号信息
// @Description 根据ID更新版本号信息
// @Tags 版本管理
// @Accept json
// @Produce json
// @Param id path string true "版本号ID"
// @Param version body model.Version true "版本号信息"
// @Success 200   "版本号更新成功"
// @Failure 400   "无效的请求体或版本号ID"
// @Failure 404   "版本号未找到"
// @Failure 500   "服务器内部错误"
// @Router /update_versions/{id} [put]
func (vc *VersionController) UpdateVersion(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"错误": "无效的版本号ID"})
		return
	}
	existingVersion, err := vc.versionService.GetVersionByID(id)
	if err != nil {
		log.Printf("根据ID寻找版本号时发生错误: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"错误": err.Error()})
		return
	}
	if existingVersion == nil {
		c.JSON(http.StatusNotFound, gin.H{"错误": "版本号未找到"})
		return
	}
	var updatedVersion model.Version
	if err := c.BindJSON(&updatedVersion); err != nil {
		log.Printf("数据绑定失败: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"错误": err.Error()})
		return
	}
	updatedVersion.ID = id
	err = vc.versionService.UpdateVersion(&updatedVersion)
	if err != nil {
		log.Printf("版本号更新失败: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"错误": err.Error()})
		return
	}
	log.Printf("成功更新版本号: %v", updatedVersion)
	c.JSON(http.StatusOK, gin.H{"消息": "版本号更新成功"})
}

// GetVersionByID 获取指定ID的版本号信息
// @Summary 获取指定ID的版本号信息
// @Description 根据ID获取版本号信息
// @Tags 版本管理
// @Accept json
// @Produce json
// @Param id path string true "版本号ID"
// @Success 200 {object} model.Version "成功获取版本号"
// @Failure 404   "版本号未找到"
// @Failure 500   "服务器内部错误"
// @Router /get_versions/{id} [get]
func (vc *VersionController) GetVersionByID(c *gin.Context) {
	id := c.Param("id")
	version, err := vc.versionService.GetVersionByID(id)
	if err != nil {
		log.Printf("获取版本信息失败: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"错误": err.Error()})
		return
	}
	if version == nil {
		version = &model.Version{}
	}
	log.Printf("成功获取版本号: %v", version)
	c.JSON(http.StatusOK, version)
}

// JoinRaftCluster 加入Raft集群
// @Summary 加入Raft集群
// @Description 将自身加入到Raft集群中
// @Tags Raft管理
// @Accept json
// @Produce json
// @Param nodeID query string true "节点ID"
// @Param nodeAddress query string true "节点地址"
// @Param portAddress query string true "端口地址"
// @Success 200   "添加节点成功"
// @Failure 500   "服务器内部错误"
// @Router /join_raft_cluster [post]
func (vc *VersionController) JoinRaftCluster(c *gin.Context) {
	nodeID := c.Query("nodeID")
	nodeAddress := c.Query("nodeAddress")
	nodePortAddress := c.Query("portAddress")
	if err := vc.versionService.JoinRaftCluster(nodeID, nodeAddress, nodePortAddress); err != nil {
		log.Printf("加入Raft集群失败: %v", err)
		c.JSON(500, gin.H{"错误": err.Error()})
	} else {
		log.Printf("成功添加节点: %s", nodeID)
		c.JSON(http.StatusOK, gin.H{"消息": "添加节点成功"})
	}
}

// LeaderHandleCommand 处理领导者命令
// @Summary 处理领导者命令
// @Description 处理领导者节点发送的命令
// @Tags Raft管理
// @Accept json
// @Produce json
// @Param cmd query string true "命令数据"
// @Success 200   "领导者节点已处理命令"
// @Failure 500   "服务器内部错误"
// @Router /leader_handle_command [post]
func (vc *VersionController) LeaderHandleCommand(c *gin.Context) {
	cmdData := c.Query("cmd")
	if err := vc.versionService.LeaderHandleCommand(cmdData); err != nil {
		log.Printf("处理领导者命令失败: %v", err)
		c.JSON(500, gin.H{"错误": err.Error()})
	} else {
		log.Printf("成功处理领导者命令")
		c.JSON(http.StatusOK, gin.H{"消息": "领导者节点已处理命令"})
	}
}

// GetLeaderPortAddress 获取领导者端口地址
// @Summary 获取领导者端口地址
// @Description 获取领导者端口的地址
// @Tags Raft管理
// @Accept json
// @Produce json
// @Success 200   "成功获取领导者端口地址"
// @Failure 500   "服务器内部错误"
// @Router /get_leader_port_address [get]
func (vc *VersionController) GetLeaderPortAddress(c *gin.Context) {
	leaderAddr := vc.versionService.HandleGetLeaderPortAddressRequest()
	if leaderAddr != "" {
		log.Printf("成功获取领导者端口地址: %s", leaderAddr)
		c.JSON(http.StatusOK, gin.H{"消息": "成功获取领导者端口地址", "领导者端口地址": leaderAddr})
	} else {
		log.Printf("获取领导者端口地址失败")
		c.JSON(http.StatusInternalServerError, gin.H{"错误": "获取领导者端口地址失败"})
	}
}

// DeleteFatalPeer 删除故障节点
// @Summary 删除故障节点
// @Description 删除故障节点
// @Tags Raft管理
// @Accept json
// @Produce json
// @Param PeerID query string true "故障节点ID"
// @Param PeerAddress query string true "故障节点地址"
// @Param PeerPortAddress query string true "故障节点端口地址"
// @Success 200   "成功删除节点"
// @Failure 500   "服务器内部错误"
// @Router /delete_fatal_peer [post]
func (vc *VersionController) DeleteFatalPeer(c *gin.Context) {
	peerID := c.Query("PeerID")
	peerAddr := c.Query("PeerAddress")
	peerPortAddr := c.Query("PeerPortAddress")
	if err := vc.versionService.HandleDeletePeerRequest(peerID, peerAddr, peerPortAddr); err != nil {
		log.Printf("删除故障节点失败: %v", err)
		c.JSON(500, gin.H{"错误": err.Error()})
	} else {
		log.Printf("成功删除节点: %s", peerID)
		c.JSON(http.StatusOK, gin.H{"消息": "成功删除节点"})
	}
}
