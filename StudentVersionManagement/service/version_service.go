package service

import (
	"StudentVersionManagement/config"
	"StudentVersionManagement/interfaces"
	"StudentVersionManagement/model"
	"StudentVersionManagement/raft"
	"StudentVersionManagement/raft/fsm"
	"StudentVersionManagement/redis"
	"encoding/json"
	"fmt"
	raftfpk "github.com/hashicorp/raft"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

// VersionService 定义版本服务层结构体
type VersionService struct {
	MysqlService *VersionMysqlService
	RedisService *VersionRedisService
	RaftNodes    *raftfpk.Raft
	node         config.Node
	peers        []*config.Peer
}

// ConvertPeersToNodes 将 []*config.Peer 转换为 []*config.Node
func ConvertPeersToNodes(peers []*config.Peer) ([]*config.Node, error) {
	nodes := make([]*config.Node, len(peers))
	for i, peer := range peers {
		node := &config.Node{
			ID:      peer.ID,
			Address: peer.Address,
			Port:    peer.Port,
		}
		nodes[i] = node
	}
	return nodes, nil
}

// NewVersionService 创建一个新的 VersionService 实例
func NewVersionService(mysqlService *VersionMysqlService, redisService *VersionRedisService, node config.Node, peers []*config.Peer) (*VersionService, error) {
	vs := &VersionService{
		MysqlService: mysqlService,
		RedisService: redisService,
		RaftNodes:    new(raftfpk.Raft),
		node:         node,
		peers:        peers,
	}

	initializer := &raft.RaftInitializerImpl{}

	// 将 []*config.Peer 转换为 []*config.Node
	nodes, err := ConvertPeersToNodes(peers)
	if err != nil {
		return nil, fmt.Errorf("peer类型转换失败: %w", err)
	}

	rNode, err := initializer.InitRaft(node, nodes, vs)
	if err != nil {
		return nil, fmt.Errorf("初始化 raft 失败: %w", err)
	}
	vs.RaftNodes = rNode
	return vs, nil
}

// 确保实现 VersionServiceInterface 接口
var _ interfaces.VersionServiceInterface = (*VersionService)(nil)

// VersionNotFoundErr 判断错误是不是没有找到版本号之类的错误 如果是那就继续去下一个数据源找 不返回
func (vs *VersionService) VersionNotFoundErr(err error) bool {
	return strings.Contains(err.Error(), fmt.Sprintf("不存在版本"))
}

// JoinRaftCluster 将节点加入 Raft 集群
func (vs *VersionService) JoinRaftCluster(ID string, Address string, Port string) error {
	// 确保只有领导者节点才会处理加入集群的请求
	if vs.RaftNodes.State() == raftfpk.Leader {
		future := vs.RaftNodes.AddVoter(raftfpk.ServerID(ID), raftfpk.ServerAddress(Address+":"+Port), 0, 0)
		if err := future.Error(); err != nil {
			return fmt.Errorf("向Raft中添加选举者失败: %w", err)
		}
		log.Printf("节点 %s 成功加入Raft集群", ID)

		nPeer := new(config.Peer)
		nPeer.ID = ID
		nPeer.Address = Address
		nPeer.Port = Port

		// 更新所有节点的Peers信息
		err := vs.applyRaftCommand("update", nil, "", 0, nPeer)
		if err != nil {
			return fmt.Errorf("更新Peers失败: %w", err)
		}
		return nil
	}
	log.Printf("节点 %s 不是领导者，无法处理加入集群的请求", ID)
	return nil
}

// applyRaftCommand 将命令提交给领导者
func (vs *VersionService) applyRaftCommand(operation string, version *model.Version, id string, examineSize int, peer *config.Peer) error {
	// 创建 Raft 命令
	cmd := fsm.Command{
		Operation:   operation,
		Version:     version,
		Id:          id,
		ExamineSize: examineSize,
		Peer:        peer,
	}
	// 序列化命令
	data, err := json.Marshal(cmd)
	if err != nil {
		return fmt.Errorf("序列化命令失败: %w", err)
	}
	// 找到领导者节点
	if vs.RaftNodes.State() == raftfpk.Leader {
		// 向领导者节点发送请求
		future := vs.RaftNodes.Apply(data, 200)
		if err := future.Error(); err != nil {
			return fmt.Errorf("向领导者节点发送请求失败，节点号 %s: %w", id, err)
		}
		// 处理响应
		if future.Error() != nil {
			return fmt.Errorf("处理响应失败: %w", future.Error())
		}
		log.Printf("命令 %s 已成功提交到Raft状态机", operation)
		return nil
	} else {
		// 如果不是领导者节点，找到领导者节点的端口，把命令交给领导者节点处理
		leaderPortAddr, err, wrongNode := vs.GetLeaderPortAddr()
		if wrongNode != nil {
			err := vs.DeleteWrongPeer(wrongNode)
			if err != nil {
				return err
			}
		}
		if err != nil {
			return fmt.Errorf("获取领导者地址失败：%w", err)
		}
		url := fmt.Sprintf("http://localhost:%s/LeaderHandleCommand?cmd=%s", leaderPortAddr, data)
		resp, err := http.Get(url)
		if err != nil {
			log.Printf("将命令 %s 发送给领导者失败：%v", data, err)
			return fmt.Errorf("将命令 %s 发送给领导者失败：%v", data, err)
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			log.Printf("读取响应体出错：%v", err)
			return err
		}

		// 解析 JSON 响应
		var result struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		}
		if err := json.Unmarshal(body, &result); err != nil {
			log.Printf("解析JSON响应出错：%v", err)
			return err
		}
		// 把错误信息返回给前端发送的对应端口
		if result.Code != 1 {
			return fmt.Errorf("领导者节点处理命令失败：%v", result.Message)
		}
		log.Printf("命令 %s 已成功提交到领导者节点", operation)
		return nil
	}
}

