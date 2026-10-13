package main

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Capture only the one-line image ID, even if a supplied host writes more.
type directV5ImageOutput struct {
	data     [65]byte
	used     int
	overflow bool
}

func (out *directV5ImageOutput) Write(p []byte) (int, error) {
	n := copy(out.data[out.used:], p)
	out.used += n
	out.overflow = out.overflow || n != len(p)
	return len(p), nil
}

func verifyDirectV5FastHost(proofPath, hostPath, contract, expectedImage string) error {
	if !isHex(contract, addressBytes) || contract != strings.ToLower(contract) ||
		!isHex(expectedImage, 32) || expectedImage != strings.ToLower(expectedImage) {
		return errors.New("invalid direct v5 host preflight inputs")
	}
	proofInfo, err := os.Lstat(proofPath)
	if err != nil || !proofInfo.Mode().IsRegular() || proofInfo.Mode().Perm()&0o077 != 0 || proofInfo.Size() > 64*1024 {
		return errors.New("direct v5 proof must be a bounded owner-only regular file")
	}
	hostPath, err = filepath.Abs(hostPath)
	if err != nil {
		return errors.New("invalid direct v5 host path")
	}
	hostInfo, err := os.Lstat(hostPath)
	if err != nil || !hostInfo.Mode().IsRegular() || hostInfo.Mode().Perm()&0o111 == 0 {
		return errors.New("direct v5 host must be an executable regular file")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	var image directV5ImageOutput
	imageCmd := exec.CommandContext(ctx, hostPath, "--image-id")
	imageCmd.Env = []string{"PATH=/usr/bin:/bin", "RISC0_DEV_MODE=0"}
	imageCmd.Stdout, imageCmd.Stderr = &image, io.Discard
	if err := imageCmd.Run(); err != nil {
		if ctx.Err() != nil {
			return errors.New("direct v5 host preflight timed out")
		}
		return errors.New("direct v5 host image check failed")
	}
	if image.overflow || string(image.data[:image.used]) != expectedImage+"\n" {
		return errors.New("direct v5 host image differs from the pinned image")
	}

	verifyCmd := exec.CommandContext(ctx, hostPath, "--verify", proofPath, "--contract", "0x"+contract)
	verifyCmd.Env = []string{"PATH=/usr/bin:/bin", "RISC0_DEV_MODE=0"}
	verifyCmd.Stdout, verifyCmd.Stderr = io.Discard, io.Discard
	if err := verifyCmd.Run(); err != nil {
		if ctx.Err() != nil {
			return errors.New("direct v5 host preflight timed out")
		}
		return errors.New("direct v5 host refused the sealed content proof")
	}
	return nil
}
