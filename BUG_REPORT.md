# Bug Report for mcp-space-browser

## Critical Security Vulnerabilities

### 1. SQL Injection Vulnerability in database.go:838
**Severity**: CRITICAL
**File**: `pkg/database/database.go`
**Line**: 838

**Description**: The `sortBy` field is directly interpolated into an SQL query without validation or sanitization, creating a SQL injection vulnerability.

```go
query += fmt.Sprintf(" ORDER BY %s %s", sortBy, sortOrder)
```

**Impact**: An attacker could inject malicious SQL code through the `sortBy` parameter, potentially:
- Reading arbitrary data from the database
- Modifying or deleting database records
- Executing arbitrary SQL commands

**Exploitation Example**:
```go
sortBy = "size; DROP TABLE entries--"
```

**Fix**: Use a whitelist to validate the sortBy field:
```go
// Validate sortBy field
validSortFields := map[string]bool{
    "size": true,
    "name": true,
    "mtime": true,
    "ctime": true,
    "path": true,
}

if !validSortFields[sortBy] {
    return nil, fmt.Errorf("invalid sort field: %s", sortBy)
}

query += fmt.Sprintf(" ORDER BY %s %s", sortBy, sortOrder)
```

---

### 2. Command Injection Vulnerability in inspect_artifacts.go
**Severity**: CRITICAL
**File**: `pkg/server/inspect_artifacts.go`
**Lines**: 368, 378, 381

**Description**: User-controlled file paths are passed directly to `exec.Command` for ffmpeg without proper sanitization or validation.

```go
// Line 368
cmd := exec.Command("ffmpeg", "-y", "-i", inputPath, "-vf", "thumbnail,scale=320:-1", "-frames:v", strconv.Itoa(frameCount), outputPath)

// Line 381
cmd := exec.Command("ffmpeg", "-y", "-i", inputPath, "-vf", selectFilter, "-frames:v", strconv.Itoa(total), pattern)
```

**Impact**: An attacker could craft malicious file paths containing shell metacharacters or command injection sequences, potentially:
- Executing arbitrary commands on the server
- Reading sensitive files
- Compromising the entire system

**Exploitation Example**:
A path like: `/tmp/file.mp4; rm -rf /; #` could lead to command injection

**Fix**: Validate and sanitize file paths before passing to external commands:
```go
func sanitizePath(path string) error {
    // Ensure path is absolute
    if !filepath.IsAbs(path) {
        return fmt.Errorf("path must be absolute")
    }

    // Check for suspicious characters
    if strings.ContainsAny(path, ";|&$`<>()") {
        return fmt.Errorf("path contains invalid characters")
    }

    // Verify file exists
    if _, err := os.Stat(path); err != nil {
        return fmt.Errorf("file does not exist: %w", err)
    }

    return nil
}

// Before calling exec.Command:
if err := sanitizePath(inputPath); err != nil {
    return err
}
```

---

## High Priority Bugs

### 3. Invalid SQL Syntax in database.go:779
**Severity**: HIGH
**File**: `pkg/database/database.go`
**Line**: 779

**Description**: The code attempts to use `LIKE ANY()` syntax, which is not valid in SQLite. SQLite doesn't support the `ANY()` operator.

```go
query += " AND path LIKE ANY(" + strings.Join(placeholders, ",") + ")"
```

**Impact**: This code will cause a SQL syntax error at runtime when filtering by file extensions, breaking the functionality.

**Error Message**:
```
SQL logic error: near "ANY": syntax error
```

**Fix**: Use multiple OR conditions or UNION for extension filtering:
```go
if filter.Extensions != nil && len(filter.Extensions) > 0 {
    conditions := make([]string, len(filter.Extensions))
    for i, ext := range filter.Extensions {
        conditions[i] = "path LIKE ?"
        args = append(args, "%."+ext)
    }
    query += " AND (" + strings.Join(conditions, " OR ") + ")"
}
```

---

### 4. Stack Overflow Risk in buildTree Function
**Severity**: HIGH
**File**: `pkg/server/server.go`
**Line**: 263

**Description**: The `buildTree` function is recursive without depth limits, which could cause stack overflow on deeply nested directory structures.

```go
func buildTree(db *database.DiskDB, root string, depth int) (*treeNode, error) {
    // ... no depth limit check ...
    for _, child := range children {
        childNode, err := buildTree(db, child.Path, depth+1)
        // ...
    }
}
```

**Impact**: Processing deeply nested directories could cause:
- Stack overflow panics
- Server crashes
- Denial of service

**Fix**: Add a maximum depth parameter and check:
```go
const maxTreeDepth = 100

