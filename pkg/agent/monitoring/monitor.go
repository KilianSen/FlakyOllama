package monitoring

import (
	"FlakyOllama/pkg/shared/models"
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/mem"
)

// Monitor collects hardware metrics.
type Monitor struct {
	status atomic.Value // stores models.NodeStatus
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	nvmlInitialized bool
}

func NewMonitor() *Monitor {
	m := &Monitor{}
	m.status.Store(models.NodeStatus{
		GPUModel: "Initializing...",
	})

	// Try to initialize NVML
	if ret := nvml.Init(); ret != nvml.SUCCESS {
		// Not necessarily an error if we are on a CPU-only or non-NVIDIA system
		m.nvmlInitialized = false
	} else {
		m.nvmlInitialized = true
	}

	return m
}

func (m *Monitor) Start(ctx context.Context) {
	m.ctx, m.cancel = context.WithCancel(ctx)
	m.wg.Add(1)
	go m.pollLoop()
}

func (m *Monitor) Stop() {
	if m.cancel != nil {
		m.cancel()
	}
	m.wg.Wait()
	if m.nvmlInitialized {
		nvml.Shutdown()
	}
}

func (m *Monitor) pollLoop() {
	defer m.wg.Done()
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	// Initial poll
	m.refreshStatus(0, 0)

	for {
		select {
		case <-ticker.C:
			// For now, we don't have maxVRAM/maxCPU here, so we pass 0
			// The Agent will apply caps if needed when it reads the status,
			// or we can pass them to Start().
			m.refreshStatus(0, 0)
		case <-m.ctx.Done():
			return
		}
	}
}

// GetStatus returns the current hardware status instantly from cache.
func (m *Monitor) GetStatus(maxVRAM uint64, maxCPU int) (models.NodeStatus, error) {
	status := m.status.Load().(models.NodeStatus)

	// Apply caps if provided
	if maxCPU > 0 && status.CPUCores > maxCPU {
		status.CPUCores = maxCPU
	}
	if maxVRAM > 0 && status.VRAMTotal > maxVRAM {
		status.VRAMTotal = maxVRAM
		if status.VRAMUsed > status.VRAMTotal {
			status.VRAMUsed = status.VRAMTotal
		}
	}

	return status, nil
}

func (m *Monitor) refreshStatus(maxVRAM uint64, maxCPU int) {
	var status models.NodeStatus

	// Get CPU cores
	count, err := cpu.Counts(true)
	if err == nil {
		status.CPUCores = count
	}

	// Get CPU usage (non-blocking call with small interval)
	cpuPercent, err := cpu.Percent(time.Millisecond*100, false)
	if err == nil && len(cpuPercent) > 0 {
		status.CPUUsage = cpuPercent[0]
	}

	// Get Memory usage
	vm, err := mem.VirtualMemory()
	if err == nil {
		status.MemoryUsage = vm.UsedPercent
		status.MemoryTotal = vm.Total
	}

	// GPU Monitoring
	if m.nvmlInitialized {
		err = m.collectNVMLMetrics(&status)
		if err != nil {
			status.HasGPU = false
			status.GPUModel = "NVIDIA (Error)"
		} else {
			status.HasGPU = true
		}
	} else {
		// Fallback or Non-NVIDIA
		status.HasGPU = false
		status.GPUModel = "CPU Only"
	}

	m.status.Store(status)
}

func (m *Monitor) collectNVMLMetrics(status *models.NodeStatus) error {
	count, ret := nvml.DeviceGetCount()
	if ret != nvml.SUCCESS {
		return fmt.Errorf("failed to get device count: %v", nvml.ErrorString(ret))
	}

	if count == 0 {
		status.HasGPU = false
		status.GPUModel = "None"
		return nil
	}

	var totalVRAM, usedVRAM uint64
	var maxTemp float64
	var modelsList []string

	for i := 0; i < count; i++ {
		device, ret := nvml.DeviceGetHandleByIndex(i)
		if ret != nvml.SUCCESS {
			continue
		}

		// Model Name
		name, ret := device.GetName()
		if ret == nvml.SUCCESS {
			modelsList = append(modelsList, name)
		}

		// Memory
		memory, ret := device.GetMemoryInfo()
		if ret == nvml.SUCCESS {
			totalVRAM += memory.Total
			usedVRAM += memory.Used
		}

		// Temperature
		temp, ret := device.GetTemperature(nvml.TEMPERATURE_GPU)
		if ret == nvml.SUCCESS {
			if float64(temp) > maxTemp {
				maxTemp = float64(temp)
			}
		}
	}

	status.VRAMTotal = totalVRAM
	status.VRAMUsed = usedVRAM
	status.GPUTemperature = maxTemp

	if len(modelsList) > 0 {
		// Just unique names for the summary string
		uniqueModels := make(map[string]bool)
		var finalModels []string
		for _, m := range modelsList {
			if !uniqueModels[m] {
				uniqueModels[m] = true
				finalModels = append(finalModels, m)
			}
		}
		if len(finalModels) == 1 && count > 1 {
			status.GPUModel = fmt.Sprintf("%dx %s", count, finalModels[0])
		} else if len(finalModels) > 1 {
			status.GPUModel = fmt.Sprintf("%d GPUs: %v", count, finalModels)
		} else {
			status.GPUModel = finalModels[0]
		}
	}

	return nil
}
