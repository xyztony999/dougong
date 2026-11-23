/**
 * 创建入口可达性测试（2026-11-23 回归锁定）：
 * v0.3.0 曾出现「弹窗/处理函数/API Action/后端端点全在，但界面触发按钮为零」
 * 的交付断裂——四个创建弹窗 show*Create 只有置 false 的赋值，用户实机「只能看」。
 * 本测试以视图源码（?raw 零依赖）断言每个创建动作的入口链路真实连通：
 *   工具栏按钮（admin 专属）→ open* 函数 → show*Create = true → 弹窗 v-model。
 * 挂载级组件测试沿全仓先例不引入（无 @vue/test-utils 依赖），以源码断言承载。
 */
import { describe, expect, it } from 'vitest'
import viewSource from '../index.vue?raw'
import { DK_ERROR_TEXT } from '@/types/docker'

/** 四个创建动作：工具栏按钮文案 → 打开弹窗函数 → 弹窗 show 变量（拍板参数集 2026-11-21） */
const CREATE_ENTRIES = [
  { label: '创建容器', open: 'openContainerCreate', show: 'showContainerCreate', modalTitle: '创建容器' },
  { label: '新建项目', open: 'openComposeCreate', show: 'showComposeCreate', modalTitle: '新建 Compose 项目' },
  { label: '创建卷', open: 'openVolumeCreate', show: 'showVolumeCreate', modalTitle: '创建卷' },
  { label: '创建网络', open: 'openNetworkCreate', show: 'showNetworkCreate', modalTitle: '创建网络' }
] as const

describe('容器管理创建入口可达性（入口缺失回归锁定）', () => {
  it.each(CREATE_ENTRIES)('$label：按钮 → open 函数 → show 置真 → 弹窗 全链连通', ({ label, open, show, modalTitle }) => {
    // 1) 工具栏触发按钮存在，且 admin 专属（readonly 负空间：不渲染非禁用）
    const buttonPattern = new RegExp(
      `v-if="authStore\\.isAdmin"[^>]*@click="${open}\\(\\)"[^>]*>\\s*${label}`
    )
    expect(viewSource, `工具栏缺少「${label}」按钮或未绑定 ${open}()`).toMatch(buttonPattern)
    // 2) open 函数定义存在且确实把弹窗置真（v0.3.0 断点：show*Create 从未 = true）
    expect(viewSource, `缺少 function ${open}() 定义`).toMatch(`function ${open}(`)
    expect(viewSource, `${show} 没有任何置 true 的入口（弹窗永远打不开）`).toContain(
      `${show}.value = true`
    )
    // 3) 弹窗本体存在且绑定同一 show 变量
    expect(viewSource, `缺少标题为「${modalTitle}」的弹窗`).toContain(`title="${modalTitle}"`)
    expect(viewSource, `弹窗未绑定 ${show}`).toContain(`v-model:show="${show}"`)
  })

  it('创建类弹窗均为 admin 专属渲染（readonly 不渲染非禁用——ADR-018 负空间）', () => {
    for (const show of CREATE_ENTRIES.map((e) => e.show)) {
      const modalPattern = new RegExp(
        `<n-modal v-if="authStore\\.isAdmin"[^>]*v-model:show="${show}"`
      )
      expect(viewSource, `${show} 弹窗缺少 admin 专属 v-if`).toMatch(modalPattern)
    }
  })

  it('容器创建表单覆盖拍板参数集（镜像/名称/端口多组/环境变量多组/挂载多组/重启策略/网络选择）', () => {
    for (const field of ['容器名', '镜像', '端口映射', '环境变量', '挂载', '重启策略', '网络']) {
      expect(viewSource, `容器创建表单缺少「${field}」字段`).toContain(`label="${field}"`)
    }
    // 多组 = 动态增删行
    for (const addLabel of ['添加端口映射', '添加环境变量', '添加挂载']) {
      expect(viewSource, `缺少「${addLabel}」动态行`).toContain(addLabel)
    }
    // 网络下拉数据源与留空默认 bridge 语义
    expect(viewSource).toContain('networkOptions')
    expect(viewSource).toContain('留空使用默认 bridge')
  })

  it('创建失败保留弹窗修正重试（沿拉取 Dialog 口径：show*Create 置 false 仅在成功路径）', () => {
    // 每个 do*Create 中 catch 分支不得关闭弹窗（源码顺序：catch 在 show*=false 之后）
    for (const fn of ['doContainerCreate', 'doVolumeCreate', 'doNetworkCreate', 'doComposeCreate']) {
      const body = viewSource.slice(viewSource.indexOf(`async function ${fn}`))
      const catchIdx = body.indexOf('} catch (e)')
      const closeIdx = body.search(/show\w+Create\.value = false/)
      expect(catchIdx, `${fn} 缺少 catch 分支`).toBeGreaterThan(-1)
      expect(closeIdx, `${fn} 缺少成功关闭弹窗路径`).toBeGreaterThan(-1)
      expect(closeIdx, `${fn} 的弹窗关闭不应出现在 catch 之后（失败要保持弹窗打开）`).toBeLessThan(catchIdx)
    }
  })

  it('DK_019~027 错误文案映射完整（前端 errText/内联展示依赖 DK_ERROR_TEXT）', () => {
    for (let i = 19; i <= 27; i++) {
      const code = `DK_0${i}`
      expect(DK_ERROR_TEXT[code], `${code} 缺少前端文案`).toBeTruthy()
    }
  })
})