func buildTree(db *database.DiskDB, root string, depth int) (*treeNode, error) {
    if depth > maxTreeDepth {
        return nil, fmt.Errorf("maximum tree depth exceeded")
    }

    // ... rest of function ...
}
```

**Note**: The codebase already has `GetTreeWithOptions` which includes depth limiting, so consider removing the vulnerable `buildTree` function or refactoring it to use the safer implementation.

---

## Medium Priority Issues

### 5. Missing Error Handling in file.Read
**Severity**: MEDIUM
**File**: `pkg/server/inspect_artifacts.go`
**Line**: 245

**Description**: The error from `file.Read()` is ignored, which could lead to incorrect MIME type detection.

```go
n, _ := file.Read(buf)
mimeType := http.DetectContentType(buf[:n])
```

**Impact**: If the read fails, `n` would be 0, and an empty buffer would be used for MIME type detection, potentially returning incorrect content types.

**Fix**:
```go
n, err := file.Read(buf)
if err != nil && err != io.EOF {
    c.String(http.StatusInternalServerError, "failed to read file")
    return
}
mimeType := http.DetectContentType(buf[:n])
```

---

### 6. Race Condition in Content Token Secret Initialization
**Severity**: MEDIUM
**File**: `pkg/server/inspect_artifacts.go`
**Lines**: 64-78

**Description**: The `initContentTokenSecret()` function checks `contentTokenSecret` for nil without synchronization, creating a potential race condition.

```go
func initContentTokenSecret() {
    if contentTokenSecret != nil {
        return
    }
    // Initialize secret
}
```

**Impact**: In a concurrent environment, multiple goroutines could initialize the secret simultaneously, potentially causing:
- Multiple cache directories
- Inconsistent token validation
- Panic from concurrent map writes

**Fix**: Use `sync.Once` for thread-safe initialization:
```go
var (
    contentTokenSecret []byte
    contentBaseURL     string
    initOnce           sync.Once
)

func initContentTokenSecret() {
    initOnce.Do(func() {
        secret := make([]byte, 32)
        if _, err := rand.Read(secret); err != nil {
            panic(fmt.Errorf("failed to initialize content token secret: %w", err))
        }
        contentTokenSecret = secret

        if err := os.MkdirAll(artifactCacheDir, 0o755); err != nil {
            panic(fmt.Errorf("failed to create artifact cache: %w", err))
        }
    })
}
```

---

### 7. Integer Type Inconsistency
**Severity**: MEDIUM
**File**: `pkg/database/jobs.go`
**Lines**: 26-30

**Description**: `IndexJobMetadata` uses `int` types for counts, but these come from `atomic.Int64` operations in `parallel.go`, requiring explicit type conversions that could overflow on 32-bit systems.

```go
// In jobs.go
type IndexJobMetadata struct {
    FilesProcessed       int   `json:"filesProcessed"`
    DirectoriesProcessed int   `json:"directoriesProcessed"`
    // ...
}

