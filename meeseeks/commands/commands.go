package commands

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"time"

	"github.com/pcarranza/meeseeks-box/command"
	"github.com/pcarranza/meeseeks-box/jobs/logs"
	"github.com/sirupsen/logrus"

	"github.com/pcarranza/meeseeks-box/jobs"

	"github.com/pcarranza/meeseeks-box/config"
)

// Command Errors
var (
	ErrCommandNotFound    = fmt.Errorf("Can't find command")
	ErrUnknownCommandType = fmt.Errorf("Unknown command type")
)

// Commands holds the final set of configured commands
type Commands struct {
	commands map[string]command.Command
}

// New builds a new commands based on a configuration
func New(cnf config.Config) (Commands, error) {
	// Add builtin commands
	commands := make(map[string]command.Command)
	for name, command := range builtInCommands {
		commands[name] = command
	}

	for name, configCommand := range cnf.Commands {
		command, err := buildCommand(configCommand)
		if err != nil {
			return Commands{}, err
		}
		commands[name] = command
	}

	commands[BuiltinHelpCommand] = helpCommand{
		commands: &commands,
		Help:     "prints all the kwnown commands and its associated help",
	}

	return Commands{
		commands: commands,
	}, nil
}

// Find looks up a command by name and returns it or an error
func (c Commands) Find(name string) (command.Command, error) {
	cmd, ok := c.commands[name]
	if !ok {
		return nil, ErrCommandNotFound
	}
	return cmd, nil
}

// buildCommand creates a command instance based on the configuration
func buildCommand(cmd config.Command) (command.Command, error) {
	switch cmd.Type {
	case config.ShellCommandType:
		return newShellCommand(cmd)
	}
	return nil, ErrUnknownCommandType
}

// ShellCommand is a command that will be executed locally through an exec.CommandContext
type shellCommand struct {
	config.Command
}

// NewShellCommand returns a new command that is executed inside a shell
func newShellCommand(cmd config.Command) (command.Command, error) {
	return shellCommand{
		Command: cmd,
	}, nil
}

// Execute implements Command.Execute for the ShellCommand
func (c shellCommand) Execute(job jobs.Job) (string, error) {
	cmdArgs := append(c.Args(), job.Request.Args...)

	ctx, cancelFunc := context.WithTimeout(context.Background(), c.Timeout())
	defer cancelFunc()

	shellCommand := exec.CommandContext(ctx, c.Cmd(), cmdArgs...)

	cmdReader, err := shellCommand.StdoutPipe()
	if err != nil {
		return "", err
	}

	buffer := bytes.NewBufferString("")
	teeReader := io.TeeReader(cmdReader, buffer)
	scanner := bufio.NewScanner(teeReader)
	go func() {
		for scanner.Scan() {
			line := fmt.Sprintln(scanner.Text())
			if e := logs.Append(job.ID, line); e != nil {
				logrus.Errorf("Could not append '%s' to job %d logs: %s", line, job.ID, e)
			}
		}
	}()

	err = shellCommand.Start()
	if e := logs.SetError(job.ID, err); e != nil {
		logrus.Errorf("Could set error to job %d: %s", job.ID, e)
		return "", err
	}

	err = shellCommand.Wait()
	if e := logs.SetError(job.ID, err); e != nil {
		logrus.Errorf("Could set error to job %d: %s", job.ID, e)
		return "", err
	}

	return buffer.String(), err
}

func (c shellCommand) HasHandshake() bool {
	return true
}

func (c shellCommand) Templates() map[string]string {
	return c.Command.Templates
}

func (c shellCommand) AuthStrategy() string {
	return c.Command.AuthStrategy
}

func (c shellCommand) AllowedGroups() []string {
	return c.Command.AllowedGroups
}

func (c shellCommand) Args() []string {
	return c.Command.Args
}

func (c shellCommand) Timeout() time.Duration {
	return c.Command.Timeout
}

func (c shellCommand) Cmd() string {
	return c.Command.Cmd
}

// Help returns the help from the configured command
func (c shellCommand) Help() string {
	return c.Command.Help
}

func (c shellCommand) Record() bool {
	return true
}