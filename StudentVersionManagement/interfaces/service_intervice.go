package interfaces

import (
	"StudentVersionManagement/config"
	"StudentVersionManagement/model"
)

// VersionServiceInterface 定义了版本管理服务的接口。
// 它提供了添加、更新、删除版本信息以及重新加载缓存数据的功能。
type VersionServiceInterface interface {
	AddVersionInternal(student *model.Version) error
	UpdateVersionInternal(student *model.Version) error
	DeleteVersionInternal(id string) error
	ReLoadCacheDataInternal() error
	PeriodicDeleteInterval(examineSize int) error
	GetLeaderPortAddr() (string, error, *config.Peer)
	UpdatePeersInternal(peer *config.Peer)
	DeleteWrongPeerInternal(peer *config.Peer)
	DeleteWrongPeer(peer *config.Peer) error
}
