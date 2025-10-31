package channelserver

import (
	"erupe-ce/common/byteframe"
	"erupe-ce/network/mhfpacket"
	"go.uber.org/zap"
	"time"
)

func handleMsgMhfAddKouryouPoint(s *Session, p mhfpacket.MHFPacket) {
	// hunting with both ranks maxed gets you these
	pkt := p.(*mhfpacket.MsgMhfAddKouryouPoint)
	saveStart := time.Now()

	s.logger.Debug("Adding Koryo points",
		zap.Uint32("charID", s.charID),
		zap.Uint32("points_to_add", pkt.KouryouPoints),
	)

	var points int
	err := s.server.db.QueryRow("UPDATE characters SET kouryou_point=COALESCE(kouryou_point + $1, $1) WHERE id=$2 RETURNING kouryou_point", pkt.KouryouPoints, s.charID).Scan(&points)
	if err != nil {
		s.logger.Error("Failed to update KouryouPoint in db",
			zap.Error(err),
			zap.Uint32("charID", s.charID),
			zap.Uint32("points_to_add", pkt.KouryouPoints),
		)
	} else {
		saveDuration := time.Since(saveStart)
		s.logger.Info("Koryo points added successfully",
			zap.Uint32("charID", s.charID),
			zap.Uint32("points_added", pkt.KouryouPoints),
			zap.Int("new_total", points),
			zap.Duration("duration", saveDuration),
		)
	}

	resp := byteframe.NewByteFrame()
	resp.WriteUint32(uint32(points))
	doAckBufSucceed(s, pkt.AckHandle, resp.Data())
}

func handleMsgMhfGetKouryouPoint(s *Session, p mhfpacket.MHFPacket) {
	pkt := p.(*mhfpacket.MsgMhfGetKouryouPoint)
	var points int
	err := s.server.db.QueryRow("SELECT COALESCE(kouryou_point, 0) FROM characters WHERE id = $1", s.charID).Scan(&points)
	if err != nil {
		s.logger.Error("Failed to get kouryou_point from db",
			zap.Error(err),
			zap.Uint32("charID", s.charID),
		)
	} else {
		s.logger.Debug("Retrieved Koryo points",
			zap.Uint32("charID", s.charID),
			zap.Int("points", points),
		)
	}
	resp := byteframe.NewByteFrame()
	resp.WriteUint32(uint32(points))
	doAckBufSucceed(s, pkt.AckHandle, resp.Data())
}

func handleMsgMhfExchangeKouryouPoint(s *Session, p mhfpacket.MHFPacket) {
	// spent at the guildmaster, 10000 a roll
	pkt := p.(*mhfpacket.MsgMhfExchangeKouryouPoint)
	saveStart := time.Now()

	s.logger.Debug("Exchanging Koryo points",
		zap.Uint32("charID", s.charID),
		zap.Uint32("points_to_spend", pkt.KouryouPoints),
	)

	var points int
	err := s.server.db.QueryRow("UPDATE characters SET kouryou_point=kouryou_point - $1 WHERE id=$2 RETURNING kouryou_point", pkt.KouryouPoints, s.charID).Scan(&points)
	if err != nil {
		s.logger.Error("Failed to exchange Koryo points",
			zap.Error(err),
			zap.Uint32("charID", s.charID),
			zap.Uint32("points_to_spend", pkt.KouryouPoints),
		)
	} else {
		saveDuration := time.Since(saveStart)
		s.logger.Info("Koryo points exchanged successfully",
			zap.Uint32("charID", s.charID),
			zap.Uint32("points_spent", pkt.KouryouPoints),
			zap.Int("remaining_points", points),
			zap.Duration("duration", saveDuration),
		)
	}

	resp := byteframe.NewByteFrame()
	resp.WriteUint32(uint32(points))
	doAckBufSucceed(s, pkt.AckHandle, resp.Data())
}
