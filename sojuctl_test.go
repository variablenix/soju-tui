package main

import (
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"
)

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
