package messages_test

import (
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/matthewaveryusa/sqlchatgpt/internal/messages"
	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/assert"
)

func TestMessages(t *testing.T) {
	// Prepare the in-memory SQLite database.
	db, err := sqlx.Connect("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("failed to connect to in-memory sqlite3 database: %v", err)
	}
	defer db.Close()

	// Test creating a new Messages instance.
	instanceHierarchy := "Test"
	msgs, err := messages.New(db, instanceHierarchy)
	if err != nil {
		t.Fatalf("failed to create new Messages instance: %v", err)
	}

	// Test appending a message.
	message := "Hello, World!"
	err = msgs.Append("user", message)
	if err != nil {
		t.Fatalf("failed to append a message: %v", err)
	}

	// Test retrieving messages.
	retrievedMessages, err := msgs.Get()
	if err != nil {
		t.Fatalf("failed to get messages: %v", err)
	}

	assert.Equal(t, 1, len(retrievedMessages))
	assert.Equal(t, message, retrievedMessages[0].Message)
	assert.True(t, time.Since(retrievedMessages[0].Timestamp.T) < 1*time.Minute)
	assert.Equal(t, msgs.Instance, retrievedMessages[0].Instance)

	// Test clearing messages.
	err = msgs.Clear()
	if err != nil {
		t.Fatalf("failed to clear messages: %v", err)
	}

	retrievedMessages, err = msgs.Get()
	if err != nil {
		t.Fatalf("failed to get messages after clearing: %v", err)
	}

	assert.Equal(t, 0, len(retrievedMessages))
}
