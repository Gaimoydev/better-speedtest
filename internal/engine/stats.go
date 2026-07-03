package engine

import "math"

type Sample struct {
	T   float64
	Bps float64
}

func Summarize(series []Sample, warmup float64) (avg, peak float64) {
	if len(series) == 0 {
		return 0, 0
	}
	var kept []float64
	for _, s := range series {
		if s.Bps > peak {
			peak = s.Bps
		}
		if s.T > warmup {
			kept = append(kept, s.Bps)
		}
	}
	if len(kept) == 0 {
		for _, s := range series {
			kept = append(kept, s.Bps)
		}
	}
	var sum float64
	for _, v := range kept {
		sum += v
	}
	return sum / float64(len(kept)), peak
}

func Jitter(xs []float64) float64 {
	if len(xs) < 2 {
		return 0
	}
	var sum float64
	for i := 1; i < len(xs); i++ {
		sum += math.Abs(xs[i] - xs[i-1])
	}
	return sum / float64(len(xs)-1)
}
