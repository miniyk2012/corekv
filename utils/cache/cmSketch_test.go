package cache

import (
	"github.com/stretchr/testify/assert"
	"log"
	"os"
	"testing"
)

func TestBasicEstimate(t *testing.T) {
	cache := NewCache(5)
	s := newCmSketch(cmDepth, 7000)
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
	s := newCmSketch(cmDepth, 7000)
	times := 10
	keyHash, _ := cache.keyToHash(3093)
	for i:=0; i<times; i++ {
		s.Increment(keyHash)
	}
	vv := s.Estimate(keyHash)
	assert.Equal(t, vv, int64(times))

	s.Reset()
	vv = s.Estimate(keyHash)
	assert.Equal(t, vv, int64(times/2))
}

// based on https://github.com/jehiah/countmin/blob/master/sketch_test.go
func TestAccuracy(t *testing.T) {
	log.SetOutput(os.Stdout)
	cache := NewCache(5)
	s, err := NewWithEstimates(0.0001, 0.9999)
	if err != nil {
		t.Error(err)
	}
	//s := newCmSketch(2, 7000)

	iterations := 5500
	var diverged int
	for i := 1; i < iterations; i++ {
		v := i % 16
		var keyHash uint64
		for j := 0; j < v; j++ {
			keyHash, _ := cache.keyToHash(i)
			s.Increment(keyHash)
		}
		vv := s.Estimate(keyHash)
		if int(vv) > v {
			diverged++
		}
	}

	var miss int
	for i := 1; i < iterations; i++ {
		vv := i % 16
		keyHash, _ := cache.keyToHash(i)
		v := s.Estimate(keyHash)
		assert.Equal(t, int(v) >= vv, true)
		if int(v) != vv {
			log.Printf("key[%d] real: %d, estimate: %d\n", i, vv, v)
			miss++
		}
	}
	log.Printf("missed %d of %d (%d diverged during adds)", miss, iterations, diverged)
}
