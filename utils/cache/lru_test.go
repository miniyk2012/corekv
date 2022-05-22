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
	data  map[uint64]*list.Element
	lru   *windowLRU
	slru  *segmentedLRU
	t     *testing.T
}

var (
	WithSLruCap = func(stageOneCap, stageTwoCap int) func(*segmentedLRU) {
		return func(slru *segmentedLRU) {
			slru.stageOneCap, slru.stageTwoCap = stageOneCap, stageTwoCap
		}
	}
)

func (s *lruTest) init(lruCap int, opts ...func(*segmentedLRU)) {
	s.data = make(map[uint64]*list.Element)
	s.lru = newWindowLRU(lruCap, s.data)
	s.slru = newSLRU(s.data, 0, 0)
	for _, opt := range opts {
		opt(s.slru)
	}
}

func (s *lruTest) assertLRULen(n int) {
	sz := len(s.data)
	lz := s.lru.list.Len()
	if sz != n || lz != n {
		s.t.Helper()
		s.t.Fatalf("unexpected data length: cache=%d list=%d, want: %d", sz, lz, n)
	}
}

func (s *lruTest) assertSLRULen(protected, probation int) {
	tz := s.slru.stageTwo.Len()
	bz := s.slru.stageOne.Len()
	if tz != protected || bz != probation {
		s.t.Helper()
		s.t.Fatalf("unexpected data length: protected=%d probation=%d, want: %d %d", tz, bz, protected, probation)
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

func (s *lruTest) assertSLRUEntry(key uint64, stage int) {
	en := s.data[key]
	if en == nil {
		s.t.Helper()
		s.t.Fatalf("entry not found in cache: key=%v", key)
	}
	item := en.Value.(*storeItem)
	ak := item.key
	av := item.value
	v := fmt.Sprintf("%d", key)
	if ak != key || av != v || item.stage != stage {
		s.t.Helper()
		s.t.Fatalf("unexpected entry: %+v, want: {key: %v, value: %v, stage: %v}", en, key, v, stage)
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

	remEn, evicted = s.lru.add(en[1])
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

func TestSegmentedLRU(t *testing.T) {
	s := lruTest{t: t}
	s.init(0, WithSLruCap(1, 2))

	en := createLRUEntries(s, 5)

	remEn, evicted := s.slru.add(en[0])
	// - | 0
	if evicted {
		t.Fatalf("unexpected entry removed: %v", remEn)
	}
	s.assertSLRULen(0, 1)
	s.assertSLRUEntry(0, STAGE_ONE)

	remEn, evicted = s.slru.add(en[1])
	// - | 1 0
	if evicted {
		t.Fatalf("unexpected entry removed: %v", remEn)
	}

	s.assertSLRULen(0, 2)
	s.assertSLRUEntry(1, STAGE_ONE)

	s.slru.get(s.data[1])
	// 1 | 0
	s.assertSLRULen(1, 1)
	s.assertSLRUEntry(1, STAGE_TWO)
	s.assertSLRUEntry(0, STAGE_ONE)

	s.slru.get(s.data[0])
	// 0 1 | -
	s.assertSLRULen(2, 0)
	s.assertSLRUEntry(0, STAGE_TWO)
	s.assertSLRUEntry(1, STAGE_TWO)

	remEn, evicted = s.slru.add(en[2])
	// 0 1 | 2
	if evicted {
		t.Fatalf("unexpected entry removed: %+v", remEn)
	}
	s.assertSLRULen(2, 1)
	s.assertSLRUEntry(2, STAGE_ONE)
	///**
	remEn, evicted = s.slru.add(en[3])
	// 0 1 | 3
	if !evicted {
		t.Fatalf("should remove one item")
	}
	s.assertSLRULen(2, 1)
	s.assertEntry(&remEn, 2, "2", STAGE_ONE)
	s.assertSLRUEntry(3, STAGE_ONE)

	s.slru.get(s.data[3]) // 交换一下
	// 3 0 | 1
	s.assertSLRULen(2, 1)
	s.assertSLRUEntry(3, STAGE_TWO)

	remEn, evicted = s.slru.add(en[4])
	// 3 0 | 4
	s.assertSLRULen(2, 1)
	s.assertEntry(&remEn, 1, "1", STAGE_ONE)
}

func TestSegmentedLR2(t *testing.T) {
	s := lruTest{t: t}
	s.init(0, WithSLruCap(2, 2))

	en := createLRUEntries(s, 5)

	remEn, evicted := s.slru.add(en[0])
	// - | 0
	if evicted {
		t.Fatalf("unexpected entry removed: %v", remEn)
	}
	s.assertSLRULen(0, 1)
	s.assertSLRUEntry(0, STAGE_ONE)

	remEn, evicted = s.slru.add(en[1])
	// - | 1 0
	if evicted {
		t.Fatalf("unexpected entry removed: %v", remEn)
	}

	s.assertSLRULen(0, 2)
	s.assertSLRUEntry(1, STAGE_ONE)

	remEn, evicted = s.slru.add(en[2])
	// - | 2 1 0
	if evicted {
		t.Fatalf("unexpected entry removed: %v", remEn)
	}
	s.assertSLRULen(0, 3)
	s.assertSLRUEntry(2, STAGE_ONE)

	remEn, evicted = s.slru.add(en[3])
	// - | 3 2 1 0
	if evicted {
		t.Fatalf("unexpected entry removed: %v", remEn)
	}
	s.assertSLRULen(0, 4)
	s.assertSLRUEntry(3, STAGE_ONE)

	remEn, evicted = s.slru.add(en[4])
	//  | 4 3 2 1
	if !evicted {
		t.Fatalf("should remove one item")
	}
	s.assertSLRULen(0, 4)
	s.assertSLRUEntry(4, STAGE_ONE)
	s.assertEntry(&remEn, 0, "0", STAGE_ONE)

	s.slru.get(s.data[2])
	s.slru.get(s.data[3])
	// 3 2  | 4 1
	s.assertSLRULen(2, 2)
	s.assertSLRUEntry(4, STAGE_ONE)
	s.assertSLRUEntry(3, STAGE_TWO)
	s.assertSLRUEntry(2, STAGE_TWO)
	s.assertSLRUEntry(1, STAGE_ONE)

	s.slru.get(s.data[1])
	// 1 3  | 2 4
	s.assertSLRULen(2, 2)
	s.assertSLRUEntry(4, STAGE_ONE)
	s.assertSLRUEntry(3, STAGE_TWO)
	s.assertSLRUEntry(2, STAGE_ONE)
	s.assertSLRUEntry(1, STAGE_TWO)
	s.assertEntry(s.slru.stageTwo.Front().Value.(*storeItem), 1, "1", STAGE_TWO)
	s.assertEntry(s.slru.stageOne.Front().Value.(*storeItem), 2, "2", STAGE_ONE)
}
