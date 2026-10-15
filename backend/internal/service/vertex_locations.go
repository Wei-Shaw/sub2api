package service

import (
	"slices"
	"strings"
)

const (
	vertexGlobalLocation = "global"
	vertexLocationUS     = "us"
	vertexLocationEU     = "eu"
)

// vertexModelLocations 列出 Vertex Gemini 模型可用的区域。Gemini 账号区域配置为多区域（us/eu）时，
// 表内声明支持该多区域的模型走对应多区域端点，其余模型（含未列出的模型）走 global。
// 账号区域为 global 或单区域时、以及非 Gemini 平台账号不查此表。
var vertexModelLocations = map[string][]string{
	"gemini-3.1-flash-image": {vertexGlobalLocation, vertexLocationUS, vertexLocationEU},
	"gemini-3.1-flash-lite":  {vertexGlobalLocation, vertexLocationUS, vertexLocationEU},
	"gemini-3.5-flash":       {vertexGlobalLocation, vertexLocationUS, vertexLocationEU},
	"gemini-3.5-flash-lite":  {vertexGlobalLocation, vertexLocationUS, vertexLocationEU},
	"gemini-3.6-flash":       {vertexGlobalLocation, vertexLocationUS, vertexLocationEU},
	"gemini-3.7-flash":       {vertexGlobalLocation, vertexLocationUS, vertexLocationEU},
	"gemini-3.8-flash":       {vertexGlobalLocation, vertexLocationUS, vertexLocationEU},
	"gemini-4-argon":         {vertexGlobalLocation, vertexLocationUS, vertexLocationEU},
}

// isVertexMultiRegion 判断区域是否为 Vertex 多区域（us/eu）。
func isVertexMultiRegion(location string) bool {
	return location == vertexLocationUS || location == vertexLocationEU
}

// vertexAPIHost 返回区域对应的 Vertex AI API 主机：global 用全局主机，多区域用 aiplatform.{loc}.rep 主机，
// 单区域用 {loc}-aiplatform 主机。
func vertexAPIHost(location string) string {
	switch {
	case location == vertexGlobalLocation:
		return vertexGlobalHost
	case isVertexMultiRegion(location):
		return "aiplatform." + location + ".rep.googleapis.com"
	default:
		return location + "-aiplatform.googleapis.com"
	}
}

// resolveVertexModelLocation 返回模型在账号区域下实际使用的区域：账号为多区域而模型不支持该多区域时用 global。
func resolveVertexModelLocation(accountLocation, model string) string {
	if !isVertexMultiRegion(accountLocation) {
		return accountLocation
	}
	if i := strings.LastIndex(model, "/"); i >= 0 {
		model = model[i+1:]
	}
	if slices.Contains(vertexModelLocations[strings.TrimSpace(model)], accountLocation) {
		return accountLocation
	}
	return vertexGlobalLocation
}
