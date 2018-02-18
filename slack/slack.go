package slack

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/gomeeseeks/meeseeks-box/auth"
	"github.com/gomeeseeks/meeseeks-box/formatter"
	"github.com/gomeeseeks/meeseeks-box/meeseeks"

	"github.com/nlopes/slack"
	"github.com/sirupsen/logrus"
)

var errIgnoredMessage = fmt.Errorf("Ignore this message")

const (
	textStyle = "text"
)

// Client is a chat client
type Client struct {
	apiClient *slack.Client
	// TODO: remove the rtm as it should only be inside the message matcher.
	// It should simply be inside there and it should pop messages matched out
	// through a channel
	rtm     *slack.RTM
	matcher messageMatcher
}

// ParseChannelLink implements the messenger.MessengerClient interface
func (c Client) ParseChannelLink(channel string) (string, error) {
	r, err := regexp.Compile(`<#(.*)\\|.*>`)
	if err != nil {
		return "", fmt.Errorf("could not compile regex for parsing the channel link: %s", err)
	}
	mm := r.FindStringSubmatch(channel)
	if len(mm) != 2 {
		return "", fmt.Errorf("invalid channel link: %s", channel)
	}
	return mm[1], nil
}

// ParseUserLink implements the messenger.MessengerClient interface
func (c Client) ParseUserLink(userLink string) (string, error) {
	r, err := regexp.Compile("<@(.*)>")
	if err != nil {
		return "", fmt.Errorf("could not compile regex for parsing the user link: %s", err)
	}
	mm := r.FindStringSubmatch(userLink)
	if len(mm) != 2 {
		return "", fmt.Errorf("invalid user link: %s", userLink)
	}
	return mm[1], nil
}

// GetUsername implements the messenger.MessengerClient interface
func (c Client) GetUsername(userID string) string {
	return c.matcher.getUser(userID)
}

// GetUserLink implements the messenger.MessengerClient interface
func (c Client) GetUserLink(userID string) string {
	return fmt.Sprintf("<@%s>", userID)
}

// GetChannel implements the messenger.MessengerClient interface
func (c Client) GetChannel(channelID string) string {
	return c.matcher.getChannel(channelID)
}

// GetChannelLink implements the messenger.MessengerClient interface
func (c Client) GetChannelLink(channelID string) string {
	return fmt.Sprintf("<#%s|%s>", channelID, c.matcher.getChannel(channelID))
}

// IsIM implements the messenger.MessengerClient interface
func (c Client) IsIM(channelID string) bool {
	return c.matcher.isIMChannel(channelID)
}

func (c Client) getReplyStyle(style string) replyStyle {
	switch style {
	case textStyle:
		return textReplyStyle{client: c.apiClient}
	default:
		return attachmentReplyStyle{client: c.apiClient}
	}
}

// ConnectionOpts groups all the connection options in a single struct
type ConnectionOpts struct {
	Debug   bool
	Token   string
	Stealth bool
}

// Connect builds a new chat client
func Connect(opts ConnectionOpts) (*Client, error) {
	if opts.Token == "" {
		return nil, fmt.Errorf("could not connect to slack: SLACK_TOKEN env var is empty")
	}

	slackClient := slack.New(opts.Token)
	slackClient.SetDebug(opts.Debug)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := slackClient.AuthTestContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("could not connect to slack: %s", err)
	}

	rtm := slackClient.NewRTM()
	go rtm.ManageConnection()

	if opts.Stealth {
		logrus.Info("Running in stealth mode")
		rtm.SetUserPresence("away")
	}

	return &Client{
		apiClient: slackClient,
		rtm:       rtm,
		matcher:   newMessageMatcher(rtm, opts.Stealth),
	}, nil
}

type messageMatcher struct {
	botID         string
	prefixMatches []string
	rtm           *slack.RTM
	stealth       bool
}

func newMessageMatcher(rtm *slack.RTM, stealth bool) messageMatcher {
	return messageMatcher{
		rtm:     rtm,
		stealth: stealth,
	}
}

// GetUser finds the username given a userID
func (m *messageMatcher) getUser(userID string) string {
	u, err := m.rtm.GetUserInfo(userID)
	if err != nil {
		logrus.Errorf("could not find user with id %s because %s, weeeird", userID, err)
		return "unknown-user"
	}
	return u.Name
}

func (m *messageMatcher) isIMChannel(channel string) bool {
	return strings.HasPrefix(channel, "D")
}

func (m *messageMatcher) shouldIgnoreUser(userID string) bool {
	if m.stealth {
		return !auth.IsKnownUser(m.getUser(userID))
	}
	return false
}

func (m *messageMatcher) shouldIgnoreChannel(channel string) bool {
	if m.stealth {
		return !m.isIMChannel(channel)
	}
	return false
}

// GetChannel returns a channel name given an ID
func (m *messageMatcher) getChannel(channelID string) string {
	if m.isIMChannel(channelID) {
		return "IM"
	}

	ch, err := m.rtm.GetChannelInfo(channelID)
	if err != nil {
		logrus.Errorf("could not find channel with id %s: %s", channelID, err)
		return "unknown-channel"
	}
	return ch.Name
}

// Init has to be delayed until the point in which the RTM is actually working.
// The simples way to do this lazily is to do it when the message listening starts
func (m *messageMatcher) init() {
	if m.botID == "" {
		m.botID = m.rtm.GetInfo().User.ID
		m.prefixMatches = []string{fmt.Sprintf("<@%s>", m.botID)}
	}
}

