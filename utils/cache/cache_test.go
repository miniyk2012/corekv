package cache

import (
	"fmt"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestCacheBasicCRUD(t *testing.T) {
	cache := NewCache(5)
	fmt.Println(cache.lru.cap, cache.slru.stageTwoCap, cache.slru.stageOneCap)
	for i := 0; i < 10; i++ {
		key := fmt.Sprintf("key%d", i)
		val := fmt.Sprintf("val%d", i)
		cache.Set(key, val)
	}
	fmt.Printf("set last: %s\n", cache)

	for i := 0; i < 1000; i++ {
		key := fmt.Sprintf("key%d", i)
		val := fmt.Sprintf("val%d", i)
		res, ok := cache.Get(key)
		if ok {
			fmt.Printf("get %s: %s\n", key, cache)
			assert.Equal(t, val, res)
			assert.Less(t, i, 10)
			continue
		}
		assert.Equal(t, res, nil)
	}
	fmt.Printf("at last: %s\n", cache)
}

func TestCacheStruct(t *testing.T) {
	cache := NewCache(500)
	type  value struct{
		name string
	}
	cache.Set("no1", value{"yangkai.04"})
	v, ok := cache.Get("no1")
	assert.True(t, ok, nil)
	assert.Equal(t, v, value{"yangkai.04"})
}


type tinyLFUTest struct {
	c   *Cache
	t   *testing.T
}


func (t *tinyLFUTest) assertCap(n int) {
	if t.c.lru.cap+t.c.slru.stageTwoCap+t.c.slru.stageOneCap != n {
		t.t.Helper()
		t.t.Fatalf("unexpected lru.cap: %d, slru.cap: %d/%d",
			t.c.lru.cap, t.c.slru.stageTwoCap, t.c.slru.stageOneCap)
	}
}

func (t *tinyLFUTest) assertLen(admission, protected, probation int) {
	sz := t.c.Len()
	az := t.c.lru.list.Len()
	tz := t.c.slru.stageTwo.Len()
	bz := t.c.slru.stageOne.Len()
	if sz != admission+protected+probation || az != admission || tz != protected || bz != probation {
		t.t.Helper()
		t.t.Fatalf("unexpected data length: cache=%d admission=%d protected=%d probation=%d, want: %d %d %d",
			sz, az, tz, bz, admission, protected, probation)
	}
}

func (t *tinyLFUTest) init(size int) {
	t.c = NewCache(size)
}

func (t *tinyLFUTest) assertEntry(en *storeItem, key uint64, v string, stage int) {
	ak := en.key
	av := en.value.(string)
	if ak != key || av != v || en.stage != stage {
		t.t.Helper()
		t.t.Fatalf("unexpected entry: %+v, want: {key: %d, value: %s, stage: %d}",
			en, key, v, stage)
	}
}

func (t *tinyLFUTest) assertLRUEntry(key uint64, stage int) {
	en, ok := t.c.data[key]
	if !ok {
		t.t.Helper()
		t.t.Fatalf("entry not found in cache: key=%v", key)
	}
	item := en.Value.(*storeItem)
	ak := item.key
	av := item.value.(string)
	v := fmt.Sprintf("%d", key)
	if ak != key || av != v || item.stage != stage {
		t.t.Helper()
		t.t.Fatalf("unexpected entry: %+v, want: {key: %d, value: %s, listID: %d}",
			item, key, v, stage)
	}
}

func TestTinyLFU(t *testing.T) {
	s := tinyLFUTest{t: t}
	s.init(200)
	s.assertCap(200)
	s.c.slru.stageTwoCap = 2
	s.c.slru.stageOneCap = 1

	type item struct {
		key int
		value string
	}
	en := make([]item, 10)
	for i := range en {
		en[i] = item{
			key:      i,
			value:    fmt.Sprintf("%d", i),
		}
	}
	for i := 0; i < 5; i++ {
		if evicted := s.c.Set(en[i].key, en[i].value); evicted {
			t.Fatalf("unexpected entry removed")
		}
	}
	fmt.Println(s.c)
	// 4 3 | - | 2 1 0
	s.assertLen(2, 0, 3)
	s.assertLRUEntry(4, 0)
	s.assertLRUEntry(3, 0)
	s.assertLRUEntry(2, STAGE_ONE)
	s.assertLRUEntry(1, STAGE_ONE)
	s.assertLRUEntry(0, STAGE_ONE)

	s.c.Get(en[1].key)
	s.c.Get(en[2].key)
	// 4 3 | 2 1 | 0
	fmt.Println(s.c)
	s.assertLen(2, 2, 1)
	s.assertLRUEntry(2, STAGE_TWO)
	s.assertLRUEntry(1, STAGE_TWO)
	s.assertLRUEntry(0, STAGE_ONE)

	remEn, evicted := s.c.set(en[5].key, en[5].value)
	fmt.Println(s.c)
	// 5 4 | 2 1 | 0
	if !evicted {
		t.Fatalf("expect an entry removed when adding %+v", en[5])
	}
	s.assertEntry(&remEn, 3, "3", 0)

	s.c.Get(en[4].key)
	s.c.Get(en[5].key)
	fmt.Println(s.c)
	remEn, evicted = s.c.set(en[6].key, en[6].value)
	fmt.Println(s.c)
	// 6 5 | 2 1 | 4
	if !evicted {
		t.Fatalf("expect an entry removed when adding %+v", en[6])
	}
	s.assertLen(2, 2, 1)
	s.assertEntry(&remEn, 0, "0", STAGE_ONE)
	n := s.c.c.Estimate(uint64(en[1].key))
	if n != 1 {
		t.Fatalf("unexpected estimate: %d %+v", n, en[1])
	}
	s.c.get(en[1].key)
	s.c.get(en[1].key)
	n = s.c.c.Estimate(uint64(en[1].key))
	if n != 3 {
		t.Fatalf("unexpected estimate: %d %+v", n, en[2])
	}

	if _, ok := s.c.Del(en[0].key); ok {
		t.Fatalf("delete non-exist ele!")
	}
	fmt.Println(s.c)
	// 6 5 | 2 1 | 4

	remEn, ok := s.c.Del(en[2].key)
	// 6 5 | 1 | 4
	if !ok {
		t.Fatalf("delete key fail %d", en[2].key)
	}
	fmt.Println(s.c)
	s.assertEntry(&remEn, 2, "2", 2)

	remEn, ok = s.c.Del(en[6].key)
	// 5 | 1 | 4
	if !ok {
		t.Fatalf("delete key fail %d", en[6].key)
	}
	fmt.Println(s.c)
	s.assertEntry(&remEn, 6, "6", 0)


	remEn, ok = s.c.Del(en[4].key)
	// 5 | 1 | -
	if !ok {
		t.Fatalf("delete key fail %d", en[4].key)
	}
	fmt.Println(s.c)
	s.assertEntry(&remEn, 4, "4", 1)
}

func TestCacheSameKey(t *testing.T) {
	cache := NewCache(500)
	key := "one"
	for i := 0; i < 10; i++ {
		val := fmt.Sprintf("val%d", i)
		cache.Set(key, val)
		res, ok := cache.Get(key)
		fmt.Println(cache)
		if ok {
			assert.Equal(t, val, res)
			continue
		}
		assert.Equal(t, res, nil)
	}
}

func TestTinyLFUSameEntry(t *testing.T) {
	s := tinyLFUTest{t: t}
	s.init(200)
	s.assertCap(200)
	s.c.slru.stageTwoCap = 2
	s.c.slru.stageOneCap = 1

	en := make([]storeItem, 10)
	sameKey := uint64(100)
	for i := 0; i < len(en); i++ {
		s.c.Set(sameKey, fmt.Sprintf("%d", i))
		v, ok := s.c.get(sameKey)
		if !ok{
			s.t.Fatalf("should get item!")
		}
		fmt.Println(s.c)
		s.assertLen(1, 0, 0)
		s.assertEntry(&v, sameKey, fmt.Sprintf("%d", i), 0)
	}
}