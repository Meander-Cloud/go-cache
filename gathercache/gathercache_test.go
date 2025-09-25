package gathercache_test

import (
	"log"
	"testing"
	"time"

	"github.com/Meander-Cloud/go-cache/gathercache"
)

type Handler struct {
}

func (h *Handler) FirstHandler(first time.Time, k uint8, v []byte) {
	log.Printf("FirstHandler: first=%v, k=%v, v=%v", first, k, v)
}

func (h *Handler) MaturityHandler(first time.Time, k uint8, v []byte) {
	log.Printf("MaturityHandler: first=%v, k=%v, v=%v", first, k, v)
}

func Test1(t *testing.T) {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds | log.Lshortfile)

	cache := gathercache.NewCache(
		&gathercache.Options[uint8, []byte]{
			Handler: &Handler{},

			GatherInterval: time.Second * 5,
			StopBehavior:   gathercache.MatureOnStop,

			LogPrefix: "Test1",
			LogDebug:  true,
		},
	)

	for i := 0; i < 12; i++ {
		scoped_i := uint8(i)
		cache.Gather(
			0,
			func(v []byte) []byte {
				return append(v, scoped_i)
			},
		)

		<-time.After(time.Second)
	}

	cache.Stop()
}
