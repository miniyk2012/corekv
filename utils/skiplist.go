package utils

import (
	"bytes"
	"github.com/hardcore-os/corekv/utils/codec"
	"math/rand"
	"sync"
	"time"
)

const (
	defaultMaxLevel = 48
)

type SkipList struct {
	header *Element

	rand *rand.Rand

	maxLevel int
	length   int
	lock     sync.RWMutex
	size     int64
}

func NewSkipList() *SkipList {
	//implement me here!!!
	s := rand.NewSource(time.Now().UnixNano())
	r := rand.New(s)
	list := SkipList{
		rand: r,
	}
	return &list
}

type Element struct {
	levels []*Element
	entry  *codec.Entry
	score  float64
}

func newElement(score float64, entry *codec.Entry, level int) *Element {
	return &Element{
		levels: make([]*Element, level+1),
		entry:  entry,
		score:  score,
	}
}

func (elem *Element) Entry() *codec.Entry {
	return elem.entry
}

func (list *SkipList) Add(data *codec.Entry) error {
	//implement me here!!!

	var preElement, curElement *Element
	curElement = list.header
	score := list.calcScore(data.Key)
	if curElement == nil {
		list.header = newElement(score, data, 0)
		return nil
	}
	if list.compare(score, data.Key, curElement) < 0 {  // data比所有的节点都小
		list.header = newElement(score, data, 0)
		for j := 0; j<=list.maxLevel; j++  {
			list.header.levels[j] = curElement
		}
		return nil
	}
	preElements := make([]*Element, list.maxLevel+1)   // 存储每层添加位置的前一个节点
	i := list.maxLevel
	for i >= 0 {
		for preElement = curElement;curElement!=nil;curElement = curElement.levels[i] {
			cmp := list.compare(score, data.Key, curElement)
			if cmp <= 0 {
				if cmp == 0 {
					curElement.Entry().Value = data.Value  // key相等Value做替换, 而非添加节点
				} else {
					preElements[i] = preElement
				}
				curElement = preElement
				i--
				break
			}
		}
		if curElement == nil {  // 说明比所有节点都大
			for j := 0; j<=list.maxLevel; j++  {
				preElements[j] = preElement
			}
			break
		}
	}


	addLevel := list.randLevel()
	if addLevel > list.maxLevel {
		list.maxLevel = addLevel
	}
	e := newElement(score, data, addLevel)
	for j := 0; j<addLevel; j++ {
		next := preElements[j].levels[j]
		preElements[j].levels[j] = e
		e.levels[j] = next
	}
	return nil
}

func (list *SkipList) Search(key []byte) (e *codec.Entry) {
	//implement me here!!!
	i := list.maxLevel
	var preElement, curElement *Element
	curElement = list.header
	score := list.calcScore(key)
	if list.compare(score, key, curElement) < 0 {
		return nil
	}
	for i >= 0 {
		for preElement = curElement;curElement!=nil;curElement = curElement.levels[i] {
			cmp := list.compare(score, key, curElement)
			if cmp == 0 {
				return curElement.Entry()
			} else if cmp < 0 {
				i--
				curElement = preElement
				break
			}
		}
		if curElement == nil {
			return nil   // 比最大的大
		}
	}
	return nil  // 应该到不了这里
}

func (list *SkipList) Close() error {
	return nil
}

func (list *SkipList) calcScore(key []byte) (score float64) {
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

func (list *SkipList) compare(score float64, key []byte, next *Element) int {
	//implement me here!!!
	if score < next.score {
		return -1
	} else if score > next.score {
		return 1
	}
	return bytes.Compare(key, next.Entry().Key)
}

func (list *SkipList) randLevel() int {
	//implement me here!!!
	var level int
	for {
		if list.rand.Intn(2) == 0 {
			return level
		} else {
			level++
		}
	}
}

func (list *SkipList) Size() int64 {
	//implement me here!!!
	return list.size
}
