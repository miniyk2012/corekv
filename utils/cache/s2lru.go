package cache

import (
	"container/list"
	"fmt"
)

type segmentedLRU struct {
	data                     map[uint64]*list.Element
	stageOneCap, stageTwoCap int
	stageOne, stageTwo       *list.List
}

const (
	STAGE_ONE = iota + 1
	STAGE_TWO
)

func newSLRU(data map[uint64]*list.Element, stageOneCap, stageTwoCap int) *segmentedLRU {
	return &segmentedLRU{
		data:        data,
		stageOneCap: stageOneCap,
		stageTwoCap: stageTwoCap,
		stageOne:    list.New(),
		stageTwo:    list.New(),
	}
}

// add window-lru淘汰下来的数据获胜, 加入slru, 这里假设一定是新值, 故只能加入到stageOne
func (slru *segmentedLRU) add(newitem storeItem) (eitem storeItem, evicted bool) {
	newitem.stage = STAGE_ONE
	// 即使stageOne的数据超过slru.stageOneCap, 只要总量未超标也还可以加入stageOne.
	// 见测试用例TestSegmentedLR2.
	if slru.stageOne.Len() < slru.stageOneCap || slru.Len() < slru.stageOneCap+slru.stageTwoCap {
		v := slru.stageOne.PushFront(&newitem)
		slru.data[newitem.key] = v
		return storeItem{}, false
	}
	back := slru.stageOne.Back()
	oldItem := back.Value.(*storeItem)
	delete(slru.data, oldItem.key) // 记得删除淘汰的数据
	eitem, *oldItem = *oldItem, newitem
	slru.stageOne.MoveToFront(back)
	slru.data[newitem.key] = back
	return eitem, true
}

// get 调整一下stageOne, stageTwo数据的顺序. 这里的v一定是老值, 分stageOne还是stageTwo2种情况
func (slru *segmentedLRU) get(v *list.Element) {
	item := v.Value.(*storeItem)
	if item.stage == STAGE_TWO {
		slru.stageTwo.MoveToFront(v)
		return
	}
	if slru.stageTwo.Len() < slru.stageTwoCap {
		item.stage = STAGE_TWO
		slru.stageOne.Remove(v)
		slru.data[item.key] = slru.stageTwo.PushFront(item)
		return
	}
	item.stage = STAGE_TWO // 升级
	back := slru.stageTwo.Back()
	bItem := back.Value.(*storeItem)
	bItem.stage = STAGE_ONE
	*item, *bItem = *bItem, *item
	slru.stageTwo.MoveToFront(back)
	slru.stageOne.MoveToFront(v)
	slru.data[bItem.key] = back   // 新值
	slru.data[item.key] = v // 旧值
}

func (slru *segmentedLRU) Len() int {
	return slru.stageTwo.Len() + slru.stageOne.Len()
}

// victim 返回需要和window-lru淘汰值竞争的数据
func (slru *segmentedLRU) victim() *storeItem {
	if slru.Len() < slru.stageOneCap+slru.stageTwoCap {
		return nil
	}

	v := slru.stageOne.Back()
	return v.Value.(*storeItem)
}

// remove an item from the slru
func (slru *segmentedLRU) remove(key uint64) *storeItem {
	val := slru.data[key]
	if val.Value.(*storeItem).stage == STAGE_ONE {
		slru.stageOne.Remove(val)
	} else {
		slru.stageTwo.Remove(val)
	}
	delete(slru.data, key)
	return val.Value.(*storeItem)
}

func (slru *segmentedLRU)String() string {
	var s string
	for e := slru.stageTwo.Front(); e != nil; e = e.Next() {
		s += fmt.Sprintf("%v,", e.Value.(*storeItem).value)
	}
	s += fmt.Sprintf(" | ")
	for e := slru.stageOne.Front(); e != nil; e = e.Next() {
		s += fmt.Sprintf("%v,", e.Value.(*storeItem).value)
	}
	return s
}
