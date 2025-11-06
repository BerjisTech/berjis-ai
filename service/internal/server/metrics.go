package server

import (
    "sync"
    "sync/atomic"
)

type counters struct {
    TotalChats       uint64
    BlockedPrompts   uint64
    TotalCompletions uint64
    TotalEmbeddings  uint64
    Errors           uint64
}

type Metrics struct {
    mu     sync.RWMutex
    C      counters
    Models map[string]uint64
}

func newMetrics() *Metrics {
    return &Metrics{Models: make(map[string]uint64)}
}

func (m *Metrics) incModel(model string) {
    if model == "" { return }
    m.mu.Lock()
    m.Models[model] = m.Models[model] + 1
    m.mu.Unlock()
}

func (m *Metrics) snapshot() (out struct {
    Counters counters            `json:"counters"`
    Models   map[string]uint64   `json:"models"`
}) {
    m.mu.RLock()
    defer m.mu.RUnlock()
    out.Counters = m.C
    out.Models = make(map[string]uint64, len(m.Models))
    for k, v := range m.Models { out.Models[k] = v }
    return
}

func (m *Metrics) incChat(model string) {
    atomic.AddUint64(&m.C.TotalChats, 1)
    m.incModel(model)
}
func (m *Metrics) incBlocked() { atomic.AddUint64(&m.C.BlockedPrompts, 1) }
func (m *Metrics) incCompletion(model string) {
    atomic.AddUint64(&m.C.TotalCompletions, 1)
    m.incModel(model)
}
func (m *Metrics) incEmbeddings() { atomic.AddUint64(&m.C.TotalEmbeddings, 1) }
func (m *Metrics) incError() { atomic.AddUint64(&m.C.Errors, 1) }

