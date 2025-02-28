/*
 * Copyright 2017 Dgraph Labs, Inc. and Contributors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

/*
Adapted from RocksDB inline skiplist.

Key differences:
- No optimization for sequential inserts (no "prev").
- No custom comparator.
- Support overwrites. This requires care when we see the same key when inserting.
  For RocksDB or LevelDB, overwrites are implemented as a newer sequence number in the key, so
	there is no need for values. We don't intend to support versioning. In-place updates of values
	would be more efficient.
- We discard all non-concurrent code.
- We do not support Splices. This simplifies the code a lot.
- No AllocateNode or other pointer arithmetic.
- We combine the findLessThan, findGreaterOrEqual, etc into one function.
*/

package utils

import (
	"fmt"
	"log"
	"math"
	"strings"
	"sync/atomic"
	_ "unsafe"

	"github.com/pkg/errors"
)

const (
	maxHeight      = 20
	heightIncrease = math.MaxUint32 / 3
)

// node 代表节点, 存储了key和value在area的偏移量, 以及每层后续节点的偏移量
type node struct {
	// Multiple parts of the value are encoded as a single uint64 so that it
	// can be atomically loaded and stored:
	//   value offset: uint32 (bits 0-31)
	//   value size  : uint16 (bits 32-63)
	value uint64 // 指向value

	// A byte slice is 24 bytes. We are trying to save space here.
	keyOffset uint32 // Immutable. No need to lock to access key.
	keySize   uint16 // Immutable. No need to lock to access key.

	// Height of the tower.
	height uint16

	// Most nodes do not need to use the full height of the tower, since the
	// probability of each successive level decreases exponentially. Because
	// these elements are never accessed, they do not need to be allocated.
	// Therefore, when a node is allocated in the arena, its memory footprint
	// is deliberately truncated to not include unneeded tower elements.
	//
	// All accesses to elements should use CAS operations, with no need to lock.
	tower [maxHeight]uint32 // 每层后续节点的偏移量.
}

// Skiplist 每个跳表关联一个area内存空间
type Skiplist struct {
	height     int32  // Current height. 1 <= height <= kMaxHeight. CAS.  节点存在于[0, height-1]层
	headOffset uint32 // 头节点偏移量, header是哑元, 不存key,value
	ref        int32  // 未来其他组件用的到
	arena      *Arena
	OnClose    func()
}

// IncrRef increases the refcount
func (s *Skiplist) IncrRef() {
	atomic.AddInt32(&s.ref, 1)
}

// DecrRef decrements the refcount, deallocating the Skiplist when done using it
func (s *Skiplist) DecrRef() {
	newRef := atomic.AddInt32(&s.ref, -1)
	if newRef > 0 {
		return
	}
	if s.OnClose != nil {
		s.OnClose()
	}

	// Indicate we are closed. Good for testing.  Also, lets GC reclaim memory. Race condition
	// here would suggest we are accessing skiplist when we are supposed to have no reference!
	s.arena = nil
}

// Draw plot Skiplist, align represents align the same node in different level
func (s *Skiplist) Draw(align bool) {
	reverseTree := make([][]string, s.getHeight())
	head := s.getHead()
	for level := int(s.getHeight()) - 1; level >= 0; level-- {
		next := head
		for {
			var nodeStr string
			next = s.getNext(next, level)
			if next != nil {
				key := next.key(s.arena)
				vs := next.getVs(s.arena)
				nodeStr = fmt.Sprintf("%s(%s)", key, vs.Value)
			} else {
				break
			}
			reverseTree[level] = append(reverseTree[level], nodeStr)
		}
	}

	// align
	if align && s.getHeight() > 1 {
		baseFloor := reverseTree[0]
		for level := 1; level < int(s.getHeight()); level++ {
			pos := 0
			for _, ele := range baseFloor {
				if pos == len(reverseTree[level]) {
					break
				}
				if ele != reverseTree[level][pos] {
					newStr := fmt.Sprintf(strings.Repeat("-", len(ele)))
					reverseTree[level] = append(reverseTree[level][:pos+1], reverseTree[level][pos:]...)
					reverseTree[level][pos] = newStr
				}
				pos++
			}
		}
	}

	// plot
	for level := int(s.getHeight()) - 1; level >= 0; level-- {
		fmt.Printf("%d: ", level)
		for pos, ele := range reverseTree[level] {
			if pos == len(reverseTree[level])-1 {
				fmt.Printf("%s", ele)
			} else {
				fmt.Printf("%s->", ele)
			}
		}
		fmt.Println()
	}
}

