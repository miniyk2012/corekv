package cache

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBasicEstimate(t *testing.T) {
	cache := NewCache(5)
	s := newCmSketch(100)
	keyHash, _ := cache.keyToHash(5)
	s.Increment(keyHash)
	vv := s.Estimate(keyHash)
	assert.Equal(t, vv, int64(1))

	keyHash, _ = cache.keyToHash(105)
	vv = s.Estimate(keyHash)
	assert.Equal(t, vv, int64(0))
}

func TestReset(t *testing.T) {
	cache := NewCache(5)
	s := newCmSketch(10)
	times := 10
	keyHash, _ := cache.keyToHash(3093)
	for i := 0; i < times; i++ {
		s.Increment(keyHash)
	}
	vv := s.Estimate(keyHash)
	assert.Equal(t, vv, int64(times))

	s.Reset()
	vv = s.Estimate(keyHash)
	assert.Equal(t, vv, int64(times/2))
}
