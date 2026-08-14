//go:build unit

package service

import ()

func valueOrEmpty(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}
