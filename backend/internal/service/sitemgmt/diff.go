package sitemgmt

import "strings"

// unifiedDiff 生成两段文本的行级差异（SITE_003 data.diff，供排障；unified
// 风格 -/+ 行）。规模防御：任一侧超 5000 行时退化为整体替换标记（LCS 矩阵
// 空间约束；diff 为排障呈现非正确性依赖）。
func unifiedDiff(a, b string) string {
	al, bl := splitLines(a), splitLines(b)
	if len(al) > 5000 || len(bl) > 5000 {
		return "--- original (too large to diff, replaced wholesale)\n+++ modified\n"
	}
	ops := lcsDiffOps(al, bl)
	if len(ops) == 0 {
		return ""
	}
	var sb strings.Builder
	for _, op := range ops {
		switch op.kind {
		case opEqual:
		case opDelete:
			sb.WriteString("-")
			sb.WriteString(al[op.a])
			sb.WriteString("\n")
		case opInsert:
			sb.WriteString("+")
			sb.WriteString(bl[op.b])
			sb.WriteString("\n")
		}
	}
	return sb.String()
}

type diffOpKind int

const (
	opEqual diffOpKind = iota
	opDelete
	opInsert
)

type diffOp struct {
	kind diffOpKind
	a, b int
}

// lcsDiffOps 经典 LCS 回溯产出删除/插入序列（O(n·m) DP；KB 级配置文件适用）。
func lcsDiffOps(a, b []string) []diffOp {
	n, m := len(a), len(b)
	dp := make([][]int, n+1)
	for i := range dp {
		dp[i] = make([]int, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if a[i] == b[j] {
				dp[i][j] = dp[i+1][j+1] + 1
			} else if dp[i+1][j] >= dp[i][j+1] {
				dp[i][j] = dp[i+1][j]
			} else {
				dp[i][j] = dp[i][j+1]
			}
		}
	}
	var ops []diffOp
	i, j := 0, 0
	for i < n && j < m {
		switch {
		case a[i] == b[j]:
			ops = append(ops, diffOp{opEqual, i, j})
			i++
			j++
		case dp[i+1][j] >= dp[i][j+1]:
			ops = append(ops, diffOp{opDelete, i, j})
			i++
		default:
			ops = append(ops, diffOp{opInsert, i, j})
			j++
		}
	}
	for ; i < n; i++ {
		ops = append(ops, diffOp{opDelete, i, j})
	}
	for ; j < m; j++ {
		ops = append(ops, diffOp{opInsert, i, j})
	}
	return ops
}

// splitLines 按行切分（保留 CRLF 归一为 LF 后内容，不含行尾换行）。
func splitLines(s string) []string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.TrimSuffix(s, "\n")
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}
