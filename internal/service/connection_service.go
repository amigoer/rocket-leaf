// Package service 提供业务服务层
package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"rocket-leaf/internal/model"
	"rocket-leaf/internal/rocketmq"
)

const (
	appConfigDirName         = "rocket-leaf"
	connectionDataFileName   = "connections.json"
	defaultConnectionTimeout = 5
)

type connectionStore struct {
	Connections []*model.Connection `json:"connections"`
}

// ConnectionService 连接管理服务
type ConnectionService struct {
	mu           sync.RWMutex
	connections  map[int]*model.Connection // 连接配置列表
	nextID       int                       // 下一个连接ID
	dataFilePath string                    // 连接配置持久化文件路径
}

// NewConnectionService 创建连接管理服务
func NewConnectionService() *ConnectionService {
	service := &ConnectionService{
		connections:  make(map[int]*model.Connection),
		nextID:       1,
		dataFilePath: resolveConnectionDataFilePath(),
	}

	if err := service.loadConnectionsFromFile(); err != nil {
		log.Printf("[ConnectionService] 加载连接配置失败: %v", err)
	}

	return service
}

func resolveConnectionDataFilePath() string {
	configDir, err := os.UserConfigDir()
	if err != nil || strings.TrimSpace(configDir) == "" {
		return connectionDataFileName
	}

	return filepath.Join(configDir, appConfigDirName, connectionDataFileName)
}

func normalizeConnectionEnv(env model.ConnectionEnv) model.ConnectionEnv {
	if env != model.EnvProduction && env != model.EnvTest && env != model.EnvDevelopment {
		return model.EnvDevelopment
	}

	return env
}

func normalizeACLConfig(enableACL bool, accessKey string, secretKey string) (bool, string, string, error) {
	accessKey = strings.TrimSpace(accessKey)
	secretKey = strings.TrimSpace(secretKey)

	if !enableACL {
		return false, "", "", nil
	}

	if accessKey == "" {
		return false, "", "", fmt.Errorf("启用 ACL 时 AccessKey 不能为空")
	}

	if secretKey == "" {
		return false, "", "", fmt.Errorf("启用 ACL 时 SecretKey 不能为空")
	}

	return true, accessKey, secretKey, nil
}

func normalizeTimeoutSec(timeoutSec int) int {
	if timeoutSec <= 0 {
		return defaultConnectionTimeout
	}

	return timeoutSec
}

func (s *ConnectionService) loadConnectionsFromFile() error {
	data, err := os.ReadFile(s.dataFilePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}

	var store connectionStore
	if err := json.Unmarshal(data, &store); err != nil {
		return err
	}

	loaded := make(map[int]*model.Connection, len(store.Connections))
	nextID := 1
	hasDefault := false

	for _, conn := range store.Connections {
		if conn == nil {
			continue
		}

		current := *conn

		if current.ID <= 0 {
			current.ID = nextID
		}
		if current.ID >= nextID {
			nextID = current.ID + 1
		}
		if _, exists := loaded[current.ID]; exists {
			current.ID = nextID
			nextID++
		}

		current.Env = normalizeConnectionEnv(current.Env)
		current.Status = model.StatusOffline
		current.LastCheck = "-"
		if current.TimeoutSec <= 0 {
			current.TimeoutSec = defaultConnectionTimeout
		}

		enableACL, accessKey, secretKey, err := normalizeACLConfig(current.EnableACL, current.AccessKey, current.SecretKey)
		if err != nil {
			enableACL = false
			accessKey = ""
			secretKey = ""
		}
		current.EnableACL = enableACL
		current.AccessKey = accessKey
		current.SecretKey = secretKey

		if current.IsDefault {
			if hasDefault {
				current.IsDefault = false
			} else {
				hasDefault = true
			}
		}

		connCopy := current
		loaded[current.ID] = &connCopy
	}

	if len(loaded) > 0 && !hasDefault {
		minID := 0
		for id := range loaded {
			if minID == 0 || id < minID {
				minID = id
			}
		}
		loaded[minID].IsDefault = true
	}

	s.connections = loaded
	s.nextID = nextID

	return nil
}

func (s *ConnectionService) saveConnectionsLocked() error {
	connections := make([]*model.Connection, 0, len(s.connections))
	for _, conn := range s.connections {
		if conn == nil {
			continue
		}
		connCopy := *conn
		connections = append(connections, &connCopy)
	}

	sort.Slice(connections, func(i, j int) bool {
		return connections[i].ID < connections[j].ID
	})

	data, err := json.MarshalIndent(connectionStore{Connections: connections}, "", "  ")
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(s.dataFilePath), 0o755); err != nil {
		return err
	}

	tempFilePath := s.dataFilePath + ".tmp"
	if err := os.WriteFile(tempFilePath, data, 0o600); err != nil {
		return err
	}

	if err := os.Rename(tempFilePath, s.dataFilePath); err != nil {
		_ = os.Remove(tempFilePath)
		return err
	}

	return nil
}

