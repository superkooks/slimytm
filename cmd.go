package main

import (
	"context"
	"os/exec"
)

// A drop-in replacement for exec.CommandContext that does not leave defunct processes.
type Cmd struct {
	ctx context.Context
	*exec.Cmd
}

func NewCommand(ctx context.Context, command string, args ...string) *Cmd {
	return &Cmd{ctx, exec.Command(command, args...)}
}

func (c *Cmd) Start() error {
	err := c.Cmd.Start()
	if err != nil {
		return err
	}
	go func() {
		<-c.ctx.Done()
		c.Cmd.Process.Kill()
		c.Cmd.Wait()
	}()
	return nil
}
