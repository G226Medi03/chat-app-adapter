// Package discord implements chat.App on top of discordgo.
package discord

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"

	"github.com/G226Medi03/chat-app-adapter/chat"
	"github.com/G226Medi03/chat-app-adapter/chunk"
)

// Options configures the Discord client.
type Options struct {
	Token   string
	AppID   string
	GuildID string
	Logger  chat.Logger
}

// Client is a discordgo-backed chat.App implementation.
type Client struct {
	opts       Options
	log        chat.Logger
	discord_go *discordgo.Session

	mu      sync.Mutex
	selfID  string
	onMsg   func(chat.Message)
	onCmd   func(chat.Input)
	waiters map[string]*waiter
}

var _ chat.App = (*Client)(nil)

type waiter struct {
	ch      chan chat.Choice
	allowed func(string) bool
}

// nopLogger is used when Options.Logger is nil.
type nopLogger struct{}

func (nopLogger) Debugf(string, ...any) {}
func (nopLogger) Infof(string, ...any)  {}
func (nopLogger) Warnf(string, ...any)  {}
func (nopLogger) Errorf(string, ...any) {}

// New creates a Discord client (does not connect).
func New(opts Options) *Client {
	log := opts.Logger
	if log == nil {
		log = nopLogger{}
	}
	return &Client{opts: opts, log: log, waiters: map[string]*waiter{}}
}

// BotID returns the bot user id ("" before connect).
func (self *Client) BotID() string {
	self.mu.Lock()
	defer self.mu.Unlock()

	if self.selfID != "" {
		return self.selfID
	}
	if self.discord_go != nil && self.discord_go.State != nil && self.discord_go.State.User != nil {
		return self.discord_go.State.User.ID
	}
	return ""
}

// Channel returns metadata for a channel.
func (self *Client) Channel(id string) (chat.Channel, error) {
	out := chat.Channel{ID: id}
	if self.discord_go == nil {
		return out, fmt.Errorf("discord not connected")
	}
	var ch *discordgo.Channel
	if v, err := self.discord_go.State.Channel(id); err == nil && v != nil {
		ch = v
	} else if v, err := self.discord_go.Channel(id); err == nil {
		ch = v
	}
	if ch == nil {
		return out, fmt.Errorf("channel not found: %s", id)
	}
	out.Name = ch.Name
	out.ParentID = ch.ParentID
	out.GuildID = ch.GuildID

	switch ch.Type {
	case discordgo.ChannelTypeDM, discordgo.ChannelTypeGroupDM:
		out.IsDM = true
	case discordgo.ChannelTypeGuildPublicThread, discordgo.ChannelTypeGuildPrivateThread, discordgo.ChannelTypeGuildNewsThread:
		out.IsThread = true
	}
	if out.IsThread && out.ParentID != "" {
		if p, err := self.discord_go.Channel(out.ParentID); err == nil && p != nil {
			out.ParentName = p.Name
		}
	}
	return out, nil
}

func truncate(s string) string {
	return chunk.Head(s, 2000)
}

// Send posts a message.
func (self *Client) Send(channelID, content string) (chat.MessageRef, error) {
	m, err := self.discord_go.ChannelMessageSendComplex(channelID, &discordgo.MessageSend{Content: truncate(content), AllowedMentions: noMentions()})
	if err != nil {
		return chat.MessageRef{}, err
	}
	return chat.MessageRef{ChannelID: m.ChannelID, MessageID: m.ID}, nil
}

// SendFile posts a message with one file attached.
func (self *Client) SendFile(channelID, content, path string) (chat.MessageRef, error) {
	f, err := os.Open(path)
	if err != nil {
		return chat.MessageRef{}, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return chat.MessageRef{}, err
	}
	if st.IsDir() {
		return chat.MessageRef{}, fmt.Errorf("not a file: %s", path)
	}
	m, err := self.discord_go.ChannelMessageSendComplex(channelID, &discordgo.MessageSend{
		Content:         truncate(content),
		Files:           []*discordgo.File{{Name: st.Name(), Reader: f}},
		AllowedMentions: noMentions(),
	})
	if err != nil {
		if isFileTooLarge(err) {
			return chat.MessageRef{}, fmt.Errorf("%w: %s (%d bytes)", chat.ErrFileTooLarge, st.Name(), st.Size())
		}
		return chat.MessageRef{}, err
	}
	return chat.MessageRef{ChannelID: m.ChannelID, MessageID: m.ID}, nil
}

