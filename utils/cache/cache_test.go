package cache

import (
	"fmt"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestCacheBasicCRUD(t *testing.T) {
	cache := NewCache(5)
	for i := 0; i < 10; i++ {
		key := fmt.Sprintf("key%d", i)
		val := fmt.Sprintf("val%d", i)
		cache.Set(key, val)
		fmt.Printf("set %s: %s\n", key, cache)
	}

	for i := 0; i < 1000; i++ {
		key := fmt.Sprintf("key%d", i)
		val := fmt.Sprintf("val%d", i)
		res, ok := cache.Get(key)
		if ok {
			fmt.Printf("get %s: %s\n", key, cache)
			assert.Equal(t, val, res)
			continue
		}
		assert.Equal(t, res, nil)

	}
	fmt.Printf("at last: %s\n", cache)
}

// func TestCacheSameKey(t *testing.T) {
// 	cache := NewCache(500)
// 	key := "one"
// 	for i := 0; i < 10; i++ {
// 		val := fmt.Sprintf("val%d", i)
// 		cache.Set(key, val)
// 		res, ok := cache.Get(key)
// 		if ok {
// 			assert.Equal(t, val, res)
// 			continue
// 		}
// 		assert.Equal(t, res, nil)
// 	}
// }
