# Batch Checksum Optimization Implementation

## Problem (Issue #2)

The original implementation made **individual HTTP requests** for each file to check if it already exists on the server:

- For each file: 1 request for `checksum100k` + 1 request for full `checksum` = **2 requests per file**
- For 1000 files: **2000 HTTP requests** just for duplicate checking
- Each request has network latency overhead (typically 10-50ms per request)
- Total time: 1000 files × 2 requests × 50ms = **100 seconds** just for checking duplicates

## Solution: Batch Checksum Checking

The new implementation uses a **three-phase approach**:

### Phase 1: Collect and Calculate Checksums (Parallel)
- Walk directory and collect all files
- Calculate checksums for all files in parallel using worker goroutines
- Store all files with their checksums

**Time**: Parallel processing, scales with number of workers

### Phase 2: Batch Check All Checksums (Minimal HTTP Requests)
- Collect all checksums into batches (100 at a time)
- Make **2 batch HTTP requests** total (one for checksum100k, one for full checksum)
- For 1000 files: **~20 HTTP requests** (10 batches × 2 types) instead of 2000

**Time**: ~20 requests × 50ms = **1 second** (vs. 100 seconds before)

### Phase 3: Upload Only New Files (Parallel)
- Compare batch results to determine which files already exist
- Upload only files that don't exist, using parallel workers

**Time**: Parallel processing, only uploads necessary files

## Performance Improvement

### Before:
- **2000 HTTP requests** for 1000 files
- **~100 seconds** just for duplicate checking
- Sequential processing of checks

### After:
- **~20 HTTP requests** for 1000 files (100x reduction!)
- **~1 second** for duplicate checking (100x faster!)
- Parallel processing throughout

### Overall Impact:
- **50-100x reduction in HTTP requests**
- **Dramatically faster duplicate detection**
- Better network efficiency
- Reduced server load

## Implementation Details

### Key Functions

#### `BatchCheckChecksums()`
```go
func (c *Client) BatchCheckChecksums(checksums []string, endpoint string) (map[string]bool, error)
```
- Takes a list of checksums and checks them all in one HTTP request
- Uses existing `/checksums` and `/checksum100k` batch endpoints
- Returns a map of checksum → exists boolean

#### `ProcessDirectory()` - Refactored
Now uses three phases:
1. **Collection Phase**: Parallel checksum calculation
2. **Batch Check Phase**: Minimal HTTP requests
3. **Upload Phase**: Parallel uploads of only new files

### Batch Size

The implementation uses a batch size of **100 checksums per request**:
- Balances request size vs. number of requests
- Prevents huge HTTP requests that might timeout
- Can be adjusted if needed (currently hardcoded to 100)

### Error Handling

- If batch check fails, falls back gracefully
- Individual file errors don't stop the entire process
- Statistics track all operations

## Code Flow

```
1. Walk Directory
   ↓
2. For each file (parallel):
   - Calculate checksum
   - Calculate checksum100k
   ↓
3. Collect all checksums
   ↓
4. Batch check checksum100k (in groups of 100)
   ↓
5. Batch check full checksum (in groups of 100)
   ↓
6. For each file:
   - If both checksums exist → Skip
   - Otherwise → Upload (parallel)
```

## Example: 1000 Files

### Old Approach:
```
File 1: Check 100k → Check full → Upload (if needed)
File 2: Check 100k → Check full → Upload (if needed)
...
File 1000: Check 100k → Check full → Upload (if needed)

Total: 2000 HTTP requests
```

### New Approach:
```
Phase 1: Calculate all checksums (parallel, ~10 workers)
Phase 2: 
  - Batch check 100k: 10 requests (100 checksums each)
  - Batch check full: 10 requests (100 checksums each)
Phase 3: Upload only new files (parallel, ~10 workers)

Total: 20 HTTP requests
```

## Benefits

1. **Massive Reduction in HTTP Requests**: 50-100x fewer requests
2. **Faster Processing**: Network latency reduced dramatically
3. **Better Server Performance**: Fewer requests = less server load
4. **Scalability**: Performance improvement increases with more files
5. **Network Efficiency**: Better use of bandwidth and connections

## Configuration

The batch size is currently hardcoded to 100. This can be made configurable if needed:

```go
batchSize := 100  // Could be: flag.Int("batch-size", 100, "Checksums per batch request")
```

## Testing Recommendations

1. Test with various file counts (10, 100, 1000, 10000)
2. Monitor HTTP request count (should be ~2% of file count)
3. Verify duplicate detection accuracy
4. Test error handling (network failures, server errors)
5. Measure total processing time improvement

## Future Enhancements

1. **Configurable Batch Size**: Allow users to adjust batch size
2. **Adaptive Batching**: Adjust batch size based on network conditions
3. **Retry Logic**: Retry failed batch requests
4. **Progress Reporting**: Show progress during batch checking phase
5. **Caching**: Cache batch results to avoid re-checking

## Summary

This optimization transforms the duplicate checking process from making **2 requests per file** to making **~2 requests per 100 files**, resulting in a **50-100x reduction in HTTP requests** and dramatically faster processing times. The implementation maintains all existing functionality while providing massive performance improvements.

