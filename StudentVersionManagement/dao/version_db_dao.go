package dao

import (
	"container/list"
	"log"
	"math/rand"
	"sync"
	"time"
)

// 默认过期时间
const defaultExpiration = time.Hour

// InMemoryDB 定义内存数据库结构体
type InMemoryDB struct {
	keyValueStore   map[string]interface{}
	expirationTimes map[string]time.Time
	accessLock      sync.RWMutex
	maxCapacity     int
	lruCacheList    *list.List
	lruCacheMap     map[string]*list.Element
	evictionRatio   float64
}

// NewInMemoryDBDao 初始化内存数据库实例
func NewInMemoryDBDao(capacity int, ratio float64) (*InMemoryDB, error) {
	if capacity <= 0 {
		return nil, logError("内存数据库容量必须大于 0")
	}
	if ratio < 0 || ratio > 1 {
		return nil, logError("缓存淘汰比例必须在 0 到 1 之间")
	}
	return &InMemoryDB{
		keyValueStore:   make(map[string]interface{}),
		expirationTimes: make(map[string]time.Time),
		maxCapacity:     capacity,
		lruCacheList:    list.New(),
		lruCacheMap:     make(map[string]*list.Element),
		evictionRatio:   ratio,
	}, nil
}

// SetValue 设置键值对并设置过期时间
func (db *InMemoryDB) SetValue(key string, value interface{}, expirationSeconds int64) error {
	if key == "" {
		return logError("键不能为空")
	}
	expirationDuration := time.Duration(expirationSeconds) * time.Second

	db.accessLock.Lock()
	defer db.accessLock.Unlock()

	if len(db.keyValueStore) >= db.maxCapacity {
		if err := db.performEviction(); err != nil {
			return err
		}
	}

	db.moveToFront(key)

	if expirationSeconds > 0 {
		expireTime := time.Now().Add(expirationDuration)
		db.expirationTimes[key] = expireTime
		db.keyValueStore[key] = value
		log.Printf("已成功添加键：%s，值：%v，过期时间：%v", key, value, expireTime)
	} else {
		db.keyValueStore[key] = value
		log.Printf("已成功添加键：%s，值：%v，此键永不过期", key, value)
	}
	return nil
}

// GetValue 获取键对应的值
func (db *InMemoryDB) GetValue(key string) (interface{}, bool, error) {
	if key == "" {
		return nil, false, logError("键不能为空")
	}

	db.accessLock.RLock()
	defer db.accessLock.RUnlock()

	if expireTime, exists := db.expirationTimes[key]; exists {
		if time.Now().After(expireTime) {
			db.accessLock.RUnlock()
			db.accessLock.Lock()
			db.removeKey(key)
			db.accessLock.Unlock()
			db.accessLock.RLock()
			log.Printf("键：%s 在：%v 时已经过期，已删除该键", key, expireTime)
			return nil, false, nil
		}
		newExpireTime := time.Now().Add(defaultExpiration)
		db.expirationTimes[key] = newExpireTime
		log.Printf("已成功延长键：%s 过期时间至：%v", key, newExpireTime)
		db.moveToFront(key)
		return db.keyValueStore[key], true, nil
	}

	value, exists := db.keyValueStore[key]
	if exists {
		db.moveToFront(key)
	}
	return value, exists, nil
}

// UpdateValue 更新键对应的值
func (db *InMemoryDB) UpdateValue(key string, value interface{}) (bool, error) {
	if key == "" {
		return false, logError("键不能为空")
	}

	db.accessLock.Lock()
	defer db.accessLock.Unlock()

	if expireTime, exists := db.expirationTimes[key]; exists {
		if time.Now().After(expireTime) {
			db.removeKey(key)
			log.Printf("键：%s 在：%v 时已经过期，已删除该键", key, expireTime)
			return false, nil
		}
		newExpireTime := time.Now().Add(defaultExpiration)
		db.expirationTimes[key] = newExpireTime
		log.Printf("已成功延长键：%s 过期时间至：%v", key, newExpireTime)
		db.keyValueStore[key] = value
		log.Printf("已成功修改键：%s 的值为：%v", key, value)
		db.moveToFront(key)
		return true, nil
	}

	if _, exists := db.keyValueStore[key]; exists {
		db.keyValueStore[key] = value
		log.Printf("已成功修改键：%s 的值为：%v", key, value)
		db.moveToFront(key)
		return true, nil
	}

	log.Printf("不存在键：%s，无法更新", key)
	return false, nil
}

