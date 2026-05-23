// Package service — plugin_settings_pubsub.go
//
// In-process pub/sub for the plugin_settings subsystem: Subscribe /
// notify / dropAllSubscribersForPlugin. See plugin_settings_types.go for
// the shared types and constants.
//
// Fan-out is intentionally in-process — sub2api currently runs as a
// single instance and a future multi-instance deployment can swap the
// implementation behind a pub/sub abstraction without touching the
// gRPC server. SETTINGS-V2-DESIGN §3.6 says "host MAY use LISTEN/NOTIFY";
// we keep that option open by routing all writes through notify().
package service

import "sync"

// pluginSettingsSubscriberBuffer is the per-subscriber channel size. We
// keep it small because the SDK side already caches the latest value;
// a slow subscriber simply drops events and recovers via Get.
const pluginSettingsSubscriberBuffer = 8

type pluginSettingsSubscriber struct {
	id     uint64
	plugin string
	key    string // empty = whole namespace
	ch     chan PluginSettingsChange
}

// Subscribe registers a fan-out channel for changes inside one plugin
// namespace. An empty key matches every change for that plugin.
//
// The returned cleanup must be called when the caller is done; it is
// safe to call multiple times.
func (s *PluginSettingsService) Subscribe(
	pluginName, key string,
) (<-chan PluginSettingsChange, func()) {
	sub := &pluginSettingsSubscriber{
		id:     s.subID.Add(1),
		plugin: pluginName,
		key:    key,
		ch:     make(chan PluginSettingsChange, pluginSettingsSubscriberBuffer),
	}
	s.subMu.Lock()
	s.subs[pluginName] = append(s.subs[pluginName], sub)
	s.subMu.Unlock()

	var once sync.Once
	cleanup := func() {
		once.Do(func() {
			s.subMu.Lock()
			defer s.subMu.Unlock()
			subs := s.subs[pluginName]
			for i, x := range subs {
				if x.id == sub.id {
					close(x.ch)
					s.subs[pluginName] = append(subs[:i], subs[i+1:]...)
					if len(s.subs[pluginName]) == 0 {
						delete(s.subs, pluginName)
					}
					return
				}
			}
		})
	}
	return sub.ch, cleanup
}

// dropAllSubscribersForPlugin closes every subscriber channel for the
// named plugin and deletes the slice entry. Used when a schema_version
// bump means existing watchers are reading values under a stale schema —
// closing the channel triggers the gRPC server's "ok=false" branch which
// ends the Watch stream cleanly. SDK runWatchLoop then reconnects and
// requests a fresh snapshot (DESIGN §4.5).
//
// Safe to call when no subscribers exist (the map lookup returns nil).
// Subscribe's cleanup func uses sync.Once and does not double-close
// because dropAll deletes the slice entry; cleanup's lookup loop becomes
// a no-op.
func (s *PluginSettingsService) dropAllSubscribersForPlugin(pluginName string) {
	s.subMu.Lock()
	defer s.subMu.Unlock()
	subs := s.subs[pluginName]
	for _, sub := range subs {
		close(sub.ch)
	}
	delete(s.subs, pluginName)
}

func (s *PluginSettingsService) notify(change PluginSettingsChange) {
	s.subMu.RLock()
	subs := append([]*pluginSettingsSubscriber(nil), s.subs[change.Plugin]...)
	s.subMu.RUnlock()
	for _, sub := range subs {
		if sub.key != "" && sub.key != change.Key {
			continue
		}
		select {
		case sub.ch <- change:
		default:
			s.logger.Warn("plugin_settings: subscriber channel full, dropping",
				"plugin", change.Plugin, "key", change.Key)
		}
	}
}
