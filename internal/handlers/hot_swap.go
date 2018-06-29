package handlers

import (
	"net/http"
	"sync/atomic"
	"unsafe"
)

type HotSwap struct {
	current unsafe.Pointer
}

func NewHotSwap(current http.Handler) *HotSwap {
	return &HotSwap{
		current: unsafe.Pointer(&current),
	}
}

func (s *HotSwap) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h := *(*http.Handler)(atomic.LoadPointer(&s.current))
	h.ServeHTTP(w, r)
}

func (s *HotSwap) Swap(newHandler http.Handler) {
	atomic.StorePointer(&s.current, unsafe.Pointer(&newHandler))
}
