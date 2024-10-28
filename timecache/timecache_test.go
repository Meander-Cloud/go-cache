package timecache_test

import (
	"fmt"
	"log"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/Meander-Cloud/go-cache/timecache"
)

type Handler[K comparable, V any] struct {
}

func (h *Handler[K, V]) ExpiryHandler(ttl [2]int64, k K, v V) {
	log.Printf("ExpiryHandler: ttl=%+v, k=%v, v=%v", ttl, k, v)
}

func TestDict(t *testing.T) {
	data := map[string]interface{}{"k": "v", "m": "1", "n": "0", "p": 1, "q": 0}
	// t.Logf("not exist: %v, %v", data["m"], data["m"] == "")
	num, ok := data["m"].(float64)
	t.Logf("m: %v, %v, %v", num, ok, reflect.TypeOf(num))
	num1, ok := data["n"].(float64)
	t.Logf("n: %v, %v, %v", num1, ok, reflect.TypeOf(num1))
	num2, ok := data["p"].(float64)
	t.Logf("p: %v, %v, %v", num2, ok, reflect.TypeOf(data["p"]))
	num3, ok := data["q"].(int)
	t.Logf("q: %v, %v, %v", num3, ok, reflect.TypeOf(data["q"]))

	dict := make(map[string]interface{})
	newInstrument, ok := dict["instrument"].(string)
	t.Logf("dict: %v, %v, %v", dict, reflect.TypeOf(dict), dict != nil)
	t.Logf("newDevice: %v, %v, %v", newInstrument, ok, newInstrument == "")
}

func TestCache(t *testing.T) {
	cache := timecache.NewCache[string, string](
		&timecache.Options[string, string]{
			Handler: &Handler[string, string]{},

			ExpireInterval: time.Minute * 5,

			LogPrefix: "TestCache",
			LogDebug:  true,
		},
	)

	cache.Set("key1", "value1")
	cache.Set("key1", "value3")
	cache.Set("key2", "value2")

	value, _ := cache.Get("key1")
	t.Logf("type of value: %v", reflect.TypeOf(value))
	if value != "value3" {
		t.Errorf("Cache.Set failed: expected value 'value3', got '%v'", value)
	}
	cache.Delete("key1")

	value, ok := cache.Get("key1")
	if ok {
		t.Errorf("Cache.Get failed: expected key not found, got '%v'", value)
	}

	value, _ = cache.Get("key3")
	t.Logf("Cache. key not exist, got '%v', ok '%v'", value, ok)

	cache.Clear()

	cache.Stop()
}

func TestCache_RaceCondition(t *testing.T) {
	cache := timecache.NewCache[string, int](
		&timecache.Options[string, int]{
			Handler: &Handler[string, int]{},

			ExpireInterval: time.Minute * 5,

			LogPrefix: "TestCache_RaceCondition",
			LogDebug:  true,
		},
	)

	numRoutines := 100
	numIterations := 1000

	var wg sync.WaitGroup
	wg.Add(numRoutines)

	for i := 0; i < numRoutines; i++ {
		go func(id int) {
			defer wg.Done()

			for j := 0; j < numIterations; j++ {
				key := fmt.Sprintf("%d-%d", id, j)
				value := id*numIterations + j + 1

				cache.Set(key, value)

				retrievedValue, _ := cache.Get(key)
				if retrievedValue != value {
					t.Errorf("Cache.Get failed: expected value %d, got %d", value, retrievedValue)
				}
			}
		}(i)
	}

	wg.Wait()

	cache.Stop()
}

func TestCache_ReadWriteConcurrency(t *testing.T) {
	cache := timecache.NewCache[string, string](
		&timecache.Options[string, string]{
			Handler: &Handler[string, string]{},

			ExpireInterval: time.Minute * 5,

			LogPrefix: "TestCache_ReadWriteConcurrency",
			LogDebug:  true,
		},
	)

	cache.Set("key", "value1")

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		cache.Set("key", "value2")
		t.Logf("%v: value2 set! ", time.Now())
	}()

	var readValue string
	go func() {
		defer wg.Done()
		readValue, _ := cache.Get("key")
		t.Logf("%v: read value: %v", time.Now(), readValue)
	}()

	wg.Wait()

	t.Logf("%v: got value: %v", time.Now(), readValue)
	value, _ := cache.Get("key")
	t.Logf("%v: read again: %v", time.Now(), value)

	cache.Stop()
}
