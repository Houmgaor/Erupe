package channelserver

import (
	"erupe-ce/common/byteframe"
	"erupe-ce/common/stringsupport"
	"erupe-ce/network/mhfpacket"
	"time"

	"go.uber.org/zap"
)

// RyoudamaCharInfo represents one entry of the personal caravan ranking.
// Unk0 is populated with the character's caravan points -- this mirrors the
// pre-existing struct shape (CID/Unk0/Name) rather than a byte-confirmed
// field semantic.
type RyoudamaCharInfo struct {
	CID  uint32
	Unk0 int32
	Name string
}

// RyoudamaBoostInfo represents caravan boost status. No data source or wire
// layout is known for this yet -- always empty.
type RyoudamaBoostInfo struct {
	Start time.Time
	End   time.Time
}

func handleMsgMhfGetRyoudama(s *Session, p mhfpacket.MHFPacket) {
	pkt := p.(*mhfpacket.MsgMhfGetRyoudama)
	var data []*byteframe.ByteFrame
	switch pkt.Request2 {
	case 4:
		// Note: CharacterSaveData.CP (model_character.go) is a separate,
		// already-shipped "caravan points" value read/written directly from
		// the ZZ save blob -- it is NOT synced with caravanRepo's points
		// here. They likely represent the same real-world value but were
		// deliberately left unreconciled in this pass.
		points, err := s.server.caravanRepo.GetPoints(s.charID)
		if err != nil {
			s.logger.Error("Failed to get caravan points", zap.Error(err))
		}
		bf := byteframe.NewByteFrame()
		bf.WriteInt32(points.Points)
		data = append(data, bf)
	case 5:
		ranking, err := s.server.caravanRepo.GetPersonalRanking()
		if err != nil {
			s.logger.Error("Failed to get caravan personal ranking", zap.Error(err))
		}
		for _, entry := range ranking {
			bf := byteframe.NewByteFrame()
			bf.WriteUint32(entry.CharID)
			bf.WriteInt32(entry.Points)
			bf.WriteBytes(stringsupport.PaddedString(entry.Name, 14, true))
			data = append(data, bf)
		}
	case 6:
		// No known data source for caravan "boost" status; left empty.
	}
	doAckEarthSucceed(s, pkt.AckHandle, data)
}

func handleMsgMhfPostRyoudama(s *Session, p mhfpacket.MHFPacket) {
	pkt := p.(*mhfpacket.MsgMhfPostRyoudama)
	// Request payload beyond AckHandle is unconfirmed, so this doesn't
	// trust client-submitted values -- points are credited server-side from
	// the quest-clear path instead. Previously this handler sent no ACK at
	// all, which is a likely softlock source for any client awaiting this
	// response.
	doAckSimpleSucceed(s, pkt.AckHandle, make([]byte, 4))
}

func handleMsgMhfGetTinyBin(s *Session, p mhfpacket.MHFPacket) {
	pkt := p.(*mhfpacket.MsgMhfGetTinyBin)
	// requested after conquest quests
	doAckBufSucceed(s, pkt.AckHandle, []byte{})
}

func handleMsgMhfPostTinyBin(s *Session, p mhfpacket.MHFPacket) {
	pkt := p.(*mhfpacket.MsgMhfPostTinyBin)
	doAckSimpleSucceed(s, pkt.AckHandle, make([]byte, 4))
}

// handleMsgMhfCaravanMyScore, handleMsgMhfCaravanRanking, and
// handleMsgMhfCaravanMyRank intentionally still return an empty ACK.
// Unlike GetRyoudama, no pre-existing struct/serialization shape exists for
// these three in Erupe, and the only prior guess at a wire format (dead,
// commented-out code from the since-superseded feature/conquest branch) was
// never confirmed against the real client. Sending an unconfirmed non-empty
// payload risks a worse outcome (client misparse/crash) than the current
// known-safe empty response.

func handleMsgMhfCaravanMyScore(s *Session, p mhfpacket.MHFPacket) {
	pkt := p.(*mhfpacket.MsgMhfCaravanMyScore)
	var data []*byteframe.ByteFrame
	doAckEarthSucceed(s, pkt.AckHandle, data)
}

func handleMsgMhfCaravanRanking(s *Session, p mhfpacket.MHFPacket) {
	pkt := p.(*mhfpacket.MsgMhfCaravanRanking)
	var data []*byteframe.ByteFrame
	doAckEarthSucceed(s, pkt.AckHandle, data)
}

func handleMsgMhfCaravanMyRank(s *Session, p mhfpacket.MHFPacket) {
	pkt := p.(*mhfpacket.MsgMhfCaravanMyRank)
	var data []*byteframe.ByteFrame
	doAckEarthSucceed(s, pkt.AckHandle, data)
}
