package holdcache

import (
	"fmt"
	"log"
	"sync"
	"time"
)

type StopBehavior uint8

const (
	NoOpOnStop       StopBehavior = 0
	CommitOnStop     StopBehavior = 1
	InvalidateOnStop StopBehavior = 2
)

type Handler[K comparable, V any] interface {
	CommitHandler(K, V)
	InvalidateHandler(K, V)
}

type Options[K comparable, V any] struct {
	Handler[K, V]

	QueueLength  uint16
	StopBehavior StopBehavior

	LogPrefix string
	LogDebug  bool
}

type entryData[V any] struct {
	v V

	seqnum uint32
	timer  *time.Timer
}

func (d *entryData[V]) reset() {
	var v V
	d.v = v

	d.seqnum = 0
	d.timer = nil
}

type timerData[K any] struct {
	k K

	seqnum uint32
}

func (t *timerData[K]) reset() {
	var k K
	t.k = k

	t.seqnum = 0
}

type Event interface {
	isEvent()
}

type holdEvent[K comparable, V any] struct {
	k K
	v V
	d time.Duration
}

func (e *holdEvent[K, V]) reset() {
	var k K
	e.k = k

	var v V
	e.v = v

	e.d = 0
}

func (e *holdEvent[K, V]) isEvent() {}

type commitEvent[K comparable, V any] struct {
	k K
	v V
}

func (e *commitEvent[K, V]) reset() {
	var k K
	e.k = k

	var v V
	e.v = v
}

func (e *commitEvent[K, V]) isEvent() {}

type invalidateEvent[K comparable] struct {
	k K
}

func (e *invalidateEvent[K]) reset() {
	var k K
	e.k = k
}

func (e *invalidateEvent[K]) isEvent() {}

