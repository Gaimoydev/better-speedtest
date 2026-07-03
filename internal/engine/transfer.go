package engine

import (
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"better-speedtest/internal/config"
)

const (
	dlUA       = "Mozilla/5.0 (Windows NT 10.0; WOW64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/100.0.4896.60 Safari/537.36"
	ulUA       = "Dalvik/1.6.0 (Linux; U; Android 4.2.2; GT-I9505 Build/JDQ39)"
	ulBoundary = "00content0boundary00"
)

type worker func(ctx context.Context, appBytes *uint64)

type Result struct {
	AvgBps   float64
	PeakBps  float64
	AppBytes uint64
	Conns    int
}

// WorkerFactory builds one transfer worker (e.g. from CDNDownload). A slice of
// them lets RunDirectionPool run several sources concurrently.
type WorkerFactory = func() worker

func RunDirection(iface string, upload bool, threads int, duration, warmup, every time.Duration, mk WorkerFactory, onTick func(t, mbps, peak float64)) Result {
	return RunDirectionPool(iface, upload, []WorkerFactory{mk}, threads, threads, duration, warmup, every, onTick)
}

func RunDirectionAdaptive(iface string, upload bool, minThreads, maxThreads int, duration, warmup, every time.Duration, mk WorkerFactory, onTick func(t, mbps, peak float64)) Result {
	return RunDirectionPool(iface, upload, []WorkerFactory{mk}, minThreads, maxThreads, duration, warmup, every, onTick)
}

// RunDirectionPool runs one or more sources concurrently (new connections are
// round-robined across them, so throughput aggregates and can exceed any single
// server's cap). It ramps total connections from minThreads toward maxThreads
// while throughput keeps rising, then rolls back to the count that gave the best
// result — so a link/CPU that can't take more connections is dialed back
// ("point-to-stop"). minThreads==maxThreads and one factory = fixed single source.
func RunDirectionPool(iface string, upload bool, mks []WorkerFactory, minThreads, maxThreads int, duration, warmup, every time.Duration, onTick func(t, mbps, peak float64)) Result {
	if len(mks) == 0 {
		return Result{}
	}
	if minThreads < 1 {
		minThreads = 1
	}
	if maxThreads < minThreads {
		maxThreads = minThreads
	}
	ctx, cancel := context.WithCancel(context.Background())
	var appBytes uint64
	var wg sync.WaitGroup
	p := &connPool{parent: ctx, appBytes: &appBytes, wg: &wg, mks: mks}
	p.add(minThreads)
	series, conns := sampleWAN(ctx, iface, upload, duration, every, &appBytes, p, maxThreads, onTick)
	cancel()
	wg.Wait()
	avg, peak := Summarize(series, warmup.Seconds())
	return Result{AvgBps: avg, PeakBps: peak, AppBytes: atomic.LoadUint64(&appBytes), Conns: conns}
}

// connPool holds live workers with per-worker cancels so the sampler can add or
// remove connections during the run, round-robining new ones across sources.
type connPool struct {
	parent   context.Context
	appBytes *uint64
	wg       *sync.WaitGroup
	mks      []WorkerFactory
	next     int
	cancels  []context.CancelFunc
}

func (p *connPool) add(k int) {
	for i := 0; i < k; i++ {
		cctx, cc := context.WithCancel(p.parent)
		w := p.mks[p.next%len(p.mks)]()
		p.next++
		p.cancels = append(p.cancels, cc)
		p.wg.Add(1)
		go func() { defer p.wg.Done(); w(cctx, p.appBytes) }()
	}
}

func (p *connPool) removeLast(k int) {
	for i := 0; i < k && len(p.cancels) > 0; i++ {
		j := len(p.cancels) - 1
		p.cancels[j]()
		p.cancels = p.cancels[:j]
	}
}

func (p *connPool) count() int { return len(p.cancels) }

