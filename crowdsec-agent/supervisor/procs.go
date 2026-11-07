package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"syscall"
)

// procs owns every wait on this process's children. It may run as PID 1 or as
// a child subreaper, so orphaned descendants are reaped here too. Starting a
// child and registering its PID happen under the same lock the reaper holds,
// so the reaper never collects a status that belongs to a tracked command.
type procs struct {
	mu      sync.Mutex
	waiters map[int]chan syscall.WaitStatus
}

func newProcs() *procs {
	return &procs{waiters: map[int]chan syscall.WaitStatus{}}
}

const prSetChildSubreaper = 36

func becomeSubreaper() error {
	_, _, errno := syscall.RawSyscall(syscall.SYS_PRCTL, prSetChildSubreaper, 1, 0)
	if errno != 0 {
		return errno
	}
	return nil
}

type proc struct {
	pid    int
	status chan syscall.WaitStatus
	cmd    *exec.Cmd
}

func (p *procs) start(cmd *exec.Cmd) (*proc, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	ch := make(chan syscall.WaitStatus, 1)
	p.waiters[cmd.Process.Pid] = ch
	return &proc{pid: cmd.Process.Pid, status: ch, cmd: cmd}, nil
}

// reap collects every exited child without blocking. Call it on SIGCHLD.
func (p *procs) reap() {
	p.mu.Lock()
	defer p.mu.Unlock()
	for {
		var status syscall.WaitStatus
		pid, err := syscall.Wait4(-1, &status, syscall.WNOHANG, nil)
		if errors.Is(err, syscall.EINTR) {
			continue
		}
		if err != nil || pid <= 0 {
			return
		}
		if ch, ok := p.waiters[pid]; ok {
			delete(p.waiters, pid)
			ch <- status
		}
	}
}

func (pr *proc) signal(sig os.Signal) {
	_ = pr.cmd.Process.Signal(sig)
}

func exitCode(status syscall.WaitStatus) int {
	switch {
	case status.Exited():
		return status.ExitStatus()
	case status.Signaled():
		return 128 + int(status.Signal())
	}
	return 1
}

func statusError(status syscall.WaitStatus) error {
	switch {
	case status.Exited() && status.ExitStatus() == 0:
		return nil
	case status.Exited():
		return fmt.Errorf("exit status %d", status.ExitStatus())
	case status.Signaled():
		return fmt.Errorf("killed by %v", status.Signal())
	}
	return errors.New("unknown exit status")
}