// newNode 在area里申请空间存储node,key,value
func newNode(arena *Arena, key []byte, v ValueStruct, height int) *node {
	// The base level is already allocated in the node struct.
	nodeOffset := arena.putNode(height)
	keyOffset := arena.putKey(key)
	val := encodeValue(arena.putVal(v), v.EncodedSize())

	node := arena.getNode(nodeOffset)
	node.keyOffset = keyOffset
	node.keySize = uint16(len(key))
	node.height = uint16(height)
	node.value = val
	return node
}

func encodeValue(valOffset uint32, valSize uint32) uint64 {
	return uint64(valSize)<<32 | uint64(valOffset)
}

func decodeValue(value uint64) (valOffset uint32, valSize uint32) {
	valOffset = uint32(value)
	valSize = uint32(value >> 32)
	return
}

// NewSkiplist makes a new empty skiplist, with a given arena size
func NewSkiplist(arenaSize int64) *Skiplist {
	arena := newArena(arenaSize)
	head := newNode(arena, nil, ValueStruct{}, maxHeight)
	ho := arena.getNodeOffset(head)
	return &Skiplist{
		height:     1,
		headOffset: ho,
		arena:      arena,
		ref:        1,
	}
}

func (n *node) getValueOffset() (uint32, uint32) {
	value := atomic.LoadUint64(&n.value)
	return decodeValue(value)
}

func (n *node) key(arena *Arena) []byte {
	return arena.getKey(n.keyOffset, n.keySize)
}

func (n *node) setValue(arena *Arena, vo uint64) {
	atomic.StoreUint64(&n.value, vo)
}

func (n *node) getNextOffset(h int) uint32 {
	return atomic.LoadUint32(&n.tower[h])
}

// casNextOffset CAS更新n的第i层的后继, 若失败返回false, 否则返回true.
func (n *node) casNextOffset(h int, old, val uint32) bool {
	return atomic.CompareAndSwapUint32(&n.tower[h], old, val)
}

// getVs return ValueStruct stored in node
func (n *node) getVs(arena *Arena) ValueStruct {
	valOffset, valSize := n.getValueOffset()
	return arena.getVal(valOffset, valSize)
}

// Returns true if key is strictly > n.key.
// If n is nil, this is an "end" marker and we return false.
//func (s *Skiplist) keyIsAfterNode(key []byte, n *node) bool {
//	AssertTrue(n != s.head)
//	return n != nil && CompareKeys(key, n.key) > 0
//}

func (s *Skiplist) randomHeight() int {
	h := 1
	for h < maxHeight && FastRand() <= heightIncrease {
		h++
	}
	return h
}

func (s *Skiplist) getNext(nd *node, height int) *node {
	return s.arena.getNode(nd.getNextOffset(height))
}

func (s *Skiplist) getHead() *node {
	return s.arena.getNode(s.headOffset)
}

