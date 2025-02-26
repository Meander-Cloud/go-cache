package throttlecache_test

import (
	"log"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Meander-Cloud/go-cache/throttlecache"
)

type Test1_Handler struct {
	logPrefix string
}

func (h *Test1_Handler) TriggerHandler(immediate bool, k uint8, v []byte) {
	log.Printf("%s: TriggerHandler: immediate=%t, k=%v, v=%v", h.logPrefix, immediate, k, v)
}

func Test1(t *testing.T) {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds | log.Lshortfile)

	cache := throttlecache.NewCache(
		&throttlecache.Options[uint8, []byte]{
			Handler: &Test1_Handler{
				logPrefix: "Test1",
			},

			QueueLength:      256,
			CooldownInterval: time.Second * 3,
			StopBehavior:     throttlecache.TriggerOnStop,

			LogPrefix: "Test1",
			LogDebug:  true,
		},
	)

	cache.Buffer(
		0,
		func(v []byte) []byte {
			return append(v, 0)
		},
	)
	<-time.After(time.Second * 4)

	for i := 0; i < 7; i++ {
		scoped_i := uint8(i)
		cache.Buffer(
			0,
			func(v []byte) []byte {
				return append(v, scoped_i)
			},
		)

		<-time.After(time.Second)
	}
	<-time.After(time.Second * 6)

	cache.Stop()
}

type Test2_Node struct {
	next *Test2_Node
	f    func()
}

func NewNode(f func()) *Test2_Node {
	return &Test2_Node{
		next: nil,
		f:    f,
	}
}

type Test2_Queue struct {
	first *Test2_Node
	last  *Test2_Node
	size  uint32
}

func NewQueue() *Test2_Queue {
	return &Test2_Queue{
		first: nil,
		last:  nil,
		size:  0,
	}
}

func NewQueueWithNode(node *Test2_Node) *Test2_Queue {
	return &Test2_Queue{
		first: node,
		last:  node,
		size:  1,
	}
}

func (q *Test2_Queue) Join(queue *Test2_Queue) {
	if queue == nil ||
		queue.first == nil {
		return
	}

	if q.last == nil {
		q.first = queue.first
		q.last = queue.last
		q.size = queue.size
		return
	}

	q.last.next = queue.first
	q.last = queue.last
	q.size += queue.size
}

func (q *Test2_Queue) Enqueue(node *Test2_Node) {
	if q.last == nil {
		q.first = node
		q.last = node
		q.size = 1
		return
	}

	q.last.next = node
	q.last = node
	q.size += 1
}

func (q *Test2_Queue) Dequeue() (*Test2_Node, bool) {
	if q.first == nil {
		return nil, false
	}

	node := q.first
	if node.next == nil {
		q.first = nil
		q.last = nil
		q.size = 0
	} else {
		q.first = node.next
		node.next = nil
		q.size -= 1
	}

	return node, true
}

type Test2_Handler struct {
	logPrefix  string
	c          *throttlecache.Cache[struct{}, *Test2_Queue]
	q          *Test2_Queue
	inShutdown atomic.Bool
}

func (h *Test2_Handler) TriggerHandler(immediate bool, _ struct{}, queue *Test2_Queue) {
	// invoked on throttlecache goroutine
	inShutdown := h.inShutdown.Load()
	log.Printf(
		"%s: TriggerHandler: immediate=%t, current queue size %d, incoming queue size %d, inShutdown=%t",
		h.logPrefix,
		immediate,
		h.q.size,
		func() uint32 {
			if queue == nil {
				return 0
			}
			return queue.size
		}(),
		inShutdown,
	)

	// join queues
	h.q.Join(queue)

	if inShutdown {
		// if in shutdown, join queues to preserve gathered nodes, but do not execute
		return
	}

	// dequeue one node for execution
	node, found := h.q.Dequeue()
	if !found {
		return
	}

	func() {
		defer func() {
			rec := recover()
			if rec != nil {
				log.Printf(
					"%s: functor recovered from panic: %+v",
					h.logPrefix,
					rec,
				)
			}
		}()
		node.f()
	}()

	if h.q.size > 0 {
		// signal for next trigger
		h.c.Buffer(
			struct{}{},
			nil,
		)
	}
}

func Test2(t *testing.T) {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds | log.Lshortfile)

	handler := &Test2_Handler{
		logPrefix:  "Test2",
		c:          nil,
		q:          NewQueue(),
		inShutdown: atomic.Bool{},
	}
	cache := throttlecache.NewCache(
		&throttlecache.Options[struct{}, *Test2_Queue]{
			Handler: handler,

			QueueLength:      256,
			CooldownInterval: time.Second * 3,
			StopBehavior:     throttlecache.TriggerOnStop,

			LogPrefix: "Test2",
			LogDebug:  true,
		},
	)
	handler.c = cache

	count := 0
	testEnqueue := func() {
		count += 1
		scopedCount := count

		cache.Buffer(
			struct{}{},
			func(queue *Test2_Queue) *Test2_Queue {
				if queue == nil {
					queue = NewQueue()
				}

				queue.Enqueue(
					NewNode(
						func() {
							// invoked on throttlecache goroutine
							log.Printf("Test2: %d", scopedCount)
						},
					),
				)
				return queue
			},
		)
	}

	testEnqueue()
	<-time.After(time.Second * 4)

	for i := 0; i < 7; i++ {
		testEnqueue()
		<-time.After(time.Second)
	}
	testEnqueue()
	testEnqueue() // based on timing, this event should be enqueued but not executed since cache stopping

	<-time.After(time.Second * 15)
	handler.inShutdown.Store(true)
	cache.Stop()
}
