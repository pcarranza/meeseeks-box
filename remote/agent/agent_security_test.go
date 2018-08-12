package agent_test

import (
	"testing"
	"time"

	"github.com/gomeeseeks/meeseeks-box/commands"
	"github.com/gomeeseeks/meeseeks-box/commands/shell"
	"github.com/gomeeseeks/meeseeks-box/meeseeks"
	"github.com/gomeeseeks/meeseeks-box/mocks"
	"github.com/gomeeseeks/meeseeks-box/remote/agent"
	"github.com/gomeeseeks/meeseeks-box/remote/server"
)

var echoCmd = shell.New(meeseeks.CommandOpts{
	Cmd:  "echo",
	Help: meeseeks.NewHelp("echo"),
})

func TestAgentCanConnectAndRegisterACommand(t *testing.T) {
	mocks.Must(t, "failed to register commands",
		commands.Register(commands.RegistrationArgs{
			Action: commands.ActionRegister,
			Kind:   commands.KindLocalCommand,
			Commands: []commands.CommandRegistration{
				{
					Name: "remote-echo",
					Cmd:  echoCmd,
				},
			},
		}))
	defer commands.Reset()

	s, err := server.New(server.Config{})
	mocks.Must(t, "failed to create grpc server", err)
	defer s.Shutdown()

	go func() {
		if err := s.Listen("localhost:9697"); err != nil {
			t.Logf("failed to start server: %s", err)
		}
	}()

	client := agent.New(agent.Configuration{
		GRPCTimeout: 1 * time.Second,
		ServerURL:   "localhost:9697",
		Labels:      map[string]string{"tier": "testing"},
	})
	mocks.Must(t, "failed to connect agent", client.Connect())

	w := make(chan interface{})
	go func() {
		w <- true
		client.Run()
	}()
	<-w
	time.Sleep(1 * time.Millisecond)
	client.Shutdown()
}

func TestAgentTLSCanConnectAndRegisterACommand(t *testing.T) {
	mocks.Must(t, "failed to register commands",
		commands.Register(commands.RegistrationArgs{
			Action: commands.ActionRegister,
			Kind:   commands.KindLocalCommand,
			Commands: []commands.CommandRegistration{
				{
					Name: "remote-echo",
					Cmd:  echoCmd,
				},
			},
		}))
	defer commands.Reset()

	s, err := server.New(server.Config{
		SecurityMode: "tls",
		CertPath:     "../../config/test-fixtures/cert.pem",
		KeyPath:      "../../config/test-fixtures/key.pem",
	})
	mocks.Must(t, "failed to create grpc server", err)
	defer s.Shutdown()

	go func() {
		mocks.Must(t, "Failed to start server", s.Listen("localhost:9699"))
	}()

	client := agent.New(agent.Configuration{
		GRPCTimeout:  1 * time.Second,
		ServerURL:    "localhost:9699",
		SecurityMode: "tls",
		CertPath:     "../../config/test-fixtures/cert.pem",
		Labels:       map[string]string{"tier": "testing"},
	})
	mocks.Must(t, "failed to connect agent", client.Connect())

	w := make(chan interface{})
	go func() {
		w <- true
		client.Run()
	}()
	<-w

	time.Sleep(1 * time.Millisecond)

	client.Shutdown()
}