// findNear finds the node near to key.
// If less=true, it finds rightmost node such that node.key < key (if allowEqual=false) or
// node.key <= key (if allowEqual=true).
// If less=false, it finds leftmost node such that node.key > key (if allowEqual=false) or
// node.key >= key (if allowEqual=true).
// Returns the node found. The bool returned is true if the node has key equal to given key.
func (s *Skiplist) findNear(key []byte, less bool, allowEqual bool) (*node, bool) {
	x := s.getHead()
	level := int(s.getHeight() - 1)
	for {
		// Assume x.key < key.
		next := s.getNext(x, level)
		if next == nil {
			// x.key < key < END OF LIST
			if level > 0 {
				// Can descend further to iterate closer to the end.
				level--
				continue
			}
			// Level=0. Cannot descend further. Let's return something that makes sense.
			if !less {
				return nil, false
			}
			// Try to return x. Make sure it is not a head node.
			if x == s.getHead() {
				return nil, false
			}
			return x, false
		}

		nextKey := next.key(s.arena)
		cmp := CompareKeys(key, nextKey)
		if cmp > 0 {
			// x.key < next.key < key. We can continue to move right.
			x = next
			continue
		}
		if cmp == 0 {
			// x.key < key == next.key.
			if allowEqual {
				return next, true
			}
			if !less {
				// We want >, so go to base level to grab the next bigger note.  想想跳表的结构哈, 需要到第0层去拿最靠近的那个节点
				return s.getNext(next, 0), false
			}
			// We want <. If not base level, we should go closer in the next level.
			// 根据这个: x.key < key == next.key
			// 不能返回本层的x, 因为下一层可能还有更接近的
			if level > 0 {
				level--
				continue
			}
			// On base level. Return x.
			if x == s.getHead() {
				return nil, false
			}
			return x, false
		}
		// cmp < 0. In other words, x.key < key < next.key
		if level > 0 {
			level--
			continue
		}
		// At base level. Need to return something.
		if !less {
			return next, false
		}
		// Try to return x. Make sure it is not a head node.
		if x == s.getHead() {
			return nil, false
		}
		return x, false
	}
}

// findSpliceForLevel returns (outBefore, outAfter) with outBefore.key <= key <= outAfter.key. 找出第i层要插入的位置的前后节点.
// The input "before" tells us where to start looking. 从起始节点开始遍历
// If we found a node with the same key, then we return outBefore = outAfter.
// Otherwise, outBefore.key < key < outAfter.key.
func (s *Skiplist) findSpliceForLevel(key []byte, before uint32, level int) (uint32, uint32) {
	for {
		// Assume before.key < key.
		beforeNode := s.arena.getNode(before)
		next := beforeNode.getNextOffset(level)
		nextNode := s.arena.getNode(next)
		if nextNode == nil {
			return before, next // 插在该层最后一个位置处
		}
		nextKey := nextNode.key(s.arena)
		cmp := CompareKeys(key, nextKey)
		if cmp == 0 {
			// Equality case.
			return next, next
		}
		if cmp < 0 {
			// before.key < key < next.key. We are done for this level.
			return before, next
		}
		before = next // Keep moving right on this level.
	}
}

func (s *Skiplist) getHeight() int32 {
	return atomic.LoadInt32(&s.height)
}

