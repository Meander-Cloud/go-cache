package pushcache

import (
	"fmt"
	"log"
	"sync"
	"time"
)

type StopBehavior uint8

const (
	NoOpOnStop StopBehavior = 0
	IdleOnStop StopBehavior = 1
)

type Handler[K comparable, V any] interface {
	FirstHandler(time.Time, K, V)
	IdleHandler(time.Time, time.Time, K, V)
}

type Options[K comparable, V any] struct {
	Handler[K, V]

	QueueLength  uint16
	IdleInterval time.Duration
	StopBehavior StopBehavior

	LogPrefix string
	LogDebug  bool
}

type entryData[V any] struct {
	v V

	first time.Time
	last  time.Time
}

func (d *entryData[V]) reset() {
	var v V
	d.v = v

	d.first = time.Time{}
	d.last = time.Time{}
}

type pushEvent[K comparable, V any] struct {
	k K
	t time.Time
	f func(V) V
}

func (e *pushEvent[K, V]) reset() {
	var k K
	e.k = k
	e.t = time.Time{}
	e.f = nil
}

type Cache[K comparable, V any] struct {
	options *Options[K, V]

	entryDataPool sync.Pool
	pushEventPool sync.Pool

	exitwg sync.WaitGroup
	exitch chan struct{}
	pushch chan *pushEvent[K, V]
	firech chan K

	// key -> entryData
	dataMap map[K]*entryData[V]
}

func NewCache[K comparable, V any](options *Options[K, V]) *Cache[K, V] {
	c := &Cache[K, V]{
		options: options,

		entryDataPool: sync.Pool{
			New: func() any {
				return &entryData[V]{}
			},
		},
		pushEventPool: sync.Pool{
			New: func() any {
				return &pushEvent[K, V]{}
			},
		},

		exitwg: sync.WaitGroup{},
		exitch: make(chan struct{}, 1),
		pushch: make(chan *pushEvent[K, V], options.QueueLength),
		firech: make(chan K, options.QueueLength),

		dataMap: make(map[K]*entryData[V]),
	}

	c.exitwg.Add(1)
	go c.process()

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

func (c *Cache[K, V]) getPushEvent() *pushEvent[K, V] {
	eAny := c.pushEventPool.Get()
	e, ok := eAny.(*pushEvent[K, V])
	if !ok {
		panic(fmt.Errorf("%s: failed to cast pushEvent, pushEventPool corrupt, eAny=%+v", c.options.LogPrefix, eAny))
	}
	return e
}

func (c *Cache[K, V]) returnPushEvent(e *pushEvent[K, V]) {
	e.reset()
	c.pushEventPool.Put(e)
}

func (c *Cache[K, V]) process() {
	log.Printf("%s: process goroutine starting", c.options.LogPrefix)

	defer func() {
		log.Printf("%s: process goroutine exiting", c.options.LogPrefix)
		c.exitwg.Done()
	}()

	push := func(e *pushEvent[K, V]) {
		defer c.returnPushEvent(e)

		k := e.k
		d, found := c.dataMap[k]

		u := func() {
			d.last = e.t

			// update value using user installed functor
			if e.f != nil {
				d.v = e.f(d.v)
			}
		}

		if !found {
			d = c.getEntryData()

			nowUTC := time.Now().UTC()
			d.first = nowUTC
			u()

			c.dataMap[k] = d

			// schedule idle marathon
			time.AfterFunc(
				d.last.Add(c.options.IdleInterval).Sub(nowUTC),
				func() {
					if c.options.LogDebug {
						log.Printf("%s: send firech, k=%v", c.options.LogPrefix, k)
					}

					c.firech <- k
				},
			)

			c.options.FirstHandler(
				d.first,
				k,
				d.v,
			)
		} else {
			u()
		}
	}

	fire := func(k K, inExit bool) {
		d, found := c.dataMap[k]
		if !found {
			log.Printf("%s: k=%v not found in dataMap", c.options.LogPrefix, k)
			return
		}

		idle := func() {
			defer c.returnEntryData(d)

			delete(c.dataMap, k)

			c.options.IdleHandler(
				d.first,
				d.last,
				k,
				d.v,
			)
		}

		if inExit {
			idle()
			return
		}

		interval := d.last.Add(c.options.IdleInterval).Sub(time.Now().UTC())
		if interval > 0 {
			// continue idle marathon
			time.AfterFunc(
				interval,
				func() {
					if c.options.LogDebug {
						log.Printf("%s: send firech, k=%v", c.options.LogPrefix, k)
					}

					c.firech <- k
				},
			)
		} else {
			idle()
		}
	}

	flush := func() {
		if c.options.StopBehavior != IdleOnStop {
			return
		}

		keyMap := make(map[K]struct{})
		for k := range c.dataMap {
			keyMap[k] = struct{}{}
		}

		for k := range keyMap {
			fire(k, true)
		}
	}

	for {
		select {
		case <-c.exitch:
			log.Printf("%s: exitch received", c.options.LogPrefix)
			flush()
			return
		case e := <-c.pushch:
			push(e)
		case k := <-c.firech:
			fire(k, false)
		}
	}
}

// f will be invoked on internal process goroutine
func (c *Cache[K, V]) Push(k K, t time.Time, f func(V) V) {
	e := c.getPushEvent()
	e.k = k
	e.t = t
	e.f = f

	c.pushch <- e
}
