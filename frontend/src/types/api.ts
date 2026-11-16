/**
 * API 公共类型 —— 与 api-account-auth.md 3.1 接口规范对齐（唯一事实源）：
 * 响应格式 `{"code": 0, "message": "ok", "data": {...}}`；code=0 成功（number），
 * 非 0 为 3.4 错误码标识（string，如 "AUTH_001"）。
 * 分页公共约定：query 用 page/pageSize（page 从 1 起），data 为 {items, total}。
 */
export interface Envelope<T> {
  code: number | string
  message: string
  data: T
}

export interface ListResult<T> {
  items: T[]
  total: number
}

export interface ListQuery {
  page: number
  pageSize: number
}
