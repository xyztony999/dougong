package panel

import (
	"fmt"
	"io"
	"regexp"
)

// entryPathCharset 是安全入口路径字符集（I-P03：^[A-Za-z0-9_-]{8,32}$）。
// 64 项字符集对 256 取模无偏（256 % 64 == 0）。
const entryPathCharset = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789_-"

// EntryPathPattern 是入口路径合法性正则（I-P03 生成器输出空间）。
var EntryPathPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{8,32}$`)

// GenerateEntryPath 生成随机安全入口路径（EntryPathGenerator 领域服务，
// api §4.1.1）：固定 16 字符（96 bit 熵）。D-P03：作为 URL 前缀段使用，
// 字符集不含路径分隔与保留字符。
func GenerateEntryPath(r io.Reader) (string, error) {
	raw := make([]byte, entryPathLen)
	if _, err := io.ReadFull(r, raw); err != nil {
		return "", fmt.Errorf("read entropy: %w", err)
	}
	out := make([]byte, entryPathLen)
	for i, b := range raw {
		out[i] = entryPathCharset[int(b)%len(entryPathCharset)]
	}
	return string(out), nil
}