// UpdatePeersInternal 更新Peers
func (vs *VersionService) UpdatePeersInternal(peer *config.Peer) {
	if vs.node.ID != peer.ID {
		vs.peers = append(vs.peers, peer)
		log.Printf("更新PeersInternal成功")
	}
}

// DeleteWrongPeer  在遍历寻找领导者地址时，如果发现他的http端口坏了，就会调用这个方法 向领导者节点发送http请求删除集群中的错误节点
// 领导者删除完集群里的节点后 会发布命令让每个节点删除错误peer
func (vs *VersionService) DeleteWrongPeer(wrongNode *config.Peer) error {
	leaderPortAddr, err, _ := vs.GetLeaderPortAddr()
	if err != nil {
		log.Printf("节点：%s 获取领导者端口地址失败：%v", vs.node.ID, err)
	}
	url := fmt.Sprintf("http://localhost:%s/DeleteWrongPeer?PeerID=%s&PeerAddress=%s&PeerPortAddress=%s", leaderPortAddr, wrongNode.ID, wrongNode.Address, wrongNode.Port)
	_, err = http.Get(url)
	if err != nil {
		log.Printf("StudentService.DeleteWrongPeer 发生错误:%v", err)
		return fmt.Errorf("StudentService.DeleteWrongPeer 发生错误:%w", err)
	}
	return nil
}

// HandleDeletePeerRequest 处理删除Peer的请求
func (vs *VersionService) HandleDeletePeerRequest(peerID string, peerAddr string, port string) error {
	if vs.RaftNodes.State() == raftfpk.Leader {
		wrongPeer := &config.Peer{
			ID:      peerID,
			Address: peerAddr,
			Port:    port,
		}
		future := vs.RaftNodes.RemoveServer(raftfpk.ServerID(peerID), 0, 0)
		if err := future.Error(); err != nil {
			return err
		}
		if wrongPeer != nil {
			log.Printf("领导者节点已将节点：%s从集群中删除", peerID)
			return vs.applyRaftCommand("deleteWrongPeer", nil, "", 0, wrongPeer)
		}
		return nil
	}
	return nil
}

