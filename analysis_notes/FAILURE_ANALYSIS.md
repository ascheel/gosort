# GoSort Application Failure Analysis & Improvement Suggestions

## Executive Summary

This document analyzes the GoSort application for potential failure points, with a focus on data loss scenarios. The analysis covers database operations, file handling, concurrency issues, error recovery, and system resilience.

## Critical Data Loss Risks

### 1. **File-Database Inconsistency (CRITICAL)**

**Location:** `cmd/api/api.go:480-509`

**Problem:**
The file is renamed to its final location (`os.Rename`) BEFORE the database insert completes. This creates a window where:
- File exists on disk but is NOT in the database
- If the server crashes between line 481 and 509, the file exists but has no database record
- Database queries won't find the file, leading to duplicate uploads
- No way to track or recover orphaned files

**Current Flow:**
```go
// Line 481: File moved to final location
os.Rename(tmpFilename, newFilename)

// Line 488-496: Race condition check
if engine.DB.ChecksumExists(actualChecksum) { ... }

// Line 499: Database insert (may fail or be buffered)
err = batchInsertBuffer.Add(&media)
```

**Impact:**
- **HIGH**: Files can exist on disk without database records
- **HIGH**: Duplicate detection fails for orphaned files
- **MEDIUM**: Storage bloat from orphaned files

**Recommendation:**
1. **Option A (Preferred)**: Insert into database FIRST, then rename file
   ```go
   // Insert to DB first
   err = batchInsertBuffer.Add(&media)
   if err != nil {
       os.Remove(tmpFilename)
       return err
   }
   // Only rename after successful DB insert
   os.Rename(tmpFilename, newFilename)
   ```

2. **Option B**: Use database transaction with file operation
   - Create a transaction that includes both file metadata and file existence
   - Use a two-phase commit approach

3. **Option C**: Implement a recovery mechanism
   - Periodic scan for files without DB entries
   - Re-index orphaned files on startup

### 2. **Batch Insert Buffer Data Loss (CRITICAL)**

**Location:** `cmd/api/api.go:55-111`, `cmd/api/api.go:668-683`

**Problem:**
Files are buffered in memory before database insertion. If the server crashes:
- Files in the buffer (up to 100 files) are lost
- Files are already on disk but not in database
- No recovery mechanism for buffered entries

**Current Implementation:**
```go
// Files added to buffer
batchInsertBuffer.Add(&media)  // May not flush immediately

// Only flushed on:
// 1. Buffer full (100 files)
// 2. Graceful shutdown (SIGINT)
```

**Impact:**
- **CRITICAL**: Up to 100 files can be lost per crash
- **HIGH**: No recovery for in-flight uploads
- **MEDIUM**: Silent data loss (no error reported to client)

**Recommendation:**
1. **Immediate Flush on File Save**: Flush buffer immediately after each file is saved
   ```go
   // After successful file save
   err = batchInsertBuffer.Add(&media)
   if err == nil {
       // Force flush to ensure DB consistency
       batchInsertBuffer.Flush()
   }
   ```

2. **Persistent Queue**: Use a persistent queue (e.g., SQLite table) instead of in-memory buffer
   - Survives crashes
   - Can be replayed on startup

3. **Write-Ahead Logging**: Implement WAL for SQLite
   - Better crash recovery
   - Reduced risk of corruption

### 3. **Race Condition in Duplicate Detection (HIGH)**

**Location:** `cmd/api/api.go:317-323`, `cmd/api/api.go:487-496`

**Problem:**
Two separate checks for duplicate files create a race condition:
1. Check before queuing (line 319)
2. Check after file save (line 488)

Between these checks, another worker could:
- Process the same file
- Save it to disk
- Create duplicate files

**Current Flow:**
```go
// Check 1: Before queuing
if engine.DB.ChecksumExists(media.Checksum) { return }

// ... file processing ...

// Check 2: After file save
if engine.DB.ChecksumExists(actualChecksum) { 
    os.Remove(newFilename)  // Cleanup
}
```

**Impact:**
- **MEDIUM**: Duplicate files can be created
- **MEDIUM**: Storage waste
- **LOW**: Database constraint violations (if UNIQUE enforced)

