package timecache_test

import (
	"log"
	"testing"
	"time"

	"github.com/Meander-Cloud/go-cache/timecache"
)

type Handler[K comparable, V any] struct {
}

func (h *Handler[K, V]) ExpiryHandler(ttl [2]int64, k K, v V) {
	log.Printf("ExpiryHandler: ttl=%+v, k=%v, v=%v", ttl, k, v)
}

func Test1(t *testing.T) {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds | log.Lshortfile)

	cache := timecache.NewCache(
		&timecache.Options[uint8, string]{
			Handler: &Handler[uint8, string]{},

			ExpireInterval: time.Second * 5,

			LogPrefix: "Test1",
			LogDebug:  true,
		},
	)

	cache.Set(1, "a")
	cache.SetX(2, "b", time.Second*3)
	cache.SetX(3, "c", time.Second*3)

	var v1, v2, v3 string
	var f1, f2, f3 bool

	<-time.After(time.Millisecond * 2990)
	v1, f1 = cache.Get(1)
	log.Printf("Test1: v1=%s, f1=%t", v1, f1)
	v2, f2 = cache.Get(2)
	log.Printf("Test1: v2=%s, f2=%t", v2, f2)
	v3, f3 = cache.Get(3)
	log.Printf("Test1: v3=%s, f3=%t", v3, f3)
	cache.SetX(3, "d", time.Second*3)

	<-time.After(time.Millisecond * 20)
	v1, f1 = cache.Get(1)
	log.Printf("Test1: v1=%s, f1=%t", v1, f1)
	v2, f2 = cache.Get(2)
	log.Printf("Test1: v2=%s, f2=%t", v2, f2)
	v3, f3 = cache.Get(3)
	log.Printf("Test1: v3=%s, f3=%t", v3, f3)

	<-time.After(time.Millisecond * 1980)
	v1, f1 = cache.Get(1)
	log.Printf("Test1: v1=%s, f1=%t", v1, f1)
	v2, f2 = cache.Get(2)
	log.Printf("Test1: v2=%s, f2=%t", v2, f2)
	v3, f3 = cache.Get(3)
	log.Printf("Test1: v3=%s, f3=%t", v3, f3)

	<-time.After(time.Millisecond * 20)
	v1, f1 = cache.Get(1)
	log.Printf("Test1: v1=%s, f1=%t", v1, f1)
	v2, f2 = cache.Get(2)
	log.Printf("Test1: v2=%s, f2=%t", v2, f2)
	v3, f3 = cache.Get(3)
	log.Printf("Test1: v3=%s, f3=%t", v3, f3)

	<-time.After(time.Millisecond * 970)
	v1, f1 = cache.Get(1)
	log.Printf("Test1: v1=%s, f1=%t", v1, f1)
	v2, f2 = cache.Get(2)
	log.Printf("Test1: v2=%s, f2=%t", v2, f2)
	v3, f3 = cache.Get(3)
	log.Printf("Test1: v3=%s, f3=%t", v3, f3)

	<-time.After(time.Millisecond * 12)
	v1, f1 = cache.Get(1)
	log.Printf("Test1: v1=%s, f1=%t", v1, f1)
	v2, f2 = cache.Get(2)
	log.Printf("Test1: v2=%s, f2=%t", v2, f2)
	v3, f3 = cache.Get(3)
	log.Printf("Test1: v3=%s, f3=%t", v3, f3)

	cache.Stop()
}