// DeleteValue 删除指定键
func (db *InMemoryDB) DeleteValue(key string) error {
	if key == "" {
		return logError("键不能为空")
	}

	db.accessLock.Lock()
	defer db.accessLock.Unlock()

	if _, exists := db.keyValueStore[key]; exists {
		db.removeKey(key)
		log.Printf("已成功删除键: %s", key)
		return nil
	}
	log.Printf("不存在键: %s，无需删除", key)
	return nil
}

// GetKeyCount 获取数据库中键值对的数量
func (db *InMemoryDB) GetKeyCount() int {
	db.accessLock.RLock()
	defer db.accessLock.RUnlock()
	return len(db.keyValueStore)
}

// removeKey 内部删除键的方法
func (db *InMemoryDB) removeKey(key string) {
	delete(db.keyValueStore, key)
	delete(db.expirationTimes, key)
	if element, exists := db.lruCacheMap[key]; exists {
		db.lruCacheList.Remove(element)
		delete(db.lruCacheMap, key)
	}
}

// moveToFront 将键移到 LRU 链表头部
func (db *InMemoryDB) moveToFront(key string) {
	if element, exists := db.lruCacheMap[key]; exists {
		db.lruCacheList.MoveToFront(element)
	} else {
		element := db.lruCacheList.PushFront(key)
		db.lruCacheMap[key] = element
	}
}

// PeriodicCleanup 定期删除过期键
func (db *InMemoryDB) PeriodicCleanup(checkSize int) error {
	if checkSize < 0 {
		return logError("检查数量不能为负数")
	}

	db.accessLock.Lock()
	defer db.accessLock.Unlock()

	keys := make([]string, 0, len(db.expirationTimes))
	for key, expireTime := range db.expirationTimes {
		if time.Now().After(expireTime) {
			keys = append(keys, key)
		}
	}

	if len(keys) > 0 {
		if len(keys) < checkSize {
			checkSize = len(keys)
		}
		rand.Shuffle(len(keys), func(i, j int) { keys[i], keys[j] = keys[j], keys[i] })
		for _, key := range keys[:checkSize] {
			db.removeKey(key)
			log.Printf("定期清理：已成功删除过期键：%s", key)
		}
	}
	return nil
}

// performEviction 执行 LRU 淘汰
func (db *InMemoryDB) performEviction() error {
	log.Printf("内存已满(已存储超过：%d 个键值对)，通过 LRU 机制淘汰：%f 比例的键", db.maxCapacity, db.evictionRatio)
	evictionCount := int(float64(db.maxCapacity) * db.evictionRatio)
	if evictionCount < 1 {
		log.Printf("淘汰比例过小，已删除最少一个键")
		evictionCount = 1
	}

	for i := 0; i < evictionCount && db.lruCacheList.Len() > 0; i++ {
		tailElement := db.lruCacheList.Back()
		if tailElement == nil {
			return logError("LRU 链表为空，无法进行淘汰操作")
		}
		key := tailElement.Value.(string)
		db.removeKey(key)
		log.Printf("LRU 淘汰：已成功删除键：%s", key)
	}
	return nil
}

// logError 记录错误日志并返回错误
func logError(msg string) error {
	log.Printf("错误: %s", msg)
	return &DBError{Message: msg}
}

// DBError 自定义错误类型
type DBError struct {
	Message string
}

func (e *DBError) Error() string {
	return e.Message
}
