package state

import "strconv"

// NodeView is a normalized snapshot of a cluster node and the pods that are
// scheduled onto it. Usage fields are -1 when metrics-server is unavailable.
type NodeView struct {
	Name       string
	Ready      bool
	CPUText    string
	MemoryText string
	Pods       []PodView

	CPUMilli     int64 // allocatable millicores
	MemBytes     int64 // allocatable bytes
	CPUUsedMilli int64 // -1 == unknown
	MemUsedBytes int64 // -1 == unknown
}

// CPUPercent returns CPU utilisation 0..100, or -1 when unknown.
func (n NodeView) CPUPercent() int {
	return pct(n.CPUUsedMilli, n.CPUMilli)
}

// MemPercent returns memory utilisation 0..100, or -1 when unknown.
func (n NodeView) MemPercent() int {
	return pct(n.MemUsedBytes, n.MemBytes)
}

// pct computes used/total as an integer percentage, clamped to [0,100], or
// -1 when either input is non-positive (unknown / missing capacity).
func pct(used, total int64) int {
	if used < 0 || total <= 0 {
		return -1
	}
	p := int(used * 100 / total)
	if p < 0 {
		return 0
	}
	if p > 100 {
		return 100
	}
	return p
}

// itoa is a tiny helper to keep callers free of strconv imports.
func itoa(i int) string { return strconv.Itoa(i) }
