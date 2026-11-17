package dbmgmt

import (
	"context"
	"strconv"
	"time"

	"github.com/dougong/dougong/internal/errcode"
	authsvc "github.com/dougong/dougong/internal/service/auth"
	storedb "github.com/dougong/dougong/internal/store/dbmgmt"
)

// RedisInfo GET /db/redis/info：info 概览（REDIS_INFO_FIELDS 白名单 sections
// 骨架，D-08；I-04 实时读取）。
func (s *Service) RedisInfo(ctx context.Context, instanceID string) (*RedisInfoData, error) {
	_, t, c, err := s.governTarget(ctx, instanceID, familyRedis)
	if err != nil {
		return nil, err
	}
	sections, err := s.redis.InfoFields(ctx, t, c)
	if err != nil {
		return nil, translateGatewayErr(err)
	}
	out := make(map[string]map[string]string, 4)
	for _, f := range RedisInfoFields {
		if out[f.Section] == nil {
			out[f.Section] = map[string]string{}
		}
		if sec, ok := sections[f.Section]; ok {
			if v, ok := sec[f.Source]; ok {
				out[f.Section][f.Field] = v
			}
		}
	}
	return &RedisInfoData{Sections: out}, nil
}

// RedisSlowlog GET /db/redis/slowlog：慢日志清单（limit 默认 50 上限 200，
// D-09；timestamp ISO-8601 UTC）。
func (s *Service) RedisSlowlog(ctx context.Context, instanceID, limitRaw string) (*SlowlogData, error) {
	_, t, c, err := s.governTarget(ctx, instanceID, familyRedis)
	if err != nil {
		return nil, err
	}
	limit := SlowlogDefault
	if limitRaw != "" {
		v, perr := strconv.Atoi(limitRaw)
		if perr != nil || v < 1 || v > SlowlogLimit {
			return nil, errcode.ErrValidation.WithFields(errcode.FieldError{
				Field: "limit", Message: "limit 须为 1~200 的整数"})
		}
		limit = v
	}
	entries, err := s.redis.Slowlog(ctx, t, c, limit)
	if err != nil {
		return nil, translateGatewayErr(err)
	}
	items := make([]SlowlogItem, 0, len(entries))
	for _, e := range entries {
		items = append(items, SlowlogItem{
			ID: e.ID, Timestamp: time.Unix(e.Timestamp, 0).UTC(),
			DurationUs: e.DurationUs, Command: e.Command,
		})
	}
	return &SlowlogData{Items: items, GeneratedAt: s.clock().UTC()}, nil
}

// SetRedisPassword POST /db/redis/password：改密两步同步（D-05：面板建连
// 即旧密码验证——requirepass 语义天然满足「验证旧密码」）。
func (s *Service) SetRedisPassword(ctx context.Context, actor *authsvc.Identity, req PasswordRequest) (*PasswordChangeData, error) {
	inst, t, c, err := s.governTargetByID(ctx, req.InstanceID, familyRedis)
	if err != nil {
		return nil, err
	}
	if err := validatePassword("newPassword", req.NewPassword); err != nil {
		return nil, err
	}
	_ = storedb.EngineRedis // 引用引擎常量（governTarget 已保证 engine=redis）
	return s.changePassword(ctx, actor, inst, t, c, req.NewPassword, ActionRedisPasswordSet)
}

// ListKeys GET /db/key/list：SCAN 游标分页（D-09：cursor 默认 0、MATCH
// pattern 默认 *、COUNT 200；pattern 校验 DBM_017 禁 KEYS 全量语义）。
func (s *Service) ListKeys(ctx context.Context, instanceID, cursorRaw, pattern string) (*KeyListData, error) {
	_, t, c, err := s.governTarget(ctx, instanceID, familyRedis)
	if err != nil {
		return nil, err
	}
	if cursorRaw == "" {
		cursorRaw = "0"
	}
	if _, cerr := strconv.ParseUint(cursorRaw, 10, 64); cerr != nil {
		return nil, errcode.ErrValidation.WithFields(errcode.FieldError{
			Field: "cursor", Message: "cursor 须为非负整数"})
	}
	if pattern == "" {
		pattern = "*"
	}
	if err := validateKeyPattern(pattern); err != nil {
		return nil, err
	}
	next, keys, err := s.redis.ScanKeys(ctx, t, c, cursorRaw, pattern, KeyScanCount)
	if err != nil {
		return nil, translateGatewayErr(err)
	}
	items := make([]KeyItem, 0, len(keys))
	for _, k := range keys {
		items = append(items, KeyItem{Key: k.Key, Type: k.Type})
	}
	return &KeyListData{
		NextCursor: next, Complete: next == "0",
		Items: items, GeneratedAt: s.clock().UTC(),
	}, nil
}

// KeyTTL GET /db/key/ttl：单 key TTL（-1=无过期如实；不存在 DBM_015）。
func (s *Service) KeyTTL(ctx context.Context, instanceID, key string) (*KeyTTLData, error) {
	_, t, c, err := s.governTarget(ctx, instanceID, familyRedis)
	if err != nil {
		return nil, err
	}
	if err := validateRedisKey(key); err != nil {
		return nil, err
	}
	ttl, err := s.redis.KeyTTL(ctx, t, c, key)
	if err != nil {
		return nil, translateGatewayErr(err)
	}
	return &KeyTTLData{Key: key, TTLSec: ttl}, nil
}
