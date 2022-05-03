package utils

import (
	"bytes"
	"sync"
)

const (
	defaultMaxLevel = 20
)

type SkipList struct {
	maxLevel   int          //sl的最大高度
	lock       sync.RWMutex //读写锁，用来实现并发安全的sl
	currHeight int32        //sl当前的最大高度
	headOffset uint32       //头结点在arena当中的偏移量
	arena      *Arena
}

func NewSkipList(arenaSize int64) *SkipList {
	area := newArena(arenaSize)
	head := newNode(area, nil, ValueStruct{}, defaultMaxLevel)
	headOffset := area.getNodeOffset(head)
	s := SkipList{
		currHeight: 1,
		headOffset: headOffset,
		arena:      area,
	}
	return &s
}

func newNode(arena *Arena, key []byte, v ValueStruct, height int) *Node {
	nodeOffset := arena.putNode(height)
	keyOffset := arena.putKey(key)
	valOffset := arena.putVal(v)
	node := arena.getNode(nodeOffset)
	node.value = encodeValue(valOffset, v.EncodedSize())
	node.keyOffset = keyOffset
	node.keySize = uint16(len(key))
	node.height = uint16(height)
	return node
}

//用来对value值进行编解码
//value = valueSize | valueOffset
func encodeValue(valOffset uint32, valSize uint32) uint64 {
	return uint64(valSize)<<32 | uint64(valOffset)
}

func decodeValue(value uint64) (valOffset uint32, valSize uint32) {
	valOffset = uint32(value)
	valSize = uint32(value >> 32)
	return
}

type Node struct {
	score float64 //加快查找，只在内存中生效，因此不需要持久化
	value uint64  //将value的off和size组装成一个uint64，实现原子化的操作

	keyOffset uint32
	keySize   uint16

	height uint16

	levels [defaultMaxLevel]uint32 //这里先按照最大高度声明，往arena中放置的时候，会计算实际高度和内存消耗
}

func (n *Node) getVs(arena *Arena) ValueStruct {
	return arena.getVal(decodeValue(n.value))
}

func (n *Node) key(arena *Arena) []byte {
	return arena.getKey(n.keyOffset, n.keySize)
}

func (n *Node) Value(arena *Arena) []byte {
	return n.getVs(arena).Value
}

// MemSize 返回SkipList所占内存大小
func (list *SkipList) MemSize() int64 { return list.arena.Size() }

func (list *SkipList) Add(data *Entry) error {
	list.lock.Lock()
	defer list.lock.Unlock()
	score := calcScore(data.Key)
	var elem *Node
	value := ValueStruct{
		Value: data.Value,
	}

	//从当前最大高度开始
	max := list.currHeight
	//拿到头节点，从第一个开始
	prevElem := list.arena.getNode(list.headOffset)
	//用来记录访问路径
	var prevElemHeaders [defaultMaxLevel]*Node

	for i := max - 1; i >= 0; {
		//keep visit path here
		prevElemHeaders[i] = prevElem

		for next := list.getNext(prevElem, int(i)); next != nil; next = list.getNext(prevElem, int(i)) {
			if comp := list.compare(score, data.Key, next); comp <= 0 {
				if comp == 0 {
					vo := list.arena.putVal(value)
					encV := encodeValue(vo, value.EncodedSize())
					next.value = encV
					return nil
				}

				//find the insert position
				break
			}

			//just like linked-list next
			prevElem = next
			prevElemHeaders[i] = prevElem
		}

		topLevel := prevElem.levels[i]

		//to skip same prevHeader's next and fill next elem into temp element
		for i--; i >= 0 && prevElem.levels[i] == topLevel; i-- {
			prevElemHeaders[i] = prevElem
		}
	}

	level := list.randLevel()

	elem = newNode(list.arena, data.Key, ValueStruct{Value: data.Value}, level)
	//to add elem to the skiplist
	off := list.arena.getNodeOffset(elem)
	for i := 0; i < level; i++ {
		elem.levels[i] = prevElemHeaders[i].levels[i]
		prevElemHeaders[i].levels[i] = off
	}

	return nil
}

func (list *SkipList) Search(key []byte) (e *Entry) {
	list.lock.RLock()
	defer list.lock.RUnlock()

	current := list.arena.getNode(list.headOffset)
	for i := int(list.currHeight - 1); i >= 0; i-- {
		for {
			next := list.arena.getNode(current.getNextOffset(i))
			if next == nil {
				// 进入下一层
				break
			}
			next.score = calcScore(next.key(list.arena))
			cmp := list.compare(calcScore(key), key, next)
			if cmp == 0 {
				return &Entry{Key: key, Value: next.Value(list.arena)}
			} else if cmp > 0 {
				// next.key < key
				current = next
			} else {
				// current.key < key < next.key, 进入下一层
				break
			}
		}
	}
	// 最底层也没找到
	return nil
}

func (list *SkipList) Close() error {
	return nil
}

func calcScore(key []byte) (score float64) {
	var hash uint64
	l := len(key)

	if l > 8 {
		l = 8
	}

	for i := 0; i < l; i++ {
		shift := uint(64 - 8 - i*8)
		hash |= uint64(key[i]) << shift
	}

	score = float64(hash)
	return
}

func (list *SkipList) compare(score float64, key []byte, next *Node) int {
	if score == next.score {
		return bytes.Compare(key, next.key(list.arena))
	}

	if score < next.score {
		return -1
	} else {
		return 1
	}
}

func (list *SkipList) randLevel() int {
	if list.maxLevel <= 1 {
		return 1
	}
	i := 1
	for ; i < list.maxLevel; i++ {
		if RandN(1000)%2 == 0 {
			return i
		}
	}
	return i
}

//拿到某个节点，在某个高度上的next节点
//如果该节点已经是该层最后一个节点（该节点的level[height]将是0），会返回nil
func (list *SkipList) getNext(e *Node, height int) *Node {
	return nil
}

type SkipListIter struct {
	list *SkipList
	elem *Node //iterator当前持有的节点
	lock sync.RWMutex
}

func (list *SkipList) NewSkipListIterator() Iterator {
	return &SkipListIter{
		list: list,
	}
}

func (iter *SkipListIter) Next() {
	AssertTrue(iter.Valid())
	iter.elem = iter.list.getNext(iter.elem, 0) //只在最底层遍历就行了
}

func (iter *SkipListIter) Valid() bool {
	return iter.elem != nil
}
func (iter *SkipListIter) Rewind() {
	head := iter.list.arena.getNode(iter.list.headOffset)
	iter.elem = iter.list.getNext(head, 0)
}

func (iter *SkipListIter) Item() Item {
	vo, vs := decodeValue(iter.elem.value)
	return &Entry{
		Key:       iter.list.arena.getKey(iter.elem.keyOffset, iter.elem.keySize),
		Value:     iter.list.arena.getVal(vo, vs).Value,
		ExpiresAt: iter.list.arena.getVal(vo, vs).ExpiresAt,
	}
}
func (iter *SkipListIter) Close() error {
	return nil
}

func (iter *SkipListIter) Seek(key []byte) {
}
