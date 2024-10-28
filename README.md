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
- example use case: batched database changes which should span at least `CooldownInterval` in between writes

Dependencies:
- this package relies on github.com/emirpasic/gods for parts of internal structures
