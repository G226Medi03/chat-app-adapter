# chat-app-adapter

A small, platform-agnostic chat abstraction for Go.

A host application drives a `chat.App` without knowing which messaging platform
is behind it. Each platform is a subpackage adapter; adding a messenger means
adding one adapter, not changing the host.

```
chat        the App interface and shared types (Message, Input, Channel, ...)
chat/discord  discordgo-backed adapter
chatcmd     platform-neutral command registry (/name and !name triggers)
chunk       text splitting for per-message character limits
```

## Install

```sh
go get github.com/G226Medi03/chat-app-adapter
```

## Usage

```go
import (
	"context"

	"github.com/G226Medi03/chat-app-adapter/chat"
	"github.com/G226Medi03/chat-app-adapter/chat/discord"
)

var app chat.App = discord.New(discord.Options{
	Token:   token,
	AppID:   appID,
	GuildID: guildID,
	Logger:  logger, // any chat.Logger, or nil to disable logging
})

app.OnMessage(func(m chat.Message) {
	_, _ = app.Reply(m.ChannelID, m.ID, "hello")
})

_ = app.Run(context.Background())
```

## Adding a platform

Implement `chat.App` in a new subpackage (for example `chat/telegram`) and pass
it to the host. Adapters must be safe for concurrent use and may invoke
`OnMessage`/`OnCommand` handlers from any goroutine.

## License

MIT
