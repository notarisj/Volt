package handlers

import (
	"bufio"
	"encoding/json"
	"fmt"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/host"
	"github.com/shirou/gopsutil/v3/mem"
)

// gatherStats collects a snapshot of system metrics.
func gatherStats() fiber.Map {
	cpuPct, _ := cpu.Percent(0, false)
	cpuInfo, _ := cpu.Info()
	vmStat, _ := mem.VirtualMemory()
	hostInfo, _ := host.Info()

	cores := 0
	if len(cpuInfo) > 0 {
		for _, ci := range cpuInfo {
			cores += int(ci.Cores)
		}
	}
	if cores == 0 {
		n, _ := cpu.Counts(true)
		cores = n
	}

	cpuPercent := 0.0
	if len(cpuPct) > 0 {
		cpuPercent = cpuPct[0]
	}

	hostname := ""
	if hostInfo != nil {
		hostname = hostInfo.Hostname
	}

	var ramUsed, ramTotal uint64
	var ramPercent float64
	if vmStat != nil {
		ramUsed = vmStat.Used
		ramTotal = vmStat.Total
		ramPercent = vmStat.UsedPercent
	}

	return fiber.Map{
		"cpu_percent": cpuPercent,
		"cpu_cores":   cores,
		"ram_used":    ramUsed,
		"ram_total":   ramTotal,
		"ram_percent": ramPercent,
		"hostname":    hostname,
	}
}

// StatsHandler returns a single JSON snapshot of system metrics.
func StatsHandler(c *fiber.Ctx) error {
	return c.JSON(gatherStats())
}

// StatsStreamHandler streams system metrics as Server-Sent Events every 2 seconds.
// The connection stays open until the client disconnects.
func StatsStreamHandler(c *fiber.Ctx) error {
	c.Set("Content-Type", "text/event-stream")
	c.Set("Cache-Control", "no-cache")
	c.Set("Connection", "keep-alive")
	c.Set("X-Accel-Buffering", "no") // disable nginx/proxy response buffering

	c.Context().SetBodyStreamWriter(func(w *bufio.Writer) {
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()

		for {
			data, err := json.Marshal(gatherStats())
			if err == nil {
				fmt.Fprintf(w, "data: %s\n\n", data)
				if err := w.Flush(); err != nil {
					return // client disconnected
				}
			}
			<-ticker.C
		}
	})
	return nil
}
