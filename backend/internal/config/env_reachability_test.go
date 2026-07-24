//go:build unit

package config

import (
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/spf13/viper"
)

// collectMapstructureKeys walks a config struct and returns every dotted key
// viper would need in order to populate it.
func collectMapstructureKeys(t reflect.Type, prefix string, out map[string]string) {
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if field.PkgPath != "" {
			continue // unexported
		}
		tag := field.Tag.Get("mapstructure")
		name, _, _ := strings.Cut(tag, ",")
		if name == "-" {
			continue
		}
		if name == "" {
			name = strings.ToLower(field.Name)
		}
		key := name
		if prefix != "" {
			key = prefix + "." + name
		}

		ft := field.Type
		for ft.Kind() == reflect.Ptr {
			ft = ft.Elem()
		}
		if ft.Kind() == reflect.Struct {
			collectMapstructureKeys(ft, key, out)
			continue
		}
		if ft.Kind() == reflect.Map {
			// A map cannot be expressed in a single environment variable, so it
			// is out of scope here — such settings need a config file either way.
			continue
		}
		out[strings.ToLower(key)] = ft.String()
	}
}

// TestConfigKeysAreEnvReachable is the systemic guard behind the image_storage
// bug: viper.Unmarshal only decodes keys returned by AllKeys(), which unions
// SetDefault keys, config-file keys and explicit BindEnv keys. AutomaticEnv can
// override a key already in that union but never introduces one, and the
// viper_bind_struct escape hatch is compiled out (we build with -tags embed).
//
// So a Config field with no registered default is unreachable by environment
// variable whenever the deployment has no config.yaml containing it — the
// operator sets the variable, the loader discards it, and the feature behaves
// as if it were never configured. That is exactly how image_storage credentials
// were lost, silently disabling async image tasks for env-driven deployments.
//
// When this fails, register a zero-valued default in setEnvReachableDefaults
// for each reported key.
func TestConfigKeysAreEnvReachable(t *testing.T) {
	bound := map[string]string{}
	collectMapstructureKeys(reflect.TypeOf(Config{}), "", bound)

	viper.Reset()
	t.Cleanup(viper.Reset)
	setDefaults()
	registered := map[string]struct{}{}
	for _, key := range viper.AllKeys() {
		registered[key] = struct{}{}
	}

	var unreachable []string
	for key, kind := range bound {
		if _, ok := registered[key]; !ok {
			unreachable = append(unreachable, key+" ("+kind+")")
		}
	}
	sort.Strings(unreachable)

	if len(unreachable) > 0 {
		t.Fatalf("%d config keys have no default registered, so their environment variables are silently ignored:\n  %s",
			len(unreachable), strings.Join(unreachable, "\n  "))
	}
}