// formatNow 格式化当前时间
func formatNow() string {
	return time.Now().Format("2006-01-02 15:04:05")
}

// GetConnections 获取所有连接配置
func (s *ConnectionService) GetConnections() []*model.Connection {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*model.Connection, 0, len(s.connections))
	for _, conn := range s.connections {
		result = append(result, conn)
	}
	return result
}

// GetConnection 获取单个连接配置
func (s *ConnectionService) GetConnection(id int) (*model.Connection, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	conn, exists := s.connections[id]
	if !exists {
		return nil, fmt.Errorf("连接不存在: %d", id)
	}
	return conn, nil
}

// AddConnection 添加新连接
func (s *ConnectionService) AddConnection(name string, env string, nameServer string, timeoutSec int, enableACL bool, accessKey string, secretKey string, remark string) (*model.Connection, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 验证环境类型
	connEnv := normalizeConnectionEnv(model.ConnectionEnv(env))

	enableACL, accessKey, secretKey, err := normalizeACLConfig(enableACL, accessKey, secretKey)
	if err != nil {
		return nil, err
	}

	conn := &model.Connection{
		ID:         s.nextID,
		Name:       name,
		Env:        connEnv,
		NameServer: nameServer,
		TimeoutSec: normalizeTimeoutSec(timeoutSec),
		EnableACL:  enableACL,
		AccessKey:  accessKey,
		SecretKey:  secretKey,
		Status:     model.StatusOffline,
		LastCheck:  "-",
		IsDefault:  len(s.connections) == 0, // 第一个连接自动设为默认
		Remark:     remark,
	}

	s.connections[s.nextID] = conn
	s.nextID++

	if err := s.saveConnectionsLocked(); err != nil {
		delete(s.connections, conn.ID)
		s.nextID--
		return nil, fmt.Errorf("保存连接配置失败: %w", err)
	}

	return conn, nil
}

// UpdateConnection 更新连接配置
func (s *ConnectionService) UpdateConnection(id int, name string, env string, nameServer string, timeoutSec int, enableACL bool, accessKey string, secretKey string, remark string) (*model.Connection, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	conn, exists := s.connections[id]
	if !exists {
		return nil, fmt.Errorf("连接不存在: %d", id)
	}

	oldNameServer := conn.NameServer

	// 验证环境类型
	connEnv := normalizeConnectionEnv(model.ConnectionEnv(env))

	enableACL, accessKey, secretKey, err := normalizeACLConfig(enableACL, accessKey, secretKey)
	if err != nil {
		return nil, err
	}

	oldConn := *conn

	conn.Name = name
	conn.Env = connEnv
	conn.NameServer = nameServer
	conn.TimeoutSec = normalizeTimeoutSec(timeoutSec)
	conn.EnableACL = enableACL
	conn.AccessKey = accessKey
	conn.SecretKey = secretKey
	conn.Remark = remark

	if err := s.saveConnectionsLocked(); err != nil {
		*conn = oldConn
		return nil, fmt.Errorf("保存连接配置失败: %w", err)
	}

	// 如果 NameServer 地址变更，需要移除旧客户端
	if oldNameServer != nameServer {
		rocketmq.GetClientManager().RemoveClient(oldNameServer)
	}

	return conn, nil
}

// DeleteConnection 删除连接
func (s *ConnectionService) DeleteConnection(id int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	conn, exists := s.connections[id]
	if !exists {
		return fmt.Errorf("连接不存在: %d", id)
	}

	// 不允许删除默认连接（如果还有其他连接）
	if conn.IsDefault && len(s.connections) > 1 {
		return fmt.Errorf("不能删除默认连接，请先设置其他连接为默认")
	}

	nameServer := conn.NameServer

	delete(s.connections, id)

	// 如果删除后还有连接，设置第一个为默认
	newDefaultID := 0
	if len(s.connections) > 0 && conn.IsDefault {
		for _, c := range s.connections {
			c.IsDefault = true
			newDefaultID = c.ID
			break
		}
	}

	deletedConn := *conn
	if err := s.saveConnectionsLocked(); err != nil {
		s.connections[id] = &deletedConn
		if newDefaultID != 0 {
			if newDefaultConn, ok := s.connections[newDefaultID]; ok {
				newDefaultConn.IsDefault = false
			}
		}
		return fmt.Errorf("保存连接配置失败: %w", err)
	}

	// 移除客户端
	rocketmq.GetClientManager().RemoveClient(nameServer)

	return nil
}

