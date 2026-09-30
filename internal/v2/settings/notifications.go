package settings

// Notifications is Settings → General → Notifications: the channels
// notices go out on and the events that send one. Each is on unless
// switched off.
type Notifications struct {
	MacOS          *bool `yaml:"macos,omitempty"`
	Bell           *bool `yaml:"bell,omitempty"`
	ActionRequired *bool `yaml:"action_required,omitempty"`
	Error          *bool `yaml:"error,omitempty"`
	PROpened       *bool `yaml:"pr_opened,omitempty"`
	PRSettled      *bool `yaml:"pr_settled,omitempty"`
}

// The channels and events, as keys of the notifications section.
const (
	NotifyMacOS          = "macos"
	NotifyBell           = "bell"
	NotifyActionRequired = "action_required"
	NotifyError          = "error"
	NotifyPROpened       = "pr_opened"
	NotifyPRSettled      = "pr_settled"
)

// On reports whether the channel or event key is on. An unknown key
// is off.
func (n Notifications) On(key string) bool {
	f := n.field(key)
	return f != nil && (*f == nil || **f)
}

// Set switches the channel or event key on or off. On is the default,
// so it isn't written down.
func (n Notifications) Set(key string, on bool) Notifications {
	if f := n.field(key); f != nil {
		*f = nil
		if !on {
			*f = &on
		}
	}
	return n
}

func (n *Notifications) field(key string) **bool {
	switch key {
	case NotifyMacOS:
		return &n.MacOS
	case NotifyBell:
		return &n.Bell
	case NotifyActionRequired:
		return &n.ActionRequired
	case NotifyError:
		return &n.Error
	case NotifyPROpened:
		return &n.PROpened
	case NotifyPRSettled:
		return &n.PRSettled
	}
	return nil
}
