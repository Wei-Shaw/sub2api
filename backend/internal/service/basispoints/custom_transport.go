package basispoints

import (
	"fmt"
	"strings"
	"unicode"
)

const customTransportPrefix = "codex2api.custom/"

// customTransportEnvelope recognizes only the explicitly tagged raw custom-tool
// transport. The caller must still check the exact catalog name, custom kind and
// call identity. Ordinary run_officejs calls continue through the JSON decoder.
func customTransportEnvelope(arguments object) (object, bool, error) {
	summary, ok := arguments["summary"].(string)
	if !ok || !strings.HasPrefix(summary, customTransportPrefix) {
		return nil, false, nil
	}
	name := strings.TrimPrefix(summary, customTransportPrefix)
	if name == "" || strings.ContainsAny(name, "/\\") || strings.IndexFunc(name, unicode.IsSpace) >= 0 || strings.IndexFunc(name, unicode.IsControl) >= 0 {
		return nil, true, fmt.Errorf("basispoints raw custom transport requires an exact nonempty catalog tool name in summary")
	}
	input, ok := arguments["code"].(string)
	if !ok {
		return nil, true, fmt.Errorf("basispoints raw custom transport code must be a string")
	}
	if len(input) > maxEnvelopeBytes {
		return nil, true, fmt.Errorf("basispoints raw custom transport code exceeds the size limit")
	}
	return object{"name": name, "input": input}, true, nil
}
