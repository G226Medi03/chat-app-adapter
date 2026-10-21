package chat

import "errors"

// ErrFileTooLarge is returned by App.SendFile when the platform rejects an
// upload because it exceeds the platform's file size limit.
var ErrFileTooLarge = errors.New("chat: file too large")
