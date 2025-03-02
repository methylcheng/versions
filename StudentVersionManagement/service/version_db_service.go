package service

import (
	"StudentVersionManagement/dao"
	"StudentVersionManagement/model"
	"errors"
	"fmt"
	"log"
)

// VersionDBService 定义内存数据库服务层结构体
type VersionDBService struct {
	inMemoryDB *dao.InMemoryDB
}

// NewVersionDBService 创建VersionDBService实例
func NewVersionDBService(db *dao.InMemoryDB) *VersionDBService {
	return &VersionDBService{
		inMemoryDB: db,
	}
}

// GetVersionByID 从内存中获取版本号
func (s *VersionDBService) GetVersionByID(ID string) (*model.Version, error) {
	value, exists, err := s.inMemoryDB.GetValue(ID)
	if err != nil {
		return nil, err
	}
	if exists {
		version, ok := value.(*model.Version)
		if !ok {
			return nil, errors.New("")
		}
		log.Printf("从内存中查找版本号：%s", ID)
		log.Printf("%v", version)
		return version, nil
	}
	return nil, fmt.Errorf("StudentMdbService.GetStudent 内存中不存在版本号：%s", ID)
}

// VersionExists 判断版本号是否存在
func (s *VersionDBService) VersionExists(ID string) error {
	_, exists, err := s.inMemoryDB.GetValue(ID)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	return fmt.Errorf("StudentMdbService.VersionExists 版本号：%s不存在", ID)
}

// AddVersion 添加版本号
func (s *VersionDBService) AddVersion(version *model.Version) error {
	err := s.inMemoryDB.SetValue(version.ID, version, 0)
	if err != nil {
		return err
	}
	log.Printf("添加版本号：%s", version.ID)
	return nil
}

// DeleteVersion 删除版本号
func (s *VersionDBService) DeleteVersion(ID string) error {
	err := s.inMemoryDB.DeleteValue(ID)
	if err != nil {
		return err
	}
	log.Printf("删除版本号：%s", ID)
	return nil
}

// UpdateVersion 更新版本号
func (s *VersionDBService) UpdateVersion(version *model.Version) error {
	if err := s.VersionExists(version.ID); err != nil {
		return err
	}

	// 假设 inMemoryDB.UpdateValue 返回两个值：bool 和 error
	updated, err := s.inMemoryDB.UpdateValue(version.ID, version)
	if err != nil {
		return err
	}
	if !updated {
		return fmt.Errorf("更新失败，版本号：%s 不存在", version.ID)
	}
	log.Printf("更新版本号：%s", version.ID)
	return nil
}

// PeriodicDelete 定期删除内存中的过期键
func (s *VersionDBService) PeriodicDelete(checkSize int) error {
	err := s.inMemoryDB.PeriodicCleanup(checkSize)
	if err != nil {
		return err
	}
	log.Printf("定期删除内存中的过期键")
	return nil
}
