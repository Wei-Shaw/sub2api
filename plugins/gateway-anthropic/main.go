// Command gateway-anthropic is the Anthropic/Claude gateway plugin for Sub2API.
//
// It migrates the Anthropic gateway subsystem (API forwarding, OAuth,
// account management, model routing) out of the core monolith into a
// standalone plugin process. The plugin runs as an independent OS process
// and talks to the core through the plugin SDK's gRPC protocol.
//
// Current status: skeleton -- declares platform metadata and account
// platform extension (validate, test, refresh, models). Actual forwarding
// logic is planned for subsequent phases.
package main

import (
	"log"

	pluginsdk "github.com/Wei-Shaw/sub2api/plugin-sdk"
)

func main() {
	if err := pluginsdk.Run(&AnthropicGatewayPlugin{}); err != nil {
		log.Fatalf("gateway-anthropic plugin exited: %v", err)
	}
}
