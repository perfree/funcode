package conversation

import (
	"time"

	"github.com/perfree/funcode/pkg/types"
)

// StoredMessage is a message with storage metadata
type StoredMessage struct {
	ID        string              `json:"id"`
	Timestamp time.Time           `json:"ts"`
	Role      types.Role          `json:"role"`
	Content   []types.ContentBlock `json:"content"`
}

// ToMessage converts to a types.Message
func (sm *StoredMessage) ToMessage() types.Message {
	return types.Message{
		Role:    sm.Role,
		Content: sm.Content,
	}
}

// FromMessage creates a StoredMessage from a types.Message
func FromMessage(id string, msg types.Message) *StoredMessage {
	return &StoredMessage{
		ID:        id,
		Timestamp: time.Now(),
		Role:      msg.Role,
		Content:   msg.Content,
	}
}