// DeleteWrongPeerInternal 删除错误Peer
func (vs *VersionService) DeleteWrongPeerInternal(wrongPeer *config.Peer) {
	for i, peer := range vs.peers {
		if peer.ID == wrongPeer.ID {
			vs.peers = append(vs.peers[:i], vs.peers[i+1:]...)
			log.Printf("节点：%s删除了Peer：%s", vs.node.ID, peer.ID)
			return
		}
	}
}

// GetLeaderPortAddr 获取领导者端口地址 向集群的各个节点都发送一个http请求 如果他是领导者节点 他就会把自己的端口号返回过来
func (vs *VersionService) GetLeaderPortAddr() (string, error, *config.Peer) {
	wrongNode := &config.Peer{}
	if vs.RaftNodes.State() == raftfpk.Leader {
		return vs.node.Port, nil, nil
	}
	for _, node := range vs.peers {
		url := fmt.Sprintf("http://localhost:%s/GetLeaderAddress", node.Port)
		resp, err := http.Get(url)
		if err != nil {
			if strings.Contains(err.Error(), "No connection could be made because the target machine actively refused it") {
				log.Printf("节点：%s端口：%s失效，在集群中删除该节点", node.ID, node.Port)
				wrongNode.ID = node.ID
				wrongNode.Address = node.Address
				wrongNode.Port = node.Port
				continue
			}
			log.Printf("请求出错：%v", err)
			return "", fmt.Errorf("请求出错：%w", err), wrongNode
		}
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			log.Printf("读取响应体出错：%v", err)
			return "", fmt.Errorf("读取响应体出错：%w", err), wrongNode
		}

		// 检查响应体长度
		if len(body) == 0 {
			log.Printf("响应体为空：%s", url)
			continue
		}

		// 解析 JSON 响应
		var result struct {
			Code int         `json:"code"`
			Data interface{} `json:"data"`
		}
		err = json.Unmarshal(body, &result)
		if err != nil {
			fmt.Printf("解析 JSON 数据出错: %v\n", err)
			return "", fmt.Errorf("解析 JSON 数据出错: %w", err), wrongNode
		}

		// 提取 leaderAddr
		leaderPortAddr, ok := result.Data.(string)
		if ok {
			log.Printf("成功获取领导者端口地址：%s", leaderPortAddr)
			return leaderPortAddr, nil, wrongNode
		}
		return "", fmt.Errorf("领导者地址 类型断言失败"), wrongNode
	}
	log.Printf("遍历结束仍没有找到领导者")
	return "", fmt.Errorf("遍历结束仍没有找到领导者"), wrongNode
}

// HandleGetLeaderPortAddressRequest 处理获取领导者地址的请求 返回领导者的端口号
func (vs *VersionService) HandleGetLeaderPortAddressRequest() string {
	if vs.RaftNodes.State() == raftfpk.Leader {
		log.Printf("节点：%s是领导者节点", vs.node.ID)
		return vs.node.Port
	}
	log.Printf("节点：%s不是领导者节点", vs.node.ID)
	return ""
}

