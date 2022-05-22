// 参考https://github.dev/goburrow/cache/blob/master/lru_test.go编写测试用例

package cache

import (
	"container/list"
	xxhash "github.com/cespare/xxhash/v2"
	"sync"
	"unsafe"
)

type Cache struct {
	m         sync.RWMutex
	lru       *windowLRU
	slru      *segmentedLRU
	door      *BloomFilter
	c         *cmSketch
	t         int32
	threshold int32
	data      map[uint64]*list.Element
}

type Options struct {
	lruPct uint8
}

func NewCache(size int) *Cache {
	const lruPct = 1
	lruSz := (lruPct * size) / 100

	if lruSz < 1 {
		lruSz = 1
	}

	slruSz := int(float64(size) * ((100 - lruPct) / 100.0))

	if slruSz < 1 {
		slruSz = 1
	}

	slruO := int(0.2 * float64(slruSz))

	if slruO < 1 {
		slruO = 1
	}

	data := make(map[uint64]*list.Element, size)

	return &Cache{
		lru:  newWindowLRU(lruSz, data),
		slru: newSLRU(data, slruO, slruSz-slruO),
		door: newFilter(size, 0.01),
		c:    newCmSketch(cmDepth, int64(size)),
		data: data,
	}

}

func (c *Cache) Len() int { return len(c.data) }

// Set 设置缓存, evicted表示是否有缓存被淘汰
func (c *Cache) Set(key interface{}, value interface{}) (evicted bool) {
	c.m.Lock()
	defer c.m.Unlock()
	if _, existed := c.get(key); !existed {
		_, evicted = c.set(key, value)
		return evicted
	}
	// 若key已存在, 只需更新value
	keyHash, _ := c.keyToHash(key)
	val, _ := c.data[keyHash]
	item := val.Value.(*storeItem)
	item.value = value
	return false
}

// set 设置缓存, evicted表示是否有缓存被淘汰, 若为true, 则storeItem是被淘汰的数据
func (c *Cache) set(key, value interface{}) (eitem storeItem, evicted bool) {
	keyHash, conflictHash := c.keyToHash(key)

	i := storeItem{
		stage:    0,
		key:      keyHash,
		conflict: conflictHash,
		value:    value,
	}

	eitem, evicted = c.lru.add(i)

	if !evicted {
		return storeItem{}, false
	}

	victim := c.slru.victim()

	if victim == nil {
		return c.slru.add(eitem)
	}

	if !c.door.Allow(uint32(keyHash)) {
		return storeItem{}, false
	}

	vcount := c.c.Estimate(victim.key)
	ocount := c.c.Estimate(eitem.key)

	if ocount < vcount {
		return storeItem{}, false
	}

	return c.slru.add(eitem)
}

func (c *Cache) Get(key interface{}) (interface{}, bool) {
	c.m.RLock()
	defer c.m.RUnlock()
	item, ok := c.get(key)
	return item.value, ok
}

func (c *Cache) get(key interface{}) (*storeItem, bool) {
	c.t++
	if c.t == c.threshold {
		c.c.Reset()
		c.door.reset()
		c.t = 0
	}

	keyHash, conflictHash := c.keyToHash(key)

	val, ok := c.data[keyHash]
	if !ok {
		c.c.Increment(keyHash)
		return nil, false
	}

	item := val.Value.(*storeItem)

	if item.conflict != conflictHash {
		c.c.Increment(keyHash)
		return nil, false
	}

	c.c.Increment(item.key)

	if item.stage == 0 {
		c.lru.get(val)
	} else {
		c.slru.get(val)
	}

	return item, true

}

func (c *Cache) Del(key interface{}) (storeItem, bool) {
	c.m.Lock()
	defer c.m.Unlock()
	return c.del(key)
}

func (c *Cache) del(key interface{}) (storeItem, bool) {
	keyHash, conflictHash := c.keyToHash(key)

	val, ok := c.data[keyHash]
	if !ok {
		return storeItem{}, false
	}

	item := val.Value.(*storeItem)

	if conflictHash != 0 && (conflictHash != item.conflict) {
		return storeItem{}, false
	}
	if item.stage == 0 {
		return *c.lru.remove(keyHash), true
	} else {
		return *c.slru.remove(keyHash), true
	}
}

func (c *Cache) keyToHash(key interface{}) (uint64, uint64) {
	if key == nil {
		return 0, 0
	}
	switch k := key.(type) {
	case uint64:
		return k, 0
	case string:
		return MemHashString(k), xxhash.Sum64String(k)
	case []byte:
		return MemHash(k), xxhash.Sum64(k)
	case byte:
		return uint64(k), 0
	case int:
		return uint64(k), 0
	case int32:
		return uint64(k), 0
	case uint32:
		return uint64(k), 0
	case int64:
		return uint64(k), 0
	default:
		panic("Key type not supported")
	}
}

type stringStruct struct {
	str unsafe.Pointer
	len int
}

//go:noescape
//go:linkname memhash runtime.memhash
func memhash(p unsafe.Pointer, h, s uintptr) uintptr

// MemHashString is the hash function used by go map, it utilizes available hardware instructions
// (behaves as aeshash if aes instruction is available).
// NOTE: The hash seed changes for every process. So, this cannot be used as a persistent hash.
func MemHashString(str string) uint64 {
	ss := (*stringStruct)(unsafe.Pointer(&str))
	return uint64(memhash(ss.str, 0, uintptr(ss.len)))
}

func MemHash(data []byte) uint64 {
	ss := (*stringStruct)(unsafe.Pointer(&data))
	return uint64(memhash(ss.str, 0, uintptr(ss.len)))
}
