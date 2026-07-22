package channelserver

import (
	"encoding/binary"
	"errors"
	"testing"

	"erupe-ce/network/mhfpacket"
)

// --- mockScenarioRepo ---

type mockScenarioRepo struct {
	scenarios []Scenario
	err       error
}

func (m *mockScenarioRepo) GetCounters() ([]Scenario, error) {
	return m.scenarios, m.err
}

func TestHandleMsgMhfInfoScenarioCounter_Empty(t *testing.T) {
	server := createMockServer()
	server.scenarioRepo = &mockScenarioRepo{}
	session := createMockSession(1, server)

	pkt := &mhfpacket.MsgMhfInfoScenarioCounter{AckHandle: 100}
	handleMsgMhfInfoScenarioCounter(session, pkt)

	select {
	case p := <-session.sendPackets:
		_, errCode, ackData := parseAckBufData(t, p.data)
		if errCode != 0 {
			t.Errorf("ErrorCode = %d, want 0", errCode)
		}
		if len(ackData) < 1 {
			t.Fatal("AckData too short")
		}
		if ackData[0] != 0 {
			t.Errorf("scenario count = %d, want 0", ackData[0])
		}
	default:
		t.Fatal("No response queued")
	}
}

func TestHandleMsgMhfInfoScenarioCounter_WithScenarios(t *testing.T) {
	server := createMockServer()
	server.scenarioRepo = &mockScenarioRepo{
		scenarios: []Scenario{
			{MainID: 1000, CategoryID: 0},
			{MainID: 2000, CategoryID: 3},
			{MainID: 3000, CategoryID: 6},
			{MainID: 4000, CategoryID: 7},
			{MainID: 5000, CategoryID: 1},
		},
	}
	session := createMockSession(1, server)

	pkt := &mhfpacket.MsgMhfInfoScenarioCounter{AckHandle: 100}
	handleMsgMhfInfoScenarioCounter(session, pkt)

	select {
	case p := <-session.sendPackets:
		_, _, data := parseAckBufData(t, p.data)
		if len(data) < 1 {
			t.Fatal("AckData too short")
		}
		count := data[0]
		if count != 5 {
			t.Errorf("scenario count = %d, want 5", count)
		}

		// Each scenario: mainID(4) + exchange(1) + categoryID(1) = 6 bytes
		expectedLen := 1 + 5*6
		if len(data) != expectedLen {
			t.Errorf("AckData len = %d, want %d", len(data), expectedLen)
		}

		// Verify first scenario (categoryID=0, exchange=false)
		mainID := binary.BigEndian.Uint32(data[1:5])
		if mainID != 1000 {
			t.Errorf("first mainID = %d, want 1000", mainID)
		}
		if data[5] != 0 {
			t.Errorf("categoryID=0 should have exchange=false, got %d", data[5])
		}

		// Verify second scenario (categoryID=3, exchange=true)
		if data[5+6] != 1 {
			t.Errorf("categoryID=3 should have exchange=true, got %d", data[5+6])
		}
	default:
		t.Fatal("No response queued")
	}
}

func TestHandleMsgMhfInfoScenarioCounter_TrimTo128(t *testing.T) {
	server := createMockServer()
	scenarios := make([]Scenario, 200)
	for i := range scenarios {
		scenarios[i] = Scenario{MainID: uint32(i), CategoryID: 0}
	}
	server.scenarioRepo = &mockScenarioRepo{scenarios: scenarios}
	session := createMockSession(1, server)

	pkt := &mhfpacket.MsgMhfInfoScenarioCounter{AckHandle: 100}
	handleMsgMhfInfoScenarioCounter(session, pkt)

	select {
	case p := <-session.sendPackets:
		_, _, data := parseAckBufData(t, p.data)
		if data[0] != 128 {
			t.Errorf("scenario count = %d, want 128 (trimmed)", data[0])
		}
	default:
		t.Fatal("No response queued")
	}
}

func TestHandleMsgMhfInfoScenarioCounter_DBError(t *testing.T) {
	server := createMockServer()
	server.scenarioRepo = &mockScenarioRepo{err: errors.New("db error")}
	session := createMockSession(1, server)

	pkt := &mhfpacket.MsgMhfInfoScenarioCounter{AckHandle: 100}
	handleMsgMhfInfoScenarioCounter(session, pkt)

	select {
	case p := <-session.sendPackets:
		if len(p.data) == 0 {
			t.Fatal("Should still respond on error")
		}
	default:
		t.Fatal("No response queued")
	}
}

func TestHandleMsgMhfInfoScenarioCounter_CategoryExchangeFlags(t *testing.T) {
	tests := []struct {
		name       string
		categoryID uint8
		wantExch   bool
	}{
		{"Basic", 0, false},
		{"Veteran", 1, false},
		{"Other (exchange)", 3, true},
		{"Pallone (exchange)", 6, true},
		{"Diva (exchange)", 7, true},
		{"Unknown category 2", 2, false},
		{"Unknown category 4", 4, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := createMockServer()
			server.scenarioRepo = &mockScenarioRepo{
				scenarios: []Scenario{{MainID: 1, CategoryID: tt.categoryID}},
			}
			session := createMockSession(1, server)

			pkt := &mhfpacket.MsgMhfInfoScenarioCounter{AckHandle: 100}
			handleMsgMhfInfoScenarioCounter(session, pkt)

			select {
			case p := <-session.sendPackets:
				_, _, data := parseAckBufData(t, p.data)
				isExchange := data[5] != 0
				if isExchange != tt.wantExch {
					t.Errorf("exchange = %v, want %v for categoryID=%d", isExchange, tt.wantExch, tt.categoryID)
				}
			default:
				t.Fatal("No response queued")
			}
		})
	}
}
