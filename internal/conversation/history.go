package conversation

import (
	"crypto/sha256"
	"fmt"

	"github.com/google/uuid"
)

// generateID creates a unique message ID
func generateID() string {
	return "msg_" + uuid.New().String()[:8]
}

// GenerateSessionID creates a unique session ID
func GenerateSessionID() string {
	return uuid.New().String()
}

// HashProjectPath creates a hash from the project path
func HashProjectPath(path string) string {
	h := sha256.Sum256([]byte(path))
	return fmt.Sprintf("%x", h[:8])
}
