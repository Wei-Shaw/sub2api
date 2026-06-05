package service

import (
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// ReplaceModelInBody rewrites the JSON body's "model" field. If the body is
// already on newModel or the JSON cannot be edited, body is returned
// unchanged.
//
// This helper is unrelated to the channel domain — it is a pure JSON utility
// that happens to live in the channel service today. It is exported so the
// host gateway can apply the same rewrite logic when forwarding requests to
// upstream APIs. T12 may relocate it to a dedicated utility package; until
// then we keep it in its own file to keep the channel CRUD/lookup files
// focused.
func ReplaceModelInBody(body []byte, newModel string) []byte {
	if len(body) == 0 {
		return body
	}
	if current := gjson.GetBytes(body, "model"); current.Exists() && current.String() == newModel {
		return body
	}
	newBody, err := sjson.SetBytes(body, "model", newModel)
	if err != nil {
		return body
	}
	return newBody
}
