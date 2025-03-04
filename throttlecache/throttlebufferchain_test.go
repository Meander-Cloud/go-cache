package throttlecache_test

import (
	"log"
	"testing"
	"time"

	"github.com/Meander-Cloud/go-cache/throttlecache"
)

type Test4_Handler struct {
	logPrefix string
}

func (h *Test4_Handler) TriggerHandler(immediate bool, _ struct{}, v []byte) {
	log.Printf("%s: TriggerHandler: immediate=%t, v=%v", h.logPrefix, immediate, v)
}

func Test4(t *testing.T) {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds | log.Lshortfile)

	bc := throttlecache.NewBufferChain(
		&throttlecache.BufferChainOptions[struct{}, byte]{
			Handler: &Test4_Handler{
				logPrefix: "Test4",
			},

			TargetSize:       10,
			CooldownInterval: time.Second * 2,

			LogPrefix: "Test4",
			LogDebug:  true,
		},
	)

	bc.Buffer(
		struct{}{},
		nil,
	)

	bc.Buffer(
		struct{}{},
		[]byte{0, 1, 2, 3},
	)

	bc.Buffer(
		struct{}{},
		[]byte{},
	)

	bc.Buffer(
		struct{}{},
		[]byte{10, 11, 12, 13, 14, 15, 16, 17},
	)

	bc.Buffer(
		struct{}{},
		[]byte{},
	)

	bc.Buffer(
		struct{}{},
		nil,
	)

	<-time.After(time.Second * 3)

	bc.Buffer(
		struct{}{},
		[]byte{20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32, 33, 34, 35},
	)

	<-time.After(time.Second * 4)

	bc.Buffer(
		struct{}{},
		[]byte{40, 41, 42},
	)

	bc.Buffer(
		struct{}{},
		[]byte{50, 51, 52},
	)

	bc.Buffer(
		struct{}{},
		[]byte{60, 61, 62},
	)

	bc.Buffer(
		struct{}{},
		[]byte{70, 71, 72, 73, 74, 75, 76, 77, 78, 79, 80, 81, 82},
	)

	bc.Buffer(
		struct{}{},
		[]byte{90, 91, 92},
	)

	bc.Buffer(
		struct{}{},
		[]byte{100, 101, 102},
	)

	bc.Buffer(
		struct{}{},
		[]byte{110, 111, 112},
	)

	<-time.After(time.Second * 4)
	bc.Shutdown()
}
