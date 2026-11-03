//go:build !windows

package client

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
)

// playerCommand is the first audio player of the system that can read raw PCM on its standard input.
func playerCommand() *exec.Cmd {
	if p, err := exec.LookPath("paplay"); err == nil {
		return exec.Command(p, "--raw", "--rate=44100", "--format=s16le", "--channels=2", "--latency-msec=60",
			"--client-name=gocraft", "--stream-name=gocraft")
	}
	if p, err := exec.LookPath("pw-cat"); err == nil {
		return exec.Command(p, "--playback", "--rate", "44100", "--channels", "2", "--format", "s16", "--latency", "60ms", "-")
	}
	if p, err := exec.LookPath("aplay"); err == nil {
		return exec.Command(p, "-q", "-t", "raw", "-f", "S16_LE", "-r", "44100", "-c", "2")
	}
	return nil
}

// audioAvailable is whether the system has a player to pipe to.
func audioAvailable() bool { return playerCommand() != nil }

// openSink starts the player and returns the pipe into its standard input.
func openSink() (*sink, error) {
	cmd := playerCommand()
	if cmd == nil {
		return nil, fmt.Errorf("no audio player found (paplay, pw-cat or aplay)")
	}
	r, w, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	cmd.Stdin = r
	if err := cmd.Start(); err != nil {
		r.Close()
		w.Close()
		return nil, err
	}
	r.Close()
	// A small pipe keeps what is queued short, so a sound is heard soon after it is played; if the system
	// refuses (F_SETPIPE_SZ), the pacing in output.run keeps it short anyway.
	syscall.Syscall(syscall.SYS_FCNTL, w.Fd(), 1031, 4096)
	go cmd.Wait()
	return &sink{w: w, close: func() { w.Close() }, paced: true, pid: cmd.Process.Pid}, nil
}
