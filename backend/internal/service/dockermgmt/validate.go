// validate.go 承载入参校验（R-14 单一校验通道在 service 层；handler 不做
// required 绑定）：tail 取值域（DK_001）、state 多值筛选值域（DK_001）、
// 镜像引用格式（DK_012）、不透明引用非空（DK_001）。
package dockermgmt

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/dougong/dougong/internal/errcode"
)

// imageRefRe 镜像引用格式（`名称[:tag]`，api §4.2.4/DK_012）：可选 registry
// 主机（域名[:端口]/）、小写路径分量（[a-z0-9] 与 . _ - __ 分隔）、可选 tag
// （[a-zA-Z0-9_][a-zA-Z0-9._-]{0,127}）。对齐 docker reference 语法主流子集。
var imageRefRe = regexp.MustCompile(
	`^(?:(?:localhost|(?:[a-zA-Z0-9](?:[a-zA-Z0-9-]*[a-zA-Z0-9])?\.)+[a-zA-Z]{2,})(?::[0-9]+)?/)?` + // registry
		`(?:[a-z0-9]+(?:(?:[._]|__|[-]+)[a-z0-9]+)*/)*` + // 路径分量
		`[a-z0-9]+(?:(?:[._]|__|[-]+)[a-z0-9]+)*` + // 名称
		`(?::[a-zA-Z0-9_][a-zA-Z0-9._-]{0,127})?$`) // tag

// ValidateImageRef 校验镜像引用格式（DK_012；空串同样非法——非空属 DK_001
// 语义，调用方前置判空）。
func ValidateImageRef(ref string) bool {
	return imageRefRe.MatchString(ref)
}

// ParseTail 解析 tail 参数（缺省 500、取值 1-2000；非法 DK_001——D1 口径-3）。
func ParseTail(raw string) (int, error) {
	if raw == "" {
		return TailDefault, nil
	}
	n, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || n < TailMin || n > TailMax {
		return 0, errcode.ErrDkInvalidParam.WithData(map[string]any{"field": "tail"})
	}
	return n, nil
}

// ParseStateFilter 解析 state 多值筛选（逗号分隔；值域=container_status 三态
// + 过渡态原始值，任一非法 DK_001；空串=无筛选，D-02）。
func ParseStateFilter(raw string) (map[string]bool, error) {
	if raw == "" {
		return nil, nil
	}
	filter := make(map[string]bool)
	for _, token := range strings.Split(raw, ",") {
		token = strings.TrimSpace(token)
		if !validStateFilters[token] {
			return nil, errcode.ErrDkInvalidParam.WithData(map[string]any{"field": "state"})
		}
		filter[token] = true
	}
	return filter, nil
}

// requireRef 不透明引用非空校验（D-11：来自各 list 响应原样透传，仅校验
// 非空不解析内容；空/纯空白 DK_001）。
func requireRef(field, value string) error {
	if strings.TrimSpace(value) == "" {
		return errcode.ErrDkInvalidParam.WithData(map[string]any{"field": field})
	}
	return nil
}
