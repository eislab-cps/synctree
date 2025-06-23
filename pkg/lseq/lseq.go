package lseq

import (
	"math/rand"
)

// Constants for LSEQ allocation
const (
	Base = 16 // Base interval range
	//Base  = 256 // Base interval range
	Bound = 10 // Controls probability of depth increase
)

type Position []int

func GeneratePositionBetweenLSEQ(left, right Position) Position {
	pos := Position{}
	level := 0

	for {
		var l, r int
		if level < len(left) {
			l = left[level]
		}
		if level < len(right) {
			r = right[level]
		} else {
			r = Base
		}

		if r-l > 1 {
			// Space exists, choose randomly in the gap
			newDigit := l + 1 + rand.Intn(r-l-1)
			return append(pos, newDigit)
		}

		// No room, copy l and go deeper
		pos = append(pos, l)
		level++
	}
}