// In parallel.go
filesProcessed := indexer.filesProcessed.Load() // returns int64
// ...
FilesProcessed: int(filesProcessed), // potential overflow on 32-bit
```

**Impact**: On 32-bit systems, very large values (>2^31) could overflow, causing:
- Incorrect progress reporting
- Negative count values
- Data corruption in job metadata

**Fix**: Use `int64` consistently:
```go
type IndexJobMetadata struct {
    FilesProcessed       int64 `json:"filesProcessed"`
    DirectoriesProcessed int64 `json:"directoriesProcessed"`
    TotalSize            int64 `json:"totalSize"`
    ErrorCount           int64 `json:"errorCount"`
    WorkerCount          int   `json:"workerCount"`
}
```

---

## Low Priority Issues

### 8. Potential Path Traversal in Artifact Cache
**Severity**: LOW
**File**: `pkg/server/inspect_artifacts.go`
**Line**: 196-211

**Description**: While the hash-based approach provides some protection, there's no explicit validation that the resulting path stays within the intended cache directory.

**Impact**: Minimal, as the path is generated from a hash, but could be an issue if the hash function is compromised.

**Fix**: Add explicit path validation:
```go
func artifactCachePath(hashKey, filename string) (string, error) {
    if len(hashKey) < 4 {
        return "", fmt.Errorf("invalid hash key for artifact cache")
    }

    dir := filepath.Join(artifactCacheDir, hashKey[:2], hashKey[2:4], hashKey)
    if err := os.MkdirAll(dir, 0o755); err != nil {
        return "", err
    }

    resultPath := filepath.Join(dir, filename)

    // Ensure the result is within artifactCacheDir
    if !strings.HasPrefix(resultPath, artifactCacheDir) {
        return "", fmt.Errorf("invalid cache path")
    }

    return resultPath, nil
}
```

---

### 9. Missing Validation for Selection Set Names
**Severity**: LOW
**File**: `pkg/database/database.go`
**Lines**: 936-979

**Description**: Selection set names and query names are used directly in database operations without validation for special characters or SQL keywords.

**Impact**: While prepared statements protect against SQL injection, invalid names could cause:
- User confusion
- Difficult-to-debug errors
- Database constraint violations

**Fix**: Add name validation:
```go
func validateName(name string) error {
    if name == "" {
        return fmt.Errorf("name cannot be empty")
    }
    if len(name) > 255 {
        return fmt.Errorf("name too long (max 255 characters)")
    }
    // Disallow special characters that could cause issues
    if strings.ContainsAny(name, "\x00\n\r\t") {
        return fmt.Errorf("name contains invalid characters")
    }
    return nil
}
```

---

### 10. Incomplete Error Handling in Parallel Indexer
**Severity**: LOW
**File**: `pkg/crawler/parallel.go`
**Line**: 438-442

**Description**: When `Submit()` fails, the code falls back to synchronous execution but doesn't distinguish between "queue full" and "pool shutting down" errors.

```go
if err := j.indexer.pool.Submit(childJob); err != nil {
    // If queue is full or pool is shutting down, process synchronously
    if err := childJob.Execute(ctx); err != nil {
        log.WithError(err).Error("Failed to process child job")
    }
}
```

**Impact**: During shutdown, new jobs shouldn't be processed at all, but the current code will process them synchronously.

**Fix**: Check the error type:
```go
if err := j.indexer.pool.Submit(childJob); err != nil {
    // Check if we should stop processing
    select {
    case <-ctx.Done():
        return ctx.Err()
    default:
    }

    // Only fall back to sync if queue is full, not if shutting down
    if errors.Is(err, queue.ErrPoolShutdown) {
        return err
    }

    // Queue full, process synchronously
    if err := childJob.Execute(ctx); err != nil {
        log.WithError(err).Error("Failed to process child job")
    }
}
```

---

## Summary

### By Severity:
- **Critical**: 2 vulnerabilities (SQL injection, Command injection)
- **High**: 2 bugs (Invalid SQL syntax, Stack overflow risk)
- **Medium**: 3 issues (Missing error handling, Race condition, Type inconsistency)
- **Low**: 3 issues (Path traversal, Name validation, Error handling)

### Recommended Priority:
1. **Immediate**: Fix critical security vulnerabilities (#1, #2)
2. **High Priority**: Fix SQL syntax error and stack overflow (#3, #4)
3. **Medium Priority**: Address race condition and error handling (#5, #6, #7)
4. **Low Priority**: Improve validation and error handling (#8, #9, #10)

### Testing Recommendations:
- Add fuzzing tests for SQL query construction
- Add tests for path sanitization with malicious inputs
- Add stress tests for deeply nested directories
- Add concurrent access tests for initialization code
- Add tests for edge cases (large numbers, 32-bit overflow)
