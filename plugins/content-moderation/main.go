// Command content-moderation is the out-of-process plugin that provides
// content moderation (审计/风控) for the Sub2API gateway. It hooks the
// gateway preflight stage via ContentInterceptExtension.Check and exposes
// an admin REST surface for configuration, logs, and user ban management.
//
// This is the skeleton: the gRPC ContentInterceptExtension wiring and the
// real moderation logic (service / handler / repository layers) are migrated
// from the core in follow-up tasks. See plugins/hello-world for the canonical
// SDK template and plugin-sdk/README.md for the full SDK reference.
package main

import (
	"log"

	pluginsdk "github.com/Wei-Shaw/sub2api/plugin-sdk"
)

func main() {
	if err := pluginsdk.Run(&ContentModerationPlugin{}); err != nil {
		log.Fatalf("content-moderation plugin exited: %v", err)
	}
}
