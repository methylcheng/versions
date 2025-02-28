package fsm

import (
	"StudentVersionManagement/config"
	"StudentVersionManagement/interfaces"
	"StudentVersionManagement/model"
	"encoding/json"
	"fmt"
	"github.com/hashicorp/raft"
	"io"
	"log"
)

// Command 定义 Raft 日志条目中的命令
// 它包含了操作类型、版本信息、唯一标识符和检查大小等信息
type Command struct {
	Operation   string         `json:"operation"`
	Version     *model.Version `json:"version,omitempty"`
	Id          string         `json:"id"`
	ExamineSize int            `json:"examine_size"`
	Peer        *config.Peer   `json:"peer"`
}

// VersionFSM 处理 Raft 日志条目的应用
// 它依赖于一个实现了 VersionServiceInterface 接口的服务来执行版本相关的操作
type VersionFSM struct {
	service interfaces.VersionServiceInterface
}

// NewVersionFSM 创建一个新的 VersionFSM 实例
func NewVersionFSM(service interfaces.VersionServiceInterface) *VersionFSM {
	return &VersionFSM{
		service: service,
	}
}

// Apply 应用 Raft 日志条目
func (f *VersionFSM) Apply(log *raft.Log) interface{} {
	var cmd Command
	if err := json.Unmarshal(log.Data, &cmd); err != nil {
		return fmt.Errorf("解析 JSON 绑定失败: %s", err)
	}
	switch cmd.Operation {
	case "add":
		return f.service.AddVersionInternal(cmd.Version)
	case "update":
		return f.service.UpdateVersionInternal(cmd.Version)
	case "delete":
		return f.service.DeleteVersionInternal(cmd.Id)
	case "reloadCacheData":
		if err := f.service.ReLoadCacheDataInternal(); err != nil {
			return fmt.Errorf("重新加载 Redis 数据失败: %s", err)
		}
		return nil
	case "updatePeers":
		f.service.UpdatePeersInternal(cmd.Peer)
		return nil
	case "deleteFatalPeer":
		f.service.DeleteWrongPeerInternal(cmd.Peer)
		return nil
	default:
		return fmt.Errorf("未知操作: %s", cmd.Operation)
	}
}

// Snapshot 创建快照
func (f *VersionFSM) Snapshot() (raft.FSMSnapshot, error) {
	// 实现快照逻辑
	return nil, nil
}

// Restore 恢复快照
func (fsm *VersionFSM) Restore(snapshot io.ReadCloser) error {
	defer snapshot.Close()
	log.Printf("开始恢复快照数据")
	return nil
}
