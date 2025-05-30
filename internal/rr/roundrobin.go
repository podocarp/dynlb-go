package rr

import "slices"

type WeightedRoundRobin struct {
	weights   []int // cap of each node divided by total cap, rounded
	rounds    int   // number of rounds of weighted round robin, equal to max weight
	currIndex int   // current index we are at for interleaved round robin
	currRound int   // the current round we are in for interleaved round robin
}

func NewWeightedRoundRobin(weights []int) *WeightedRoundRobin {
	return &WeightedRoundRobin{
		weights:   weights,
		rounds:    0,
		currIndex: 0,
		currRound: 0,
	}
}

func (r *WeightedRoundRobin) advanceIndex() {
	r.currIndex++
	if r.currIndex >= len(r.weights) {
		r.currIndex = 0
		r.currRound++
		if r.currRound > r.rounds {
			r.currRound = 1
		}
	}
}

func (r *WeightedRoundRobin) Dispatch() int {
	for {
		if r.weights[r.currIndex] >= r.currRound {
			break
		} else {
			r.advanceIndex()
		}
	}
	index := r.currIndex
	r.advanceIndex()

	return index
}

func (r *WeightedRoundRobin) GetWeights() []int {
	return r.weights
}

func (r *WeightedRoundRobin) UpdateWeights(weights []int) {
	r.weights = weights
	r.rounds = slices.Max(r.weights)
}
