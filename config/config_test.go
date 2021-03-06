package config_test

import (
	"fmt"
	"io"
	"reflect"
	"strings"
	"testing"
	"time"

	"gitlab.com/yakshaving.art/meeseeks-box/auth"
	"gitlab.com/yakshaving.art/meeseeks-box/meeseeks"

	"gitlab.com/yakshaving.art/meeseeks-box/commands"
	"gitlab.com/yakshaving.art/meeseeks-box/config"
	"gitlab.com/yakshaving.art/meeseeks-box/mocks"
	"gitlab.com/yakshaving.art/meeseeks-box/persistence/db"
	"gitlab.com/yakshaving.art/meeseeks-box/text/formatter"
	"github.com/renstrom/dedent"
)

func Test_ConfigurationReading(t *testing.T) {
	defaultColors := formatter.MessageColors{
		Info:    formatter.DefaultInfoColorMessage,
		Error:   formatter.DefaultErrColorMessage,
		Success: formatter.DefaultSuccessColorMessage,
	}
	defaultDatabase := db.DatabaseConfig{
		Path:    "meeseeks.db",
		Mode:    0600,
		Timeout: 2 * time.Second,
	}
	tt := []struct {
		Name     string
		Content  string
		Expected config.Config
	}{
		{
			"Default configuration",
			"",
			config.Config{
				Format: formatter.FormatConfig{
					Colors:     defaultColors,
					ReplyStyle: map[string]string{},
				},
				Database: defaultDatabase,
				Pool:     20,
			},
		},
		{
			"With messages",
			dedent.Dedent(`
				format:
				  messages:
				    handshake: ["hallo"]
				`),
			config.Config{
				Format: formatter.FormatConfig{
					Colors:     defaultColors,
					ReplyStyle: map[string]string{},
					Messages: map[string][]string{
						"handshake": {"hallo"},
					},
				},
				Database: defaultDatabase,
				Pool:     20,
			},
		},
		{
			"With colors",
			dedent.Dedent(`
				format:
				  colors:
				    info: "#FFFFFF"
				    success: "#CCCCCC"
				    error: "#000000"
				`),
			config.Config{
				Format: formatter.FormatConfig{
					Colors: formatter.MessageColors{
						Info:    "#FFFFFF",
						Success: "#CCCCCC",
						Error:   "#000000",
					},
					ReplyStyle: map[string]string{},
				},
				Database: defaultDatabase,
				Pool:     20,
			},
		},
		{
			"With commands",
			dedent.Dedent(`
				commands:
				  something:
				    command: "ssh"
				    authorized: ["someone"]
				    args: ["none"]
				`),
			config.Config{
				Commands: map[string]config.Command{
					"something": {
						Cmd:  "ssh",
						Args: []string{"none"},
					},
				},
				Format: formatter.FormatConfig{
					Colors:     defaultColors,
					ReplyStyle: map[string]string{},
				},
				Database: defaultDatabase,
				Pool:     20,
			},
		},
	}
	for _, tc := range tt {
		t.Run(tc.Name, func(t *testing.T) {
			c, err := config.New(strings.NewReader(tc.Content))
			if err != nil {
				t.Fatalf("failed to parse configuration: %s", err)
			}
			if !reflect.DeepEqual(tc.Expected, c) {
				t.Fatalf("configuration is not as expected; got %#v instead of %#v", c, tc.Expected)
			}

		})
	}
}

func Test_Errors(t *testing.T) {
	tc := []struct {
		name     string
		reader   io.Reader
		expected string
	}{
		{
			"invalid configuration",
			strings.NewReader("	invalid"),
			"could not parse configuration: yaml: found character that cannot start any token",
		},
		{
			"bad reader",
			badReader{},
			"could not read configuration: bad reader",
		},
	}
	for _, tc := range tc {
		t.Run(tc.name, func(t *testing.T) {
			_, err := config.New(tc.reader)
			mocks.AssertEquals(t, err.Error(), tc.expected)
		})
	}
}

type badReader struct {
}

func (badReader) Read(b []byte) (n int, err error) {
	return 0, fmt.Errorf("bad reader")
}

func Test_ConfigurationLoading(t *testing.T) {
	_, err := config.ReadFile("./test-fixtures/empty-config.yml")
	mocks.AssertEquals(t, nil, err)
}

func Test_ConfigurationLoadNonExistingFile(t *testing.T) {
	_, err := config.ReadFile("./test-fixtures/non-existing-config.yml")
	mocks.AssertEquals(t, "could not open configuration file ./test-fixtures/non-existing-config.yml: open ./test-fixtures/non-existing-config.yml: no such file or directory", err.Error())
}

func Test_ConfigurationBasicLoading(t *testing.T) {
	c, err := config.ReadFile("./test-fixtures/basic-config.yml")
	mocks.Must(t, "could not read configuration file", err)
	mocks.AssertEquals(t, "./meeseeks-workspace.db", c.Database.Path)
	mocks.AssertEquals(t, 2, len(c.Commands))

	mocks.Must(t, "failed to load configuration", config.LoadConfiguration(c))
}

func TestReloadingConfigurationReplacesThings(t *testing.T) {
	c, err := config.ReadFile("./test-fixtures/basic-config.yml")
	mocks.Must(t, "could not read configuration file", err)
	mocks.Must(t, "failed to load configuration", config.LoadConfiguration(c))

	mocks.AssertEquals(t, []string{"pablo"}, auth.GetGroups()["admin"])

	first, ok := commands.Find(&meeseeks.Request{
		Command: "echo",
	})
	mocks.AssertEquals(t, true, ok)

	_, ok = commands.Find(&meeseeks.Request{
		Command: "echo-2",
	})
	mocks.AssertEquals(t, true, ok)

	c, err = config.ReadFile("./test-fixtures/basic-config.1.yml")
	mocks.Must(t, "could not read the second configuration file", err)
	mocks.Must(t, "failed to load the second configuration", config.LoadConfiguration(c))

	mocks.AssertEquals(t, []string{"daniele", "pablo"}, auth.GetGroups()["admin"])

	second, ok := commands.Find(&meeseeks.Request{
		Command: "echo",
	})
	mocks.AssertEquals(t, true, ok)

	_, ok = commands.Find(&meeseeks.Request{
		Command: "echo-2",
	})
	mocks.AssertEquals(t, false, ok)

	mocks.AssertEquals(t, first.GetCmd(), second.GetCmd())
	mocks.AssertEquals(t, first.GetAllowedChannels(), second.GetAllowedChannels())
	mocks.AssertEquals(t, first.HasHandshake(), second.HasHandshake())
	mocks.AssertEquals(t, first.MustRecord(), second.MustRecord())

	mocks.AssertEquals(t, first.GetTimeout(), 5*time.Second)
	mocks.AssertEquals(t, second.GetTimeout(), 10*time.Second)

	mocks.AssertEquals(t, first.GetAuthStrategy(), "any")
	mocks.AssertEquals(t, second.GetAuthStrategy(), "group")
}
