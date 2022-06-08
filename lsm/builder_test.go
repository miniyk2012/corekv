package lsm

import (
	"fmt"
	"github.com/stretchr/testify/assert"
	"sort"
	"testing"
)

func TestHeader(t *testing.T) {
	assert.Equal(t, uint16(4), headerSize)
}


func TestSortSearch(t *testing.T) {
	idx := sort.Search(5, func(i int) bool {
		fmt.Println(i)
		return i>3
	})
	assert.Equal(t, 4, idx)
}