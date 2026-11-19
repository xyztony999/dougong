package dbmgmt

import (
	"context"
	"fmt"

	"github.com/dougong/dougong/internal/errcode"
	storedb "github.com/dougong/dougong/internal/store/dbmgmt"
)

// 本文件是 database-mgmt 对 db-tools 的进程内 OHS（api-database-mgmt.md
// §4.9/§5.1 v2.4 登记 / api-db-tools.md D-10 消费契约兑现）：工具凭据取出
// 与实例就绪校验。vault 解密仅服务端（I-02：明文仅调用方内存态用毕即弃，
// 不落库不入日志不入审计）；本域不复制不持久化对方凭据。

// ToolCreds 工具凭据明文形态（db-tools D-10 契约；Host/Port 供 phpMyAdmin
// signon 建连与 mongo-express 连接串构造——socket 模式 Host=localhost/
// Port=0，mysqli localhost 语义走 socket）。
type ToolCreds struct {
	Username string
	Password string
	Host     string
	Port     int
	Engine   string
}

// ReadyInfo 实例就绪结论（存在+credentialsSet+引擎匹配；mongo-express 另需
// TCP 连接模式——工具经网络连接串建连）。
type ReadyInfo struct {
	Ready  bool
	Reason string
	Engine string
}

// InstanceReadyForTool 实例就绪校验（db-tools D-10：phpmyadmin→mysql/
// mariadb；mongo_express→mongodb；redis 无对应工具）。不要求可达（凭据
// 就绪即可打开工具，引擎启停归服务管理）。
func (s *Service) InstanceReadyForTool(ctx context.Context, instanceID int64) (ReadyInfo, error) {
	_, info, err := s.readyInstanceForTool(ctx, instanceID)
	if err != nil {
		return ReadyInfo{}, err
	}
	return info, nil
}

// InstanceCredentialsForTool 凭据取出（vault 解密仅服务端；就绪前置不满足
// 返回错误——调用方 db-tools 按 DBT_006/007 语义呈现）。
func (s *Service) InstanceCredentialsForTool(ctx context.Context, instanceID int64) (ToolCreds, error) {
	inst, info, err := s.readyInstanceForTool(ctx, instanceID)
	if err != nil {
		return ToolCreds{}, err
	}
	if !info.Ready {
		return ToolCreds{}, errcode.ErrDBMCredentialsNotSet.WithData(map[string]string{"reason": info.Reason})
	}
	creds, err := decryptCreds(s.vault, inst)
	if err != nil {
		return ToolCreds{}, err
	}
	host, port := "localhost", 0
	if inst.ConnMode == storedb.ConnModeTCP {
		if inst.Host != nil {
			host = *inst.Host
		}
		if inst.Port != nil {
			port = *inst.Port
		}
	}
	return ToolCreds{
		Username: creds.Username, Password: creds.Password,
		Host: host, Port: port, Engine: inst.Engine,
	}, nil
}

// readyInstanceForTool 就绪判定内部编排（档案存在+引擎匹配+凭据已配置+
// mongo TCP 形态）。
func (s *Service) readyInstanceForTool(ctx context.Context, instanceID int64) (*storedb.Instance, ReadyInfo, error) {
	inst, err := s.store.Instance().GetByID(ctx, instanceID)
	if err != nil {
		return nil, ReadyInfo{}, fmt.Errorf("dbmgmt: tool ready get instance: %w", err)
	}
	if inst == nil {
		return nil, ReadyInfo{Ready: false, Reason: "实例未纳管（请到数据库管理探测纳管）"}, nil
	}
	info := ReadyInfo{Engine: inst.Engine}
	switch {
	case storedb.IsMySQLEngine(inst.Engine):
		// phpmyadmin 目标引擎——就绪判定继续。
	case inst.Engine == storedb.EngineMongoDB:
		if inst.ConnMode != storedb.ConnModeTCP {
			info.Reason = "mongo-express 须 TCP 连接模式实例（socket 模式不支持连接串建连）"
			return inst, info, nil
		}
	default:
		info.Reason = "引擎无对应工具（phpmyadmin→mysql/mariadb；mongo_express→mongodb）"
		return inst, info, nil
	}
	if !inst.CredentialsSet() {
		info.Reason = "凭据未配置（请到数据库管理配置实例凭据）"
		return inst, info, nil
	}
	info.Ready = true
	return inst, info, nil
}
