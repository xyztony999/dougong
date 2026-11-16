/**
 * panel-core api 层 —— 7 Action 与 api-panel-core.md 接口一一对应（S7，无虚构接口）。
 * 命名遵循 Action 定义（PageSpec 第 2 节）。
 */
import type {
  CertUploadData,
  CertUploadReq,
  EntryPathRegeneratedData,
  MenuListData,
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
