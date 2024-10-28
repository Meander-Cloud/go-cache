package throttlecache_test

import (
	"log"
	"testing"
	"time"

	"github.com/Meander-Cloud/go-cache/throttlecache"
)

type Handler struct {
}

func (h *Handler) TriggerHandler(immediate bool, k uint8, v []byte) {
	log.Printf("TriggerHandler: immediate=%t, k=%v, v=%v", immediate, k, v)
}

func Test1(t *testing.T) {
	cache := throttlecache.NewCache[uint8, []byte](
		&throttlecache.Options[uint8, []byte]{
			Handler: &Handler{},

			QueueLength:      256,
			CooldownInterval: time.Second * 3,

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

	for i := 0; i < 5; i++ {
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
