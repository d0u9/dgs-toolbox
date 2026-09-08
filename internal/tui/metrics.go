package tui

import (
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	psnet "github.com/shirou/gopsutil/v4/net"
)

type metricCounters struct {
	at                        time.Time
	cpuTotal, cpuIdle         float64
	networkRead, networkWrite uint64
	diskRead, diskWrite       uint64
}

type metricsMsg struct {
	counters metricCounters
	err      error
}

type shellMetrics struct {
	previous               metricCounters
	hasPrevious            bool
	ready                  bool
	cpuPercent             float64
	networkDown, networkUp float64
	diskRead, diskWrite    float64
}

func sampleMetricsCmd() tea.Cmd {
	return func() tea.Msg {
		counters, err := readMetricCounters()
		return metricsMsg{counters: counters, err: err}
	}
}

func readMetricCounters() (metricCounters, error) {
	times, err := cpu.Times(false)
	if err != nil || len(times) == 0 {
		return metricCounters{}, err
	}
	value := times[0]
	counters := metricCounters{
		at:       time.Now(),
		cpuIdle:  value.Idle,
		cpuTotal: value.User + value.System + value.Idle + value.Nice + value.Iowait + value.Irq + value.Softirq + value.Steal,
	}
	network, err := psnet.IOCounters(true)
	if err != nil {
		return metricCounters{}, err
	}
	for _, device := range network {
		if strings.HasPrefix(strings.ToLower(device.Name), "lo") {
			continue
		}
		counters.networkRead += device.BytesRecv
		counters.networkWrite += device.BytesSent
	}
	disks, err := disk.IOCounters()
	if err != nil {
		return metricCounters{}, err
	}
	for _, device := range disks {
		counters.diskRead += device.ReadBytes
		counters.diskWrite += device.WriteBytes
	}
	return counters, nil
}

func (m *shellMetrics) update(current metricCounters) {
	if !m.hasPrevious {
		m.previous, m.hasPrevious = current, true
		return
	}
	seconds := current.at.Sub(m.previous.at).Seconds()
	if seconds <= 0 {
		return
	}
	total := current.cpuTotal - m.previous.cpuTotal
	idle := current.cpuIdle - m.previous.cpuIdle
	if total > 0 {
		m.cpuPercent = maxFloat(0, minFloat(100, 100*(total-idle)/total))
	}
	m.networkDown = counterRate(current.networkRead, m.previous.networkRead, seconds)
	m.networkUp = counterRate(current.networkWrite, m.previous.networkWrite, seconds)
	m.diskRead = counterRate(current.diskRead, m.previous.diskRead, seconds)
	m.diskWrite = counterRate(current.diskWrite, m.previous.diskWrite, seconds)
	m.previous, m.ready = current, true
}

func counterRate(current, previous uint64, seconds float64) float64 {
	if current < previous {
		return 0
	}
	return float64(current-previous) / seconds
}

func minFloat(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func maxFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
