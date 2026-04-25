// Command channel-management is the Sub2API channel-management plugin.
//
// It migrates the channel CRUD, channel pricing, and (in future) channel
// monitoring features out of the core monolith into a standalone plugin
// process. The plugin runs as an independent OS process and talks to the
// core's DB and Redis through the plugin SDK's gRPC proxies.
package main

import (
	"log"

	pluginsdk "github.com/Wei-Shaw/sub2api/plugin-sdk"
)

func main() {
	if err := pluginsdk.Run(&ChannelPlugin{}); err != nil {
		log.Fatalf("channel-management plugin exited: %v", err)
	}
}
