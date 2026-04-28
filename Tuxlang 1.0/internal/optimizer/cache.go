package optimizer

import "sync"

// StringIntern optimizes string storage by deduplicating identical strings
type StringIntern struct {
	mu      sync.RWMutex
	intern  map[string]string
	maxSize int
	usage   int
}

// NewStringIntern creates a new string interning pool
func NewStringIntern(maxSize int) *StringIntern {
	return &StringIntern{
		intern:  make(map[string]string),
		maxSize: maxSize,
		usage:   0,
	}
}

// Intern returns a shared instance of the string
func (si *StringIntern) Intern(s string) string {
	si.mu.RLock()
	if interned, ok := si.intern[s]; ok {
		si.mu.RUnlock()
		return interned
	}
	si.mu.RUnlock()

	si.mu.Lock()
	defer si.mu.Unlock()

	// Double-check pattern
	if interned, ok := si.intern[s]; ok {
		return interned
	}

	// Check if we need to make room
	if si.usage+len(s) > si.maxSize {
		// Reset interning pool (could implement LRU in future)
		si.intern = make(map[string]string)
		si.usage = 0
	}

	si.intern[s] = s
	si.usage += len(s)
	return s
}

// ValueCache caches computed values to avoid recomputation
type ValueCache struct {
	mu    sync.RWMutex
	cache map[string]interface{}
}

// NewValueCache creates a new value cache
func NewValueCache() *ValueCache {
	return &ValueCache{
		cache: make(map[string]interface{}),
	}
}

// Get retrieves a cached value
func (vc *ValueCache) Get(key string) (interface{}, bool) {
	vc.mu.RLock()
	defer vc.mu.RUnlock()
	val, ok := vc.cache[key]
	return val, ok
}

// Set caches a value
func (vc *ValueCache) Set(key string, value interface{}) {
	vc.mu.Lock()
	defer vc.mu.Unlock()
	vc.cache[key] = value
}

// Clear clears the cache
func (vc *ValueCache) Clear() {
	vc.mu.Lock()
	defer vc.mu.Unlock()
	vc.cache = make(map[string]interface{})
}

// InstructionStats tracks instruction execution for profiling
type InstructionStats struct {
	mu    sync.Mutex
	stats map[byte]int64
}

// NewInstructionStats creates a new stats tracker
func NewInstructionStats() *InstructionStats {
	return &InstructionStats{
		stats: make(map[byte]int64),
	}
}

// Record increments count for an instruction
func (is *InstructionStats) Record(op byte) {
	is.mu.Lock()
	defer is.mu.Unlock()
	is.stats[op]++
}

// GetStats returns a copy of current stats
func (is *InstructionStats) GetStats() map[byte]int64 {
	is.mu.Lock()
	defer is.mu.Unlock()
	result := make(map[byte]int64)
	for k, v := range is.stats {
		result[k] = v
	}
	return result
}

// MemoryPool preallocates memory for common operations
type MemoryPool struct {
	// Pools for different allocation sizes
	arrayPool  sync.Pool // For []interface{} arrays
	mapPool    sync.Pool // For maps
	stringPool sync.Pool // For string builders
}

// NewMemoryPool creates a new memory pool
func NewMemoryPool() *MemoryPool {
	return &MemoryPool{}
}

// GetArray gets a preallocated array slice
func (mp *MemoryPool) GetArray(capacity int) []interface{} {
	if v := mp.arrayPool.Get(); v != nil {
		arr := v.([]interface{})
		if cap(arr) >= capacity {
			return arr[:0]
		}
	}
	return make([]interface{}, 0, capacity)
}

// PutArray returns an array to the pool
func (mp *MemoryPool) PutArray(arr []interface{}) {
	if cap(arr) > 0 && cap(arr) <= 1024 {
		mp.arrayPool.Put(arr[:0])
	}
}

// GetMap gets a preallocated map
func (mp *MemoryPool) GetMap(capacity int) map[interface{}]interface{} {
	if v := mp.mapPool.Get(); v != nil {
		return v.(map[interface{}]interface{})
	}
	return make(map[interface{}]interface{}, capacity)
}

// PutMap returns a map to the pool
func (mp *MemoryPool) PutMap(m map[interface{}]interface{}) {
	if len(m) == 0 && len(m) <= 256 {
		// Clear and return to pool
		mp.mapPool.Put(m)
	}
}

// CompilationCache caches compiled bytecode to avoid recompilation
type CompilationCache struct {
	mu      sync.RWMutex
	cache   map[string][]byte
	maxSize int
	usage   int
}

// NewCompilationCache creates a new compilation cache
func NewCompilationCache(maxSize int) *CompilationCache {
	return &CompilationCache{
		cache:   make(map[string][]byte),
		maxSize: maxSize,
		usage:   0,
	}
}

// Get retrieves cached bytecode
func (cc *CompilationCache) Get(sourceHash string) ([]byte, bool) {
	cc.mu.RLock()
	defer cc.mu.RUnlock()
	bytecode, ok := cc.cache[sourceHash]
	return bytecode, ok
}

// Set caches compiled bytecode
func (cc *CompilationCache) Set(sourceHash string, bytecode []byte) {
	cc.mu.Lock()
	defer cc.mu.Unlock()

	if cc.usage+len(bytecode) > cc.maxSize {
		// Clear cache if exceeds size
		cc.cache = make(map[string][]byte)
		cc.usage = 0
	}

	cc.cache[sourceHash] = bytecode
	cc.usage += len(bytecode)
}

// Clear clears the cache
func (cc *CompilationCache) Clear() {
	cc.mu.Lock()
	defer cc.mu.Unlock()
	cc.cache = make(map[string][]byte)
	cc.usage = 0
}
