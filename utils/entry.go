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

import (
	"encoding/binary"
	"time"
)

type ValueStruct struct {
	Meta      byte
	Value     []byte
	ExpiresAt uint64

	Version uint64 // This field is not serialized. Only for internal usage.
}

// value只持久化具体的value值和过期时间
func (vs *ValueStruct) EncodedSize() uint32 {
	sz := len(vs.Value) + 1 // meta
	enc := sizeVarint(vs.ExpiresAt)
	return uint32(sz + enc)
}

// DecodeValue
func (vs *ValueStruct) DecodeValue(buf []byte) {
	vs.Meta = buf[0]
	var sz int
	vs.ExpiresAt, sz = binary.Uvarint(buf[1:])
	vs.Value = buf[1+sz:]
}

// EncodeValue 对value进行编码，并将编码后的字节写入byte
// 这里将过期时间和value的值一起编码
// 返回编码的长度(似乎没用到)
func (vs *ValueStruct) EncodeValue(b []byte) uint32 {
	b[0] = vs.Meta
	sz := binary.PutUvarint(b[1:], vs.ExpiresAt)
	n := copy(b[1+sz:], vs.Value)
	return uint32(1 + sz + n)
}

// sizeVarint EncodeValue方法中的PutUvarint就是用varint编码来存储数据的, 因此这里计算了varint编码所占的内存大小, 以保证2者一致.
// varint简单来说就是每7位存一字节, 头上补个1, 好处是如果x是很小的数, 高位都是0, 那么只需存低位的值, 1字节就够了, 相比uint64原来是8字节的, 大大缩减了存储空间
func sizeVarint(x uint64) (n int) {
	for {
		n++
		x >>= 7
		if x == 0 {
			break
		}
	}
	return n
}

//Entry 最外层写入的结构体
type Entry struct {
	Key       []byte
	Value     []byte
	ExpiresAt uint64

	Meta         byte
	Version      uint64
	Offset       uint32
	Hlen         int // Length of the header.
	ValThreshold int64
}

// NewEntry_
func NewEntry(key, value []byte) *Entry {
	return &Entry{
		Key:   key,
		Value: value,
	}
}

// Entry_
func (e *Entry) Entry() *Entry {
	return e
}

func (e *Entry) IsDeletedOrExpired() bool {
	if e.Value == nil {
		return true
	}

	if e.ExpiresAt == 0 {
		return false
	}

	return e.ExpiresAt <= uint64(time.Now().Unix())
}

// WithTTL _
func (e *Entry) WithTTL(dur time.Duration) *Entry {
	e.ExpiresAt = uint64(time.Now().Add(dur).Unix())
	return e
}

// EncodedSize is the size of the ValueStruct when encoded
func (e *Entry) EncodedSize() uint32 {
	sz := len(e.Value)
	enc := sizeVarint(uint64(e.Meta))
	enc += sizeVarint(e.ExpiresAt)
	return uint32(sz + enc)
}

// EstimateSize
func (e *Entry) EstimateSize(threshold int) int {
	// TODO: 是否考虑 user meta?
	if len(e.Value) < threshold {
		return len(e.Key) + len(e.Value) + 1 // Meta
	}
	return len(e.Key) + 12 + 1 // 12 for ValuePointer, 2 for meta.
}

// header 对象
// header is used in value log as a header before Entry.
type Header struct {
	KLen      uint32
	VLen      uint32
	ExpiresAt uint64
	Meta      byte
}

// +------+----------+------------+--------------+-----------+
// | Meta | UserMeta | Key Length | Value Length | ExpiresAt |
// +------+----------+------------+--------------+-----------+
func (h Header) Encode(out []byte) int {
	out[0] = h.Meta
	index := 1
	index += binary.PutUvarint(out[index:], uint64(h.KLen))
	index += binary.PutUvarint(out[index:], uint64(h.VLen))
	index += binary.PutUvarint(out[index:], h.ExpiresAt)
	return index
}

// Decode decodes the given header from the provided byte slice.
// Returns the number of bytes read.
func (h *Header) Decode(buf []byte) int {
	h.Meta = buf[0]
	index := 1
	klen, count := binary.Uvarint(buf[index:])
	h.KLen = uint32(klen)
	index += count
	vlen, count := binary.Uvarint(buf[index:])
	h.VLen = uint32(vlen)
	index += count
	h.ExpiresAt, count = binary.Uvarint(buf[index:])
	return index + count
}

// DecodeFrom reads the header from the hashReader.
// Returns the number of bytes read.
func (h *Header) DecodeFrom(reader *HashReader) (int, error) {
	var err error
	h.Meta, err = reader.ReadByte()
	if err != nil {
		return 0, err
	}
	klen, err := binary.ReadUvarint(reader)
	if err != nil {
		return 0, err
	}
	h.KLen = uint32(klen)
	vlen, err := binary.ReadUvarint(reader)
	if err != nil {
		return 0, err
	}
	h.VLen = uint32(vlen)
	h.ExpiresAt, err = binary.ReadUvarint(reader)
	if err != nil {
		return 0, err
	}
	return reader.BytesRead, nil
}
