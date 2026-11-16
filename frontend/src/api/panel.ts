/**
 * panel-core api 层 —— 10 Action 与 api-panel-core.md v2.0 接口一一对应（S7，无虚构接口）。
 * 命名遵循 Action 定义（PageSpec 第 2 节；v1.1 增量 +certStatus/certRequest/certRetryRenewal，
 * 域名绑定/解绑并入 configUpdate.bindDomain——api D-P12，不设独立接口）。
 */
import type {
  CertUploadData,
  CertUploadReq,
  EntryPathRegeneratedData,
  MenuListData,
  PanelCertActionResultData,
  PanelCertStatusData,
  PanelConfigDetail,
  PanelConfigUpdateReq,
  PanelVersionData,
  UpdateCheckResultData
} from '@/types/panel'
import { request } from './http'

/** ---- config（api 4.1.4） ---- */

export function configDetail(): Promise<PanelConfigDetail> {
  return request<PanelConfigDetail>('/panel/config/detail')
}

/** 部分字段集更新（各 Card 分域提交）；响应 = 更新后配置视图（结构同 detail） */
export function configUpdate(req: PanelConfigUpdateReq): Promise<PanelConfigDetail> {
  return request<PanelConfigDetail>('/panel/config/update', { method: 'POST', body: req })
}

export function entryPathRegenerate(): Promise<EntryPathRegeneratedData> {
  return request<EntryPathRegeneratedData>('/panel/config/regenerate-entry-path', {
    method: 'POST'
  })
}

export function certUpload(req: CertUploadReq): Promise<CertUploadData> {
  return request<CertUploadData>('/panel/config/upload-certificate', { method: 'POST', body: req })
}

/** ---- cert（api 4.1.4 v2.0：面板证书 ACME，HTTP-01 经服务器已有 nginx——site-mgmt 通道，D-P13/D-P15） ---- */

/** 状态视图（两角色；进入 settings 与 configDetail 并行拉取，申请/重试成功后 re-fetch） */
export function certStatus(): Promise<PanelCertStatusData> {
  return request<PanelCertStatusData>('/panel/cert/status')
}

/** ACME 申请（admin；同步终态 ≤ACME_TIMEOUT=120s——调用方需 loading 态 + 防重复触发，D-P13） */
export function certRequest(): Promise<PanelCertActionResultData> {
  return request<PanelCertActionResultData>('/panel/cert/request', { method: 'POST' })
}

/** renew_failed 手动重试（admin；进入 4.1.3 同一通道，同步终态同上） */
export function certRetryRenewal(): Promise<PanelCertActionResultData> {
  return request<PanelCertActionResultData>('/panel/cert/retry-renewal', { method: 'POST' })
}

/** ---- menu（D-P07：服务端按角色过滤，前端不二次过滤） ---- */

export function menuList(): Promise<MenuListData> {
  return request<MenuListData>('/panel/menu/list')
}

/** ---- system ---- */

export function versionGet(): Promise<PanelVersionData> {
  return request<PanelVersionData>('/panel/system/version')
}

/** 手动触发单次版本检查（I-P05 双重门控：开关开启 + admin 显式触发；无自动重试 D-P09） */
export function updateCheckRun(): Promise<UpdateCheckResultData> {
  return request<UpdateCheckResultData>('/panel/system/update-check', { method: 'POST' })
}
