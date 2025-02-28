package raft

import (
	"StudentVersionManagement/config"
	"StudentVersionManagement/interfaces"
	"StudentVersionManagement/raft/fsm"
	nodePackage "StudentVersionManagement/raft/node"
	"github.com/hashicorp/raft"
	"log"
)

// RaftInitializerImpl 实现 RaftInitializer 接口
type RaftInitializerImpl struct{}

// InitRaft 方法用于初始化一个 Raft 节点。
// 它创建一个有限状态机（FSM）实例并与 Raft 节点关联，该节点能够与其它节点一起工作，形成一个 Raft 集群。
func (r *RaftInitializerImpl) InitRaft(node config.Node, peers []*config.Node, service interfaces.VersionServiceInterface) (*raft.Raft, error) {
	log.Printf("Initializing Raft node:ID = %s, Address = %s...", node.ID, node.Address)
	// 创建一个有限状态机（FSM）实例，它将处理Raft节点的状态变化。
	fsmInstance := fsm.NewVersionFSM(service)
	// 使用提供的ID、地址、同伴节点列表和FSM实例来创建一个新的Raft节点。
	raftNode, err := nodePackage.NewRaftNode(node, peers, fsmInstance)
	if err != nil {
		// 如果创建Raft节点时发生错误，返回nil和错误信息。
		log.Printf("初始化raft节点失败: %v", err)
		return nil, err
	}

	// 返回成功创建的Raft节点指针和nil错误。
	log.Printf("Raft节点初始化成功:ID = %s, Address = %s", node.ID, node.Address)
	return raftNode, nil
}
