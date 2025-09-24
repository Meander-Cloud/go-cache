package gathercache

import (
	"fmt"
	"log"
	"sync"
	"time"
)

type StopBehavior uint8

const (
	NoOpOnStop   StopBehavior = 0
	MatureOnStop StopBehavior = 1
)

type Handler[K comparable, V any] interface {
	FirstHandler(time.Time, K, V)
	MaturityHandler(time.Time, K, V)
}

type Options[K comparable, V any] struct {
	Handler[K, V]

	QueueLength    uint16
	GatherInterval time.Duration
	StopBehavior   StopBehavior

	LogPrefix string
	LogDebug  bool
}

type entryData[V any] struct {
	v V

	first time.Time
}

func (d *entryData[V]) reset() {
	var v V
	d.v = v

	d.first = time.Time{}
}

type gatherEvent[K comparable, V any] struct {
	k K
	f func(V) V
}

func (e *gatherEvent[K, V]) reset() {
	var k K
	e.k = k
	e.f = nil
}

type Cache[K comparable, V any] struct {
	options *Options[K, V]

	entryDataPool   sync.Pool
	gatherEventPool sync.Pool

	exitwg   sync.WaitGroup
	exitch   chan struct{}
	gatherch chan *gatherEvent[K, V]
	maturech chan K

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
		gatherEventPool: sync.Pool{
			New: func() any {
				return &gatherEvent[K, V]{}
			},
		},

		exitwg:   sync.WaitGroup{},
		exitch:   make(chan struct{}, 1),
		gatherch: make(chan *gatherEvent[K, V], options.QueueLength),
		maturech: make(chan K, options.QueueLength),

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

func (c *Cache[K, V]) getGatherEvent() *gatherEvent[K, V] {
	eAny := c.gatherEventPool.Get()
	e, ok := eAny.(*gatherEvent[K, V])
	if !ok {
		panic(fmt.Errorf("%s: failed to cast gatherEvent, gatherEventPool corrupt, eAny=%+v", c.options.LogPrefix, eAny))
	}
	return e
}

func (c *Cache[K, V]) returnGatherEvent(e *gatherEvent[K, V]) {
	e.reset()
	c.gatherEventPool.Put(e)
}

func (c *Cache[K, V]) process() {
	log.Printf("%s: process goroutine starting", c.options.LogPrefix)

	defer func() {
		log.Printf("%s: process goroutine exiting", c.options.LogPrefix)
		c.exitwg.Done()
	}()

	gather := func(e *gatherEvent[K, V]) {
		defer c.returnGatherEvent(e)

		k := e.k
		d, found := c.dataMap[k]

		u := func() {
			// update value using user installed functor
			if e.f != nil {
				d.v = e.f(d.v)
			}
		}

		if !found {
			d = c.getEntryData()

			d.first = time.Now().UTC()
			u()

			c.dataMap[k] = d

			// schedule maturity
			time.AfterFunc(
				c.options.GatherInterval,
				func() {
					if c.options.LogDebug {
						log.Printf("%s: send maturech, k=%v", c.options.LogPrefix, k)
					}

					c.maturech <- k
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

	mature := func(k K) {
		d, found := c.dataMap[k]
		if !found {
			log.Printf("%s: k=%v not found in dataMap", c.options.LogPrefix, k)
			return
		}

		defer c.returnEntryData(d)

		delete(c.dataMap, k)

		c.options.MaturityHandler(
			d.first,
			k,
			d.v,
		)
	}

	flush := func() {
		if c.options.StopBehavior != MatureOnStop {
			return
		}

		keyMap := make(map[K]struct{})
		for k := range c.dataMap {
			keyMap[k] = struct{}{}
		}

		for k := range keyMap {
			mature(k)
		}
	}

	for {
		select {
		case <-c.exitch:
			log.Printf("%s: exitch received", c.options.LogPrefix)
			flush()
			return
		case e := <-c.gatherch:
			gather(e)
		case k := <-c.maturech:
			mature(k)
		}
	}
}

// f will be invoked on internal process goroutine
func (c *Cache[K, V]) Gather(k K, f func(V) V) {
	e := c.getGatherEvent()
	e.k = k
	e.f = f

	c.gatherch <- e
}
