package utils

import (
	"fmt"
	"github.com/hardcore-os/corekv/utils/codec"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sync"
	"testing"
)

func RandString(len int) string {
	bytes := make([]byte, len)
	for i := 0; i < len; i++ {
		b := r.Intn(26) + 65
		bytes[i] = byte(b)
	}
	return string(bytes)
}

func TestSkipList_compare(t *testing.T) {
	list := SkipList{
		header:   nil,
		rand:     nil,
		maxLevel: 0,
		length:   0,
	}

	byte1 := []byte("1")
	byte2 := []byte("2")
	entry1 := codec.NewEntry(byte1, byte1)

	byte1score := list.calcScore(byte1)
	byte2score := list.calcScore(byte2)

	elem := &Element{
		levels: nil,
		entry:  entry1,
		score:  byte2score,
	}

	assert.Equal(t, -1, list.compare(byte1score, byte1, elem))
}

/**
	构造一个这样的跳表, 做Search的测试
	A------------------E
	A--------C---------E
	A---B----C----D----E
 */
func TestSkipSearch(t *testing.T) {
	list := NewSkipList()
	list.maxLevel = 2
	entryA := codec.NewEntry([]byte("A"), []byte("AVal"))
	entryB := codec.NewEntry([]byte("B"), []byte("BVal"))
	entryC := codec.NewEntry([]byte("C"), []byte("CVal"))
	entryD := codec.NewEntry([]byte("D"), []byte("DVal"))
	entryE := codec.NewEntry([]byte("E"), []byte("EVal"))
	elementA := newElement(list.calcScore(entryA.Key), entryA, 2)
	elementB := newElement(list.calcScore(entryB.Key), entryB, 0)
	elementC := newElement(list.calcScore(entryC.Key), entryC, 1)
	elementD := newElement(list.calcScore(entryD.Key), entryD, 0)
	elementE := newElement(list.calcScore(entryE.Key), entryE, 2)
	elementA.levels[2], elementA.levels[1], elementA.levels[0] = elementE, elementC, elementB
	elementB.levels[0] = elementC
	elementC.levels[1], elementC.levels[0] = elementE, elementD
	elementD.levels[0] = elementE
	elementE.levels[2], elementE.levels[1], elementE.levels[0] = nil, nil, nil
	list.header = elementA
	list.Draw()
	list.Search([]byte("D"))

	assert.Equal(t, entryE.Value, list.Search(entryE.Key).Value)
	assert.Equal(t, entryA.Value, list.Search(entryA.Key).Value)
	assert.Equal(t, entryC.Value, list.Search(entryC.Key).Value)
	assert.Equal(t, entryD.Value, list.Search(entryD.Key).Value)
	assert.Nil(t, list.Search([]byte("0")))
	assert.Nil(t, list.Search([]byte("F")))
}

func TestSkipListBasicCRUD(t *testing.T) {
	list := NewSkipList()

	//Put & Get
	entry1 := codec.NewEntry([]byte("Key1"), []byte("Val1"))
	assert.Nil(t, list.Add(entry1))
	assert.Equal(t, entry1.Value, list.Search(entry1.Key).Value)

	entry2 := codec.NewEntry([]byte("Key2"), []byte("Val2"))
	assert.Nil(t, list.Add(entry2))
	assert.Equal(t, entry2.Value, list.Search(entry2.Key).Value)

	//Get a not exist entry
	assert.Nil(t, list.Search([]byte("noexist")))

	//Update a entry
	entry2_new := codec.NewEntry([]byte("Key1"), []byte("Val1+1"))
	assert.Nil(t, list.Add(entry2_new))
	assert.Equal(t, entry2_new.Value, list.Search(entry2_new.Key).Value)

	// 乱序的加入数据
	n := 100
	randKeys := make([][]byte, n)
	for i:=0; i<n; i++ {
		key := RandString(16)
		entry_rand := codec.NewEntry([]byte(key), []byte(key))
		assert.Nil(t, list.Add(entry_rand))
		randKeys[i] = []byte(key)
	}
	for _, key := range randKeys {
		entry := list.Search(key)
		assert.Equal(t,key, entry.Value)
	}
}

func Benchmark_SkipListBasicCRUD(b *testing.B) {
	list := NewSkipList()
	key, val := "", ""
	maxTime := 1000000
	for i := maxTime; i > 0; i-- {
		//number := rand.Intn(10000)
		key, val = fmt.Sprintf("Key%d", i), fmt.Sprintf("Val%d", i)
		entry := codec.NewEntry([]byte(key), []byte(val))
		res := list.Add(entry)
		assert.Equal(b, nil, res)
		searchVal := list.Search([]byte(key))
		assert.Equal(b, []byte(val), searchVal.Value)
	}
	assert.Equal(b, int64(maxTime), list.size)
}

func TestDrawSkipList(t *testing.T) {
	keys := []string{
		"1", "5", "3","2","6", "0","9", "4", "8", "4", "7",
	}
	list := NewSkipList()
	for _, key := range keys {
		entry := codec.NewEntry([]byte(key), []byte(key))
		list.Add(entry)
	}
	assert.Equal(t, 10, int(list.Size()))
	for _, key :=range keys {
		v := list.Search([]byte(key))
		if v != nil {
			assert.Equal(t, []byte(key), v.Value)
		} else {
			t.FailNow()
		}
	}
	list.Draw()
	fmt.Println()
	entry := codec.NewEntry([]byte("3"), []byte("a3"))
	list.Add(entry)
	list.Draw()
	fmt.Println()

	entry = codec.NewEntry([]byte("6"), []byte("a6"))
	list.Add(entry)
	list.Draw()


	keys = []string{
		"9", "8", "7", "6", "5",
	}
	list = NewSkipList()
	for _, key := range keys {
		entry := codec.NewEntry([]byte(key), []byte(key))
		list.Add(entry)
	}
	list.Draw()

	keys = []string{
		"1", "2", "3", "4", "8",
	}
	list = NewSkipList()
	for _, key := range keys {
		entry := codec.NewEntry([]byte(key), []byte(key))
		list.Add(entry)
	}
	list.Draw()
}

func TestConcurrentBasic(t *testing.T) {
	const n = 1000
	l := NewSkipList()
	var wg sync.WaitGroup
	key := func(i int) []byte {
		return []byte(fmt.Sprintf("%05d", i))
	}
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			assert.Nil(t, l.Add(codec.NewEntry(key(i), key(i))))
		}(i)
	}
	wg.Wait()

	// Check values. Concurrent reads.
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			v := l.Search(key(i))
			if v != nil {
				require.EqualValues(t, key(i), v.Value)
				return
			}
			require.Nil(t, v)
		}(i)
	}
	wg.Wait()
}

func Benchmark_ConcurrentBasic(b *testing.B) {
	n := 1000
	l := NewSkipList()
	var wg sync.WaitGroup
	key := func(i int) []byte {
		return []byte(fmt.Sprintf("%05d", i))
	}
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			the_key := key(i)
			assert.Nil(b, l.Add(codec.NewEntry(the_key, the_key)))
		}(i)
	}
	wg.Wait()
	assert.Equal(b, int64(n), l.size)
	// Check values. Concurrent reads.
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			the_key := key(i)
			v := l.Search(the_key)
			if v != nil {
				require.EqualValues(b, the_key, v.Value)
				return
			} else {
				panic("fail")
			}
		}(i)
	}
	wg.Wait()
}