// isFileTooLarge reports whether Discord rejected the upload for its size.
func isFileTooLarge(err error) bool {
	var re *discordgo.RESTError
	if errors.As(err, &re) {
		if re.Response != nil && re.Response.StatusCode == http.StatusRequestEntityTooLarge {
			return true
		}
		if re.Message != nil && re.Message.Code == 40005 {
			return true
		}
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "40005") || strings.Contains(msg, "request entity too large") ||
		strings.Contains(msg, "too large") || strings.Contains(msg, "413")
}

// Reply posts a message as a reply to replyTo.
func (self *Client) Reply(channelID string, replyTo string, content string) (chat.MessageRef, error) {
	m, err := self.discord_go.ChannelMessageSendComplex(channelID, &discordgo.MessageSend{
		Content:         truncate(content),
		Reference:       &discordgo.MessageReference{MessageID: replyTo, ChannelID: channelID},
		AllowedMentions: noMentions(),
	})
	if err != nil {
		return chat.MessageRef{}, err
	}
	return chat.MessageRef{ChannelID: m.ChannelID, MessageID: m.ID}, nil
}

// Edit replaces the content of a message.
func (self *Client) Edit(ref chat.MessageRef, content string) error {
	body := truncate(content)
	_, err := self.discord_go.ChannelMessageEditComplex(&discordgo.MessageEdit{
		ID: ref.MessageID, Channel: ref.ChannelID, Content: &body, AllowedMentions: noMentions(),
	})
	return err
}

// Delete removes a message.
func (self *Client) Delete(ref chat.MessageRef) error {
	return self.discord_go.ChannelMessageDelete(ref.ChannelID, ref.MessageID)
}

// SendButtons posts a message with an action row of buttons.
func (self *Client) SendButtons(channelID, content string, buttons []chat.Button) (chat.MessageRef, error) {
	row := discordgo.ActionsRow{Components: make([]discordgo.MessageComponent, 0, len(buttons))}
	for _, b := range buttons {
		row.Components = append(row.Components, discordgo.Button{
			Label: b.Label, Style: dgStyle(b.Style), CustomID: b.ID, Disabled: b.Disabled,
		})
	}
	m, err := self.discord_go.ChannelMessageSendComplex(channelID, &discordgo.MessageSend{
		Content: truncate(content), Components: []discordgo.MessageComponent{row}, AllowedMentions: noMentions(),
	})
	if err != nil {
		return chat.MessageRef{}, err
	}
	return chat.MessageRef{ChannelID: m.ChannelID, MessageID: m.ID}, nil
}

// ClearButtons removes components (and sets content).
func (self *Client) ClearButtons(ref chat.MessageRef, content string) error {
	body := truncate(content)
	empty := []discordgo.MessageComponent{}
	_, err := self.discord_go.ChannelMessageEditComplex(&discordgo.MessageEdit{
		ID: ref.MessageID, Channel: ref.ChannelID, Content: &body, Components: &empty,
	})
	return err
}

// StartThread creates a thread on messageID and returns it.
func (self *Client) StartThread(channelID, messageID, name string) (chat.Channel, error) {
	if len([]rune(name)) > 90 {
		name = string([]rune(name)[:90])
	}
	th, err := self.discord_go.MessageThreadStart(channelID, messageID, name, 1440)
	if err != nil {
		return chat.Channel{}, err
	}
	return chat.Channel{
		ID:       th.ID,
		Name:     th.Name,
		ParentID: th.ParentID,
		GuildID:  th.GuildID,
		IsThread: true,
	}, nil
}

