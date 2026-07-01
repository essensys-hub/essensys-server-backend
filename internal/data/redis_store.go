package data

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"


	"github.com/go-redis/redis/v8"
	"github.com/essensys-hub/essensys-server-backend/pkg/protocol"
)

// RedisStore implements Store interface with Redis backing
type RedisStore struct {
	client *redis.Client
	ctx    context.Context
}

// NewRedisStore creates a new RedisStore instance
func NewRedisStore(addr, password string, db int) *RedisStore {
	rdb := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password, // no password set
		DB:       db,       // use default DB
	})

	return &RedisStore{
		client: rdb,
		ctx:    context.Background(),
	}
}

// Helper to generate keys
func (rs *RedisStore) getExchangeKey(clientID string) string {
	if clientID == "" {
		clientID = "default"
	}
	return fmt.Sprintf("essensys:client:%s:exchange", clientID)
}

func (rs *RedisStore) getConnectedKey(clientID string) string {
	if clientID == "" {
		clientID = "default"
	}
	return fmt.Sprintf("essensys:client:%s:connected", clientID)
}

func (rs *RedisStore) getGlobalActionQueueKey() string {
	return "essensys:global:actions"
}

// -- Exchange Table Operations --

func (rs *RedisStore) GetValue(clientID string, index int) (string, bool) {
	val, err := rs.client.HGet(rs.ctx, rs.getExchangeKey(clientID), strconv.Itoa(index)).Result()
	if err == redis.Nil {
		return "", false
	}
	if err != nil {
		// Log error? In this interface we just return false
		return "", false
	}
	return val, true
}

func (rs *RedisStore) SetValue(clientID string, index int, value string) {
	rs.client.HSet(rs.ctx, rs.getExchangeKey(clientID), strconv.Itoa(index), value)
}

func (rs *RedisStore) GetAllValues(clientID string, indices []int) []protocol.ExchangeKV {
	if len(indices) == 0 {
		return []protocol.ExchangeKV{}
	}

	// Redis HMGet takes fields as variadic strings
	fields := make([]string, len(indices))
	for i, idx := range indices {
		fields[i] = strconv.Itoa(idx)
	}

	// Execute HMGet
	vals, err := rs.client.HMGet(rs.ctx, rs.getExchangeKey(clientID), fields...).Result()
	if err != nil {
		return []protocol.ExchangeKV{}
	}

	result := make([]protocol.ExchangeKV, 0, len(indices))
	for i, val := range vals {
		// val is interface{}, can be nil if not found
		if val != nil {
			if strVal, ok := val.(string); ok {
				result = append(result, protocol.ExchangeKV{
					K: indices[i],
					V: strVal,
				})
			}
		}
	}
	return result
}

func (rs *RedisStore) GetFullTable(clientID string) map[int]string {
	vals, err := rs.client.HGetAll(rs.ctx, rs.getExchangeKey(clientID)).Result()
	if err != nil {
		return map[int]string{}
	}

	result := make(map[int]string)
	for k, v := range vals {
		if index, err := strconv.Atoi(k); err == nil {
			result[index] = v
		}
	}
	return result
}

// -- Action Queue Operations --

// Note: MemoryStore uses a global queue. We replicate this behavior.
// We store actions as JSON strings in a Redis List.

func (rs *RedisStore) EnqueueAction(clientID string, action protocol.Action) {
	data, err := json.Marshal(action)
	if err != nil {
		return
	}
	// RPUSH to add to the end of the queue
	err = rs.client.RPush(rs.ctx, rs.getGlobalActionQueueKey(), string(data)).Err()
    if err != nil {
        fmt.Printf("[Redis] Error enqueuing action: %v\n", err)
    }
}

func (rs *RedisStore) DequeueActions(clientID string) []protocol.Action {
	// LRANGE 0 -1 to get all elements without removing
	vals, err := rs.client.LRange(rs.ctx, rs.getGlobalActionQueueKey(), 0, -1).Result()
	if err != nil {
        fmt.Printf("[Redis] Error dequeuing actions: %v\n", err)
		return []protocol.Action{}
	}

	actions := make([]protocol.Action, 0, len(vals))
	for _, v := range vals {
		var act protocol.Action
		if err := json.Unmarshal([]byte(v), &act); err == nil {
			actions = append(actions, act)
		}
	}
	return actions
}

