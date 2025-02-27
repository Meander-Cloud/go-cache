package throttlecache_test

import (
	"log"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Meander-Cloud/go-cache/throttlecache"
)

func Test2(t *testing.T) {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds | log.Lshortfile)

	queue := throttlecache.NewQueue[struct{}](
		&throttlecache.QueueOptions{
			CooldownInterval: time.Second * 3,

			LogPrefix: "Test2",
			LogDebug:  true,
		},
	)

	count := 0
	make := func() func() {
		count += 1
		scopedCount := count

		return func() {
			// invoked on throttlecache goroutine
			log.Printf("Test2: %d", scopedCount)
		}
	}

	queue.Enqueue(struct{}{}, make())
	<-time.After(time.Second * 4)

	for i := 0; i < 7; i++ {
		queue.Enqueue(struct{}{}, make())
		<-time.After(time.Second)
	}
	queue.Enqueue(struct{}{}, make())
	queue.Enqueue(struct{}{}, make()) // based on timing, this event should be enqueued but not executed since in shutdown

	<-time.After(time.Second * 15)
	queue.Shutdown()

	queue.Enqueue(struct{}{}, make()) // enqueue fails
}

func Test3(t *testing.T) {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds | log.Lshortfile)

	queue := throttlecache.NewQueue[string](
		&throttlecache.QueueOptions{
			CooldownInterval: time.Second * 2,

			LogPrefix: "Test3",
			LogDebug:  true,
		},
	)

	count := atomic.Uint32{}
	make := func() func() {
		scopedCount := count.Add(1)

		return func() {
			// invoked on throttlecache goroutine
			log.Printf("Test3: %d", scopedCount)
		}
	}

	go func() {
		for i := 0; i < 8; i++ {
			queue.Enqueue("A", make())
			<-time.After(time.Millisecond * 2400)
		}
	}()

	<-time.After(time.Second)
	go func() {
		for i := 0; i < 10; i++ {
			queue.Enqueue("B", make())
			<-time.After(time.Millisecond * 800)
		}
	}()

	<-time.After(time.Second * 22)
	queue.Shutdown()
}
