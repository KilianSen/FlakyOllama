package monitoring

import (
	"FlakyOllama/pkg/shared/models"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/mem"
)

// Monitor collects hardware metrics.
type Monitor struct {
}

func NewMonitor() *Monitor {
	return &Monitor{}
}

// GetStatus returns the current hardware status, applying caps if provided.
func (m *Monitor) GetStatus(maxVRAM uint64, maxCPU int) (models.NodeStatus, error) {
	var status models.NodeStatus

	// Get CPU cores
	count, err := cpu.Counts(true)
	if err == nil {
		status.CPUCores = count
		if maxCPU > 0 && status.CPUCores > maxCPU {
			status.CPUCores = maxCPU
		}
	}

	// Get CPU usage
	cpuPercent, err := cpu.Percent(time.Millisecond*50, false)
	if err == nil && len(cpuPercent) > 0 {
		status.CPUUsage = cpuPercent[0]
	}

	// Get Memory usage
	vm, err := mem.VirtualMemory()
	if err == nil {
		status.MemoryUsage = vm.UsedPercent
		status.MemoryTotal = vm.Total
	}

	// Real GPU monitoring via nvidia-smi
	err = m.collectGPUMetrics(&status)
	if err != nil {
		status.HasGPU = false
		status.GPUModel = "CPU Only"
		status.VRAMTotal = 0
		status.VRAMUsed = 0
		status.GPUTemperature = 0
	} else {
		status.HasGPU = true
		if maxVRAM > 0 && status.VRAMTotal > maxVRAM {
			status.VRAMTotal = maxVRAM
			if status.VRAMUsed > status.VRAMTotal {
				status.VRAMUsed = status.VRAMTotal
			}
		}
	}

	return status, nil
}

func (m *Monitor) collectGPUMetrics(status *models.NodeStatus) error {
	cmd := exec.Command("nvidia-smi", "--query-gpu=name,memory.total,memory.used,temperature.gpu", "--format=csv,noheader,nounits")
	output, err := cmd.Output()
	if err != nil {
		return err
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	if len(lines) == 0 {
		return nil
	}

	// Just take the first GPU for now
	parts := strings.Split(lines[0], ",")
	if len(parts) < 4 {
		return nil
	}

	status.GPUModel = strings.TrimSpace(parts[0])
	total, _ := strconv.ParseUint(strings.TrimSpace(parts[1]), 10, 64)
	used, _ := strconv.ParseUint(strings.TrimSpace(parts[2]), 10, 64)
	temp, _ := strconv.ParseFloat(strings.TrimSpace(parts[3]), 64)

	status.VRAMTotal = total * 1024 * 1024 // MiB -> Bytes
	status.VRAMUsed = used * 1024 * 1024
	status.GPUTemperature = temp

	return nil
}
