package throttlecache

import (
	"log"
	"time"
)

type Buffer[V any] struct {
	next *Buffer[V]
	buf  []V
}

func NewBuffer[V any](buf []V) *Buffer[V] {
	return &Buffer[V]{
		next: nil,
		buf:  buf,
	}
}

func (b *Buffer[V]) Size() uint32 {
	if b == nil {
		return 0
	}

	return uint32(len(b.buf))
}

type Chain[V any] struct {
	first *Buffer[V]
	last  *Buffer[V]
	size  uint32
}

func NewChain[V any]() *Chain[V] {
	return &Chain[V]{
		first: nil,
		last:  nil,
		size:  0,
	}
}

func (c *Chain[V]) Size() uint32 {
	if c == nil {
		return 0
	}

	return c.size
}

func (c *Chain[V]) Append(buffer *Buffer[V]) {
	if c.last == nil {
		c.first = buffer
		c.last = buffer
		c.size = buffer.Size()
		return
	}

	c.last.next = buffer
	c.last = buffer
	c.size += buffer.Size()
}

func (c *Chain[V]) Adjust(target uint32) {
	if c.size <= target {
		return
	}

	for {
		b := c.first
		if b == nil {
			return
		}

		// retain buffers such that total size is no less than target
		bSize := b.Size()
		if c.size < target+bSize {
			return
		}

		// remove front
		if b.next == nil {
			c.first = nil
			c.last = nil
			c.size = 0
			return
		} else {
			c.first = b.next
			b.next = nil
			c.size -= bSize
			continue
		}
	}
}

func (c *Chain[V]) Render(target uint32) []V {
	if c == nil {
		return nil
	}

	c.Adjust(target)

	if c.first == nil ||
		c.size == 0 {
		return []V{}
	}

	var buf []V
	var firstBufferIndex uint32 = 0
	var firstBufferSize uint32 = c.first.Size()

	if c.size <= target {
		buf = make([]V, 0, c.size)
	} else {
		buf = make([]V, 0, target)

		index := c.size - target
		if index < firstBufferSize {
			firstBufferIndex = index
		} else {
			log.Printf(
				"invalid chain state, chainSize=%d, target=%d, firstBufferSize=%d",
				c.size,
				target,
				firstBufferSize,
			)
			return nil
		}
	}

	buf = append(
		buf,
		c.first.buf[firstBufferIndex:]...,
	)

	b := c.first.next
	for {
		if b == nil {
			break
		}

		buf = append(
			buf,
			b.buf...,
		)

		b = b.next
	}

	return buf
}

type BufferChainOptions[K comparable, V any] struct {
	Handler[K, []V]

	TargetSize       uint32
	CooldownInterval time.Duration

	LogPrefix string
	LogDebug  bool
}

type BufferChain[K comparable, V any] struct {
	options *BufferChainOptions[K, V]
	cache   *Cache[K, *Chain[V]]
}

func NewBufferChain[K comparable, V any](options *BufferChainOptions[K, V]) *BufferChain[K, V] {
	z := &BufferChain[K, V]{
		options: options,
		cache:   nil,
	}
	cache := NewCache(
		&Options[K, *Chain[V]]{
			Handler: z,

			CooldownInterval: options.CooldownInterval,
			StopBehavior:     NoOpOnStop,

			LogPrefix: options.LogPrefix,
			LogDebug:  options.LogDebug,
		},
	)
	z.cache = cache

	return z
}

func (z *BufferChain[K, V]) Shutdown() {
	z.cache.Stop()
}

func (z *BufferChain[K, V]) TriggerHandler(immediate bool, k K, chain *Chain[V]) {
	chainSize := chain.Size()

	if z.options.LogDebug {
		log.Printf(
			"%s: immediate=%t, k=%v, chain<%d/%d>",
			z.options.LogPrefix,
			immediate,
			k,
			chainSize,
			z.options.TargetSize,
		)
	}

	buf := chain.Render(
		z.options.TargetSize,
	)

	func() {
		defer func() {
			rec := recover()
			if rec != nil {
				log.Printf(
					"%s: k=%v, functor recovered from panic: %+v",
					z.options.LogPrefix,
					k,
					rec,
				)
			}
		}()
		z.options.TriggerHandler(
			immediate,
			k,
			buf,
		)
	}()
}

func (z *BufferChain[K, V]) Buffer(k K, buf []V) {
	z.cache.Buffer(
		k,
		func(chain *Chain[V]) *Chain[V] {
			if chain == nil {
				chain = NewChain[V]()
			}

			chain.Append(
				NewBuffer(buf),
			)

			chain.Adjust(
				z.options.TargetSize,
			)

			return chain
		},
	)
}
