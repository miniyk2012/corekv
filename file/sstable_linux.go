//go:build linux
// +build linux

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

package file

import (
	"io"
	"os"
	"sync"
	"syscall"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/hardcore-os/corekv/pb"
	"github.com/hardcore-os/corekv/utils"
	"github.com/pkg/errors"
)

// SSTable 文件的内存封装
type SSTable struct {
	lock           *sync.RWMutex
	f              *MmapFile
	maxKey         []byte
	minKey         []byte
	idxTables      *pb.TableIndex
	hasBloomFilter bool
	idxLen         int
	idxStart       int
	fid            uint64
	createdAt      time.Time
}

// OpenSStable 打开一个 sst文件
func OpenSStable(opt *Options) *SSTable {
	omf, err := OpenMmapFile(opt.FileName, os.O_CREATE|os.O_RDWR, opt.MaxSz)
	utils.Err(err)
	return &SSTable{f: omf, fid: opt.FID, lock: &sync.RWMutex{}}
}

// Init 初始化
func (ss *SSTable) Init() error {
	var ko *pb.BlockOffset // 存储首个BlockOffset
	var err error
	if ko, err = ss.initTable(); err != nil {
		return err
	}
	// 从文件中获取创建时间
	stat, _ := ss.f.Fd.Stat()
	statType := stat.Sys().(*syscall.Stat_t)
	ss.createdAt = time.Unix(statType.Ctim.Sec, statType.Ctim.Nsec)
	// init min key
	keyBytes := ko.GetKey()
	minKey := make([]byte, len(keyBytes))
	copy(minKey, keyBytes)
	ss.minKey = minKey
	ss.maxKey = minKey
	return nil
}

// SetMaxKey max 需要使用table的迭代器，来获取最后一个block的最后一个key
func (ss *SSTable) SetMaxKey(maxKey []byte) {
	ss.maxKey = maxKey
}

// initTable 解析mmap内的tableIndex, 设置到SSTable.idxTables上, 并返回第一个BlockOffset
func (ss *SSTable) initTable() (bo *pb.BlockOffset, err error) {
	readPos := len(ss.f.Data)

	// Read checksum len from the last 4 bytes.
	readPos -= 4
	buf := ss.readCheckError(readPos, 4)
	checksumLen := int(utils.BytesToU32(buf))
	if checksumLen < 0 {
		return nil, errors.New("checksum length less than zero. Data corrupted")
	}

	// Read checksum.
	readPos -= checksumLen
	expectedChk := ss.readCheckError(readPos, checksumLen)

	// Read index size from the footer.
	readPos -= 4
	buf = ss.readCheckError(readPos, 4)
	ss.idxLen = int(utils.BytesToU32(buf))

	// Read index.
	readPos -= ss.idxLen
	ss.idxStart = readPos
	data := ss.readCheckError(readPos, ss.idxLen)
	if err := utils.VerifyChecksum(data, expectedChk); err != nil {
		return nil, errors.Wrapf(err, "failed to verify checksum for table: %s", ss.f.Fd.Name())
	}
	indexTable := &pb.TableIndex{}
	if err := proto.Unmarshal(data, indexTable); err != nil {
		return nil, err
	}
	ss.idxTables = indexTable

	ss.hasBloomFilter = len(indexTable.BloomFilter) > 0
	if len(indexTable.GetOffsets()) > 0 {
		return indexTable.GetOffsets()[0], nil
	}
	return nil, errors.New("read index fail, offset is nil")
}

// Close 关闭
func (ss *SSTable) Close() error {
	return ss.f.Close()
}

// Indexs _
func (ss *SSTable) Indexs() *pb.TableIndex {
	return ss.idxTables
}

// MaxKey 当前最大的key
func (ss *SSTable) MaxKey() []byte {
	return ss.maxKey
}

// MinKey 当前最小的key
func (ss *SSTable) MinKey() []byte {
	return ss.minKey
}

// FID 获取fid
func (ss *SSTable) FID() uint64 {
	return ss.fid
}

// HasBloomFilter _
func (ss *SSTable) HasBloomFilter() bool {
	return ss.hasBloomFilter
}

func (ss *SSTable) read(off, sz int) ([]byte, error) {
	if len(ss.f.Data) > 0 {
		if len(ss.f.Data[off:]) < sz {
			return nil, io.EOF
		}
		return ss.f.Data[off : off+sz], nil
	}

	res := make([]byte, sz)
	_, err := ss.f.Fd.ReadAt(res, int64(off))
	return res, err
}
func (ss *SSTable) readCheckError(off, sz int) []byte {
	buf, err := ss.read(off, sz)
	utils.Panic(err)
	return buf
}

// Bytes returns data starting from offset off of size sz. If there's not enough data, it would
// return nil slice and io.EOF.
func (ss *SSTable) Bytes(off, sz int) ([]byte, error) {
	return ss.f.Bytes(off, sz)
}

// Size 返回底层文件的尺寸
func (ss *SSTable) Size() int64 {
	fileStats, err := ss.f.Fd.Stat()
	utils.Panic(err)
	return fileStats.Size()
}

// GetCreatedAt _
func (ss *SSTable) GetCreatedAt() *time.Time {
	return &ss.createdAt
}

// SetCreatedAt _
func (ss *SSTable) SetCreatedAt(t *time.Time) {
	ss.createdAt = *t
}

// Detele _
func (ss *SSTable) Detele() error {
	return ss.f.Delete()
}

// Truncature _
func (ss *SSTable) Truncature(size int64) error {
	return ss.f.Truncature(size)
}