// Add inserts the key-value pair.
func (s *Skiplist) Add(e *Entry) {
	// Since we allow overwrite, we may not need to create a new node. We might not even need to
	// increase the height. Let's defer these actions.
	key, v := e.Key, ValueStruct{
		Meta:      e.Meta,
		Value:     e.Value,
		ExpiresAt: e.ExpiresAt,
		Version:   e.Version,
	}

	listHeight := s.getHeight()
	var prev [maxHeight + 1]uint32
	var next [maxHeight + 1]uint32
	prev[listHeight] = s.headOffset  // 最高层是第listHeight-1层, 它的前一个节点是head, 为了逻辑的统一(适配下面的prev[v+1]), 就给prev[listHeight]赋值为head
	for i := int(listHeight) - 1; i >= 0; i-- {
		// Use higher level to speed up for current level.
		prev[i], next[i] = s.findSpliceForLevel(key, prev[i+1], i)  // 刚开始进入下一层时,prev[i+1]就是前一个节点. 找出第i层要插入的位置的前后节点.
		// 要插入的key在第i层的前一个节点是prev[i], 后一个节点是next[i]
		if prev[i] == next[i] {
			// 不怕并发更新同一个value. 因为节点关系没有断
			vo := s.arena.putVal(v)
			encValue := encodeValue(vo, v.EncodedSize())  // 算出新值的偏移量和长度
			prevNode := s.arena.getNode(prev[i])
			prevNode.setValue(s.arena, encValue)  // 指向新的value, 原value占着茅坑不拉屎放那了, 反正也没指针指向它.
			return
		}
	}
	// 上面是为了获得key在每层的前后节点

	// 若不是对已存在的Key更新, 那么We do need to create a new node.
	height := s.randomHeight()
	x := newNode(s.arena, key, v, height)

	// Try to increase s.height via CAS.
	listHeight = s.getHeight()
	for height > int(listHeight) {  // 防并发. 可能有多个请求都会要求增加跳表高度, 总之保证跳表的高度(listHeight)>=要插入的高度(height)
		if atomic.CompareAndSwapInt32(&s.height, listHeight, int32(height)) {  // CAS(&addr, old, new) 语义: 如果*addr=old, 则设置为new; 否则说明其他协程更改过, 失败
			// Successfully increased skiplist.height.
			break
		}
		listHeight = s.getHeight()  // 重新获取listHeight值(因为其他并发请求更新导致listHeight发生变化)
	}

	// We always insert from the base level and up. After you add a node in base level, we cannot
	// create a node in the level above because it would have discovered the node in the base level.
	// 保证从0层开始插入node, 上面这句话意思是如果多个协程同时插入同一个key的话, 会在底层时就发现对方.
	for i := 0; i < height; i++ {
		for {  // 这里的for是无限循环, 直到CAS更新该层成功
			if s.arena.getNode(prev[i]) == nil { // 这里是跳表层数增长时才有的逻辑. 如果原来只有2层, 突然增加到10层, 那么pre[3~10]都是nil
				// 这里也不用考虑并发, 因为节点前后关系在这里并不会修改
				AssertTrue(i > 1) // This cannot happen in base level. 第0层的前一个节点是pre[1], 肯定不为nil. L364:prev[listHeight] = s.headOffset决定的
				// We haven't computed prev, next for this level because height exceeds old listHeight.
				// For these levels, we expect the lists to be sparse, so we can just search from head.
				// 如果并发请求导致在这些新的层有了一些节点, 肯定是稀疏的(因此从head开始找也不会慢), 在其中找到前后节点; 当然也可能没并发请求, 这些层都是空的. findSpliceForLevel对它们处理逻辑是一致的
				prev[i], next[i] = s.findSpliceForLevel(key, s.headOffset, i)
				// Someone adds the exact same key before we are able to do so. This can only happen on
				// the base level. But we know we are not on the base level. 见L424的解释
				AssertTrue(prev[i] != next[i])
			}
			x.tower[i] = next[i]  // 新节点连后继不会有并发问题
			pnode := s.arena.getNode(prev[i])
			if pnode.casNextOffset(i, next[i], s.arena.getNodeOffset(x)) {  // 前驱更新要并发
				// Managed to insert x between prev[i] and next[i]. Go to the next level.
				break  // 前驱后面CAS连上了x, 该层插入成功, 跳出循环进入下一层
			}
			// CAS failed. We need to recompute prev and next.  这是CAS失败的弥补逻辑, 重新找一下key的前驱和后继
			// It is unlikely to be helpful to try to use a different level as we redo the search,
			// because it is unlikely that lots of nodes are inserted between prev[i] and next[i].
			prev[i], next[i] = s.findSpliceForLevel(key, prev[i], i)  // 重新获取next[i]
			if prev[i] == next[i] {
				// 因为所有并发请求都从i是从0开始往上逐层插入的.
				// 如果有2个协程A,B它们有相同的key并发插入.
				// 要么B先插入第0层, A协程在第0层发现相等, 更新值value后就跳出了. B还得勤勤恳恳把剩下的每层都插入, 但value却是A的值.
				// 要么A先插入0层, 那么A不会发现相同的key, 它会勤勤恳恳插入每一层, 被B占了便宜.
				AssertTruef(i == 0, "Equality can happen only on base level: %d", i)
				vo := s.arena.putVal(v)
				encValue := encodeValue(vo, v.EncodedSize())
				prevNode := s.arena.getNode(prev[i])
				prevNode.setValue(s.arena, encValue)
				return
			}
		}
	}
}

// Empty returns if the Skiplist is empty.
func (s *Skiplist) Empty() bool {
	return s.findLast() == nil
}

// findLast returns the last element. If head (empty list), we return nil. All the find functions
// will NEVER return the head nodes.
func (s *Skiplist) findLast() *node {  // 从最高层往后找速度快(logn);  而从第0层逐个找虽然也行, 但速度慢(n)
	n := s.getHead()
	level := int(s.getHeight()) - 1
	for {
		next := s.getNext(n, level)
		if next != nil {
			n = next
			continue
		}
		if level == 0 {
			if n == s.getHead() {
				return nil
			}
			return n
		}
		level--
	}
}

