package throttlecache_test

import (
	"log"
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
