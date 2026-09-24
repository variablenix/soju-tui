package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestSojuCtlOutputBoundary(t *testing.T) {
	cancelled := false
	output := &sojuCtlOutput{cancel: func() { cancelled = true }}
	data := []byte(strings.Repeat("x", sojuCtlOutputLimit))
	if n, err := output.Write(data); n != len(data) || err != nil || cancelled {
		t.Fatalf("exact limit write: n=%d err=%v cancelled=%v", n, err, cancelled)
	}
	_, _ = output.Write([]byte("extra"))
	if !cancelled || !output.exceeded || output.buffer.Len() != sojuCtlOutputLimit {
		t.Fatal("overflow was not bounded and cancelled")
	}
}

func TestSojuCtlSubprocess(t *testing.T) {
	if os.Getenv("SOJU_TUI_OUTPUT_TEST") != "1" {
		return
	}
	switch os.Args[len(os.Args)-1] {
	case "overflow":
		fmt.Print(strings.Repeat("x", sojuCtlOutputLimit+1))
	case "pipes":
		child := exec.Command(os.Args[0], "-test.run=^TestSojuCtlSubprocess$", "--", "hold")
		child.Stdout, child.Stderr = os.Stdout, os.Stderr
		if err := child.Start(); err != nil {
			os.Exit(2)
		}
		_ = child.Process.Release()
	case "hold":
		time.Sleep(4 * time.Second)
	case "normal":
		fmt.Fprint(os.Stdout, "normal stdout\n")
		fmt.Fprint(os.Stderr, "normal stderr\n")
	case "timeout":
		time.Sleep(4 * time.Second)
	}
	os.Exit(0)
}

func TestSojuCtlBoundedExecution(t *testing.T) {
	path, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("SOJU_TUI_OUTPUT_TEST", "1")
	for _, mode := range []string{"normal", "overflow", "pipes", "timeout"} {
		t.Run(mode, func(t *testing.T) {
			timeout := 10 * time.Second
			if mode == "timeout" {
				timeout = 100 * time.Millisecond
			}
			start := time.Now()
			output, err := (&SojuCtl{Path: path, Timeout: timeout}).Run(context.Background(), []string{"-test.run=^TestSojuCtlSubprocess$", "--", mode})
			if mode == "normal" {
				if err != nil || !strings.Contains(output, "normal stdout") || !strings.Contains(output, "normal stderr") {
					t.Fatalf("normal execution: %q %v", output, err)
				}
				return
			}
			if err == nil {
				t.Fatal("expected execution error")
			}
			if mode == "overflow" && (output != "" || !strings.Contains(err.Error(), "output exceeded")) {
				t.Fatalf("overflow result: length=%d err=%v", len(output), err)
			}
			if mode == "timeout" && !strings.Contains(err.Error(), "timed out") {
				t.Fatalf("timeout result: %v", err)
			}
			if mode == "pipes" && time.Since(start) >= 3*time.Second {
				t.Fatal("descendant pipes blocked command completion")
			}
		})
	}
}

func TestSojuCtlReportsParentCancellation(t *testing.T) {
	truePath, err := exec.LookPath("true")
	if err != nil {
		t.Skip("true executable is unavailable")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = (&SojuCtl{Path: truePath, Timeout: time.Second}).Run(ctx, nil)
	if err == nil || !strings.Contains(err.Error(), "cancelled") {
		t.Fatalf("cancellation error = %v", err)
	}
}
