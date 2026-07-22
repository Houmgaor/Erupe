package channelserver

import (
	"errors"
	"testing"

	"go.uber.org/zap"
)

func TestMailService_Send(t *testing.T) {
	mock := &mockMailRepo{}
	logger, _ := zap.NewDevelopment()
	svc := NewMailService(mock, nil, logger)

	err := svc.Send(1, 42, "Hello", "World", 500, 3)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if len(mock.sentMails) != 1 {
		t.Fatalf("Expected 1 mail, got %d", len(mock.sentMails))
	}
	m := mock.sentMails[0]
	if m.senderID != 1 {
		t.Errorf("SenderID = %d, want 1", m.senderID)
	}
	if m.recipientID != 42 {
		t.Errorf("RecipientID = %d, want 42", m.recipientID)
	}
	if m.subject != "Hello" {
		t.Errorf("Subject = %q, want %q", m.subject, "Hello")
	}
	if m.itemID != 500 {
		t.Errorf("ItemID = %d, want 500", m.itemID)
	}
	if m.itemAmount != 3 {
		t.Errorf("Quantity = %d, want 3", m.itemAmount)
	}
	if m.isGuildInvite || m.isSystemMessage {
		t.Error("Should not be guild invite or system message")
	}
}

func TestMailService_Send_Error(t *testing.T) {
	mock := &mockMailRepo{sendErr: errors.New("db fail")}
	logger, _ := zap.NewDevelopment()
	svc := NewMailService(mock, nil, logger)

	err := svc.Send(1, 42, "Hello", "World", 0, 0)
	if err == nil {
		t.Fatal("Expected error, got nil")
	}
}

func TestMailService_SendSystem(t *testing.T) {
	mock := &mockMailRepo{}
	logger, _ := zap.NewDevelopment()
	svc := NewMailService(mock, nil, logger)

	err := svc.SendSystem(42, "System Alert", "Something happened")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if len(mock.sentMails) != 1 {
		t.Fatalf("Expected 1 mail, got %d", len(mock.sentMails))
	}
	m := mock.sentMails[0]
	if m.senderID != 0 {
		t.Errorf("SenderID = %d, want 0 (system)", m.senderID)
	}
	if m.recipientID != 42 {
		t.Errorf("RecipientID = %d, want 42", m.recipientID)
	}
	if !m.isSystemMessage {
		t.Error("Should be system message")
	}
	if m.isGuildInvite {
		t.Error("Should not be guild invite")
	}
}

func TestMailService_SendGuildInvite(t *testing.T) {
	mock := &mockMailRepo{}
	logger, _ := zap.NewDevelopment()
	svc := NewMailService(mock, nil, logger)

	err := svc.SendGuildInvite(1, 42, "Invite", "Join us")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if len(mock.sentMails) != 1 {
		t.Fatalf("Expected 1 mail, got %d", len(mock.sentMails))
	}
	m := mock.sentMails[0]
	if !m.isGuildInvite {
		t.Error("Should be guild invite")
	}
	if m.isSystemMessage {
		t.Error("Should not be system message")
	}
}

func TestMailService_BroadcastToGuild(t *testing.T) {
	mailMock := &mockMailRepo{}
	guildMock := &mockGuildRepo{
		members: []*GuildMember{
			{CharID: 100},
			{CharID: 200},
			{CharID: 300},
		},
	}
	logger, _ := zap.NewDevelopment()
	svc := NewMailService(mailMock, guildMock, logger)

	err := svc.BroadcastToGuild(1, 10, "News", "Update")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if len(mailMock.sentMails) != 3 {
		t.Fatalf("Expected 3 mails, got %d", len(mailMock.sentMails))
	}
	recipients := map[uint32]bool{}
	for _, m := range mailMock.sentMails {
		recipients[m.recipientID] = true
		if m.senderID != 1 {
			t.Errorf("SenderID = %d, want 1", m.senderID)
		}
	}
	if !recipients[100] || !recipients[200] || !recipients[300] {
		t.Errorf("Expected recipients 100, 200, 300, got %v", recipients)
	}
}

func TestMailService_BroadcastToGuild_GetMembersError(t *testing.T) {
	mailMock := &mockMailRepo{}
	guildMock := &mockGuildRepo{getMembersErr: errors.New("db fail")}
	logger, _ := zap.NewDevelopment()
	svc := NewMailService(mailMock, guildMock, logger)

	err := svc.BroadcastToGuild(1, 10, "News", "Update")
	if err == nil {
		t.Fatal("Expected error, got nil")
	}
	if len(mailMock.sentMails) != 0 {
		t.Errorf("No mails should be sent on error, got %d", len(mailMock.sentMails))
	}
}

func TestMailService_BroadcastToGuild_SendError(t *testing.T) {
	mailMock := &mockMailRepo{sendErr: errors.New("db fail")}
	guildMock := &mockGuildRepo{
		members: []*GuildMember{
			{CharID: 100},
		},
	}
	logger, _ := zap.NewDevelopment()
	svc := NewMailService(mailMock, guildMock, logger)

	err := svc.BroadcastToGuild(1, 10, "News", "Update")
	if err == nil {
		t.Fatal("Expected error, got nil")
	}
}
