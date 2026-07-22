package channelserver

import (
	"erupe-ce/common/byteframe"
	"erupe-ce/network/mhfpacket"
	"testing"
)

func TestHandleMsgMhfGetRyoudama_Case4_RealPoints(t *testing.T) {
	srv := createMockServer()
	srv.caravanRepo = &mockCaravanRepo{points: CaravanPoints{Points: 4200}}
	s := createMockSession(100, srv)

	pkt := &mhfpacket.MsgMhfGetRyoudama{AckHandle: 1, Request2: 4}
	handleMsgMhfGetRyoudama(s, pkt)

	ack := readAck(t, s)
	if ack.ErrorCode != 0 {
		t.Fatalf("expected success ack, got error code %d", ack.ErrorCode)
	}
	bf := byteframe.NewByteFrameFromBytes(ack.Payload)
	_ = bf.ReadUint32() // EarthID
	_ = bf.ReadUint32() // reserved
	_ = bf.ReadUint32() // reserved
	count := bf.ReadUint32()
	if count != 1 {
		t.Fatalf("expected 1 entry, got %d", count)
	}
	points := bf.ReadInt32()
	if points != 4200 {
		t.Errorf("expected points=4200, got %d", points)
	}
}

func TestHandleMsgMhfGetRyoudama_Case5_Ranking(t *testing.T) {
	srv := createMockServer()
	srv.caravanRepo = &mockCaravanRepo{
		personalRank: []CaravanRankEntry{
			{CharID: 42, Name: "Hunter", Points: 900},
		},
	}
	s := createMockSession(100, srv)

	pkt := &mhfpacket.MsgMhfGetRyoudama{AckHandle: 1, Request2: 5}
	handleMsgMhfGetRyoudama(s, pkt)

	ack := readAck(t, s)
	bf := byteframe.NewByteFrameFromBytes(ack.Payload)
	_ = bf.ReadUint32()
	_ = bf.ReadUint32()
	_ = bf.ReadUint32()
	count := bf.ReadUint32()
	if count != 1 {
		t.Fatalf("expected 1 ranking entry, got %d", count)
	}
	cid := bf.ReadUint32()
	points := bf.ReadInt32()
	if cid != 42 || points != 900 {
		t.Errorf("expected cid=42 points=900, got cid=%d points=%d", cid, points)
	}
}

func TestHandleMsgMhfGetRyoudama_Case6_BoostInfoEmpty(t *testing.T) {
	srv := createMockServer()
	srv.caravanRepo = &mockCaravanRepo{}
	s := createMockSession(100, srv)

	pkt := &mhfpacket.MsgMhfGetRyoudama{AckHandle: 1, Request2: 6}
	handleMsgMhfGetRyoudama(s, pkt)

	ack := readAck(t, s)
	bf := byteframe.NewByteFrameFromBytes(ack.Payload)
	_ = bf.ReadUint32()
	_ = bf.ReadUint32()
	_ = bf.ReadUint32()
	count := bf.ReadUint32()
	if count != 0 {
		t.Errorf("expected 0 boost-info entries (no known data source), got %d", count)
	}
}

func TestHandleMsgMhfGetRyoudama_UnknownRequest2_NoEntries(t *testing.T) {
	srv := createMockServer()
	srv.caravanRepo = &mockCaravanRepo{}
	s := createMockSession(100, srv)

	pkt := &mhfpacket.MsgMhfGetRyoudama{AckHandle: 1}
	handleMsgMhfGetRyoudama(s, pkt)

	ack := readAck(t, s)
	if len(ack.Payload) == 0 {
		t.Error("expected a well-formed (if empty) ack payload")
	}
}

func TestHandleMsgMhfPostRyoudama_AcksInsteadOfSoftlocking(t *testing.T) {
	srv := createMockServer()
	s := createMockSession(100, srv)

	pkt := &mhfpacket.MsgMhfPostRyoudama{AckHandle: 7}
	handleMsgMhfPostRyoudama(s, pkt)

	ack := readAck(t, s)
	if ack.ErrorCode != 0 {
		t.Errorf("expected success ack (previously this handler sent no ack at all), got error code %d", ack.ErrorCode)
	}
	if ack.AckHandle != 7 {
		t.Errorf("expected ack handle 7, got %d", ack.AckHandle)
	}
}

func TestHandleMsgMhfGetTinyBin(t *testing.T) {
	server := createMockServer()
	session := createMockSession(1, server)

	pkt := &mhfpacket.MsgMhfGetTinyBin{
		AckHandle: 12345,
	}

	handleMsgMhfGetTinyBin(session, pkt)

	select {
	case p := <-session.sendPackets:
		if p.data == nil {
			t.Error("Response packet data should not be nil")
		}
	default:
		t.Error("No response packet queued")
	}
}

func TestHandleMsgMhfPostTinyBin(t *testing.T) {
	server := createMockServer()
	session := createMockSession(1, server)

	pkt := &mhfpacket.MsgMhfPostTinyBin{
		AckHandle: 12345,
	}

	handleMsgMhfPostTinyBin(session, pkt)

	select {
	case p := <-session.sendPackets:
		if len(p.data) == 0 {
			t.Error("Response packet should have data")
		}
	default:
		t.Error("No response packet queued")
	}
}

func TestHandleMsgMhfCaravanMyScore_StillEmpty(t *testing.T) {
	srv := createMockServer()
	s := createMockSession(100, srv)

	pkt := &mhfpacket.MsgMhfCaravanMyScore{AckHandle: 1}
	handleMsgMhfCaravanMyScore(s, pkt)

	ack := readAck(t, s)
	bf := byteframe.NewByteFrameFromBytes(ack.Payload)
	_ = bf.ReadUint32()
	_ = bf.ReadUint32()
	_ = bf.ReadUint32()
	count := bf.ReadUint32()
	if count != 0 {
		t.Errorf("expected 0 entries (wire format unconfirmed), got %d", count)
	}
}

func TestHandleMsgMhfCaravanRanking_StillEmpty(t *testing.T) {
	srv := createMockServer()
	s := createMockSession(100, srv)

	pkt := &mhfpacket.MsgMhfCaravanRanking{AckHandle: 1}
	handleMsgMhfCaravanRanking(s, pkt)

	ack := readAck(t, s)
	bf := byteframe.NewByteFrameFromBytes(ack.Payload)
	_ = bf.ReadUint32()
	_ = bf.ReadUint32()
	_ = bf.ReadUint32()
	count := bf.ReadUint32()
	if count != 0 {
		t.Errorf("expected 0 entries (wire format unconfirmed), got %d", count)
	}
}

func TestHandleMsgMhfCaravanMyRank_StillEmpty(t *testing.T) {
	srv := createMockServer()
	s := createMockSession(100, srv)

	pkt := &mhfpacket.MsgMhfCaravanMyRank{AckHandle: 1}
	handleMsgMhfCaravanMyRank(s, pkt)

	ack := readAck(t, s)
	bf := byteframe.NewByteFrameFromBytes(ack.Payload)
	_ = bf.ReadUint32()
	_ = bf.ReadUint32()
	_ = bf.ReadUint32()
	count := bf.ReadUint32()
	if count != 0 {
		t.Errorf("expected 0 entries (wire format unconfirmed), got %d", count)
	}
}