// ApplyRaftCommandToLeader 将命令提交给领导者处理
func (vs *VersionService) ApplyRaftCommandToLeader(operation string, version *model.Version, id string, examineSize int, peer *config.Peer) error {
	// 创建 Node 命令
	cmd := fsm.Command{
		Operation:   operation,
		Version:     version,
		Id:          id,
		ExamineSize: examineSize,
		Peer:        peer,
	}
	// 序列化命令
	data, err := json.Marshal(cmd)
	if err != nil {
		return fmt.Errorf("ApplyRaftCommandToLeader Marshal 发生错误: %w", err)
	}
	//如果自己是领导者节点 那就处理这个命令
	if vs.RaftNodes.State() == raftfpk.Leader {
		// 提交命令到领导者 Node 节点
		future := vs.RaftNodes.Apply(data, 500)
		if err = future.Error(); err != nil {
			return fmt.Errorf("ApplyRaftCommandToLeader 处理命令失败：%w", err)
		}
		// 处理响应
		result := future.Response()
		if resultErr, ok := result.(error); ok {
			return resultErr
		}
		log.Printf("领导者节点已接收并提交命令到状态机")
		return nil
	} else {
		//如果不是 那就找到领导者节点的端口 把命令交给领导者节点处理
		leaderPortAddr, err, wrongNode := vs.GetLeaderPortAddr()
		if wrongNode != nil {
			err := vs.DeleteWrongPeer(wrongNode)
			if err != nil {
				return err
			}
		}
		if err != nil {
			return fmt.Errorf("StudentService.ApplyRaftCommandToLeader 获取领导者地址失败：%w", err)
		}
		url := fmt.Sprintf("http://localhost:%s/LeaderHandleCommand?cmd=%s", leaderPortAddr, data)
		resp, err := http.Get(url)
		if err != nil {
			log.Printf("将cmd命令：%s发送给领导者失败：%v", data, err)
			return fmt.Errorf("将cmd命令：%s发送给领导者失败：%v", data, err)
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			log.Printf("读取响应体出错：%v", err)
			return err
		}

		// 解析 JSON 响应
		var result struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		}
		err = json.Unmarshal(body, &result)
		if err != nil {
			fmt.Printf("解析 JSON 数据出错: %v\n", err)
		}
		//把错误信息返回给前端发送的对应端口
		if result.Code != 1 {
			return fmt.Errorf("领导者节点处理命令失败：%v", result.Message)
		}
		return nil
	}
}

// LeaderHandleCommand 领导者节点会处理命令 并发送到状态机
func (vs *VersionService) LeaderHandleCommand(cmd string) error {
	future := vs.RaftNodes.Apply([]byte(cmd), 200)
	if err := future.Error(); err != nil {
		return fmt.Errorf("处理命令发生错误: %w", err)
	}
	result := future.Response()
	if resultErr, ok := result.(error); ok {
		return resultErr
	}
	log.Printf("领导者节点已接收并提交命令到状态机")
	return nil
}

// RestoreCacheData 恢复缓存机制 mysql有事务可以很方便地回滚 此函数专门用于恢复缓存的数据
func (vs *VersionService) RestoreCacheData(id string) error {
	//如果要恢复数据 mysql的事务会回滚 所以这个时候找到的版本还是一开始的版本
	versionBack, err := vs.MysqlService.GetVersionFromMysql(id)
	if err != nil {
		return fmt.Errorf("从mysql中查询失败: %w", err)
	}
	if err := vs.RedisService.AddVersion(versionBack); err != nil {
		return fmt.Errorf("向redis中添加失败: %w", err)
	}
	return nil
}

// ReLoadCacheDataInternal 重新加载缓存数据（内部方法）
func (vs *VersionService) ReLoadCacheDataInternal() error {
	// 获取分布式锁
	lockKey := "reload_redis_lock"
	expiration := 10 * time.Second
	acquired, err := redis.AcquireLock(lockKey, expiration)
	if err != nil {
		log.Printf("获取分布式锁失败: %v", err)
		return err
	}
	if !acquired {
		log.Printf("无法获取锁，另一个进程正在持有它。")
		return fmt.Errorf("无法获取锁，另一个进程正在持有它。")
	}
	defer func() {
		// 释放分布式锁
		if _, err := redis.ReleaseLock(lockKey); err != nil {
			log.Printf("无法释放锁: %v", err)
		}
	}()

	// 获取所有版本
	versions, err := vs.MysqlService.GetAllVersions()
	if err != nil {
		log.Printf("无法从mysql中获取所有版本信息: %v", err)
		return err
	}
	// 添加版本到 Redis
	for _, version := range versions {
		if err := vs.RedisService.AddVersion(version); err != nil {
			log.Printf("无法向redis中添加版本信息: %v", err)
			return err
		}
	}
	return nil
}

