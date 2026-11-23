package metrics

import (
	"encoding/json"
	"os"
	"runtime"
	"time"
)

type Sample struct {
	Time         time.Time `json:"time"`
	OS           string    `json:"os"`
	GoHeap       uint64    `json:"go_heap_bytes"`
	GoSystem     uint64    `json:"go_system_bytes"`
	WorkingSet   uint64    `json:"working_set_bytes,omitempty"`
	PrivateBytes uint64    `json:"private_bytes,omitempty"`
	CPUSeconds   float64   `json:"cpu_seconds,omitempty"`
}

func Current() Sample {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	s := Sample{Time: time.Now(), OS: runtime.GOOS, GoHeap: m.HeapAlloc, GoSystem: m.Sys}
	platform(&s)
	return s
}
func Write(path string) {
	b, e := json.MarshalIndent(Current(), "", "  ")
	if e == nil {
		_ = os.WriteFile(path, b, 0600)
	}
}
