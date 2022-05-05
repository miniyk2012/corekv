// Copyright 2021 hardcore-os Project Authors
//
// Licensed under the Apache License, Version 2.0 (the "License")
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package utils

import "math"

// Filter is an encoded set of []byte keys.
type Filter []byte

// MayContainKey _
func (f Filter) MayContainKey(k []byte) bool {
	return f.MayContain(Hash(k))
}

// MayContain returns whether the filter may contain given key. False positives
// are possible, where it returns true for keys not in the original set.
func (f Filter) MayContain(h uint32) bool {
	//Implement me here!!!
	//在这里实现判断一个数据是否在bloom过滤器中
	//思路是K个Hash函数计算，对应位置全1返回true, 有0就返回false
	if len(f) < 2 {
		return false
	}
	nBits := (len(f) - 1) * 8
	k := f[len(f)-1]
	if k > 30 {
		// This is reserved for potentially new encodings for short Bloom filters.
		// Consider it a match.
		return true
	}
	delta := h>>17 | h<<15
	for j := uint8(0); j < k; j++ {
		bitPosition := h % uint32(nBits)
		if f[bitPosition/8]&(1<<(bitPosition%8)) == 0 {
			return false
		}
		h += delta
	}
	return true
}

// NewFilter returns a new Bloom filter that encodes a set of []byte keys with
// the given number of bits per key, approximately.
//
// A good bitsPerKey value is 10, which yields a filter with ~ 1% false
// positive rate.
//  bloom过滤器提供的接口只有一次性初始化所有key的接口, 未提供每次往里面加一个key的方法
// 因为每次sstable的dump是一整个文件一次性dump, 因此key一次性放到一个bloom过滤器里就行了
func NewFilter(keys []uint32, bitsPerKey int) Filter {
	return Filter(appendFilter(keys, bitsPerKey))
}

// BloomBitsPerKey returns the bits per key required by bloomfilter based on
// the false positive rate.
func BloomBitsPerKey(numEntries int, fp float64) int {
	//Implement me here!!!
	//阅读bloom论文实现，并在这里编写公式
	//传入参数numEntries是bloom中存储的数据个数，fp是false positive假阳性率
	size := -1 * float64(numEntries) * math.Log(fp) / math.Pow(float64(0.69314718056), 2)
	locs := math.Ceil(size / float64(numEntries))
	return int(locs)
}

func appendFilter(keys []uint32, bitsPerKey int) []byte {
	//Implement me here!!!
	//在这里实现将多个Key值放入到bloom过滤器中
	if bitsPerKey < 0 {
		bitsPerKey = 0
	}
	k := uint32(float64(bitsPerKey) * 0.69)
	if k < 1 {
		k = 1
	}
	if k > 30 {
		k = 30
	}
	nBits := bitsPerKey * len(keys) // 用于存储hash值的bit长度
	if nBits < 64 {
		nBits = 64
	}
	nBytes := (nBits + 7) / 8        // 需要多少长度的字节存储所有bits
	filter := make([]byte, nBytes+1) // 最后一位存储k
	nBits = nBytes * 8               // nBits必须是8的倍数
	for _, h := range keys {
		delta := h>>17 | h<<15
		for j := uint32(0); j < k; j++ {
			bitPosition := h % uint32(nBits)
			filter[bitPosition/8] |= 1 << (bitPosition % 8)
			h += delta
		}
	}
	filter[nBytes] = uint8(k)
	return filter
}

// Hash implements a hashing algorithm similar to the Murmur hash.
func Hash(b []byte) uint32 {
	//Implement me here!!!
	//在这里实现高效的HashFunction
	const (
		seed = 0xbc9f1d34
		m    = 0xc6a4a793
	)
	var h uint32 = uint32(seed) ^ uint32(len(b))*m

	for ; len(b) >= 4; b = b[4:] {
		h += uint32(b[0]) | uint32(b[1])<<8 | uint32(b[2])<<16 | uint32(b[3])<<24
		h *= m
		h ^= h >> 16
	}
	switch len(b) {
	case 3:
		h += uint32(b[2]) << 16
		fallthrough
	case 2:
		h += uint32(b[1]) << 8
		fallthrough
	case 1:
		h += uint32(b[0])
		h *= m
		h ^= h >> 24
	}
	return h
}
