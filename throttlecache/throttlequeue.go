package throttlecache

import (
	"fmt"
	"log"
	"sync/atomic"
	"time"
)

type Node struct {
	next *Node
	f    func()
}

func NewNode(f func()) *Node {
	return &Node{
		next: nil,
		f:    f,
	}
}

type List struct {
	first *Node
	last  *Node
	size  uint32
}

func NewList() *List {
	return &List{
		first: nil,
		last:  nil,
		size:  0,
	}
}

func (l *List) Size() uint32 {
	if l == nil {
		return 0
	}

	return l.size
}

func (l *List) Join(list *List) {
	if list == nil ||
		list.first == nil {
		return
	}

	if l.last == nil {
		l.first = list.first
		l.last = list.last
		l.size = list.size
		return
	}

	l.last.next = list.first
	l.last = list.last
	l.size += list.size
}

func (l *List) Append(node *Node) {
	if l.last == nil {
		l.first = node
		l.last = node
		l.size = 1
		return
	}

	l.last.next = node
	l.last = node
	l.size += 1
}

func (l *List) PopFront() (*Node, bool) {
	if l.first == nil {
		return nil, false
	}

	node := l.first
	if node.next == nil {
		l.first = nil
		l.last = nil
		l.size = 0
	} else {
		l.first = node.next
		node.next = nil
		l.size -= 1
	}

	return node, true
}

type QueueOptions struct {
	CooldownInterval time.Duration

	LogPrefix string
	LogDebug  bool
}

type Queue[K comparable] struct {
	options    *QueueOptions
	inShutdown atomic.Bool
	cache      *Cache[K, *List]
	listMap    map[K]*List
}

func NewQueue[K comparable](options *QueueOptions) *Queue[K] {
	q := &Queue[K]{
		options:    options,
		inShutdown: atomic.Bool{},
		cache:      nil,
		listMap:    make(map[K]*List),
	}
	cache := NewCache(
		&Options[K, *List]{
			Handler: q,

			CooldownInterval: options.CooldownInterval,
			StopBehavior:     TriggerOnStop,

			LogPrefix: options.LogPrefix,
			LogDebug:  options.LogDebug,
		},
	)
	q.cache = cache

	return q
}

func (q *Queue[K]) Shutdown() {
	q.inShutdown.Store(true)
	q.cache.Stop()
}

func (q *Queue[K]) TriggerHandler(immediate bool, k K, list *List) {
	// invoked on throttlecache goroutine
	inShutdown := q.inShutdown.Load()

	l, found := q.listMap[k]
	if !found {
		l = NewList()

		q.listMap[k] = l
	}

	if q.options.LogDebug {
		log.Printf(
			"%s: immediate=%t, k=%v, queue<%d>, buffer<%d>, inShutdown=%t",
			q.options.LogPrefix,
			immediate,
			k,
			l.Size(),
			list.Size(),
			inShutdown,
		)
	}

	// join buffer list into queue list
	l.Join(list)

	if inShutdown {
		// if in shutdown, join lists to preserve gathered nodes, but do not execute
		return
	}

	// dequeue one node for execution
	node, found := l.PopFront()
	if !found {
		return
	}

	func() {
		defer func() {
			rec := recover()
			if rec != nil {
				log.Printf(
					"%s: k=%v, functor recovered from panic: %+v",
					q.options.LogPrefix,
					k,
					rec,
				)
			}
		}()
		node.f()
	}()

	queueSize := l.Size()

	if q.options.LogDebug {
		log.Printf(
			"%s: k=%v, queue<%d>",
			q.options.LogPrefix,
			k,
			queueSize,
		)
	}

	// signal for next trigger, in case no new node is enqueued
	if queueSize > 0 {
		q.cache.Buffer(
			k,
			nil,
		)
	}
}

func (q *Queue[K]) Enqueue(k K, f func()) error {
	inShutdown := q.inShutdown.Load()
	if inShutdown {
		err := fmt.Errorf(
			"%s: k=%v, inShutdown=%t, enqueue failed",
			q.options.LogPrefix,
			k,
			inShutdown,
		)
		log.Printf("%s", err.Error())
		return err
	}

	q.cache.Buffer(
		k,
		func(list *List) *List {
			if list == nil {
				list = NewList()
			}

			list.Append(
				NewNode(f),
			)

			return list
		},
	)
	return nil
}
