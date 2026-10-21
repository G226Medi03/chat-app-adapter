package discord

import (
	"github.com/bwmarrin/discordgo"

	"github.com/G226Medi03/chat-app-adapter/chat"
)

// toAttachments converts discordgo attachments into the platform-neutral form.
func toAttachments(in []*discordgo.MessageAttachment) []chat.Attachment {
	if len(in) == 0 {
		return nil
	}
	out := make([]chat.Attachment, 0, len(in))
	for _, a := range in {
		if a == nil {
			continue
		}
		out = append(out, chat.Attachment{
			ID:          a.ID,
			Name:        a.Filename,
			URL:         a.URL,
			Size:        int64(a.Size),
			ContentType: a.ContentType,
		})
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