func (m *messageMatcher) Matches(message *slack.MessageEvent) (Message, error) {
	m.init()

	if text, ok := m.shouldCare(message); ok {
		username := m.getUser(message.User)
		channel := m.getChannel(message.Channel)
		isIM := m.isIMChannel(message.Channel)

		return Message{
			text:      text,
			userID:    message.User,
			channelID: message.Channel,
			username:  username,
			channel:   channel,
			isIM:      isIM,
		}, nil
	}
	return Message{}, errIgnoredMessage
}

func (m *messageMatcher) isMyself(message *slack.MessageEvent) bool {
	return message.User == m.botID
}

func (m *messageMatcher) shouldCare(message *slack.MessageEvent) (string, bool) {
	if m.isMyself(message) {
		logrus.Debug("It's myself, ignoring message")
		return "", false
	}
	if m.shouldIgnoreUser(message.User) {
		logrus.Debugf("Received message '%s' from unknown user %s while in stealth mode, ignoring",
			message.Text, m.getUser(message.User))
		return "", false
	}
	if m.shouldIgnoreChannel(message.Channel) {
		logrus.Debugf("Received message '%s' in public channel %s while in stealth mode, ignoring",
			message.Text, m.getChannel(message.Channel))
		return "", false
	}
	if m.isIMChannel(message.Channel) {
		logrus.Debugf("Channel %s is IM channel, responding...", message.Channel)
		return message.Text, true
	}
	for _, match := range m.prefixMatches {
		if strings.HasPrefix(message.Text, match) {
			logrus.Debugf("Message '%s' matches prefix, responding...", message.Text)
			return strings.TrimSpace(message.Text[len(match):]), true
		}
	}
	return "", false
}

// ListenMessages listens to messages and sends the matching ones through the channel
func (c *Client) ListenMessages(ch chan<- meeseeks.Message) {
	logrus.Infof("Listening Slack RTM Messages")

	for msg := range c.rtm.IncomingEvents {
		switch ev := msg.Data.(type) {
		case *slack.MessageEvent:
			message, err := c.matcher.Matches(ev)
			if err != nil {
				continue
			}

			logrus.Debugf("Sending Slack message %#v to messages channel", message)
			ch <- message

		default:
			logrus.Debugf("Ignored Slack Event %#v", ev)
		}
	}
	logrus.Infof("Stopped listening to messages")
}

// Reply replies to the user building a regular message
func (c *Client) Reply(r formatter.Reply) {

	c.getReplyStyle(r.ReplyStyle()).Reply(r)
}

type replyStyle interface {
	Reply(formatter.Reply)
}

type attachmentReplyStyle struct {
	client *slack.Client
}

func (a attachmentReplyStyle) Reply(r formatter.Reply) {
	content, err := r.Render()
	if err != nil {
		logrus.Errorf("failed to render reply %#v: %s", r, err)
		return
	}
	color := r.Color()

	logrus.Debugf("Rendering attachment with content %s with color %s", content, color)

	params := slack.PostMessageParameters{
		AsUser: true,
		Attachments: []slack.Attachment{
			{
				Text:       content,
				Color:      color,
				MarkdownIn: []string{"text"},
			},
		},
	}
	logrus.Debugf("Replying in Slack %s with %#v", r.ChannelID(), params)
	if _, _, err = a.client.PostMessage(r.ChannelID(), "", params); err != nil {
		logrus.Errorf("failed post attachment message %s on %s: %s", content, r.ChannelID(), err)
	}
}

type textReplyStyle struct {
	client *slack.Client
}

func (t textReplyStyle) Reply(r formatter.Reply) {
	content, err := r.Render()
	if err != nil {
		logrus.Errorf("failed to render reply %#v: %s", r, err)
		return
	}

	params := slack.PostMessageParameters{
		AsUser:      true,
		Markdown:    true,
		UnfurlLinks: true,
		UnfurlMedia: true,
	}
	logrus.Debugf("Replying in Slack %s with %#v and text: %s", r.ChannelID(), params, content)
	if _, _, err = t.client.PostMessage(r.ChannelID(), content, params); err != nil {
		logrus.Errorf("failed post message %s on %s: %s", content, r.ChannelID(), err)
	}
}

// Message a chat message
type Message struct {
	text      string
	channel   string
	channelID string
	username  string
	userID    string
	isIM      bool
}

// GetText returns the message text
func (m Message) GetText() string {
	return m.text
}

// GetUserID returns the user ID
func (m Message) GetUserID() string {
	return m.userID
}

// GetUserLink returns the user id formatted for using in a slack message
func (m Message) GetUserLink() string {
	return fmt.Sprintf("<@%s>", m.userID)
}

// GetUsername returns the user friendly username
func (m Message) GetUsername() string {
	return m.username
}

// GetChannelID returns the channel id from the which the message was sent
func (m Message) GetChannelID() string {
	return m.channelID
}

// GetChannel returns the channel from which the message was sent
func (m Message) GetChannel() string {
	return m.channel
}

// GetChannelLink returns the channel that slack will turn into a link
func (m Message) GetChannelLink() string {
	return fmt.Sprintf("<#%s|%s>", m.channelID, m.channel)
}

// IsIM returns if the message is an IM message
func (m Message) IsIM() bool {
	return m.isIM
}
