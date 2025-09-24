package timecache

import (
	"fmt"
	"log"
	"math"
	"sync"
	"time"

	rbt "github.com/emirpasic/gods/v2/trees/redblacktree"
)

type Handler[K comparable, V any] interface {
	ExpiryHandler([2]int64, K, V)
}

type Options[K comparable, V any] struct {
	Handler[K, V]

	ExpireInterval time.Duration

	LogPrefix string
	LogDebug  bool
}

type entryData[V any] struct {
	value V

	// epoch, sequence
	ttl [2]int64
}

func (e *entryData[V]) reset() {
	var value V
	e.value = value

	e.ttl[0] = 0
	e.ttl[1] = 0
}

type Cache[K comparable, V any] struct {
	options *Options[K, V]

	entryDataPool sync.Pool

	exitwg sync.WaitGroup
	exitch chan struct{}

	mutex    sync.RWMutex
	sequence int64

	// key -> entryData
	dataMap map[K]*entryData[V]

	// epoch, sequence -> key
	ttlTree *rbt.Tree[[2]int64, K]
}

func ttlTreeComparator(a, b [2]int64) int {
	if a[0] > b[0] {
		return 1
	} else if a[0] < b[0] {
		return -1
	}

	if a[1] > b[1] {
		return 1
	} else if a[1] < b[1] {
		return -1
	}

	return 0
}

func NewCache[K comparable, V any](options *Options[K, V]) *Cache[K, V] {
	c := &Cache[K, V]{
		options: options,

		entryDataPool: sync.Pool{
			New: func() any {
				return &entryData[V]{}
			},
		},

		exitwg: sync.WaitGroup{},
		exitch: make(chan struct{}, 1),

		mutex:    sync.RWMutex{},
		sequence: 0,

		dataMap: make(map[K]*entryData[V]),
		ttlTree: rbt.NewWith[[2]int64, K](ttlTreeComparator),
	}

	c.exitwg.Add(1)
	go c.lifecycle()

	return c
}

func (c *Cache[K, V]) Stop() {
	log.Printf("%s: synchronized stop starting", c.options.LogPrefix)

	select {
	case c.exitch <- struct{}{}:
	default:
		log.Printf("%s: exitch already signaled", c.options.LogPrefix)
	}

	c.exitwg.Wait()
	log.Printf("%s: synchronized stop done", c.options.LogPrefix)
}

func (c *Cache[K, V]) getEntryData() *entryData[V] {
	dAny := c.entryDataPool.Get()
	d, ok := dAny.(*entryData[V])
	if !ok {
		panic(fmt.Errorf("%s: failed to cast entryData, entryDataPool corrupt, dAny=%+v", c.options.LogPrefix, dAny))
	}
	return d
}

func (c *Cache[K, V]) returnEntryData(d *entryData[V]) {
	d.reset()
	c.entryDataPool.Put(d)
}

func (c *Cache[K, V]) lifecycle() {
	log.Printf("%s: lifecycle management goroutine starting", c.options.LogPrefix)

	defer func() {
		log.Printf("%s: lifecycle management goroutine exiting", c.options.LogPrefix)
		c.exitwg.Done()
	}()

	ticker := time.NewTicker(c.options.ExpireInterval)
	defer ticker.Stop()

	for {
		select {
		case <-c.exitch:
			log.Printf("%s: exitch received", c.options.LogPrefix)
			return
		case <-ticker.C:
			err := func() error {
				now := time.Now().UTC().UnixMilli()
				remTree := rbt.NewWith[[2]int64, K](ttlTreeComparator)
				remMap := make(map[K]*entryData[V])

				func() {
					// limit lock scope
					c.mutex.Lock()
					defer c.mutex.Unlock()

					it := c.ttlTree.Iterator()
					for it.Next() {
						ttl := it.Key()

						if ttl[0] > now {
							// since ttlTree is ordered by epoch
							// all subsequent elements must have expiration in the future
							break
						}

						remTree.Put(ttl, it.Value())
					}

					it = remTree.Iterator()
					for it.Next() {
						key := it.Value()
						entry, found := c.dataMap[key]
						if !found {
							log.Printf("%s: failed to find key=%v, dataMap corrupt", c.options.LogPrefix, key)
							continue
						}

						// remove from cache
						c.ttlTree.Remove(entry.ttl)
						delete(c.dataMap, key)

						// save to local map for invoking user installed handler
						remMap[key] = entry
					}
				}()

				it := remTree.Iterator()
				for it.Next() {
					key := it.Value()
					entry, found := remMap[key]
					if !found {
						log.Printf("%s: failed to find key=%v, remMap corrupt", c.options.LogPrefix, key)
						continue
					}

					// invoke user installed handler outside of critical path
					// in case user logic re-adds to cache and tries to acquire lock
					c.options.ExpiryHandler(
						it.Key(),
						key,
						entry.value,
					)

					// return buffer to pool
					c.returnEntryData(entry)
				}

				return nil
			}()
			if err != nil {
				return
			}
		}
	}
}

// c.mutex write lock is assumed to have been acquired
func (c *Cache[K, V]) advanceSequence() {
	if c.sequence == math.MaxInt64 {
		c.sequence = 1
	} else {
		c.sequence += 1
	}
}

func (c *Cache[K, V]) Get(key K) (V, bool) {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	entry, found := c.dataMap[key]
	if !found {
		var value V
		return value, false
	}

	if time.Now().UTC().UnixMilli() >= entry.ttl[0] {
		// entry has expired, defer cleanup to ticker routine
		var value V
		return value, false
	} else {
		return entry.value, true
	}
}

func (c *Cache[K, V]) SetX(key K, value V, expire time.Duration) {
	entry := c.getEntryData()
	entry.value = value
	entry.ttl[0] = time.Now().UTC().Add(expire).UnixMilli()

	c.mutex.Lock()
	defer c.mutex.Unlock()

	entry.ttl[1] = c.sequence
	c.advanceSequence()

	entryCached, found := c.dataMap[key]
	if found {
		c.ttlTree.Remove(entryCached.ttl)
		delete(c.dataMap, key)
		c.returnEntryData(entryCached)
	}

	c.dataMap[key] = entry
	c.ttlTree.Put(entry.ttl, key)
}

func (c *Cache[K, V]) Set(key K, value V) {
	// since options is modified only in New, this is safe to access directly
	c.SetX(key, value, c.options.ExpireInterval)
}

func (c *Cache[K, V]) Delete(key K) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	entry, found := c.dataMap[key]
	if !found {
		return
	}

	c.ttlTree.Remove(entry.ttl)
	delete(c.dataMap, key)
	c.returnEntryData(entry)
}

func (c *Cache[K, V]) Clear() {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	c.dataMap = make(map[K]*entryData[V])
	c.ttlTree.Clear()
	// leave c.entryDataPool as is, since entryData from map are not returned, this has same effect as clearing pool
}
