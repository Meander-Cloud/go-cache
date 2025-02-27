package throttlecache

import (
	"fmt"
	"log"
	"sync"
	"time"
)

type StopBehavior uint8

const (
	NoOpOnStop    StopBehavior = 0
	TriggerOnStop StopBehavior = 1
)

type Handler[K comparable, V any] interface {
	TriggerHandler(bool, K, V)
}

type Options[K comparable, V any] struct {
	Handler[K, V]

	QueueLength      uint16
	CooldownInterval time.Duration
	StopBehavior     StopBehavior

	LogPrefix string
	LogDebug  bool
}

type entryData[V any] struct {
	v V

	first   time.Time
	hasData bool
}

func (d *entryData[V]) reset() {
	var v V
	d.v = v

	d.first = time.Time{}
	d.hasData = false
}

func (d *entryData[V]) triggered() {
	var v V
	d.v = v

	d.hasData = false
}

type bufferEvent[K comparable, V any] struct {
	k K
	f func(V) V
}

func (e *bufferEvent[K, V]) reset() {
	var k K
	e.k = k
	e.f = nil
}

type Cache[K comparable, V any] struct {
	options *Options[K, V]

	entryDataPool   sync.Pool
	bufferEventPool sync.Pool

	exitwg    sync.WaitGroup
	exitch    chan struct{}
	bufferch  chan *bufferEvent[K, V]
	triggerch chan K

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
		bufferEventPool: sync.Pool{
			New: func() any {
				return &bufferEvent[K, V]{}
			},
		},

		exitwg:    sync.WaitGroup{},
		exitch:    make(chan struct{}, 1),
		bufferch:  make(chan *bufferEvent[K, V], options.QueueLength),
		triggerch: make(chan K, options.QueueLength),

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
		panic(fmt.Errorf("failed to cast entryData, entryDataPool corrupt, dAny=%+v", dAny))
	}
	return d
}

func (c *Cache[K, V]) returnEntryData(d *entryData[V]) {
	d.reset()
	c.entryDataPool.Put(d)
}

func (c *Cache[K, V]) getBufferEvent() *bufferEvent[K, V] {
	eAny := c.bufferEventPool.Get()
	e, ok := eAny.(*bufferEvent[K, V])
	if !ok {
		panic(fmt.Errorf("failed to cast bufferEvent, bufferEventPool corrupt, eAny=%+v", eAny))
	}
	return e
}

func (c *Cache[K, V]) returnBufferEvent(e *bufferEvent[K, V]) {
	e.reset()
	c.bufferEventPool.Put(e)
}

func (c *Cache[K, V]) process() {
	log.Printf("%s: process goroutine starting", c.options.LogPrefix)

	defer func() {
		log.Printf("%s: process goroutine exiting", c.options.LogPrefix)
		c.exitwg.Done()
	}()

	buffer := func(e *bufferEvent[K, V]) {
		defer c.returnBufferEvent(e)

		k := e.k
		d, found := c.dataMap[k]

		u := func() {
			// update value using user installed functor
			if e.f != nil {
				d.v = e.f(d.v)
			}

			d.hasData = true
		}

		if !found {
			d = c.getEntryData()

			d.first = time.Now().UTC()
			u()

			c.dataMap[k] = d

			c.options.TriggerHandler(
				true, // immediate
				k,
				d.v,
			)

			// since triggered, reset data
			d.triggered()

			// schedule trigger check
			time.AfterFunc(
				c.options.CooldownInterval,
				func() {
					if c.options.LogDebug {
						log.Printf("%s: send triggerch, k=%v", c.options.LogPrefix, k)
					}

					c.triggerch <- k
				},
			)
		} else {
			u()
		}
	}

	trigger := func(k K, inExit bool) {
		d, found := c.dataMap[k]
		if !found {
			log.Printf("%s: k=%v not found in dataMap", c.options.LogPrefix, k)
			return
		}

		invoke := func() {
			c.options.TriggerHandler(
				false, // has waited
				k,
				d.v,
			)

			// since triggered, reset data
			d.triggered()
		}

		purge := func() {
			defer c.returnEntryData(d)

			if c.options.LogDebug {
				log.Printf("%s: purging k=%v, first=%v", c.options.LogPrefix, k, d.first)
			}

			delete(c.dataMap, k)
		}

		if inExit {
			if d.hasData {
				invoke()
			}

			purge()
			return
		}

		if d.hasData {
			// there was data within current cooldown cycle,
			// trigger, reset data, schedule next cycle
			invoke()

			time.AfterFunc(
				c.options.CooldownInterval,
				func() {
					if c.options.LogDebug {
						log.Printf("%s: send triggerch, k=%v", c.options.LogPrefix, k)
					}

					c.triggerch <- k
				},
			)
		} else {
			// no data within current cooldown cycle, safe to purge cache,
			// so next buffer request on this key will start new flow
			purge()
		}
	}

	flush := func() {
		if c.options.StopBehavior != TriggerOnStop {
			return
		}

		keyMap := make(map[K]struct{})
		for k := range c.dataMap {
			keyMap[k] = struct{}{}
		}

		for k := range keyMap {
			trigger(k, true)
		}
	}

	for {
		select {
		case <-c.exitch:
			log.Printf("%s: exitch received", c.options.LogPrefix)
			flush()
			return
		case e := <-c.bufferch:
			buffer(e)
		case k := <-c.triggerch:
			trigger(k, false)
		}
	}
}

// f will be invoked on internal process goroutine
func (c *Cache[K, V]) Buffer(k K, f func(V) V) {
	e := c.getBufferEvent()
	e.k = k
	e.f = f

	c.bufferch <- e
}

// f will be invoked on internal process goroutine
func (c *Cache[K, V]) TryBuffer(k K, f func(V) V) {
	e := c.getBufferEvent()
	e.k = k
	e.f = f

	select {
	case c.bufferch <- e:
	default:
		if c.options.LogDebug {
			log.Printf("%s: k=%v, failed to buffer event", c.options.LogPrefix, k)
		}
	}
}
