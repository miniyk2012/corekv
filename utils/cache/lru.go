package cache

import "container/list"

type windowLRU struct {
	data map[uint64]*list.Element  // data的value存储了链表元素的地址
	cap  int
	list *list.List  // 链表元素存储了storeItem的地址
}

type storeItem struct {
	stage    int
	key      uint64   // key-value对的key的Hash值, 后面统一称作keyHash, 不同的key的值可能相同. map里是用keyHash作为键的.
	conflict uint64   // key-value对key的另一种算法的Hash值. keyHash+conflict合起来基本能保证唯一性.
	value    interface{}
}

func newWindowLRU(size int, data map[uint64]*list.Element) *windowLRU {
	return &windowLRU{
		data: data,
		cap:  size,
		list: list.New(),
	}
}

func (lru *windowLRU) add(newitem storeItem) (eitem storeItem, evicted bool) {
	if lru.list.Len() < lru.cap {
		lru.data[newitem.key] = lru.list.PushFront(&newitem)
		return storeItem{}, false
	}

	evictItem := lru.list.Back()
	item := evictItem.Value.(*storeItem)

	delete(lru.data, item.key)

	eitem, *item = *item, newitem

	lru.data[item.key] = evictItem
	lru.list.MoveToFront(evictItem)
	return eitem, true
}

func (lru *windowLRU) get(v *list.Element) {
	lru.list.MoveToFront(v)
}
