package internal

import (
	"sync"
)

// SecureBuffer is a byte buffer that zeros its contents when released.
// This prevents sensitive data from remaining in memory after use.
// Use for intermediate results during sensitive data processing.
type SecureBuffer struct {
	data []byte
}

// secureBufferPool is a pool for reusing secure buffers.
var secureBufferPool = sync.Pool{
	New: func() any {
		return &SecureBuffer{
			data: make([]byte, 0, 1024),
		}
	},
}

// NewSecureBuffer creates a new SecureBuffer from the pool.
// Remember to call Release when done to zero and return the buffer.
func NewSecureBuffer() *SecureBuffer {
	return secureBufferPool.Get().(*SecureBuffer)
}

// Write appends data to the buffer.
func (sb *SecureBuffer) Write(p []byte) (int, error) {
	if sb == nil {
		return 0, nil
	}
	sb.data = append(sb.data, p...)
	return len(p), nil
}

// WriteString appends a string to the buffer.
func (sb *SecureBuffer) WriteString(s string) (int, error) {
	if sb == nil {
		return 0, nil
	}
	sb.data = append(sb.data, s...)
	return len(s), nil
}

// Bytes returns the buffer contents.
// IMPORTANT: The returned slice is valid only until Release is called.
func (sb *SecureBuffer) Bytes() []byte {
	if sb == nil {
		return nil
	}
	return sb.data
}

// String returns the buffer contents as a string.
func (sb *SecureBuffer) String() string {
	if sb == nil {
		return ""
	}
	return string(sb.data)
}

// Len returns the current length of the buffer.
func (sb *SecureBuffer) Len() int {
	if sb == nil {
		return 0
	}
	return len(sb.data)
}

// Cap returns the capacity of the buffer.
func (sb *SecureBuffer) Cap() int {
	if sb == nil {
		return 0
	}
	return cap(sb.data)
}

// Reset clears the buffer (zeros data) and prepares for reuse.
func (sb *SecureBuffer) Reset() {
	if sb == nil {
		return
	}
	// Zero the data before resetting
	for i := range sb.data {
		sb.data[i] = 0
	}
	sb.data = sb.data[:0]
}

// Release zeros the buffer and returns it to the pool.
// After calling Release, the buffer must not be used.
//
// IMPORTANT: Any slices obtained from Bytes() become invalid after Release.
// Do not store or use references to the internal buffer after calling Release.
func (sb *SecureBuffer) Release() {
	if sb == nil {
		return
	}

	// Zero all data
	for i := range sb.data {
		sb.data[i] = 0
	}

	// Reset slice length but keep capacity
	sb.data = sb.data[:0]

	// Return to pool
	secureBufferPool.Put(sb)
}

// Grow grows the buffer capacity to guarantee space for n more bytes.
//
// SECURITY: Fully zeros old data including capacity portion before reallocation
// to prevent sensitive data from remaining in memory.
//
// IMPORTANT: After calling Grow, any slices previously obtained from Bytes()
// may point to old memory that has been zeroed. Do not retain references
// to the internal buffer across Grow calls.
func (sb *SecureBuffer) Grow(n int) {
	if sb == nil {
		return
	}
	if cap(sb.data)-len(sb.data) < n {
		// Need to grow
		newCap := cap(sb.data) * 2
		if newCap < len(sb.data)+n {
			newCap = len(sb.data) + n
		}
		newData := make([]byte, len(sb.data), newCap)
		copy(newData, sb.data)
		// SECURITY: Zero ALL old data including capacity portion
		// This ensures no sensitive data remains in the old slice.
		// Use slice expression to access full capacity for zeroing.
		for i := range sb.data[:cap(sb.data)] {
			sb.data[i] = 0
		}
		sb.data = newData
	}
}

// SecureString is a string wrapper that can be explicitly cleared.
// Unlike regular strings which are immutable and may remain in memory
// until garbage collection, SecureString can be cleared immediately.
type SecureString struct {
	data []byte
}

// NewSecureString creates a new SecureString from a regular string.
func NewSecureString(s string) *SecureString {
	data := make([]byte, len(s))
	copy(data, s)
	return &SecureString{data: data}
}

// String returns the string value.
func (ss *SecureString) String() string {
	if ss == nil {
		return ""
	}
	return string(ss.data)
}

// Bytes returns the underlying bytes.
func (ss *SecureString) Bytes() []byte {
	if ss == nil {
		return nil
	}
	return ss.data
}

// Len returns the length of the string.
func (ss *SecureString) Len() int {
	if ss == nil {
		return 0
	}
	return len(ss.data)
}

// Clear zeros the string data and releases it.
// After calling Clear, the SecureString must not be used.
func (ss *SecureString) Clear() {
	if ss == nil {
		return
	}

	// Zero the data
	for i := range ss.data {
		ss.data[i] = 0
	}

	// Clear the slice reference
	ss.data = nil
}

// Equals compares the SecureString with another string in constant time.
// This prevents timing attacks when comparing sensitive values.
func (ss *SecureString) Equals(s string) bool {
	if ss == nil || len(ss.data) != len(s) {
		return false
	}

	result := byte(0)
	for i := 0; i < len(ss.data); i++ {
		result |= ss.data[i] ^ s[i]
	}

	return result == 0
}

// SecureBytes is a byte slice wrapper that zeros its contents when cleared.
type SecureBytes struct {
	data []byte
}

// NewSecureBytes creates a new SecureBytes from a byte slice.
// The data is copied to a new allocation.
func NewSecureBytes(data []byte) *SecureBytes {
	d := make([]byte, len(data))
	copy(d, data)
	return &SecureBytes{data: d}
}

// Bytes returns the underlying bytes.
func (sb *SecureBytes) Bytes() []byte {
	if sb == nil {
		return nil
	}
	return sb.data
}

// Len returns the length of the data.
func (sb *SecureBytes) Len() int {
	if sb == nil {
		return 0
	}
	return len(sb.data)
}

// Clear zeros the data and releases it.
func (sb *SecureBytes) Clear() {
	if sb == nil {
		return
	}

	for i := range sb.data {
		sb.data[i] = 0
	}

	sb.data = nil
}

// Equals compares the SecureBytes with another byte slice in constant time.
func (sb *SecureBytes) Equals(other []byte) bool {
	if sb == nil || len(sb.data) != len(other) {
		return false
	}

	result := byte(0)
	for i := 0; i < len(sb.data); i++ {
		result |= sb.data[i] ^ other[i]
	}

	return result == 0
}

// WipeBytes zeros a byte slice in place.
// Use this to clear sensitive data from regular byte slices.
func WipeBytes(data []byte) {
	for i := range data {
		data[i] = 0
	}
}