// Search gets the value associated with the key. It returns a valid value if it finds equal or earlier
// version of the same key.
func (s *Skiplist) Search(key []byte) ValueStruct {
	n, _ := s.findNear(key, false, true) // findGreaterOrEqual.
	if n == nil {
		return ValueStruct{}
	}

	nextKey := s.arena.getKey(n.keyOffset, n.keySize)
	if !SameKey(key, nextKey) {
		return ValueStruct{}
	}

	valOffset, valSize := n.getValueOffset()
	vs := s.arena.getVal(valOffset, valSize)
	vs.ExpiresAt = ParseTs(nextKey)
	return vs
}

// NewSkipListIterator returns a skiplist iterator.  You have to Close() the iterator.
func (s *Skiplist) NewSkipListIterator() Iterator {
	s.IncrRef()
	return &SkipListIterator{list: s}
}

// MemSize returns the size of the Skiplist in terms of how much memory is used within its internal
// arena.
func (s *Skiplist) MemSize() int64 { return s.arena.size() }

// SkipListIterator is an iterator over skiplist object. For new objects, you just
// need to initialize Iterator.list.
type SkipListIterator struct {
	list *Skiplist
	n    *node
}

func (s *SkipListIterator) Rewind() {
	s.SeekToFirst()
}

func (s *SkipListIterator) Item() Item {
	return &Entry{
		Key:       s.Key(),
		Value:     s.Value().Value,
		ExpiresAt: s.Value().ExpiresAt,
		Meta:      s.Value().Meta,
		Version:   s.Value().Version,
	}
}

// Close frees the resources held by the iterator
func (s *SkipListIterator) Close() error {
	s.list.DecrRef()
	return nil
}

// Valid returns true iff the iterator is positioned at a valid node.
func (s *SkipListIterator) Valid() bool { return s.n != nil }

// Key returns the key at the current position.
func (s *SkipListIterator) Key() []byte {
	return s.list.arena.getKey(s.n.keyOffset, s.n.keySize)
}

// Value returns value.
func (s *SkipListIterator) Value() ValueStruct {
	valOffset, valSize := s.n.getValueOffset()
	return s.list.arena.getVal(valOffset, valSize)
}

// ValueUint64 returns the uint64 value of the current node.
func (s *SkipListIterator) ValueUint64() uint64 {
	return s.n.value
}

// Next advances to the next position.
func (s *SkipListIterator) Next() {
	AssertTrue(s.Valid())
	s.n = s.list.getNext(s.n, 0)
}

// Prev advances to the previous position.
func (s *SkipListIterator) Prev() {
	AssertTrue(s.Valid())
	s.n, _ = s.list.findNear(s.Key(), true, false) // find <. No equality allowed.
}

// Seek advances to the first entry with a key >= target.
func (s *SkipListIterator) Seek(target []byte) {
	s.n, _ = s.list.findNear(target, false, true) // find >=.
}

// SeekForPrev finds an entry with key <= target.
func (s *SkipListIterator) SeekForPrev(target []byte) {
	s.n, _ = s.list.findNear(target, true, true) // find <=.
}

// SeekToFirst seeks position at the first entry in list.
// Final state of iterator is Valid() iff list is not empty.
func (s *SkipListIterator) SeekToFirst() {
	s.n = s.list.getNext(s.list.getHead(), 0)
}

// SeekToLast seeks position at the last entry in list.
// Final state of iterator is Valid() iff list is not empty.
func (s *SkipListIterator) SeekToLast() {
	s.n = s.list.findLast()
}

// UniIterator is a unidirectional memtable iterator. It is a thin wrapper around
// Iterator. We like to keep Iterator as before, because it is more powerful and
// we might support bidirectional iterators in the future.
type UniIterator struct {
	iter     *Iterator
	reversed bool
}

// FastRand is a fast thread local random function.
//go:linkname FastRand runtime.fastrand
func FastRand() uint32

// AssertTruef is AssertTrue with extra info.
func AssertTruef(b bool, format string, args ...interface{}) {
	if !b {
		log.Fatalf("%+v", errors.Errorf(format, args...))
	}
}