// AwaitChoice waits for a button click on ref, filtered by allowed.
func (self *Client) AwaitChoice(ref chat.MessageRef, timeout time.Duration, allowed func(userID string) bool) (chat.Choice, error) {
	ch := make(chan chat.Choice, 1)
	self.mu.Lock()
	self.waiters[ref.MessageID] = &waiter{ch: ch, allowed: allowed}
	self.mu.Unlock()
	defer func() {
		self.mu.Lock()
		delete(self.waiters, ref.MessageID)
		self.mu.Unlock()
	}()

	select {
	case choice := <-ch:
		return choice, nil
	case <-time.After(timeout):
		return chat.Choice{}, fmt.Errorf("permission timeout")
	}
}

// OnMessage registers the inbound message handler.
func (self *Client) OnMessage(fn func(chat.Message)) {
	self.mu.Lock()
	self.onMsg = fn
	self.mu.Unlock()
}

// OnCommand registers the inbound slash-command handler.
func (self *Client) OnCommand(fn func(chat.Input)) {
	self.mu.Lock()
	self.onCmd = fn
	self.mu.Unlock()
}

func (self *Client) messageHandler() func(chat.Message) {
	self.mu.Lock()
	defer self.mu.Unlock()
	return self.onMsg
}

func (self *Client) commandHandler() func(chat.Input) {
	self.mu.Lock()
	defer self.mu.Unlock()
	return self.onCmd
}

// RegisterCommands overwrites the Discord application commands.
func (self *Client) RegisterCommands(specs []chat.CommandSpec) error {
	if self.opts.AppID == "" || self.opts.Token == "" {
		return fmt.Errorf("app id / token required")
	}
	if self.discord_go == nil {
		self.discord_go = self.newSession()
	}
	cmds := make([]*discordgo.ApplicationCommand, 0, len(specs))
	for _, s := range specs {
		ac := &discordgo.ApplicationCommand{Name: s.Name, Description: s.Description}
		for _, o := range s.Options {
			ac.Options = append(ac.Options, &discordgo.ApplicationCommandOption{
				Type: discordgo.ApplicationCommandOptionString, Name: o.Name, Description: o.Description, Required: o.Required,
			})
		}
		cmds = append(cmds, ac)
	}
	created, err := self.discord_go.ApplicationCommandBulkOverwrite(self.opts.AppID, self.opts.GuildID, cmds)
	if err != nil {
		return err
	}
	self.log.Infof("[deploy] 슬래시 커맨드 %d개 등록", len(created))
	return nil
}

func (self *Client) newSession() *discordgo.Session {
	dg, err := discordgo.New("Bot " + self.opts.Token)
	if err != nil {
		self.log.Errorf("[discord] 세션 생성 실패: %v", err)
		return nil
	}
	dg.Identify.Intents = discordgo.IntentsGuilds |
		discordgo.IntentsGuildMessages |
		discordgo.IntentsMessageContent |
		discordgo.IntentsDirectMessages
	return dg
}

// Run connects to Discord and blocks until ctx is cancelled.
func (self *Client) Run(ctx context.Context) error {
	if self.opts.Token == "" {
		return fmt.Errorf("DISCORD_BOT_TOKEN required")
	}
	if self.discord_go == nil {
		self.discord_go = self.newSession()
		if self.discord_go == nil {
			return fmt.Errorf("discord session init failed")
		}
	}
	self.discord_go.AddHandler(self.onReady)
	self.discord_go.AddHandler(self.onMessageCreate)
	self.discord_go.AddHandler(self.onInteraction)
	if err := self.discord_go.Open(); err != nil {
		return fmt.Errorf("gateway 연결 실패: %w", err)
	}
	<-ctx.Done()
	return nil
}

// Close disconnects from Discord.
func (self *Client) Close() error {
	if self.discord_go != nil {
		return self.discord_go.Close()
	}
	return nil
}

// ---- gateway handlers ----------------------------------------------------