**Recommendation:**
1. **Database-Level Locking**: Use SELECT FOR UPDATE or advisory locks
   ```go
   // Lock the checksum row
   tx, _ := db.Begin()
   row := tx.QueryRow("SELECT checksum FROM media WHERE checksum = ? FOR UPDATE", checksum)
   // Process file
   tx.Commit()
   ```

2. **Atomic Insert with ON CONFLICT**: Use SQLite's INSERT OR IGNORE
   ```sql
   INSERT OR IGNORE INTO media (...) VALUES (...)
   ```

3. **Distributed Lock**: Use Redis or file-based locking for multi-server deployments

### 4. **Transaction Rollback Affects Entire Batch (HIGH)**

**Location:** `internal/sortengine/db.go:86-120`

**Problem:**
If one file in a batch fails (e.g., duplicate constraint), the entire batch is rolled back:
- 99 other valid files are also rolled back
- All files must be retried
- No granular error handling

**Current Implementation:**
```go
for _, media := range batch {
    _, err := stmt.Exec(...)
    if err != nil {
        tx.Rollback()  // Entire batch fails
        return err
    }
}
```

**Impact:**
- **MEDIUM**: Valid files are rejected due to one invalid file
- **MEDIUM**: Poor user experience (retry entire batch)
- **LOW**: Performance degradation from retries

**Recommendation:**
1. **Individual Error Handling**: Catch errors per file, continue with others
   ```go
   var failed []*Media
   for _, media := range batch {
       _, err := stmt.Exec(...)
       if err != nil {
           failed = append(failed, media)
           continue
       }
   }
   // Report failed files separately
   ```

2. **Pre-validation**: Check for duplicates before batching
   - Reduces batch failures
   - Better error messages

3. **Smaller Batches**: Reduce batch size for better error isolation

### 5. **Temp File Cleanup on Crash (MEDIUM)**

**Location:** `cmd/api/api.go:373`, `cmd/api/api.go:423-424`

**Problem:**
Temp files (`.download` extension) are created but may not be cleaned up on crash:
- Temp files accumulate over time
- Disk space waste
- No cleanup mechanism

**Current Implementation:**
```go
tmpFilename := fmt.Sprintf("%s.download", newFilename)
// ... file operations ...
// Cleanup only on explicit errors
os.Remove(tmpFilename)
```

**Impact:**
- **MEDIUM**: Disk space waste
- **LOW**: Cluttered filesystem
- **LOW**: Potential confusion

**Recommendation:**
1. **Startup Cleanup**: Scan for `.download` files on startup and remove
   ```go
   func cleanupTempFiles() {
       filepath.Walk(saveDir, func(path string, info os.FileInfo, err error) error {
           if strings.HasSuffix(path, ".download") {
               os.Remove(path)
           }
           return nil
       })
   }
   ```

2. **Periodic Cleanup**: Background goroutine to clean old temp files

3. **Better Temp File Naming**: Include timestamp for easier identification

## Concurrency & Race Conditions

### 6. **TOCTOU (Time-Of-Check-Time-Of-Use) Race Condition (HIGH)**

**Location:** `cmd/api/api.go:319`, `cmd/api/api.go:488`

**Problem:**
The checksum existence check and file insertion are not atomic:
- Check: "Does checksum exist?" → No
- [Another worker inserts same file]
- Insert: "Insert file" → Fails or creates duplicate

**Impact:**
- **HIGH**: Duplicate files can be created
- **MEDIUM**: Database constraint violations

**Recommendation:**
1. **Database Constraints**: Enforce UNIQUE constraint on checksum
   ```sql
   CREATE UNIQUE INDEX idx_checksum_unique ON media(checksum)
   ```

2. **Atomic Operations**: Use database transactions with proper locking

3. **Idempotent Operations**: Make inserts idempotent (INSERT OR IGNORE)

### 7. **Unsafe Mutex Usage in Batch Buffer (MEDIUM)**

**Location:** `cmd/api/api.go:105-110`

**Problem:**
The mutex is unlocked during database operation, allowing concurrent access:
```go
b.mu.Unlock()  // Lock released
err := engine.DB.AddFilesToDBBatch(batch, b.batchSize)
b.mu.Lock()    // Lock re-acquired
```

While this prevents blocking, it allows:
- New files to be added to buffer during DB operation
- Potential buffer corruption if Add() is called during flush