func sampleWAN(ctx context.Context, iface string, upload bool, duration, every time.Duration, appBytes *uint64, p *connPool, maxN int, onTick func(t, mbps, peak float64)) ([]Sample, int) {
	useIface := false
	if iface != "" {
		if _, _, ok := IfaceBytes(iface); ok {
			useIface = true
		}
	}
	pick := func() (uint64, bool) {
		if useIface {
			rx, tx, ok := IfaceBytes(iface)
			if upload {
				return tx, ok
			}
			return rx, ok
		}
		return atomic.LoadUint64(appBytes), true
	}
	start := time.Now()
	prev, _ := pick()
	prevT := start
	var series []Sample
	var peak float64
	ramping := p.count() < maxN
	var bestBps float64
	bestN := p.count()
	tick := time.NewTicker(every)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return series, p.count()
		case <-tick.C:
			cur, ok := pick()
			now := time.Now()
			el := now.Sub(start).Seconds()
			dt := now.Sub(prevT).Seconds()
			if ok && cur >= prev && dt > 0 {
				bps := float64(cur-prev) * 8.0 / dt
				series = append(series, Sample{T: el, Bps: bps})
				if bps > peak {
					peak = bps
				}
				if onTick != nil {
					onTick(el, bps/1e6, peak/1e6)
				}
				// Adaptive "fast-far, fine-near" ramp: while adding connections
				// still raises throughput, grow by doubling when the gain is big
				// (far below the knee) and by ~+25% when the gain is small (nearing
				// the knee) — so it saturates quickly yet lands on a precise count.
				// When it plateaus/drops (≤5% gain), roll back to the best count and
				// stop; because near-knee steps are small, that roll-back is small.
				if ramping {
					ratio := 2.0
					if bestBps > 0 {
						ratio = bps / bestBps
					}
					if ratio <= 1.05 {
						if p.count() > bestN {
							p.removeLast(p.count() - bestN)
						}
						ramping = false
					} else {
						bestBps = bps
						bestN = p.count()
						step := p.count() / 4
						if ratio > 1.30 {
							step = p.count()
						}
						if step < 4 {
							step = 4
						}
						if p.count()+step > maxN {
							step = maxN - p.count()
						}
						if step > 0 {
							p.add(step)
						} else {
							ramping = false
						}
					}
				}
			}
			if ok {
				prev, prevT = cur, now
			}
			if el >= duration.Seconds() {
				return series, p.count()
			}
		}
	}
}

func CNSpeedDownload(cfg *config.Config, ip, port, keyNoPrefix string) func() worker {
	path := cfg.CNSpeed.Paths["node_download"]
	return func() worker {
		return func(ctx context.Context, appBytes *uint64) {
			buf := make([]byte, 64*1024)
			for ctx.Err() == nil {
				c, err := net.DialTimeout("tcp", net.JoinHostPort(ip, port), 15*time.Second)
				if err != nil {
					time.Sleep(200 * time.Millisecond)
					continue
				}
				go func(conn net.Conn) { <-ctx.Done(); conn.Close() }(c) // abort a blocked Read on cancel
				req := fmt.Sprintf("GET %s?r=%d&key=%s HTTP/1.1\r\nAccept: */*\r\nConnection: close\r\nUser-Agent: %s\r\nHost:%s:%s\r\n\r\n",
					path, time.Now().Unix(), keyNoPrefix, dlUA, ip, port)
				_ = c.SetWriteDeadline(time.Now().Add(15 * time.Second))
				if _, err := io.WriteString(c, req); err != nil {
					c.Close()
					continue
				}
				if !readOKStatus(c) {
					c.Close()
					return
				}
				for ctx.Err() == nil {
					_ = c.SetReadDeadline(time.Now().Add(30 * time.Second))
					n, err := c.Read(buf)
					if n > 0 {
						atomic.AddUint64(appBytes, uint64(n))
					}
					if err != nil {
						break
					}
				}
				c.Close()
			}
		}
	}
}

