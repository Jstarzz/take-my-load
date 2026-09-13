package worker

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	"github.com/Jstarzz/take-my-load/internal/protocol"
)

type Executor interface {
	Run(context.Context, protocol.WorkerAssignment) error
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

func (e *BlastExecutor) Run(ctx context.Context, assignment protocol.WorkerAssignment) error {
	if assignment.Engine != "blast" {
		return fmt.Errorf("unsupported execution engine %q", assignment.Engine)
	}
	if assignment.RequestsPerSecond <= 0 || assignment.DurationSeconds <= 0 {
		return fmt.Errorf("invalid assignment rate=%d duration=%d", assignment.RequestsPerSecond, assignment.DurationSeconds)
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
			return fmt.Errorf("blast execution: %w", err)
		}
		return fmt.Errorf("blast execution: %w: %s", err, message)
	}
	if strings.TrimSpace(stdout.String()) == "" {
		return fmt.Errorf("blast execution returned no summary")
	}
	return nil
}
