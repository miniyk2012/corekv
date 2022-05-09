package cache

import (
	"fmt"
	"math/rand"
	"time"
)

const (
	cmDepth = 4
)

type cmSketch struct {
	rows [cmDepth]cmRow
	seed [cmDepth]uint64  // cmDepth个hash函数
	mask uint64  // 对hash取余的功能. 2的次幂下按位与（&）代替取余（%）运算. hash%numCounters=hash&(numCounters-1)
}

func newCmSketch(numCounters int64) *cmSketch {
	if numCounters == 0 {
		panic("cmSketch: invalid numCounters")
	}

	numCounters = next2Power(numCounters)
	sketch := &cmSketch{mask: uint64(numCounters - 1)}
	source := rand.New(rand.NewSource(time.Now().UnixNano()))

	for i := 0; i < cmDepth; i++ {
		sketch.seed[i] = source.Uint64()
		sketch.rows[i] = newCmRow(numCounters)
	}

	return sketch
}

// Increment 增加hash值(由key决定)的频率
func (s *cmSketch) Increment(hashed uint64) {
	for i := range s.rows {
		// mask: uint64(numCounters - 1)
		s.rows[i].increment((hashed ^ s.seed[i]) & s.mask)  // hash & s.mask保证了传入的n一定小于等于rows分配到的numCounter数.
	}
}

func (s *cmSketch) Estimate(hashed uint64) int64 {
	min := byte(255)
	for i := range s.rows {
		val := s.rows[i].get((hashed ^ s.seed[i]) & s.mask)
		if val < min {
			min = val
		}
	}

	return int64(min)
}

// Reset halves all counter values.
func (s *cmSketch) Reset() {
	for _, r := range s.rows {
		r.reset()
	}
}

// Clear zeroes all counters.
func (s *cmSketch) Clear() {
	for _, r := range s.rows {
		r.clear()
	}
}

// next2Power 快速计算最接近x的二次幂的算法
//比如x=5，返回8
//x = 110，返回128

//2^n
//1000000 (n个0）
//01111111（n个1） + 1
// x = 1001010 = 1111111 + 1 =10000000
func next2Power(x int64) int64 {
	x--
	x |= x >> 1
	x |= x >> 2
	x |= x >> 4
	x |= x >> 8
	x |= x >> 16
	x |= x >> 32
	x++
	return x
}

type cmRow []byte

// newCmRow 创建能存放32个counter的bitmap
// 1 byte = 2 counter. 因此只需要[numCounters/2]byte就行了
func newCmRow(numCounters int64) cmRow {
	return make(cmRow, numCounters/2)
}

func (r cmRow) get(n uint64) byte {
	return r[n/2] >> ((n & 1) * 4) & 0x0f
}

// increment 根据hash值增加对应的counter
// n: 是key的hash值, 需要外部保证传入的n一定小于等于cmRow分配到的numCounter数.
// 0000,0000 | 0000,0000 | 0000,0000 make(byte[], 3) = 6counter
func (r cmRow) increment(n uint64) {
	// 定位到第i个byte
	i := n / 2
	// 右移距离, n=偶数时为s0, 奇数为s=4
	s := (n & 1) * 4
	// 取前4个bit还是后4个bit
	v := (r[i] >> s) & 0x0f
	// 未超出最大计数时, 计数+1
	if v < 15 {
		r[i] += 1 << s
	}
}

// reset 减半(保鲜), 特别巧妙的方法, 体会一下
func (r cmRow) reset() {
	for i := range r {
		r[i] = (r[i] >> 1) & 0x77  // 0111 0111
	}
}

func (r cmRow) clear() {
	for i := range r {
		r[i] = 0
	}
}

func (r cmRow) string() string {
	s := ""
	for i := uint64(0); i < uint64(len(r)*2); i++ {
		s += fmt.Sprintf("%02d ", (r[(i/2)]>>((i&1)*4))&0x0f)
	}
	s = s[:len(s)-1]
	return s
}
