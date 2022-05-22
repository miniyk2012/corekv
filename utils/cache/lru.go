package cache

import "container/list"

type windowLRU struct {
	data map[uint64]*list.Element
	cap  int
	list *list.List
}

type storeItem struct {
	stage    int
	key      uint64
	conflict uint64
	value    interface{}
}

func newWindowLRU(size int, data map[uint64]*list.Element) *windowLRU {
	return &windowLRU{
		data: data,
		cap:  size,
		list: list.New(),
	}
}

// add 这里不需要考虑同key场景, 同key操作在Cache的外层API通过Get一下再更新value来实现
func (lru *windowLRU) add(newitem storeItem) (eitem storeItem, evicted bool) {
	//implement me here!!!
	if lru.cap > lru.list.Len() {
		lru.data[newitem.key] = lru.list.PushFront(&newitem)
		return storeItem{}, false
	}
	// 把尾结点移到头结点, 数据填充为新的数据地址, 并且把旧数据拿出来淘汰
	tail := lru.list.Back()
	old := tail.Value.(*storeItem)

	delete(lru.data, old.key)

	// 尽量不要引用传入的newitem
	//tail.Value = &newitem

	// 把传入的newitem拷贝一份
	eitem, *old = *old, newitem

	lru.data[newitem.key] = tail
	lru.list.MoveToFront(tail)
	return eitem, true
}

// get 的方法默认v是存在的, 只需把它移到头部
func (lru *windowLRU) get(v *list.Element) {
	//implement me here!!!
	lru.list.MoveToFront(v)
}

// remove an item from the lru
func (lru *windowLRU) remove(key uint64) *storeItem {
	val := lru.data[key]
	lru.list.Remove(val)
	delete(lru.data, key)
	return val.Value.(*storeItem)
}
