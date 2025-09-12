package actions

import (
	"bufio"
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

func Ping(ctx context.Context, ip string, outputChan chan<- string) {
	defer close(outputChan)

	if strings.ContainsAny(ip, ";|&`") {
		outputChan <- "Invalid IP address."
		return
	}

	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.CommandContext(ctx, "ping", "-n", "4", ip)
	default: 
		cmd = exec.CommandContext(ctx, "ping", "-c", "4", ip)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		outputChan <- fmt.Sprintf("Error creating stdout pipe: %v", err)
		return
	}

	if err := cmd.Start(); err != nil {
		outputChan <- fmt.Sprintf("Error starting ping command: %v", err)
		return
	}

	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		select {
		case <-ctx.Done():
			_ = cmd.Process.Kill()
			return
		case outputChan <- scanner.Text():
		}
	}

	if err := cmd.Wait(); err != nil {
		if _, ok := err.(*exec.ExitError); !ok {
			outputChan <- fmt.Sprintf("Ping command finished with error: %v", err)
		}
	}
}

