package pushcache_test

import (
	"log"
	"testing"
	"time"

	"github.com/Meander-Cloud/go-cache/pushcache"
)

type Handler struct {
}

func (h *Handler) FirstHandler(first time.Time, k uint8, v []byte) {
	log.Printf("FirstHandler: first=%v, k=%v, v=%v", first, k, v)
}

func (h *Handler) IdleHandler(first, last time.Time, k uint8, v []byte) {
	log.Printf("IdleHandler: first=%v, last=%v, k=%v, v=%v", first, last, k, v)
}

func Test1(t *testing.T) {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds | log.Lshortfile)

	cache := pushcache.NewCache(
		&pushcache.Options[uint8, []byte]{
			Handler: &Handler{},

			IdleInterval: time.Second * 3,
			StopBehavior: pushcache.IdleOnStop,

			LogPrefix: "Test1",
			LogDebug:  true,
		},
	)

	for i := 0; i < 5; i++ {
		scoped_i := uint8(i)
		cache.Push(
			0,
			time.Now().UTC(),
			func(v []byte) []byte {
				return append(v, scoped_i)
			},
		)

		<-time.After(time.Second)
	}

	<-time.After(time.Second * 3)

	for i := 0; i < 5; i++ {
		scoped_i := uint8(i)
		cache.Push(
			0,
			time.Now().UTC(),
			func(v []byte) []byte {
				return append(v, scoped_i)
			},
		)

		<-time.After(time.Second)
	}

	cache.Stop()
}