func (self *Client) onReady(s *discordgo.Session, r *discordgo.Ready) {
	self.mu.Lock()
	self.selfID = r.User.ID
	self.mu.Unlock()
	self.log.Infof("[ready] %s (%s) · 길드 %d개", r.User.String(), r.User.ID, len(r.Guilds))
}

func (self *Client) onMessageCreate(s *discordgo.Session, m *discordgo.MessageCreate) {
	if m == nil || m.Author == nil || m.Author.Bot || m.WebhookID != "" {
		return
	}
	msg := chat.Message{
		ID:           m.ID,
		ChannelID:    m.ChannelID,
		AuthorID:     m.Author.ID,
		AuthorName:   m.Author.Username,
		Content:      m.Content,
		Attachments:  toAttachments(m.Attachments),
		MentionedBot: self.mentioned(m),
		GuildID:      m.GuildID,
		Timestamp:    m.Timestamp,
	}
	if ch, err := self.Channel(m.ChannelID); err == nil {
		msg.IsDM = ch.IsDM
	}
	if fn := self.messageHandler(); fn != nil {
		go fn(msg)
	}
}

func (self *Client) mentioned(m *discordgo.MessageCreate) bool {
	id := self.BotID()
	if id == "" {
		return false
	}
	for _, u := range m.Mentions {
		if u != nil && u.ID == id {
			return true
		}
	}
	return false
}

func (self *Client) onInteraction(s *discordgo.Session, i *discordgo.InteractionCreate) {
	switch i.Type {
	case discordgo.InteractionApplicationCommand:
		go self.handleSlash(i)
	case discordgo.InteractionMessageComponent:
		go self.handleComponent(i)
	}
}

func (self *Client) handleSlash(i *discordgo.InteractionCreate) {
	data := i.ApplicationCommandData()
	args := map[string]string{}
	for _, o := range data.Options {
		if o == nil {
			continue
		}
		if v, ok := o.Value.(string); ok {
			args[o.Name] = v
		}
	}
	responded := false
	reply := func(content string) {
		content = chunk.Head(content, 1990)
		if !responded {
			responded = true
			_ = self.discord_go.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
			})
		}
		_, _ = self.discord_go.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{Content: content, AllowedMentions: noMentions()})
	}
	if fn := self.commandHandler(); fn != nil {
		fn(chat.Input{
			Name: data.Name, Args: args, UserID: interactionUserID(i),
			ChannelID: i.ChannelID, IsSlash: true, Reply: reply,
		})
	}
}

func (self *Client) handleComponent(i *discordgo.InteractionCreate) {
	uid := interactionUserID(i)
	msgID := ""
	if i.Message != nil {
		msgID = i.Message.ID
	}
	self.mu.Lock()
	w := self.waiters[msgID]
	self.mu.Unlock()
	if w == nil {
		return
	}
	if w.allowed != nil && !w.allowed(uid) {
		_ = self.discord_go.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{Content: "권한이 없습니다(owner 전용).", Flags: discordgo.MessageFlagsEphemeral},
		})
		return
	}
	// Acknowledge by removing components (bridge sets the final content).
	empty := []discordgo.MessageComponent{}
	_ = self.discord_go.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseUpdateMessage,
		Data: &discordgo.InteractionResponseData{Components: empty},
	})
	select {
	case w.ch <- chat.Choice{ButtonID: i.MessageComponentData().CustomID, UserID: uid}:
	default:
	}
}

func interactionUserID(i *discordgo.InteractionCreate) string {
	if i.Member != nil && i.Member.User != nil {
		return i.Member.User.ID
	}
	if i.User != nil {
		return i.User.ID
	}
	return ""
}

func dgStyle(s chat.Style) discordgo.ButtonStyle {
	switch s {
	case chat.StylePrimary:
		return discordgo.PrimaryButton
	case chat.StyleSuccess:
		return discordgo.SuccessButton
	case chat.StyleDanger:
		return discordgo.DangerButton
	default:
		return discordgo.SecondaryButton
	}
}

func noMentions() *discordgo.MessageAllowedMentions {
	return &discordgo.MessageAllowedMentions{Parse: []discordgo.AllowedMentionType{}}
}
