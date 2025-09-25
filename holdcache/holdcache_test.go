package holdcache_test

import (
	"log"
	"testing"
	"time"

	"github.com/Meander-Cloud/go-cache/holdcache"
)

type Handler struct {
}

func (h *Handler) CommitHandler(k uint8, v func() string) {
	log.Printf("CommitHandler: k=%d, v=%s", k, v())
}

func (h *Handler) InvalidateHandler(k uint8, v func() string) {
	log.Printf("InvalidateHandler: k=%d, v=%s", k, v())
}

func Test1(t *testing.T) {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds | log.Lshortfile)

	cache := holdcache.NewCache(
		&holdcache.Options[uint8, func() string]{
			Handler: &Handler{},

			StopBehavior: holdcache.InvalidateOnStop,

			LogPrefix: "Test1",
			LogDebug:  true,
		},
	)

	cache.Hold(
		0,
		func() string {
			return "1"
		},
		time.Second*5,
	)

	<-time.After(time.Second * 2)
	cache.Hold(
		0,
		func() string {
			return "2"
		},
		time.Second*2,
	)

	<-time.After(time.Second * 3)
	cache.Hold(
		0,
		func() string {
			return "3"
		},
		time.Second*5,
	)

	<-time.After(time.Second * 2)
	cache.Invalidate(
		0,
	)

	<-time.After(time.Second)
	cache.Hold(
		0,
		func() string {
			return "4"
		},
		time.Second*5,
	)

	<-time.After(time.Second * 2)
	cache.Commit(
		0,
		func() string {
			return "5"
		},
	)

	<-time.After(time.Second)
	cache.Hold(
		0,
		func() string {
			return "6"
		},
		time.Second*5,
	)

	<-time.After(time.Second)
	cache.Hold(
		0,
		func() string {
			return "7"
		},
		time.Second*5,
	)

	<-time.After(time.Second * 2)
	cache.Stop()
}
