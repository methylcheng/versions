package node

import (
	"StudentVersionManagement/config"
	"fmt"
	"github.com/hashicorp/raft"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// 定义一个全局的通道，用于通知领导者已选出
var elected = make(chan struct{})
var closeOnce sync.Once

// NewRaftNode 创建一个新的 Raft 节点
func NewRaftNode(node config.Node, peers []*config.Node, f raft.FSM) (*raft.Raft, error) {
	log.Printf("创建 Raft 节点 ID: %s, 地址: %s", node.ID, node.Address)

	raftConfig := raft.DefaultConfig()
	raftConfig.LocalID = raft.ServerID(node.ID)
	raftConfig.SnapshotInterval = 200 * time.Second
	raftConfig.SnapshotThreshold = 1000

	logStore := raft.NewInmemStore()
	stableStore := raft.NewInmemStore()

	snapshotDirMutex.Lock()
	snapshotDir := filepath.Join("snapshots", node.ID)
	if err := os.MkdirAll(snapshotDir, 0755); err != nil {
		snapshotDirMutex.Unlock()
		log.Printf("创建快照目录失败: %v", err)
		return nil, fmt.Errorf("创建快照目录失败: %w", err)
	}
	snapshotStore, err := raft.NewFileSnapshotStore(snapshotDir, 1, os.Stderr)
	snapshotDirMutex.Unlock()
	if err != nil {
		log.Printf("创建快照存储失败: %v", err)
		return nil, fmt.Errorf("创建快照存储失败: %w", err)
	}

	transport, err := raft.NewTCPTransport("localhost:0", nil, 3, 10*time.Second, os.Stderr)
	if err != nil {
		log.Printf("创建传输层失败: %v", err)
		return nil, fmt.Errorf("创建传输层失败: %w", err)
	}
	if transport == nil {
		log.Printf("创建传输层失败")
		return nil, fmt.Errorf("创建传输层失败")
	}
	log.Printf("成功创建 Raft 节点 ID: %s, 地址: %s", node.ID, node.Address)

	raftNode, err := raft.NewRaft(raftConfig, f, logStore, stableStore, snapshotStore, transport)
	if err != nil {
		log.Printf("创建 Raft 节点失败: %v", err)
		return nil, fmt.Errorf("创建 Raft 节点失败: %w", err)
	}

	go func() {
		for {
			if raftNode.State() == raft.Leader {
				closeOnce.Do(func() {
					close(elected)
					log.Printf("节点 %s 成为领导者", node.ID)
				})
				break
			}
			time.Sleep(100 * time.Millisecond)
		}
	}()

	if len(peers) == 0 {
		configuration := raft.Configuration{
			Servers: []raft.Server{
				{
					ID:      raft.ServerID(node.ID),
					Address: raft.ServerAddress(node.Address),
				},
			},
		}
		future := raftNode.BootstrapCluster(configuration)
		if err := future.Error(); err != nil {
			log.Printf("节点初始化集群失败: %v", err)
			return nil, fmt.Errorf("节点初始化集群失败: %w", err)
		}
	} else {
		select {
		case <-elected:
		case <-time.After(5 * time.Second):
			log.Printf("节点 %s 等待选举超时...", node.ID)
			url := fmt.Sprintf("http://localhost:8080/join_raft_cluster?ID=%s&Address=%s&port=%s", node.ID, node.Address, node.Port)
			_, err := http.Get(url)
			if err != nil {
				log.Printf("加入集群失败: %v", err)
				return nil, fmt.Errorf("加入集群失败: %w", err)
			}
		}
	}
	return raftNode, nil
}

var snapshotDirMutex sync.Mutex
