package template

import (
	"bytes"
	"fmt"
	"math/rand"
	"strings"

	humanize "github.com/dustin/go-humanize"
	log "github.com/sirupsen/logrus"
	tmpl "text/template"
)

// Template names used for rendering
const (
	Handshake      = "handshake"
	Success        = "success"
	Failure        = "failure"
	UnknownCommand = "unknowncommand"
	Unauthorized   = "unauthorized"
)

// Default command templates
var (
	DefaultHandshakeTemplate = fmt.Sprintf("{{ AnyValue \"%s\" . }}", Handshake)
	DefaultSuccessTemplate   = fmt.Sprintf("{{ .userlink }} {{ AnyValue \"%s\" . }}"+
		"{{ with $out := .output }}\n```\n{{ $out }}```{{ end }}", Success)
	DefaultFailureTemplate = fmt.Sprintf("{{ .userlink }} {{ AnyValue \"%s\" . }} :disappointed: {{ .error }}"+
		"{{ with $out := .output }}\n```\n{{ $out }}```{{ end }}", Failure)
	DefaultUnknownCommandTemplate = fmt.Sprintf("{{ .userlink }} {{ AnyValue \"%s\" . }} {{ .command }}",
		UnknownCommand)
	DefaultUnauthorizedTemplate = fmt.Sprintf("{{ .userlink }} {{ AnyValue \"%s\" . }} {{ .command }}: {{ .error }}",
		Unauthorized)
)

// GetDefaultTemplates returns a map with the default templates
func GetDefaultTemplates() map[string]string {
	return map[string]string{
		Handshake:      DefaultHandshakeTemplate,
		Success:        DefaultSuccessTemplate,
		Failure:        DefaultFailureTemplate,
		UnknownCommand: DefaultUnknownCommandTemplate,
		Unauthorized:   DefaultUnauthorizedTemplate,
	}
}

// Default messages
var (
	DefaultHandshakeMessages = []string{"I'm Mr Meeseeks! look at me!", "Mr Meeseeks!",
		"Ooh, yeah! Can do!", "Ooh, ok!", "Yes, siree!",
		"Ooh, I'm Mr. Meeseeks! Look at me!"}
	DefaultSuccessMessages        = []string{"All done!", "Mr Meeseeks", "Uuuuh, nice!"}
	DefaultFailedMessages         = []string{"Uuuh!, no, it failed"}
	DefaultUnauthorizedMessages   = []string{"Uuuuh, yeah! you are not allowed to do"}
	DefaultUnknownCommandMessages = []string{"Uuuh! no, I don't know how to do"}
)

// GetDefaultMessages returns a map with the default messages
func GetDefaultMessages() map[string][]string {
	return map[string][]string{
		Handshake:      DefaultHandshakeMessages,
		Success:        DefaultSuccessMessages,
		Failure:        DefaultFailedMessages,
		UnknownCommand: DefaultUnknownCommandMessages,
		Unauthorized:   DefaultUnauthorizedMessages,
	}
}

// Templates is a set of templates for the basic operations
type Templates struct {
	renderers      map[string]Renderer
	defaultPayload map[string]interface{}
}

// TemplatesBuilder is a helper object that is used to build the template renderers
type TemplatesBuilder struct {
	messages  map[string][]string
	templates map[string]string
}

// NewBuilder creates a new template builder fill with default values
func NewBuilder() *TemplatesBuilder {
	return &TemplatesBuilder{
		templates: GetDefaultTemplates(),
		messages:  GetDefaultMessages(),
	}
}

// WithMessages allows to change messages from the template builder
func (b *TemplatesBuilder) WithMessages(messages map[string][]string) *TemplatesBuilder {
	for name, message := range messages {
		b.messages[name] = message
	}
	return b
}

// WithTemplates allows to change templates from the template builder
func (b *TemplatesBuilder) WithTemplates(templates map[string]string) *TemplatesBuilder {
	for name, template := range templates {
		b.templates[name] = template
	}
	return b
}

// Clone returns a copy of this template builder
func (b *TemplatesBuilder) Clone() *TemplatesBuilder {
	return NewBuilder().WithMessages(b.messages).WithTemplates(b.templates)
}

// Build creates a Templates object will all the necessary renderers initialized
func (b *TemplatesBuilder) Build() Templates {
	renderers := make(map[string]Renderer)
	for name, template := range b.templates {
		renderer, err := New(name, template)
		if err != nil {
			log.Fatalf("could not parse %s template: %s", name, err)
		}
		renderers[name] = renderer
	}

	payload := map[string]interface{}{}
	for k, v := range b.messages {
		payload[k] = v
	}

	return Templates{
		renderers:      renderers,
		defaultPayload: payload,
	}
}

// Render renders the template matching the action with the passed in payload
func (t Templates) Render(action string, payload map[string]interface{}) (string, error) {
	p := t.newPayload()
	for k, v := range payload {
		p[k] = v
	}

	renderer, ok := t.renderers[action]
	if !ok {
		return "", fmt.Errorf("could not find action %s", action)
	}

	output, err := renderer.Render(p)
	if err != nil {
		return "", fmt.Errorf("failed to execute template %s: %s", renderer.template.Name(), err)
	}

	return output, nil
}

func (t Templates) newPayload() map[string]interface{} {
	p := map[string]interface{}{}
	for k, v := range t.defaultPayload {
		p[k] = v
	}
	return p
}

func anyValue(key string, payload map[string]interface{}) (string, error) {
	values, ok := payload[key]
	if !ok {
		return "", fmt.Errorf("%s is not loaded in the payload", key)
	}
	slice, ok := values.([]string)
	if !ok {
		return "", fmt.Errorf("%s is not a string slice", key)
	}
	return slice[rand.Intn(len(slice))], nil
}

// Renderer is a pre rendered template used to reply
type Renderer struct {
	template *tmpl.Template
}

// New creates a new ReplyTemplate pre-parsing the template
func New(name, template string) (Renderer, error) {
	t, err := tmpl.New(name).Funcs(tmpl.FuncMap{
		"AnyValue":       anyValue,
		"HumanizeTime":   humanize.Time,
		"HumanizeSize":   humanize.Bytes,
		"HumanizeNumber": humanize.Ftoa,
		"Join":           strings.Join,
	}).Parse(template)
	if err != nil {
		return Renderer{}, fmt.Errorf("could not parse template %s: %s", name, err)
	}
	return Renderer{
		template: t,
	}, nil
}

// Render renders the template with the passed in data
func (r Renderer) Render(data map[string]interface{}) (string, error) {
	b := bytes.NewBuffer([]byte{})
	err := r.template.Execute(b, data)

	if err != nil {
		return "", fmt.Errorf("failed to execute template %s: %s", r.template.Name(), err)
	}

	return b.String(), nil
}
