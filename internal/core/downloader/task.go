package downloader

import (
	"time"
)

// Priority levels for download tasks.
const (
	PriorityCritical = 100 // Manifests, version JSON, client JAR
	PriorityHigh     = 75  // Core libraries, natives
	PriorityNormal   = 50  // Assets, mods
	PriorityLow      = 25  // Resource packs, skins, non-essential textures
)

// DownloadTask represents an individual file download operation.
type DownloadTask struct {
	ID             string
	URL            string
	Mirrors        []string
	DestPath       string
	ExpectedSHA1   string
	ExpectedSHA256 string
	ExpectedSize   int64
	Priority       int
	Index          int // internal index for heap.Interface
	CreatedAt      time.Time
}

// TaskPriorityQueue implements heap.Interface for DownloadTask.
type TaskPriorityQueue []*DownloadTask

func (pq TaskPriorityQueue) Len() int { return len(pq) }

func (pq TaskPriorityQueue) Less(i, j int) bool {
	// Higher priority comes first; if equal, older task comes first (FIFO within same priority)
	if pq[i].Priority == pq[j].Priority {
		return pq[i].CreatedAt.Before(pq[j].CreatedAt)
	}
	return pq[i].Priority > pq[j].Priority
}

func (pq TaskPriorityQueue) Swap(i, j int) {
	pq[i], pq[j] = pq[j], pq[i]
	pq[i].Index = i
	pq[j].Index = j
}

func (pq *TaskPriorityQueue) Push(x any) {
	n := len(*pq)
	item := x.(*DownloadTask)
	item.Index = n
	*pq = append(*pq, item)
}

func (pq *TaskPriorityQueue) Pop() any {
	old := *pq
	n := len(old)
	item := old[n-1]
	old[n-1] = nil
	item.Index = -1
	*pq = old[0 : n-1]
	return item
}