**Impact:**
- **LOW**: Potential buffer corruption (rare)
- **LOW**: Race conditions in buffer management

**Recommendation:**
1. **Keep Lock During Operation**: Only release if operation is truly blocking
2. **Use Channel Instead**: Use channels for thread-safe buffering
3. **Separate Flush Goroutine**: Dedicated goroutine for flushing

## Error Handling & Recovery

### 8. **Silent File Removal Failures (MEDIUM)**

**Location:** `cmd/api/api.go:489-492`, `cmd/api/api.go:501-504`

**Problem:**
File removal errors are logged but not handled:
```go
err2 := os.Remove(newFilename)
if err2 != nil {
    fmt.Printf("Error removing file: %s\n", err2.Error())
    // Error ignored - file remains on disk
}
```

**Impact:**
- **MEDIUM**: Orphaned files remain on disk
- **LOW**: Storage waste
- **LOW**: Potential security issue (unintended file exposure)

**Recommendation:**
1. **Retry Logic**: Implement retry with exponential backoff
2. **Alerting**: Log to monitoring system for manual intervention
3. **Background Cleanup**: Periodic job to clean orphaned files

### 9. **No Database Connection Error Recovery (MEDIUM)**

**Location:** `internal/sortengine/db.go:186-190`

**Problem:**
Database connection errors are not retried:
- Network issues cause immediate failure
- No connection pooling recovery
- No retry mechanism

**Impact:**
- **MEDIUM**: Temporary network issues cause permanent failures
- **LOW**: Poor resilience

**Recommendation:**
1. **Connection Retry**: Implement retry logic with exponential backoff
2. **Health Checks**: Periodic connection health checks
3. **Circuit Breaker**: Prevent cascading failures

### 10. **No WAL Mode for SQLite (MEDIUM)**

**Location:** `internal/sortengine/db.go:186`

**Problem:**
SQLite is not configured with WAL (Write-Ahead Logging) mode:
- Slower crash recovery
- Higher risk of corruption
- Poor concurrent read performance

**Impact:**
- **MEDIUM**: Slower crash recovery
- **MEDIUM**: Higher corruption risk
- **LOW**: Reduced concurrent read performance

**Recommendation:**
1. **Enable WAL Mode**: 
   ```go
   d.db.Exec("PRAGMA journal_mode=WAL")
   d.db.Exec("PRAGMA synchronous=NORMAL")  // Better performance, still safe
   ```

2. **Configure Checkpointing**: Automatic WAL checkpointing

## Resource Management

### 11. **Potential File Handle Leaks (LOW)**

**Location:** `cmd/api/api.go:379-394`

**Problem:**
File handles are properly closed with `defer`, but errors in the middle could leave handles open:
- Early returns may not execute all defers
- Context cancellation may interrupt cleanup

**Impact:**
- **LOW**: File handle exhaustion (rare with proper defers)
- **LOW**: Resource leaks

**Recommendation:**
1. **Ensure Defers Execute**: Use context with proper cancellation
2. **Resource Pooling**: Use connection/file pools
3. **Monitoring**: Track open file handles

### 12. **Database Connection Pool Exhaustion (LOW)**

**Location:** `internal/sortengine/db.go:195-197`

**Problem:**
Connection pool settings may be insufficient under high load:
```go
d.db.SetMaxOpenConns(25)  // May be too low for high concurrency
d.db.SetMaxIdleConns(5)    // May be too low
```

**Impact:**
- **LOW**: Connection pool exhaustion under high load
- **LOW**: Performance degradation

**Recommendation:**
1. **Tune Pool Settings**: Adjust based on load testing
2. **Monitor Connections**: Track connection pool usage
3. **Dynamic Scaling**: Adjust pool size based on load

## System Resilience

### 13. **No Graceful Shutdown for In-Flight Requests (MEDIUM)**

**Location:** `cmd/api/api.go:668-683`

**Problem:**
On shutdown (SIGINT), the server:
- Stops accepting new requests
- Flushes batch buffer
- But doesn't wait for in-flight uploads to complete

**Impact:**
- **MEDIUM**: In-flight uploads may be interrupted
- **MEDIUM**: Partial file writes
- **LOW**: Client errors

