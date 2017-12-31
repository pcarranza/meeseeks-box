package meeseeks

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"time"

	log "github.com/sirupsen/logrus"

	"gitlab.com/mr-meeseeks/meeseeks-box/config"
	parser "gitlab.com/mr-meeseeks/meeseeks-box/meeseeks/commandparser"
	"gitlab.com/mr-meeseeks/meeseeks-box/meeseeks/template"
)

var (
	errCommandNotFound = errors.New("Could not find command")
	errNoCommandToRun  = errors.New("No command to run")
)

var builtinCommands = map[string]config.Command{
	"echo": config.Command{
		Cmd:          "echo",
		Timeout:      5,
		AuthStrategy: config.AuthStrategyAny,
	},
}

// Message interface to interact with an abstract message
type Message interface {
	GetText() string
	GetChannel() string
	GetUserFrom() string
}

// Client interface that provides a way of replying to messages on a channel
type Client interface {
	Reply(text, channel string)
	ReplyIM(text, user string) error
}

// Meeseeks is the command execution engine
type Meeseeks struct {
	client   Client
	config   config.Config
	commands map[string]config.Command
}

// New creates a new Meeseeks service
func New(client Client, config config.Config) Meeseeks {
	return Meeseeks{
		client:   client,
		config:   config,
		commands: union(builtinCommands, config.Commands),
	}
}

// Process processes a received message
func (m Meeseeks) Process(message Message) {
	args, err := parser.ParseCommand(message.GetText())
	if err != nil {
		m.replyWithError(message, err, "")
	}

	if len(args) == 0 {
		m.replyWithError(message, errNoCommandToRun, "")
		return
	}

	cmd, err := m.findCommand(args[0])
	if err != nil {
		m.replyWithError(message, err, "")
		return
	}

	out, err := executeCommand(cmd, args[1:]...)
	if err != nil {
		m.replyWithError(message, err, out)
		return
	}

	m.replyWithSuccess(message, out)
}

func (m Meeseeks) replyWithError(message Message, err error, out string) {
	p := m.newReplyPayload()
	p["user"] = message.GetUserFrom()
	p["error"] = err.Error()
	p["output"] = out

	msg, err := template.DefaultTemplates().Failure.Render(p)
	if err != nil {
		log.Fatalf("could not render failure template %s; payload: %+v", err, p)
	}
	m.client.Reply(msg, message.GetChannel())
}

func (m Meeseeks) replyWithSuccess(message Message, out string) {
	p := m.newReplyPayload()
	p["user"] = message.GetUserFrom()
	p["output"] = out

	msg, err := template.DefaultTemplates().Success.Render(p)
	if err != nil {
		log.Fatalf("could not render success template %s; payload: %+v", err, p)
	}
	m.client.Reply(msg, message.GetChannel())
}

func (m Meeseeks) newReplyPayload() template.Payload {
	p := template.Payload{}
	for k, v := range m.config.Messages {
		p[k] = v
	}
	return p
}

func (m Meeseeks) findCommand(command string) (config.Command, error) {
	cmd, ok := m.commands[command]
	if !ok {
		return config.Command{}, fmt.Errorf("%s '%s'", errCommandNotFound, command)
	}
	return cmd, nil
}

func union(maps ...map[string]config.Command) map[string]config.Command {
	newMap := make(map[string]config.Command)
	for _, m := range maps {
		for k, v := range m {
			newMap[k] = v
		}
	}
	return newMap
}

func executeCommand(cmd config.Command, args ...string) (string, error) {
	timeout := time.Duration(cmd.Timeout) * time.Second
	args = append(cmd.Args, args...)

	ctx, cancelFunc := context.WithTimeout(context.Background(), timeout)
	defer cancelFunc()

	c := exec.CommandContext(ctx, cmd.Cmd, args...)
	out, err := c.CombinedOutput()

	return string(out), err
}
