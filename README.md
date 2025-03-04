# go-cache
Caching utilities

timecache.Cache[K, V]:
- each cached entry has an associated ttl
- expired entries are evicted upon `ExpireInterval` ticks and will invoke `ExpiryHandler`
- example use case: cached tokens and links which are valid for only a given duration

gathercache.Cache[K, V]:
- upon first `Gather` for a given key in a cycle, `FirstHandler` is invoked
- subsequent `Gather` calls intend to accumulate value via user installed functor
- eventually upon `GatherInterval` since first call, `MaturityHandler` is invoked on final value
- example use case: reading and accumulating socket data, then logging the result upon `GatherInterval`

pushcache.Cache[K, V]:
- upon first `Push` for a given key in a cycle, `FirstHandler` is invoked
- subsequent `Push` calls intend to accumulate value via user installed functor
- each `Push` call will extend cycle, until no `Push` is seen within the latest `IdleInterval`
- `IdleHandler` is invoked on final value at the end of each cycle
- example use case: alarm on user's first chat message, for ongoing chat do not alarm unless `IdleInterval` has elapsed

throttlecache.Cache[K, V]:
- upon first `Buffer` for a given key in a cycle, `TriggerHandler` is invoked with immediate as true
- subsequent `Buffer` calls intend to accumulate value via user installed functor
- `TriggerHandler` is invoked with immediate as false, after each ensuing `CooldownInterval` during which at least one `Buffer` call is seen
- if no `Buffer` call is seen in the latest `CooldownInterval`, cycle ends
- example use cases:
  - batched database changes which should span at least `CooldownInterval` in between writes
  - throttled consumption of event queues, see example use of [`throttlecache.Queue[K]`](throttlecache/throttlequeue_test.go)
  - buffering latest bytes per throttle period in a data stream, see example use of [`throttlecache.BufferChain[K, V]`](throttlecache/throttlebufferchain_test.go)

holdcache.Cache[K, V]:
- for any given key, if value added via `Hold` call is held till expiry, `CommitHandler` is invoked
- while value is being held, any subsequent `Hold` call will invalidate previous value, invoking `InvalidateHandler`
- `Commit` call will cause `InvalidateHandler` to be invoked on held value, and `CommitHandler` invoked on incoming value
- `Invalidate` call will invalidate held value if present and invoke `InvalidateHandler`
- example use case: scheduling device power off at a future time, which invalidates upon any user activity

Dependencies:
- this package relies on github.com/emirpasic/gods for parts of internal structures