**Recommendation:**
1. **Graceful Shutdown**: Wait for in-flight requests to complete
   ```go
   // Stop accepting new requests
   server.Shutdown(ctx)
   // Wait for upload queue to drain
   uploadQueue.Shutdown()
   // Flush batch buffer
   batchInsertBuffer.Flush()
   ```

2. **Request Timeout**: Set reasonable timeouts for in-flight requests
3. **Status Endpoint**: Expose shutdown status for monitoring

### 14. **No Health Check Endpoint (LOW)**

**Problem:**
No health check endpoint for monitoring:
- Can't detect if server is healthy
- Load balancers can't route traffic properly
- No way to check database connectivity

**Impact:**
- **LOW**: Poor observability
- **LOW**: Difficult to monitor

**Recommendation:**
1. **Health Endpoint**: `/health` endpoint that checks:
   - Database connectivity
   - Disk space
   - Memory usage

2. **Readiness Endpoint**: `/ready` for startup checks
3. **Metrics Endpoint**: `/metrics` for Prometheus

## Security Concerns

### 15. **No Input Validation on File Paths (MEDIUM)**

**Location:** `internal/sortengine/engine.go:57-88`

**Problem:**
File paths are constructed from user input without validation:
- Path traversal attacks possible
- No sanitization of filenames
- No size limits

**Impact:**
- **MEDIUM**: Path traversal attacks
- **MEDIUM**: File system access outside intended directory
- **LOW**: Storage exhaustion

**Recommendation:**
1. **Path Validation**: Ensure paths stay within saveDir
   ```go
   absPath, _ := filepath.Abs(newFilename)
   absSaveDir, _ := filepath.Abs(saveDir)
   if !strings.HasPrefix(absPath, absSaveDir) {
       return error("path traversal detected")
   }
   ```

2. **Filename Sanitization**: Remove dangerous characters
3. **Size Limits**: Enforce maximum file sizes

## Performance Issues

### 16. **Inefficient Duplicate Check (LOW)**

**Location:** `cmd/api/api.go:319`, `cmd/api/api.go:488`

**Problem:**
Duplicate checks are performed twice:
- Once before queuing
- Once after file save

This doubles database queries for each file.

**Impact:**
- **LOW**: Unnecessary database load
- **LOW**: Slight performance impact

**Recommendation:**
1. **Single Check**: Only check after file save (before DB insert)
2. **Database Constraints**: Rely on UNIQUE constraint for final check
3. **Caching**: Cache recent checksums to reduce DB queries

## Summary of Recommendations by Priority

### Critical (Fix Immediately)
1. ✅ **File-Database Inconsistency**: Insert to DB before renaming file
2. ✅ **Batch Buffer Data Loss**: Flush buffer immediately or use persistent queue
3. ✅ **Race Condition in Duplicate Detection**: Use database-level locking

### High Priority
4. ✅ **Transaction Rollback**: Individual error handling in batches
5. ✅ **TOCTOU Race Condition**: Atomic operations with database constraints
6. ✅ **Temp File Cleanup**: Startup cleanup mechanism

### Medium Priority
7. ✅ **Silent File Removal Failures**: Retry logic and alerting
8. ✅ **Database Connection Recovery**: Retry mechanism
9. ✅ **WAL Mode**: Enable SQLite WAL mode
10. ✅ **Graceful Shutdown**: Wait for in-flight requests
11. ✅ **Input Validation**: Path traversal protection

### Low Priority
12. ✅ **File Handle Leaks**: Ensure proper cleanup
13. ✅ **Connection Pool Tuning**: Adjust based on load
14. ✅ **Health Check Endpoint**: Add monitoring endpoints
15. ✅ **Inefficient Duplicate Check**: Optimize query pattern

## Testing Recommendations

1. **Crash Testing**: Simulate crashes during file uploads
2. **Concurrency Testing**: Test with multiple simultaneous uploads of same file
3. **Recovery Testing**: Verify recovery after crashes
4. **Load Testing**: Test under high concurrent load
5. **Failure Injection**: Test database failures, disk full, etc.

## Monitoring Recommendations

1. **File-Database Consistency**: Periodic scan for orphaned files
2. **Batch Buffer Metrics**: Track buffer size and flush frequency
3. **Error Rates**: Monitor error rates by type
4. **Database Health**: Monitor connection pool, query performance
5. **Disk Space**: Monitor available disk space
6. **Upload Success Rate**: Track successful vs failed uploads

