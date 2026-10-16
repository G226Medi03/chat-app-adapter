// Package chat defines a platform-agnostic chat abstraction.
//
// An App is a connected messaging surface — Discord today, other messengers
// later — that a host application drives without knowing which platform is
// behind it. Platform implementations live in subpackages, for example
// chat/discord.
package chat

import (
	"context"
	"time"
)

// App is a chat platform the host talks to. Implementations must be safe for
// concurrent use: handlers registered with OnMessage and OnCommand may be
// invoked from any goroutine.
type App interface {
	// Run connects to the platform and blocks until ctx is cancelled.
	Run(ctx context.Context) error
	// Close disconnects from the platform.
	Close() error
	// BotID returns the connected bot's user id ("" before connect).
	BotID() string

	// OnMessage registers the inbound message handler.
	OnMessage(fn func(Message))
	// OnCommand registers the inbound command handler.
	OnCommand(fn func(Input))
	// RegisterCommands publishes the platform's slash-command definitions.
	RegisterCommands(specs []CommandSpec) error

	// Channel returns metadata for a channel.
	Channel(id string) (Channel, error)
	// StartThread creates a thread on messageID and returns it.
	StartThread(channelID, messageID, name string) (Channel, error)

	// Send posts a message.
	Send(channelID, content string) (MessageRef, error)
	// Reply posts a message as a reply to replyTo.
	Reply(channelID string, replyTo string, content string) (MessageRef, error)
	// Edit replaces the content of a message.
	Edit(ref MessageRef, content string) error

	// SendButtons posts a message with an action row of buttons.
	SendButtons(channelID, content string, buttons []Button) (MessageRef, error)
	// ClearButtons removes components (and sets content).
	ClearButtons(ref MessageRef, content string) error
	// AwaitChoice waits for a button click on ref, filtered by allowed.
	AwaitChoice(ref MessageRef, timeout time.Duration, allowed func(userID string) bool) (Choice, error)
}