func (rs *RedisStore) AcknowledgeAction(clientID string, guid string) (*protocol.Action, bool) {
	// To remove a specific item from a Redis List by value (LREM), we need the exact value (JSON).
	// Since we don't have the original JSON here, we have to fetch, find, and remove.
	// WARNING: This is a race condition prone implementation if multiple consumers exist,
	// but acceptable for this specific legacy context where concurrency is low.
	
	key := rs.getGlobalActionQueueKey()
	vals, err := rs.client.LRange(rs.ctx, key, 0, -1).Result()
	if err != nil {
		return nil, false
	}

	for _, v := range vals {
		var act protocol.Action
		if err := json.Unmarshal([]byte(v), &act); err == nil {
			if act.GUID == guid {
				// Found it. Remove 1 occurrence of this exact value.
				rs.client.LRem(rs.ctx, key, 1, v)
				return &act, true
			}
		}
	}
	return nil, false
}

// -- Client Management --

func (rs *RedisStore) IsClientConnected(clientID string) bool {
	// Check if key exists and is true. 
	// We might also check TTL if we implemented heartbeats, but following MemoryStore logic:
	val, err := rs.client.Get(rs.ctx, rs.getConnectedKey(clientID)).Result()
	if err != nil {
		return false
	}
	return val == "true"
}

func (rs *RedisStore) SetClientConnected(clientID string, connected bool) {
	key := rs.getConnectedKey(clientID)
	if connected {
		rs.client.Set(rs.ctx, key, "true", 0) // No expiration for now
	} else {
		rs.client.Set(rs.ctx, key, "false", 0)
	}
}

func (rs *RedisStore) getLastPollKey(clientID string) string {
	return fmt.Sprintf("essensys:client:%s:last_poll", clientID)
}

func (rs *RedisStore) RecordClientPoll(clientID string, at time.Time) {
	rs.client.Set(rs.ctx, rs.getLastPollKey(clientID), at.UTC().Format(time.RFC3339Nano), 0)
	rs.client.Set(rs.ctx, "essensys:meta:last_polled_client", clientID, 0)
}

func (rs *RedisStore) GetClientLastPoll(clientID string) (time.Time, bool) {
	val, err := rs.client.Get(rs.ctx, rs.getLastPollKey(clientID)).Result()
	if err != nil {
		return time.Time{}, false
	}
	t, err := time.Parse(time.RFC3339Nano, val)
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}

func (rs *RedisStore) GetLastPolledClientID() (string, bool) {
	val, err := rs.client.Get(rs.ctx, "essensys:meta:last_polled_client").Result()
	if err != nil || val == "" {
		return "", false
	}
	return val, true
}

// -- Auth Info Management --

func (rs *RedisStore) getAuthInfoKey(clientID string) string {
	if clientID == "" {
		clientID = "default"
	}
	return fmt.Sprintf("essensys:client:%s:authinfo", clientID)
}

func (rs *RedisStore) SetAuthInfo(clientID, ip, auth, version string) {
	key := rs.getAuthInfoKey(clientID)
	rs.client.HSet(rs.ctx, key, map[string]interface{}{
		"ip":      ip,
		"auth":    auth,
		"version": version,
		"updated": time.Now().Format(time.RFC3339),
	})
}

func (rs *RedisStore) GetAuthInfo(clientID string) (string, string, string, bool) {
	key := rs.getAuthInfoKey(clientID)
	vals, err := rs.client.HMGet(rs.ctx, key, "ip", "auth", "version").Result()
	if err != nil {
		return "", "", "", false
	}
	
	if len(vals) < 3 || vals[0] == nil {
		return "", "", "", false
	}
	
	ip, _ := vals[0].(string)
	auth, _ := vals[1].(string)
	version, _ := vals[2].(string)
	
	return ip, auth, version, true
}
