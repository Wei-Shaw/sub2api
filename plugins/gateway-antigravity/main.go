// Command gateway-antigravity is the Antigravity gateway plugin for Sub2API.
//
// It migrates the Antigravity gateway subsystem (API forwarding, OAuth,
// account management, model routing) out of the core monolith into a
// standalone plugin process. The plugin runs as an independent OS process
// and talks to the core through the plugin SDK's gRPC protocol.
//
// Current status: skeleton — declares platform metadata and account
// platform extension stubs. Actual forwarding logic (4500+ lines in
// backend/internal/service/antigravity_gateway_service.go) is planned for
// subsequent phases.
package main

import (
	"log"

	pluginsdk "github.com/Wei-Shaw/sub2api/plugin-sdk"
)

func main() {
	if err := pluginsdk.Run(&AntigravityGatewayPlugin{}); err != nil {
		log.Fatalf("gateway-antigravity plugin exited: %v", err)
	}
}