// AddVersionInternal 添加版本信息
func (vs *VersionService) AddVersionInternal(version *model.Version) error {
	// 获取分布式锁
	lockKey := "add_version_lock"
	expiration := 10 * time.Minute
	acquired, err := redis.AcquireLock(lockKey, expiration)
	if err != nil {
		return fmt.Errorf("获取分布式锁失败: %w", err)
	}
	if !acquired {
		return fmt.Errorf("无法获取锁，另一个进程正在持有它。")
	}
	defer func() {
		// 释放分布式锁
		if _, err := redis.ReleaseLock(lockKey); err != nil {
			log.Printf("无法释放锁: %v", err)
		}
	}()

	// 开始 MySQL 事务
	tx := vs.MysqlService.mysqlDao.DB.Begin()
	if tx.Error != nil {
		return fmt.Errorf("未能开始 MySQL 事务: %w", tx.Error)
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
			log.Printf("发生错误: %v", r)
		}
	}()
	// 在 MySQL 数据库事务中添加版本信息
	if err := vs.MysqlService.AddVersionToMysql(version); err != nil {
		tx.Rollback()
		return fmt.Errorf("无法向mysql中添加版本信息: %w", err)
	}
	// MySQL 数据库事务提交成功后，尝试添加到redis
	if err := vs.RedisService.AddVersion(version); err != nil {
		tx.Rollback()
		return fmt.Errorf("无法向redis中添加版本信息: %w", err)
	}
	if err := tx.Commit().Error; err != nil {
		return fmt.Errorf("提交MySQL事务失败: %w", err)
	}
	return nil
}

// GetVersionByID 获取指定 ID 的版本信息
func (vs *VersionService) GetVersionByID(id string) (*model.Version, error) {
	// 尝试从缓存获取版本
	version, err := vs.RedisService.GetVersionByID(id)
	if err == nil {
		return version, nil
	}

	// 如果缓存中没有找到版本，则从数据库获取。如果确定缓存中没有该版本信息，则向缓存中添加该版本信息
	if vs.VersionNotFoundErr(err) {
		version, err = vs.MysqlService.GetVersionFromMysql(id)
		if err != nil {
			return nil, fmt.Errorf("从mysql获取版本失败: %w", err)
		}
		if err := vs.RedisService.AddVersion(version); err != nil {
			return nil, fmt.Errorf("向redis添加版本失败: %w", err)
		}
		return version, nil
	}
	return nil, err
}

// UpdateVersionInternal 内部更新版本信息的方法
// 该方法接收一个版本信息指针，使用 MySQL 事务进行更新操作，并更新 Redis 缓存
func (vs *VersionService) UpdateVersionInternal(version *model.Version) error {
	// 开始 MySQL 事务
	tx := vs.MysqlService.mysqlDao.DB.Begin()
	if tx.Error != nil {
		// 记录错误日志
		log.Printf("启动MySQL事务失败: %v", tx.Error)
		return fmt.Errorf("启动MySQL事务失败: %w", tx.Error)
	}
	// 确保在发生 panic 时回滚事务
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
			log.Printf("发生错误: %v", r)
		}
	}()

	// 先检查 MySQL 中版本是否存在
	if err := vs.MysqlService.VersionExists(version.ID); err != nil {
		// 回滚事务
		tx.Rollback()
		// 记录错误日志
		log.Printf("VersionMysqlService.UpdateVersion 更新版本：%s失败：%v", version.ID, err)
		return fmt.Errorf("VersionMysqlService.UpdateVersion 更新版本：%s失败：%w", version.ID, err)
	}

	// 再检查 Redis 中版本是否存在
	if err := vs.RedisService.VersionExists(version.ID); err != nil {
		// 回滚事务
		tx.Rollback()
		// 记录错误日志
		log.Printf("Redis 中版本号：%s 不存在：%v", version.ID, err)
		return fmt.Errorf("Redis 中版本号：%s 不存在：%w", version.ID, err)
	}

	// 调用数据层代码更新 MySQL 中的版本信息
	if err := vs.MysqlService.UpdateVersion(tx, version); err != nil {
		// 回滚事务
		tx.Rollback()
		// 记录错误日志
		log.Printf("更新版本号失败：%v", err)
		return fmt.Errorf("更新版本号失败：%w", err)
	}

	// MySQL 数据库事务提交成功后，尝试更新缓存，还要确保数据一致性
	if err := vs.RedisService.UpdateVersion(version); err != nil {
		// 回滚事务
		tx.Rollback()
		// 记录错误日志
		log.Printf("更新版本到redis失败: %v", err)
		return fmt.Errorf("更新版本到redis失败: %w", err)
	}

	if err := tx.Commit().Error; err != nil {
		// 记录错误日志
		log.Printf("提交MySQL事务失败: %v", err)
		return fmt.Errorf("提交MySQL事务失败: %w", err)
	}

	return nil
}

