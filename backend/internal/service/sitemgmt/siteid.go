package sitemgmt

import (
	"strconv"
	"strings"

	"github.com/dougong/dougong/internal/errcode"
)

// buildSiteID 生成站点标识（D-02：{来源文件绝对路径}#{server 块序号}）。
func buildSiteID(file string, index int) string {
	return file + "#" + strconv.Itoa(index)
}

// parseSiteID 解析站点标识（SITE_001：非「文件路径#序号」形态非法，R-14）。
func parseSiteID(id string) (string, int, error) {
	i := strings.LastIndex(id, "#")
	if i <= 0 || i == len(id)-1 {
		return "", 0, errcode.ErrSiteInvalidID
	}
	n, err := strconv.Atoi(id[i+1:])
	if err != nil || n < 0 {
		return "", 0, errcode.ErrSiteInvalidID
	}
	return id[:i], n, nil
}
