package fileman

import (
	"context"
	"strconv"
	"strings"

	"github.com/dougong/dougong/internal/errcode"
	"github.com/dougong/dougong/internal/service/audit"
	authsvc "github.com/dougong/dougong/internal/service/auth"
)

// Chmod POST /files/entry/chmod（admin+CSRF）：mode ∈ 0~0777 八进制串
// （FILE_015）；仅目标自身不递归（D-06）。
func (s *Service) Chmod(ctx context.Context, actor *authsvc.Identity, path, mode string) (*ChmodData, error) {
	action := ActionFileChmod
	fail := func(err error) error {
		if isScopeReject(err) {
			return err
		}
		s.auditAction(ctx, actor, audit.CategoryChange, action, ObjectTypeFileEntry,
			path, audit.ResultFailure, detailError(err))
		return err
	}
	if !validModeString(mode) {
		return nil, fail(errcode.ErrFileInvalidPermission)
	}
	real, err := s.guard.Resolve(path)
	if err != nil {
		return nil, fail(s.mapResolveErr(err, errcode.ErrFileEntryNotFound))
	}
	if err := s.gw.Chmod(ctx, real, normalizeMode(mode)); err != nil {
		return nil, fail(s.mapFSErr(err))
	}
	s.auditAction(ctx, actor, audit.CategoryChange, action, ObjectTypeFileEntry,
		real, audit.ResultSuccess, map[string]any{"mode": normalizeMode(mode)})
	return &ChmodData{Path: real, Mode: normalizeMode(mode)}, nil
}

// Chown POST /files/entry/chown（admin+CSRF）：ownerUser/ownerGroup 至少一项；
// 候选实况存在校验（FILE_015，D-14 实时读取）；仅目标自身（D-06）。
func (s *Service) Chown(ctx context.Context, actor *authsvc.Identity,
	path, ownerUser, ownerGroup string) (*ChownData, error) {
	action := ActionFileChown
	fail := func(err error) error {
		if isScopeReject(err) {
			return err
		}
		s.auditAction(ctx, actor, audit.CategoryChange, action, ObjectTypeFileEntry,
			path, audit.ResultFailure, detailError(err))
		return err
	}
	if ownerUser == "" && ownerGroup == "" { // 至少一项（§4.2.4）
		return nil, fail(errcode.ErrFileInvalidPermission)
	}
	real, err := s.guard.Resolve(path)
	if err != nil {
		return nil, fail(s.mapResolveErr(err, errcode.ErrFileEntryNotFound))
	}
	// 候选实况校验 + uid/gid 解析（D-14：实时读取无缓存，单一校验通道）。
	accounts, err := s.gw.ListSystemAccounts(ctx)
	if err != nil {
		return nil, fail(s.mapFSErr(err))
	}
	uid, gid := -1, -1 // -1 = 不变（gateway 契约）
	for _, u := range accounts.Users {
		if ownerUser != "" && u.Name == ownerUser {
			uid = u.UID
			break
		}
	}
	for _, g := range accounts.Groups {
		if ownerGroup != "" && g.Name == ownerGroup {
			gid = g.GID
			break
		}
	}
	if (ownerUser != "" && uid < 0) || (ownerGroup != "" && gid < 0) {
		return nil, fail(errcode.ErrFileInvalidPermission) // 候选不存在（FILE_015）
	}
	if err := s.gw.Chown(ctx, real, uid, gid); err != nil {
		return nil, fail(s.mapFSErr(err))
	}
	// 响应携带修改后实况（未变更项取当前快照值）。
	snap, err := s.gw.Stat(ctx, real)
	if err != nil {
		snap = EntrySnapshot{}
	}
	user, group := ownerUser, ownerGroup
	if user == "" {
		user = snap.OwnerUser
	}
	if group == "" {
		group = snap.OwnerGroup
	}
	s.auditAction(ctx, actor, audit.CategoryChange, action, ObjectTypeFileEntry,
		real, audit.ResultSuccess, map[string]any{"ownerUser": user, "ownerGroup": group})
	return &ChownData{Path: real, OwnerUser: user, OwnerGroup: group}, nil
}

// SystemAccounts GET /files/system-accounts/list（admin）：实时读取系统用户
// 与组清单（D-14；仅 name/uid、name/gid 下发——载荷考虑）。查看类不审计。
func (s *Service) SystemAccounts(ctx context.Context) (*SystemAccountsData, error) {
	accounts, err := s.gw.ListSystemAccounts(ctx)
	if err != nil {
		return nil, s.mapFSErr(err)
	}
	return &SystemAccountsData{Users: accounts.Users, Groups: accounts.Groups}, nil
}

// validModeString 校验八进制权限位形态：0?[0-7]{3} 且值 ≤ 0777（FILE_015）。
func validModeString(mode string) bool {
	if len(mode) != 3 && len(mode) != 4 {
		return false
	}
	if len(mode) == 4 && mode[0] != '0' {
		return false
	}
	digits := mode
	if len(mode) == 4 {
		digits = mode[1:]
	}
	for _, r := range digits {
		if r < '0' || r > '7' {
			return false
		}
	}
	v, err := strconv.ParseUint(digits, 8, 32)
	if err != nil {
		return false
	}
	return v <= 0o777
}

// normalizeMode 归一为四位八进制形态（如 "0644"）。
func normalizeMode(mode string) string {
	if len(mode) == 3 {
		return "0" + mode
	}
	return strings.TrimSpace(mode)
}
