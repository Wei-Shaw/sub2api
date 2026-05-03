// Command payment is the Sub2API payment plugin.
//
// It migrates the payment subsystem (orders, providers, fulfillment,
// refunds, resume tokens, webhooks) out of the core monolith into a
// standalone plugin process. The plugin runs as an independent OS process
// and talks to the core's DB and Redis through the plugin SDK's gRPC
// proxies, and to the core's user/balance/subscription services through
// the SDK's HostPaymentClient.
package main

import (
	"log"

	pluginsdk "github.com/Wei-Shaw/sub2api/plugin-sdk"
)

func main() {
	if err := pluginsdk.Run(&PaymentPlugin{}); err != nil {
		log.Fatalf("payment plugin exited: %v", err)
	}
}
