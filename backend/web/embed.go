// Package web 内嵌前端构建产物（go:embed，ADR-009 单二进制部署）。
//
// 构建流程：cd frontend && npm run build，产物拷贝到本包 dist/ 目录
// （仓库根 Makefile 的 `make web` target 封装该流程；dist/ 为构建产物
// 不入 git，占位 .gitkeep 保证全新 clone 可编译——嵌入空目录）。
package web

import "embed"

// DistFS 是前端 dist 构建产物（vite outDir=dist）。
//
//go:embed all:dist
var DistFS embed.FS
