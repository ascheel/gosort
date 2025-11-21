# Rate Limiting & Request Queue Optimization Implementation

## Problem (Issue #9)

The original API implementation had **no rate limiting or request queuing**:

- All upload requests were processed immediately and synchronously
- Server could be overwhelmed by many concurrent uploads
- No graceful degradation under load
- No protection against request floods
- Risk of resource exhaustion (memory, file handles, database connections)

**Impact:**
- Server could become unresponsive under high load
- No way to control throughput
- Potential for denial of service
- Poor user experience during peak times

## Solution: Rate Limiting & Worker Pool Queue

The new implementation adds:

1. **Token Bucket Rate Limiter**: Controls requests per second
2. **Worker Pool**: Processes uploads concurrently with configurable worker count
3. **Request Queue**: Buffers requests when workers are busy
4. **Graceful Degradation**: Returns appropriate HTTP status codes when overloaded

**Result:** Server can handle high load gracefully with controlled throughput and resource usage.

## Performance Improvement

### Before:
- **All requests processed immediately** (no queuing)
- **No rate limiting** (unlimited concurrent uploads)
- **Risk of resource exhaustion** under high load
- **No graceful degradation** (server could crash)

### After:
- **Configurable worker pool** (default: 10 workers)
- **Rate limiting** (default: 50 requests/second)
- **Request queue** buffers excess requests
- **Graceful degradation** with HTTP 503 (Service Unavailable) when queue is full
- **HTTP 429 (Too Many Requests)** when rate limit exceeded

### Result:
- **Better stability** under load
- **Controlled resource usage** (memory, file handles, database connections)
- **Predictable throughput** (rate limiting)
- **Graceful handling** of overload conditions

## Implementation Details

### 1. Token Bucket Rate Limiter

The rate limiter uses a **token bucket algorithm**:

```go
type RateLimiter struct {
    tokens       chan struct{}
    refillTicker *time.Ticker
    rate         int // Requests per second
    capacity     int // Maximum tokens (burst capacity)
}
```

**How it works:**
- Tokens are added to the bucket at a fixed rate (e.g., 50 per second)
- Each request consumes one token
- If no token is available, the request is rate-limited
- Burst capacity allows handling short spikes (2x the rate)

**Benefits:**
- Smooth rate limiting (not just hard caps)
- Allows short bursts while maintaining average rate
- Thread-safe using channels

### 2. Upload Queue with Worker Pool

The upload queue manages a pool of worker goroutines:

```go
type UploadQueue struct {
    queue       chan UploadRequest
    workers     int
    wg          sync.WaitGroup
    rateLimiter *RateLimiter
}
```

**How it works:**
- Requests are enqueued when received
- Worker pool processes requests concurrently
- Rate limiter controls processing speed
- Queue buffers requests when workers are busy

**Benefits:**
- Controlled concurrency (prevents resource exhaustion)
- Better resource utilization (workers process in parallel)
- Graceful handling of queue overflow

### 3. Request Flow

**Before (Synchronous):**
```
Client Request → pushFile() → Process Upload → Response
```

**After (Asynchronous with Queue):**
```
Client Request → pushFile() → Enqueue → Response (queued)
                                    ↓
                            Worker Pool → Rate Limiter → Process Upload → Response
```

**Key Changes:**
1. `pushFile()` validates and enqueues requests (fast response)
2. Worker pool processes requests asynchronously
3. Rate limiter controls throughput
4. Responses are sent by workers after processing

### 4. Command-Line Configuration

New flags for controlling rate limiting and worker pool:

```bash
-upload-workers int
    Number of concurrent upload workers (default: 10)

-rate-limit int
    Maximum uploads per second (rate limiting) (default: 50)
```

**Example:**
```bash
# High-throughput server (20 workers, 100 req/sec)
./api -upload-workers 20 -rate-limit 100

# Low-resource server (5 workers, 10 req/sec)
./api -upload-workers 5 -rate-limit 10
```

### 5. HTTP Status Codes

The implementation returns appropriate status codes:

- **200 OK**: Upload successful
- **409 Conflict**: File already exists (duplicate)
- **400 Bad Request**: Invalid request (missing file, bad JSON, etc.)
- **429 Too Many Requests**: Rate limit exceeded
- **503 Service Unavailable**: Queue is full (server overloaded)

### 6. Graceful Shutdown

The upload queue supports graceful shutdown:

```go
// On SIGINT, shutdown queue gracefully
uploadQueue.Shutdown()
```

**Process:**
1. Stop accepting new requests (close queue channel)
2. Wait for workers to finish processing current requests
3. Stop rate limiter ticker
4. Exit cleanly

## Configuration Recommendations

### Small Server (Low Resources)
```bash
-upload-workers 5
-rate-limit 10
```
- **5 workers**: Limited concurrency
- **10 req/sec**: Low throughput
- **Good for**: Development, low-traffic servers

### Medium Server (Default)
```bash
-upload-workers 10
-rate-limit 50
```
- **10 workers**: Moderate concurrency
- **50 req/sec**: Medium throughput
- **Good for**: Most production deployments

### Large Server (High Resources)
```bash
-upload-workers 20
-rate-limit 100
```
- **20 workers**: High concurrency
- **100 req/sec**: High throughput
- **Good for**: High-traffic servers with good hardware

### Considerations:
- **Workers**: Should match CPU cores (or 2x for I/O-bound workloads)
- **Rate Limit**: Should match disk I/O capacity and database performance
- **Queue Size**: Automatically set to `workers * 2` for buffering

## Testing Rate Limiting

To test rate limiting, you can use a tool like `ab` (Apache Bench):

```bash
# Send 1000 requests with 100 concurrent connections
ab -n 1000 -c 100 -p testfile.json -T multipart/form-data http://localhost:8080/file
```

**Expected behavior:**
- First requests succeed (within rate limit)
- Some requests get 429 (Too Many Requests) when rate limit exceeded
- Some requests get 503 (Service Unavailable) when queue is full
- Server remains stable and responsive

## Benefits Summary

1. **Stability**: Server won't crash under high load
2. **Resource Control**: Predictable memory and file handle usage
3. **Throughput Control**: Configurable requests per second
4. **Graceful Degradation**: Appropriate error responses when overloaded
5. **Scalability**: Can tune for different server sizes
6. **Production Ready**: Handles real-world load patterns

## Future Enhancements

Potential improvements:
- **Per-client rate limiting**: Limit individual clients
- **Adaptive rate limiting**: Adjust based on server load
- **Priority queue**: Prioritize certain requests
- **Metrics**: Track queue depth, rate limit hits, etc.
- **Health endpoint**: Report queue status and rate limit status

