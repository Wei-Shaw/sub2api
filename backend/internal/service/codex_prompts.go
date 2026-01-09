package service

import _ "embed"

//go:embed prompts/codex_opencode_bridge.txt
var codexOpenCodeBridge string

//go:embed prompts/tool_remap_message.txt
var codexToolRemapMessage string