// DeleteVersionInternal 删除版本信息
func (vs *VersionService) DeleteVersionInternal(id string) error {
	// 获取分布式锁
	lockKey := "delete_version_lock"
	expiration := 10 * time.Second
	acquired, err := redis.AcquireLock(lockKey, expiration)
	if err != nil {
		return fmt.Errorf("获取锁失败: %w", err)
	}
	if !acquired {
		return fmt.Errorf("获取锁失败，另一个进程持有该锁")
	}
	defer func() {
		// 释放分布式锁
		if _, err := redis.ReleaseLock(lockKey); err != nil {
			log.Printf("释放锁失败: %v", err)
		}
	}()

	// 开始 MySQL 事务
	tx := vs.MysqlService.mysqlDao.DB.Begin()
	if tx.Error != nil {
		return fmt.Errorf("启动MySQL事务失败: %w", tx.Error)
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
			log.Printf("发生错误: %v", r)
		}
	}()
	if err := vs.MysqlService.DeleteVersion(id); err != nil {
		if !vs.VersionNotFoundErr(err) {
			tx.Rollback()
			return fmt.Errorf("从mysql中删除版本失败: %w", err)
		}
	}
	if err := vs.RedisService.DeleteVersion(id); err != nil {
		if !vs.VersionNotFoundErr(err) {
			tx.Rollback()
			return fmt.Errorf("从redis删除版本失败: %w", err)
		}
	}
	if err := tx.Commit().Error; err != nil {
		return fmt.Errorf("提交MySQL事务失败: %w", err)
	}
	return nil
}

// ReLoadCacheData 重新加载缓存 并提交给Raft节点
func (vs *VersionService) ReLoadCacheData(interval time.Duration) error {
	// 每隔一段时间重新加载缓存
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			// 重新加载缓存
			if err := vs.ReLoadCacheDataInternal(); err != nil {
				log.Printf("重新加载缓存失败: %v", err)
			}
		}
	}
}

// AddVersion 接收添加版本命令 提交给Raft节点
func (vs *VersionService) AddVersion(version *model.Version) error {
	// 提交给Raft节点
	return vs.applyRaftCommand("add", version, "", 0, nil)
}

// UpdateVersion 提交更新版本信息的请求给 Raft 节点
// 该方法接收一个版本信息指针，将更新操作提交给 Raft 节点
func (vs *VersionService) UpdateVersion(version *model.Version) error {
	// 提交给 Raft 节点
	return vs.applyRaftCommand("update", version, "", 0, nil)
}

// DeleteVersion 接收删除版本命令 提交给Raft节点
func (vs *VersionService) DeleteVersion(id string) error {
	// 提交给Raft节点
	return vs.applyRaftCommand("delete", nil, id, 0, nil)
}
