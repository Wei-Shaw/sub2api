# Plugin SDK — Redis API

The plugin SDK exposes a Redis client that mirrors the
[`github.com/redis/go-redis/v9`](https://pkg.go.dev/github.com/redis/go-redis/v9)
`Cmdable` surface. Plugins can use Redis the same way they would with native
go-redis, while the core enforces per-plugin key namespaces so two plugins
can never collide on the same key by accident.

## Quick start

```go
func (p *MyPlugin) Init(ctx pluginsdk.PluginContext) error {
    rdb := ctx.Redis()

    // Plain string commands work as in go-redis.
    if err := rdb.Set(stdctx.Background(), "session:42", "online"); err != nil {
        return err
    }
    val, err := rdb.Get(stdctx.Background(), "session:42")
    // val == "online"
    _ = err

    // Cmder-style for newer methods:
    n, err := rdb.Incr(stdctx.Background(), "counter").Result()
    _ = n; _ = err

    return nil
}
```

The SDK silently rewrites every key argument to `plugin:<plugin_name>:<key>`
before sending the command to Redis. From the plugin's point of view this
is invisible: writing `session:42` reads back as `session:42`. From the
Redis server's point of view the key is actually
`plugin:my-plugin:session:42`.

## Universal `Do()`

Anything that does not have a typed Cmdable wrapper can still be issued via
`Do()`:

```go
reply := rdb.Do(ctx, "OBJECT",
    []int{1},                  // arg index 1 (i.e. the key) is a key
    "ENCODING", "session:42",
).AsString()
encoding, err := reply.Result()
```

`Do(command, keyPositions, args...)` arguments:

| Parameter      | Description                                                                           |
| -------------- | ------------------------------------------------------------------------------------- |
| `command`      | The Redis command name (case-insensitive).                                            |
| `keyPositions` | Zero-based indices into `args` of arguments that are Redis keys. SDK applies the prefix to those. Pass `nil` for commands without keys (PING). |
| `args`         | The remaining command arguments. `string`, `[]byte`, integers, floats, `bool`, `time.Duration`, `time.Time`, and any `fmt.Stringer` are accepted. |

The returned `*DoCmd` exposes:

- `Reply() *pb.DoReply` — the raw protobuf reply.
- `AsString()`, `AsInt()`, `AsBool()`, `AsFloat()`, `AsStringSlice()`,
  `AsStringStringMap()`, `AsSlice()`, `AsStatus()` — typed accessors that
  return one of the Cmder structs and apply the same Result/Val/Err idiom
  as go-redis.

## Supported commands

The first wave covers the commands plugin code routinely needs. Adding a
new one is mechanical: write a one-liner that calls `Do(...)` with the
right command name + key positions, and parse the reply with one of the
existing helpers.

| Group         | Commands                                                                                  |
| ------------- | ----------------------------------------------------------------------------------------- |
| Generic       | Del, Exists, Expire, ExpireAt, PExpire, TTL, PTTL, Type, Rename, Persist, Keys, Scan      |
| String        | Get, Set, SetEx, SetNX, GetSet, Incr, IncrBy, Decr, DecrBy, IncrByFloat, Append, StrLen, MGet, MSet |
| Hash          | HGet, HSet, HDel, HGetAll, HExists, HKeys, HVals, HLen, HIncrBy, HIncrByFloat, HMGet, HMSet |
| List          | LPush, RPush, LPop, RPop, LRange, LLen, LIndex, LRem, LTrim                               |
| Set           | SAdd, SRem, SMembers, SIsMember, SCard, SPop, SRandMember                                 |
| Sorted Set    | ZAdd, ZRem, ZRange, ZRangeByScore, ZRevRange, ZScore, ZIncrBy, ZRank, ZCard, ZCount       |
| Pub/Sub       | Publish, Subscribe                                                                        |
| Scripting     | Eval, EvalSha, ScriptLoad                                                                 |
| Server        | Ping                                                                                      |
| Universal     | Do                                                                                        |

Anything else (BLPop, OBJECT, MEMORY USAGE, CLUSTER NODES, …) goes through
`Do()`.

## Key namespacing — what gets a prefix

| Argument type           | Prefixed?                          |
| ----------------------- | ---------------------------------- |
| Single-key commands     | Yes (the SDK knows the key index)  |
| Multi-key commands      | Yes (e.g. MGET, EXISTS, MSET keys) |
| Hash field names        | No (fields are not keys)           |
| List/Set members        | No                                 |
| ZSet members            | No                                 |
| Pub/Sub channels        | No (channels are not keys)         |
| `KEYS pattern`          | Yes (the pattern is namespaced too — a plugin running `KEYS *` only sees its own keys) |
| `SCAN MATCH pattern`    | Yes                                |

Pub/Sub channels are intentionally unprefixed. Plugins that need
discriminating channels embed the plugin name in the channel string
(e.g. `plugin:channel:invalidate`).

## Opting out — `Raw()` for shared keys

Some plugins legitimately read or write keys defined by other components.
The canonical example is `channel-management`, which writes the keys
documented in `plugins/channel-management/GATEWAY_CACHE_SPEC.md`; the
gateway's `ChannelCacheReader` reads those keys directly. If the SDK
auto-prefixed them the gateway would never find them.

The opt-out is two-step:

1. Declare the capability in the manifest:

   ```go
   func (p *Plugin) Manifest() *pluginsdk.Manifest {
       return &pluginsdk.Manifest{
           Name: "channel-management",
           // ...
           Capabilities: []string{pluginsdk.CapabilityRedisRawKeys},
       }
   }
   ```

2. Obtain a raw-key client from the SDK:

   ```go
   raw := ctx.Redis().Raw()
   if raw == nil {
       // The core did not grant the capability — typically because the
       // core's allow-list does not include it. Fall back gracefully.
       return errors.New("redis_raw_keys not granted")
   }
   raw.Set(ctx2, "channel:active", payload)
   ```

The SDK returns `nil` from `Raw()` when the plugin lacks the capability so
callers can branch deliberately rather than discovering the failure at
runtime via mysteriously absent keys.

The core enforces the capability twice:

- At Init time: `manager.go` filters the requested capabilities through
  the core's allow-list before forwarding them to the SDK.
- At every `Do(raw_key=true)` call: the core verifies the caller's
  identity (via the `x-sub2api-plugin` gRPC metadata header) and rejects
  raw_key requests from plugins that did not have the capability granted.

## Pub/Sub

Subscribe channels are non-namespaced and the message payload is opaque.
Plugins that publish should embed the plugin name in the channel:

```go
rdb.Publish(ctx, "plugin:my-plugin:invalidate", payload)
ch, err := rdb.Subscribe(ctx, "plugin:other-plugin:invalidate")
```

Subscribe returns a Go channel that closes when the gRPC stream ends or
ctx is cancelled. Errors that terminate the stream are logged via the
SDK's tagged logger; callers detect termination by observing the channel
close.

## Migration from v0

Plugins built against the original 10-method SDK keep working. The legacy
methods (`Get`, `Set`, `SetEx`, `Del`, `HGet`, `HSet`, `HGetAll`, `HDel`,
`Publish`, `Subscribe`) preserved their signatures and now route through
`Do()` internally. The two adjustments worth making at upgrade time are:

1. Remove any manual `plugin:<name>:` prefix from keys — the SDK now adds
   it for you. Double-prefixing is harmless for writes (keys land at
   `plugin:foo:plugin:foo:bar`) but no other component will ever read
   them.
2. If your plugin shares keys with the core or other plugins (the channel-management
   pattern), declare `CapabilityRedisRawKeys` in the manifest and use
   `ctx.Redis().Raw()` for those operations.
