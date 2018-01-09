package config

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"time"

	log "github.com/sirupsen/logrus"
	yaml "gopkg.in/yaml.v2"
)

// Authorization Strategies determine who has access to what
const (
	AuthStrategyAny          = "any"
	AuthStrategyAllowedGroup = "group"
	AuthStrategyNone         = "none"
)

// AdminGroup is the default admin group used by builtin commands
const AdminGroup = "admin"

// Defaults for commands
const (
	DefaultCommandTimeout = 60 * time.Second
)

// Default colors
const (
	DefaultInfoColorMessage    = ""
	DefaultErrColorMessage     = "#cc3300"
	DefaultSuccessColorMessage = "#009900"
)

// Command types
const (
	BuiltinCommandType = iota
	ShellCommandType
	RemoteCommandType
)

// Builtin Commands Names
const (
	BuiltinVersionCommand = "version"
	BuiltinHelpCommand    = "help"
	BuiltinGroupsCommand  = "groups"
	BuiltinJobsCommand    = "jobs"
)

// New parses the configuration from a reader into an object and returns it
func New(r io.Reader) (Config, error) {
	c := Config{
		Database: Database{
			Path:    "meeseeks.db",
			Mode:    0600,
			Timeout: 2 * time.Second,
		},
		Colors: MessageColors{
			Info:    DefaultInfoColorMessage,
			Success: DefaultSuccessColorMessage,
			Error:   DefaultErrColorMessage,
		},
	}

	b, err := ioutil.ReadAll(r)
	if err != nil {
		return c, fmt.Errorf("could not read configuration: %s", err)
	}

	err = yaml.Unmarshal(b, &c)
	if err != nil {
		return c, fmt.Errorf("could not parse configuration: %s", err)
	}

	for name, command := range c.Commands {
		if command.AuthStrategy == "" {
			log.Debugf("Applying default AuthStrategy %s to command %s", AuthStrategyNone, name)
			command.AuthStrategy = AuthStrategyNone
		}
		if command.Timeout == 0 {
			log.Debugf("Applying default Timeout %d to command %s", DefaultCommandTimeout, name)
			command.Timeout = DefaultCommandTimeout
		} else {
			command.Timeout *= time.Second
			log.Infof("Command timeout for %s is %d", name, command.Timeout)
		}

		// All configured commands are shell type
		command.Type = ShellCommandType

		c.Commands[name] = command // Re-set the command
	}

	return c, nil
}

// Config is the struct used to load MrMeeseeks configuration yaml
type Config struct {
	Database Database            `yaml:"db"`
	Messages map[string][]string `yaml:"messages"`
	Commands map[string]Command  `yaml:"commands"`
	Colors   MessageColors       `yaml:"colors"`
	Groups   map[string][]string `yaml:"groups"`
}

// Command is the struct that handles a command configuration
type Command struct {
	Cmd           string            `yaml:"command"`
	Args          []string          `yaml:"arguments"`
	AllowedGroups []string          `yaml:"allowed_groups"`
	AuthStrategy  string            `yaml:"auth_strategy"`
	Timeout       time.Duration     `yaml:"timeout"`
	Templates     map[string]string `yaml:"templates"`
	Help          string            `yaml:"help"`
	Type          int
}

// MessageColors contains the configured reply message colora
type MessageColors struct {
	Info    string `yaml:"info"`
	Success string `yaml:"success"`
	Error   string `yaml:"error"`
}

// Database holds the configuration for the BoltDB database
type Database struct {
	Path    string        `yaml:"path"`
	Timeout time.Duration `yaml:"timeout"`
	Mode    os.FileMode   `yaml:"file_mode"`
}