func CNSpeedUpload(cfg *config.Config, ip, port, keyNoPrefix string) func() worker {
	path := cfg.CNSpeed.Paths["node_upload"]
	return func() worker {
		chunk := make([]byte, 64*1024)
		_, _ = rand.Read(chunk)
		return func(ctx context.Context, appBytes *uint64) {
			for ctx.Err() == nil {
				c, err := net.DialTimeout("tcp", net.JoinHostPort(ip, port), 15*time.Second)
				if err != nil {
					time.Sleep(200 * time.Millisecond)
					continue
				}
				go func(conn net.Conn) { <-ctx.Done(); conn.Close() }(c) // abort a blocked Write on cancel
				head := fmt.Sprintf("POST %s HTTP/1.1\r\nConnection: close\r\nCache-Control: no-cache\r\nCharset: UTF-8\r\nKey: 1-%s\r\nContent-Type: multipart/form-data;boundary=%s\r\nUser-Agent: %s\r\nHost:%s:%s\r\nContent-Length: 900000000\r\n\r\n--%s\r\nContent-Disposition: form-data; name=\"upload\";filename=\"SPEED_%d\"\r\n\r\n",
					path, keyNoPrefix, ulBoundary, ulUA, ip, port, ulBoundary, time.Now().Unix())
				_ = c.SetWriteDeadline(time.Now().Add(90 * time.Second))
				if _, err := io.WriteString(c, head); err != nil {
					c.Close()
					continue
				}
				for ctx.Err() == nil {
					_ = c.SetWriteDeadline(time.Now().Add(30 * time.Second))
					n, err := c.Write(chunk)
					if n > 0 {
						atomic.AddUint64(appBytes, uint64(n))
					}
					if err != nil {
						break
					}
				}
				c.Close()
			}
		}
	}
}

func pickUA(ua string) string {
	if ua == "" {
		return dlUA
	}
	return ua
}

func CDNDownload(client *http.Client, url, ua string) func() worker {
	return func() worker {
		return func(ctx context.Context, appBytes *uint64) {
			buf := make([]byte, 64*1024)
			for ctx.Err() == nil {
				req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
				if err != nil {
					return
				}
				req.Header.Set("User-Agent", pickUA(ua))
				resp, err := client.Do(req)
				if err != nil {
					time.Sleep(200 * time.Millisecond)
					continue
				}
				if resp.StatusCode/100 != 2 {
					resp.Body.Close()
					return
				}
				for ctx.Err() == nil {
					n, err := resp.Body.Read(buf)
					if n > 0 {
						atomic.AddUint64(appBytes, uint64(n))
					}
					if err != nil {
						break
					}
				}
				resp.Body.Close()
			}
		}
	}
}

func CDNUpload(client *http.Client, url, ua string, gotOK *uint32) func() worker {
	return func() worker {
		chunk := make([]byte, 64*1024)
		_, _ = rand.Read(chunk)
		return func(ctx context.Context, appBytes *uint64) {
			for ctx.Err() == nil {
				body := &ctxRandReader{ctx: ctx, chunk: chunk, n: appBytes}
				req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, body)
				if err != nil {
					return
				}
				req.Header.Set("User-Agent", pickUA(ua))
				req.Header.Set("Content-Type", "application/octet-stream")
				resp, err := client.Do(req)
				if err != nil {
					time.Sleep(200 * time.Millisecond)
					continue
				}
				if resp.StatusCode/100 != 2 {
					resp.Body.Close()
					return
				}
				atomic.StoreUint32(gotOK, 1)
				io.Copy(io.Discard, resp.Body)
				resp.Body.Close()
			}
		}
	}
}

type ctxRandReader struct {
	ctx   context.Context
	chunk []byte
	n     *uint64
}

func (r *ctxRandReader) Read(p []byte) (int, error) {
	if r.ctx.Err() != nil {
		return 0, io.EOF
	}
	m := copy(p, r.chunk)
	atomic.AddUint64(r.n, uint64(m))
	return m, nil
}

func readOKStatus(c net.Conn) bool {
	_ = c.SetReadDeadline(time.Now().Add(15 * time.Second))
	buf := make([]byte, 4096)
	n, err := c.Read(buf)
	if n <= 0 || err != nil {
		return false
	}
	line := string(buf[:n])
	return strings.HasPrefix(line, "HTTP/1.1 200") || strings.HasPrefix(line, "HTTP/1.0 200")
}
