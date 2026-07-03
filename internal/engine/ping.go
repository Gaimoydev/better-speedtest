package engine

import (
	"net"
	"time"
)

func TCPPing(host, port string, count int, timeout time.Duration) (avg, jitter float64, ok bool) {
	var xs []float64
	for i := 0; i < count; i++ {
		t0 := time.Now()
		c, err := net.DialTimeout("tcp", net.JoinHostPort(host, port), timeout)
		if err != nil {
			continue
		}
		xs = append(xs, float64(time.Since(t0).Microseconds())/1000.0)
		_ = c.Close()
		time.Sleep(30 * time.Millisecond)
	}
	if len(xs) == 0 {
		return 0, 0, false
	}
	var sum float64
	for _, v := range xs {
		sum += v
	}
	return sum / float64(len(xs)), Jitter(xs), true
}
