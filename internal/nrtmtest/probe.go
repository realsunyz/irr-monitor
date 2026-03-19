package nrtmtest

import (
	"context"
	"fmt"
	"time"

	"github.com/realSunyz/irr-monitor/internal/nrtm"
)

type Result struct {
	Registry      nrtm.Registry
	Serial        int64
	SerialLatency time.Duration
	TCPLatency    time.Duration
	QueryLatency  time.Duration
	ResponseLine  string
}

func Probe(ctx context.Context, registry nrtm.Registry, timeout time.Duration) (*Result, error) {
	result := &Result{Registry: registry}
	client := nrtm.NewClient(registry, timeout)

	serialStart := time.Now()
	serial, err := client.CurrentSerial(ctx)
	result.SerialLatency = time.Since(serialStart)
	if err != nil {
		return result, fmt.Errorf("fetch current serial: %w", err)
	}
	result.Serial = serial

	tcpStart := time.Now()
	line, err := client.FirstResponseLine(ctx, serial)
	result.TCPLatency = time.Since(tcpStart)
	if err != nil {
		return result, fmt.Errorf("query %s: %w", registry.Name, err)
	}
	result.QueryLatency = result.TCPLatency
	result.ResponseLine = line
	return result, nil
}
