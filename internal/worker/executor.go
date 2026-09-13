package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	"github.com/Jstarzz/take-my-load/internal/protocol"
)

type Executor interface {
	Run(context.Context, protocol.WorkerAssignment) (protocol.ExecutionSummary, error)
}

type BlastExecutor struct {
	binary      string
	concurrency int
}

func NewBlastExecutor(binary string, concurrency int) *BlastExecutor {
	if binary == "" {
		binary = "tml-blast"
	}
	if concurrency <= 0 {
		concurrency = 4096
	}
	return &BlastExecutor{binary: binary, concurrency: concurrency}
}

func (e *BlastExecutor) Run(ctx context.Context, assignment protocol.WorkerAssignment) (protocol.ExecutionSummary, error) {
	if assignment.Engine != "blast" {
		return protocol.ExecutionSummary{}, fmt.Errorf("unsupported execution engine %q", assignment.Engine)
	}
	if assignment.RequestsPerSecond <= 0 || assignment.DurationSeconds <= 0 {
		return protocol.ExecutionSummary{}, fmt.Errorf("invalid assignment rate=%d duration=%d", assignment.RequestsPerSecond, assignment.DurationSeconds)
	}

	cmd := exec.CommandContext(
		ctx,
		e.binary,
		"run",
		"--target", assignment.Target,
		"--rps", strconv.FormatInt(assignment.RequestsPerSecond, 10),
		"--duration-seconds", strconv.FormatInt(assignment.DurationSeconds, 10),
		"--concurrency", strconv.Itoa(e.concurrency),
	)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		message := strings.TrimSpace(stderr.String())
		if len(message) > 4096 {
			message = message[:4096]
		}
		if message == "" {
			return protocol.ExecutionSummary{}, fmt.Errorf("blast execution: %w", err)
		}
		return protocol.ExecutionSummary{}, fmt.Errorf("blast execution: %w: %s", err, message)
	}
	if strings.TrimSpace(stdout.String()) == "" {
		return protocol.ExecutionSummary{}, fmt.Errorf("blast execution returned no summary")
	}
	var summary protocol.ExecutionSummary
	if err := json.Unmarshal(stdout.Bytes(), &summary); err != nil {
		return protocol.ExecutionSummary{}, fmt.Errorf("decode blast summary: %w", err)
	}
	return summary, nil
}
