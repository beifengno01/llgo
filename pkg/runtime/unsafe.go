// Copyright 2012 Andrew Wilkins.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package runtime

import "unsafe"

// FIXME #llgo name: llvm.trap
//
// Temporarily disabled use of llvm.trap while
// function representation changes are worked out.
func llvm_trap() {}

// #llgo name: reflect.unsafe_New
func unsafe_New(t *rtype) unsafe.Pointer {
	ptr := malloc(t.size)
	bzero(ptr, t.size)
	return ptr
}

// #llgo name: reflect.unsafe_NewArray
func unsafe_NewArray(t *rtype, n int) unsafe.Pointer {
	ptr := malloc(t.size * uintptr(n))
	bzero(ptr, t.size*uintptr(n))
	return ptr
}
