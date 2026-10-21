package discord

import (
	"testing"

	"github.com/bwmarrin/discordgo"
)

func TestToAttachments(t *testing.T) {
	if got := toAttachments(nil); got != nil {
		t.Fatalf("nil input: %#v", got)
	}
	in := []*discordgo.MessageAttachment{
		nil,
		{ID: "1", Filename: "a.pdf", URL: "https://cdn/a.pdf", Size: 1234, ContentType: "application/pdf"},
	}
	got := toAttachments(in)
	if len(got) != 1 {
		t.Fatalf("want 1 attachment, got %d", len(got))
	}
	a := got[0]
	if a.ID != "1" || a.Name != "a.pdf" || a.URL != "https://cdn/a.pdf" || a.Size != 1234 || a.ContentType != "application/pdf" {
		t.Fatalf("unexpected attachment: %#v", a)
	}
}
