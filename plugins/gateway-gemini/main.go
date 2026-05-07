// Command gateway-gemini is the Gemini gateway plugin for Sub2API.
//
// It migrates the Gemini gateway subsystem (API forwarding, OAuth,
// service account management, model routing) out of the core monolith
// into a standalone plugin process. The plugin communicates with the
// core through the plugin SDK's gRPC protocol.
//
// Current status: skeleton — declares platform metadata and account
// platform extension stubs. Actual forwarding logic is planned for
// subsequent phases.
package main

import (
	"log"

	pluginsdk "github.com/Wei-Shaw/sub2api/plugin-sdk"
)

func main() {
	if err := pluginsdk.Run(&GeminiGatewayPlugin{}); err != nil {
		log.Fatalf("gateway-gemini plugin exited: %v", err)
	}
}
