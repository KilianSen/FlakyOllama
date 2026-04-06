package monitoring

import (
	"FlakyOllama/pkg/models"
	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/mem"
	"math/rand"
	"time"
)

// Monitor collects hardware metrics.
type Monitor struct {
}

func NewMonitor() *Monitor {
	return &Monitor{}
}

// GetStatus returns the current hardware status.
func (m *Monitor) GetStatus() (models.NodeStatus, error) {
	var status models.NodeStatus

	// Get CPU usage
	cpuPercent, err := cpu.Percent(time.Millisecond*50, false)
	if err == nil && len(cpuPercent) > 0 {
		status.CPUUsage = cpuPercent[0]
	}

	// Get Memory usage
	vm, err := mem.VirtualMemory()
	if err == nil {
		status.MemoryUsage = vm.UsedPercent
	}

	// Mocking GPU for now (as it's hardware dependent)
	// In a real implementation, we'd use NVML or parse nvidia-smi.
	status.VRAMTotal = 8 * 1024 * 1024 * 1024 // 8GB
	status.VRAMUsed = uint64(rand.Intn(4 * 1024 * 1024 * 1024))
	status.GPUTemperature = 40.0 + rand.Float64()*20.0

	return status, nil
}
