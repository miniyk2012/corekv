/*
	参考了https://github.com/goburrow/cache/blob/master/lru_test.go的代码编写的测试用例
*/
package cache

import (
	"container/list"
	"fmt"
	"testing"
)

type lruTest struct {
	cache Cache
	data map[uint64]*list.Element
	lru  *windowLRU
	t    *testing.T
}

func (s *lruTest)init(size int) {
	s.data = make(map[uint64]*list.Element)
	s.lru = newWindowLRU(size, s.data)
}

func (s *lruTest) assertLRULen(n int) {
	sz := len(s.data)
	lz := s.lru.list.Len()
	if sz != n || lz != n {
		s.t.Helper()
		s.t.Fatalf("unexpected data length: cache=%d list=%d, want: %d", sz, lz, n)
	}
}

func (s *lruTest) assertEntry(en *storeItem, key uint64, v string, stage int) {
	if en == nil {
		s.t.Helper()
		s.t.Fatalf("unexpected entry: %v", en)
	}
	ak := en.key
	av := en.value
	if ak != key || av != v || en.stage != stage {
		s.t.Helper()
		s.t.Fatalf("unexpected entry: %+v, want: {key: %d, value: %s, listID: %d}",
			en, key, v, stage)
	}
}

func (s *lruTest) assertLRUEntry(key uint64) {
	en := s.data[key]
	if en == nil {
		s.t.Helper()
		s.t.Fatalf("entry not found in cache: key=%v", key)
	}

	item := en.Value.(*storeItem)
	ak := item.key
	av := item.value
	v := fmt.Sprintf("%d", key)
	if ak != key || av != v || item.stage != 0 {
		s.t.Helper()
		s.t.Fatalf("unexpected entry: %+v, want: {key: %v, value: %v, listID: %v}", en, key, v, 0)
	}
}

func createLRUEntries(s lruTest, n int) []storeItem {
	en := make([]storeItem, n)
	for i := range en {
		_, conflictHash := s.cache.keyToHash(i)
		en[i] = storeItem{
			stage:    0,
			key:      uint64(i),
			conflict: conflictHash,
			value:    fmt.Sprintf("%d", i),
		}
	}
	return en
}

func TestLRU(t *testing.T) {
	s := lruTest{t: t}
	s.init(3)
	en := createLRUEntries(s, 4)
	remEn, evicted := s.lru.add(en[0])
	// 0
	if evicted {
		t.Fatalf("unexpected entry removed: %v", remEn)
	}
	s.assertLRULen(1)
	s.assertLRUEntry(0)

	remEn, evicted  = s.lru.add(en[1])
	// 1 0
	if evicted {
		t.Fatalf("unexpected entry removed: %v", remEn)
	}
	s.assertLRULen(2)
	s.assertLRUEntry(1)
	s.assertLRUEntry(0)


	s.lru.get(s.data[0])
	// 0 1

	remEn, evicted = s.lru.add(en[2])
	// 2 0 1
	if evicted {
		t.Fatalf("unexpected entry removed: %+v", remEn)
	}
	s.assertLRULen(3)

	remEn, evicted = s.lru.add(en[3])
	// 3 2 0
	if !evicted {
		t.Fatalf("entry not removed: %+v", remEn)
	}
	s.assertEntry(&remEn, 1, "1", 0)
	s.assertLRULen(3)
	s.assertLRUEntry(3)
	s.assertLRUEntry(2)
	s.assertLRUEntry(0)
}
