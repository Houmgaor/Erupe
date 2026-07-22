package channelserver

import (
	"testing"

	"erupe-ce/network/mhfpacket"
)

// emptyEnumerateRankingPayloadLen is the safe shape returned when there is no
// active tournament: 16 bytes of zeroed timestamps + 4 bytes now + 1 byte state +
// 2 bytes empty Pascal string (length 1 + null terminator) + 2 bytes numEvents +
// 1 byte numCups.
const emptyEnumerateRankingPayloadLen = 16 + 4 + 1 + 2 + 2 + 1

// Regression: a tournament row with non-positive timestamps used to be emitted
// to the client with state=3, crashing every ZZ quest counter
// (see Mezeporta/Erupe#193). The handler must treat such rows the same as no
// active tournament and return the empty 25-byte shape with state=0.
func TestHandleMsgMhfEnumerateRanking_ZeroTimestampsTreatedAsEmpty(t *testing.T) {
	cases := []struct {
		name string
		row  *Tournament
	}{
		{"all zero", &Tournament{ID: 1}},
		{"only reward_end set", &Tournament{ID: 2, RewardEnd: 9_999_999_999}},
		{"only start_time set", &Tournament{ID: 3, StartTime: 1}},
		{"negative ranking_end", &Tournament{ID: 4, StartTime: 1, EntryEnd: 2, RankingEnd: -1, RewardEnd: 3}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := createMockServer()
			server.tournamentRepo = &mockTournamentRepo{active: tc.row}
			session := createMockSession(1, server)

			handleMsgMhfEnumerateRanking(session, &mhfpacket.MsgMhfEnumerateRanking{AckHandle: 1})

			select {
			case p := <-session.sendPackets:
				// 10-byte ACK header: opcode(2) + ackHandle(4) + isBuffer(1) + errorCode(1) + payloadSize(2).
				const ackHeaderLen = 10
				if len(p.data) < ackHeaderLen+emptyEnumerateRankingPayloadLen {
					t.Fatalf("response too short: got %d bytes, want >= %d",
						len(p.data), ackHeaderLen+emptyEnumerateRankingPayloadLen)
				}
				payload := p.data[ackHeaderLen:]
				if len(payload) != emptyEnumerateRankingPayloadLen {
					t.Fatalf("payload length: got %d, want %d (any other length means a tournament was emitted)",
						len(payload), emptyEnumerateRankingPayloadLen)
				}
				// First 16 bytes must be zero (no leaked timestamps).
				for i := 0; i < 16; i++ {
					if payload[i] != 0 {
						t.Errorf("payload[%d] = %#x, want 0 (timestamp leak)", i, payload[i])
					}
				}
				// State byte (offset 20) must be 0 — emitting state=3 with zero timestamps
				// is exactly what crashes the client.
				if payload[20] != 0 {
					t.Errorf("state byte = %d, want 0", payload[20])
				}
			default:
				t.Fatal("no response packet queued")
			}
		})
	}
}

func TestTournamentState_RejectsNonPositiveTimestamps(t *testing.T) {
	now := int64(1_700_000_000)
	cases := []struct {
		name string
		t    *Tournament
	}{
		{"nil", nil},
		{"all zero", &Tournament{}},
		{"reward_end only", &Tournament{RewardEnd: now + 1000}},
		{"missing entry_end", &Tournament{StartTime: now - 1, RankingEnd: now + 1, RewardEnd: now + 2}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tournamentState(now, tc.t); got != 0 {
				t.Errorf("tournamentState = %d, want 0 for malformed row", got)
			}
		})
	}
}

func TestHandleMsgMhfInfoTournament_Type0(t *testing.T) {
	server := createMockServer()
	session := createMockSession(1, server)

	pkt := &mhfpacket.MsgMhfInfoTournament{
		AckHandle: 12345,
		QueryType: 0,
	}

	handleMsgMhfInfoTournament(session, pkt)

	// Verify response packet was queued
	select {
	case p := <-session.sendPackets:
		if len(p.data) == 0 {
			t.Error("Response packet should have data")
		}
	default:
		t.Error("No response packet queued")
	}
}

func TestHandleMsgMhfInfoTournament_Type1(t *testing.T) {
	server := createMockServer()
	session := createMockSession(1, server)

	pkt := &mhfpacket.MsgMhfInfoTournament{
		AckHandle: 12345,
		QueryType: 1,
	}

	handleMsgMhfInfoTournament(session, pkt)

	// Verify response packet was queued
	select {
	case p := <-session.sendPackets:
		if len(p.data) == 0 {
			t.Error("Response packet should have data")
		}
	default:
		t.Error("No response packet queued")
	}
}

func TestHandleMsgMhfEntryTournament(t *testing.T) {
	server := createMockServer()
	session := createMockSession(1, server)

	pkt := &mhfpacket.MsgMhfEntryTournament{
		AckHandle: 12345,
	}

	handleMsgMhfEntryTournament(session, pkt)

	select {
	case p := <-session.sendPackets:
		if len(p.data) == 0 {
			t.Error("Response packet should have data")
		}
	default:
		t.Error("No response packet queued")
	}
}

func TestHandleMsgMhfAcquireTournament(t *testing.T) {
	server := createMockServer()
	session := createMockSession(1, server)

	pkt := &mhfpacket.MsgMhfAcquireTournament{
		AckHandle: 12345,
	}

	handleMsgMhfAcquireTournament(session, pkt)

	select {
	case p := <-session.sendPackets:
		if len(p.data) == 0 {
			t.Error("Response packet should have data")
		}
	default:
		t.Error("No response packet queued")
	}
}
