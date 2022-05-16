package cache

import (
	"errors"
	"fmt"
	"math"
	"math/rand"
	"time"
)

const (
	cmDepth = 4
)

type cmSketch struct {
	rows []cmRow
	seed []uint64
	mask uint64
}

// NewWithEstimates creates a new Count-Min Sketch with given error rate and confidence.
// Accuracy guarantees will be made in terms of a pair of user specified parameters,
// ε and δ, meaning that the error in answering a query is within a factor of ε with
// probability δ
func NewWithEstimates(epsilon, delta float64) (*cmSketch, error) {
	if epsilon <= 0 || epsilon >= 1 {
		return nil, errors.New("countminsketch: value of epsilon should be in range of (0, 1)")
	}
	if delta <= 0 || delta >= 1 {
		return nil, errors.New("countminsketch: value of delta should be in range of (0, 1)")
	}

	w := int64(math.Ceil(2 / epsilon))  // hash value range
	d := int64(math.Ceil(math.Log(1-delta) / math.Log(0.5)))
	fmt.Printf("ε: %f, δ: %f -> d: %d, w: %d\n", epsilon, delta, d, w)
	return newCmSketch(d, w), nil
}

// New creates a new Count-Min Sketch with _d_ hashing functions
// and _w_ hash value range
func newCmSketch(d int64, numCounters int64) *cmSketch {
	if numCounters == 0 {
		panic("cmSketch: invalid numCounters")
	}

	numCounters = next2Power(numCounters)
	sketch := &cmSketch{mask: uint64(numCounters - 1)}
	source := rand.New(rand.NewSource(time.Now().UnixNano()))
	sketch.seed = make([]uint64, d)
	sketch.rows = make([]cmRow, d)
	for i := 0; i < int(d); i++ {
		sketch.seed[i] = source.Uint64()
		sketch.rows[i] = newCmRow(numCounters)
	}
	return sketch
}

func (s *cmSketch) Increment(hashed uint64) {
	for i := range s.rows {
		s.rows[i].increment((hashed ^ s.seed[i]) & s.mask)
	}
}

// Estimate 估计出频率, 算法是取4个值中的最小值
func (s *cmSketch) Estimate(hashed uint64) int64 {
	//implement me here!!!
	var minFreq byte = 255
	for i := 0; i < len(s.rows); i++ {
		//var n = (hashed ^ s.seed[i]) & s.mask
		//idx := n / 2
		//shift := (n & 1) * 4
		//v := (s.rows[i][idx] >> shift) & 0x0f
		v := s.rows[i].get((hashed ^ s.seed[i]) & s.mask)
		if v < minFreq {
			minFreq = v
		}
	}
	return int64(minFreq)
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

func newCmRow(numCounters int64) cmRow {
	return make(cmRow, numCounters/2)
}

func (r cmRow) get(n uint64) byte {
	return r[n/2] >> ((n & 1) * 4) & 0x0f
}

func (r cmRow) increment(n uint64) {
	i := n / 2
	s := (n & 1) * 4
	v := (r[i] >> s) & 0x0f
	if v < 15 {
		r[i] += 1 << s
	}
}

func (r cmRow) reset() {
	for i := range r {
		r[i] = (r[i] >> 1) & 0x77
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