type Cache[K comparable, V any] struct {
	options *Options[K, V]

	entryDataPool       sync.Pool
	timerDataPool       sync.Pool
	holdEventPool       sync.Pool
	commitEventPool     sync.Pool
	invalidateEventPool sync.Pool

	exitwg  sync.WaitGroup
	exitch  chan struct{}
	eventch chan Event
	timerch chan *timerData[K]

	seqnum  uint32
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
		timerDataPool: sync.Pool{
			New: func() any {
				return &timerData[K]{}
			},
		},
		holdEventPool: sync.Pool{
			New: func() any {
				return &holdEvent[K, V]{}
			},
		},
		commitEventPool: sync.Pool{
			New: func() any {
				return &commitEvent[K, V]{}
			},
		},
		invalidateEventPool: sync.Pool{
			New: func() any {
				return &invalidateEvent[K]{}
			},
		},

		exitwg:  sync.WaitGroup{},
		exitch:  make(chan struct{}, 1),
		eventch: make(chan Event, options.QueueLength),
		timerch: make(chan *timerData[K], options.QueueLength),

		seqnum:  0,
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

func (c *Cache[K, V]) getTimerData() *timerData[K] {
	tAny := c.timerDataPool.Get()
	t, ok := tAny.(*timerData[K])
	if !ok {
		panic(fmt.Errorf("failed to cast timerData, timerDataPool corrupt, tAny=%+v", tAny))
	}
	return t
}

func (c *Cache[K, V]) returnTimerData(t *timerData[K]) {
	t.reset()
	c.timerDataPool.Put(t)
}

func (c *Cache[K, V]) getHoldEvent() *holdEvent[K, V] {
	eAny := c.holdEventPool.Get()
	e, ok := eAny.(*holdEvent[K, V])
	if !ok {
		panic(fmt.Errorf("failed to cast holdEvent, holdEventPool corrupt, eAny=%+v", eAny))
	}
	return e
}

func (c *Cache[K, V]) returnHoldEvent(e *holdEvent[K, V]) {
	e.reset()
	c.holdEventPool.Put(e)
}

func (c *Cache[K, V]) getCommitEvent() *commitEvent[K, V] {
	eAny := c.commitEventPool.Get()
	e, ok := eAny.(*commitEvent[K, V])
	if !ok {
		panic(fmt.Errorf("failed to cast commitEvent, commitEventPool corrupt, eAny=%+v", eAny))
	}
	return e
}

func (c *Cache[K, V]) returnCommitEvent(e *commitEvent[K, V]) {
	e.reset()
	c.commitEventPool.Put(e)
}

func (c *Cache[K, V]) getInvalidateEvent() *invalidateEvent[K] {
	eAny := c.invalidateEventPool.Get()
	e, ok := eAny.(*invalidateEvent[K])
	if !ok {
		panic(fmt.Errorf("failed to cast invalidateEvent, invalidateEventPool corrupt, eAny=%+v", eAny))
	}
	return e
}

func (c *Cache[K, V]) returnInvalidateEvent(e *invalidateEvent[K]) {
	e.reset()
	c.invalidateEventPool.Put(e)
}

func (c *Cache[K, V]) process() {
	log.Printf("%s: process goroutine starting", c.options.LogPrefix)

	defer func() {
		log.Printf("%s: process goroutine exiting", c.options.LogPrefix)
		c.exitwg.Done()
	}()

	invalidateHeld := func(k K, dCached *entryData[V]) {
		// invalidate previously held data
		dCached.timer.Stop()
		// note that in the race condition where previous timer has already triggered concurrently
		// seqnum check in timer() will drop such data to maintain consistency

		defer c.returnEntryData(dCached)

		delete(c.dataMap, k)

		c.options.InvalidateHandler(
			k,
			dCached.v,
		)
	}

	handleHold := func(e *holdEvent[K, V]) {
		defer c.returnHoldEvent(e)

		// advance
		c.seqnum++

		k := e.k
		dCached, found := c.dataMap[k]

		createNew := func() {
			d := c.getEntryData()

			d.v = e.v
			d.seqnum = c.seqnum

			c.dataMap[k] = d

			// schedule timer
			t := c.getTimerData()
			t.k = k
			t.seqnum = d.seqnum

			d.timer = time.AfterFunc(
				e.d,
				func() {
					if c.options.LogDebug {
						log.Printf("%s: send timerch, k=%v, seqnum=%d", c.options.LogPrefix, t.k, t.seqnum)
					}

					c.timerch <- t
				},
			)
		}

		if found {
			invalidateHeld(k, dCached)
		}

		createNew()
	}

	handleCommit := func(e *commitEvent[K, V]) {
		defer c.returnCommitEvent(e)

		// advance
		c.seqnum++

		k := e.k
		dCached, found := c.dataMap[k]

		if found {
			invalidateHeld(k, dCached)
		}

		c.options.CommitHandler(
			k,
			e.v,
		)
		// no need to cache
	}

	handleInvalidate := func(e *invalidateEvent[K]) {
		defer c.returnInvalidateEvent(e)

		// advance
		c.seqnum++

		k := e.k
		dCached, found := c.dataMap[k]

		if found {
			invalidateHeld(k, dCached)
		}
	}

	handleTimer := func(t *timerData[K]) {
		defer c.returnTimerData(t)

		k := t.k
		dCached, found := c.dataMap[k]

		if !found {
			if c.options.LogDebug {
				log.Printf("%s: k=%v not found in dataMap", c.options.LogPrefix, k)
			}
			return
		}

		if t.seqnum != dCached.seqnum {
			if c.options.LogDebug {
				log.Printf("%s: k=%v, t.seqnum=%d, dCached.seqnum=%d, no-op", c.options.LogPrefix, k, t.seqnum, dCached.seqnum)
			}
			return
		}

		defer c.returnEntryData(dCached)

		delete(c.dataMap, k)

		c.options.CommitHandler(
			k,
			dCached.v,
		)
	}

	flush := func() {
		if c.options.StopBehavior == NoOpOnStop {
			return
		}

		scopedMap := make(map[K]V)
		for k, d := range c.dataMap {
			scopedMap[k] = d.v
		}

		for k, v := range scopedMap {
			switch c.options.StopBehavior {
			case CommitOnStop:
				e := c.getCommitEvent()
				e.k = k
				e.v = v
				handleCommit(e)
			case InvalidateOnStop:
				e := c.getInvalidateEvent()
				e.k = k
				handleInvalidate(e)
			}
		}
	}

	for {
		select {
		case <-c.exitch:
			log.Printf("%s: exitch received", c.options.LogPrefix)
			flush()
			return
		case event := <-c.eventch:
			switch e := event.(type) {
			case *holdEvent[K, V]:
				handleHold(e)
			case *commitEvent[K, V]:
				handleCommit(e)
			case *invalidateEvent[K]:
				handleInvalidate(e)
			}
		case t := <-c.timerch:
			handleTimer(t)
		}
	}
}

func (c *Cache[K, V]) Hold(k K, v V, d time.Duration) {
	e := c.getHoldEvent()
	e.k = k
	e.v = v
	e.d = d

	c.eventch <- e
}

func (c *Cache[K, V]) Commit(k K, v V) {
	e := c.getCommitEvent()
	e.k = k
	e.v = v

	c.eventch <- e
}

func (c *Cache[K, V]) Invalidate(k K) {
	e := c.getInvalidateEvent()
	e.k = k

	c.eventch <- e
}