// TestConnection 测试连接
func (s *ConnectionService) TestConnection(id int) (string, error) {
	s.mu.Lock()
	conn, exists := s.connections[id]
	if !exists {
		s.mu.Unlock()
		return "", fmt.Errorf("连接不存在: %d", id)
	}
	nameServer := conn.NameServer
	timeout := time.Duration(conn.TimeoutSec) * time.Second
	enableACL := conn.EnableACL
	accessKey := conn.AccessKey
	secretKey := conn.SecretKey
	s.mu.Unlock()

	// 测试连接
	err := rocketmq.GetClientManager().TestConnection(nameServer, timeout, enableACL, accessKey, secretKey)

	s.mu.Lock()
	defer s.mu.Unlock()

	// 更新连接状态
	if conn, exists := s.connections[id]; exists {
		conn.LastCheck = formatNow()
		if err == nil {
			conn.Status = model.StatusOnline
			return "online", nil
		} else {
			conn.Status = model.StatusOffline
			return "offline", err
		}
	}

	return "offline", err
}

// SetDefaultConnection 设置默认连接
func (s *ConnectionService) SetDefaultConnection(id int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	conn, exists := s.connections[id]
	if !exists {
		return fmt.Errorf("连接不存在: %d", id)
	}

	if conn.IsDefault {
		return rocketmq.GetClientManager().SetDefaultConnection(conn.NameServer)
	}

	previousDefaultNameServer := ""
	for _, c := range s.connections {
		if c.IsDefault {
			previousDefaultNameServer = c.NameServer
			break
		}
	}

	if err := rocketmq.GetClientManager().SetDefaultConnection(conn.NameServer); err != nil {
		return err
	}

	// 取消其他连接的默认状态
	for _, c := range s.connections {
		c.IsDefault = false
	}

	conn.IsDefault = true

	if err := s.saveConnectionsLocked(); err != nil {
		for _, c := range s.connections {
			c.IsDefault = false
			if c.NameServer == previousDefaultNameServer {
				c.IsDefault = true
			}
		}
		if previousDefaultNameServer != "" && previousDefaultNameServer != conn.NameServer {
			if resetErr := rocketmq.GetClientManager().SetDefaultConnection(previousDefaultNameServer); resetErr != nil {
				log.Printf("[ConnectionService] 回滚默认连接失败: %v", resetErr)
			}
		}
		return fmt.Errorf("保存连接配置失败: %w", err)
	}

	return nil
}

// ConnectDefault 连接默认连接
func (s *ConnectionService) ConnectDefault() error {
	s.mu.RLock()
	var defaultConn *model.Connection
	for _, conn := range s.connections {
		if conn.IsDefault {
			defaultConn = conn
			break
		}
	}
	s.mu.RUnlock()

	if defaultConn == nil {
		return fmt.Errorf("无默认连接配置")
	}

	timeout := time.Duration(defaultConn.TimeoutSec) * time.Second
	_, err := rocketmq.GetClientManager().CreateClient(
		defaultConn.NameServer,
		timeout,
		defaultConn.EnableACL,
		defaultConn.AccessKey,
		defaultConn.SecretKey,
	)
	if err != nil {
		return err
	}

	return rocketmq.GetClientManager().SetDefaultConnection(defaultConn.NameServer)
}

// Connect 连接指定连接
func (s *ConnectionService) Connect(id int) error {
	s.mu.RLock()
	conn, exists := s.connections[id]
	if !exists {
		s.mu.RUnlock()
		return fmt.Errorf("连接不存在: %d", id)
	}
	nameServer := conn.NameServer
	timeout := time.Duration(conn.TimeoutSec) * time.Second
	enableACL := conn.EnableACL
	accessKey := conn.AccessKey
	secretKey := conn.SecretKey
	s.mu.RUnlock()

	_, err := rocketmq.GetClientManager().CreateClient(nameServer, timeout, enableACL, accessKey, secretKey)
	if err != nil {
		return err
	}

	// 更新连接状态
	s.mu.Lock()
	if conn, exists := s.connections[id]; exists {
		conn.Status = model.StatusOnline
		conn.LastCheck = formatNow()
	}
	s.mu.Unlock()

	return nil
}

// Disconnect 断开指定连接
func (s *ConnectionService) Disconnect(id int) error {
	s.mu.Lock()
	conn, exists := s.connections[id]
	if !exists {
		s.mu.Unlock()
		return fmt.Errorf("连接不存在: %d", id)
	}
	nameServer := conn.NameServer
	wasDefault := conn.IsDefault
	s.mu.Unlock()

	rocketmq.GetClientManager().RemoveClient(nameServer)

	s.mu.Lock()
	if c, ok := s.connections[id]; ok {
		c.Status = model.StatusOffline
		c.LastCheck = formatNow()
	}
	var newDefaultNameServer string
	if wasDefault {
		for _, c := range s.connections {
			if c.ID != id {
				c.IsDefault = true
				newDefaultNameServer = c.NameServer
				break
			}
		}
	}
	err := s.saveConnectionsLocked()
	s.mu.Unlock()
	if err != nil {
		return err
	}
	if newDefaultNameServer != "" {
		_ = rocketmq.GetClientManager().SetDefaultConnection(newDefaultNameServer)
	}
	return nil
}
