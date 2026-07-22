package channelserver

import (
	"time"

	"erupe-ce/common/byteframe"
	"erupe-ce/network/mhfpacket"

	"go.uber.org/zap"
)

// PaperMissionTimetable represents a daily mission schedule entry.
type PaperMissionTimetable struct {
	Start time.Time
	End   time.Time
}

// PaperMissionData represents daily mission details.
type PaperMissionData struct {
	Unk0            uint8
	Unk1            uint8
	Unk2            int16
	Reward1ID       uint16
	Reward1Quantity uint8
	Reward2ID       uint16
	Reward2Quantity uint8
}

// PaperMission represents a daily mission wrapper.
type PaperMission struct {
	Timetables []PaperMissionTimetable
	Data       []PaperMissionData
}

// PaperData represents complete daily paper data.
type PaperData struct {
	Unk0 uint16
	Unk1 int16
	Unk2 int16
	Unk3 int16
	Unk4 int16
	Unk5 int16
	Unk6 int16
}

// PaperGift represents a paper gift reward entry.
type PaperGift struct {
	Unk0 uint16
	Unk1 uint8
	Unk2 uint8
	Unk3 uint16
}

func handleMsgMhfGetPaperData(s *Session, p mhfpacket.MHFPacket) {
	pkt := p.(*mhfpacket.MsgMhfGetPaperData)
	var data []*byteframe.ByteFrame

	var paperData []PaperData
	var paperMissions PaperMission
	var paperGift []PaperGift

	switch pkt.DataType {
	case 0:
		paperMissions = PaperMission{
			[]PaperMissionTimetable{{TimeMidnight(), TimeMidnight().Add(24 * time.Hour)}},
			[]PaperMissionData{},
		}
	case 5:
		paperData = paperDataTower
	case 6:
		paperData = paperDataTowerScaling
	default:
		if pkt.DataType < 1000 {
			s.logger.Info("PaperData request for unknown type", zap.Uint32("DataType", pkt.DataType))
		}
	}

	if pkt.DataType > 1000 {
		_, ok := paperGiftData[pkt.DataType]
		if ok {
			paperGift = paperGiftData[pkt.DataType]
		} else {
			s.logger.Info("PaperGift request for unknown type", zap.Uint32("DataType", pkt.DataType))
		}
		for _, gift := range paperGift {
			bf := byteframe.NewByteFrame()
			bf.WriteUint16(gift.Unk0)
			bf.WriteUint8(gift.Unk1)
			bf.WriteUint8(gift.Unk2)
			bf.WriteUint16(gift.Unk3)
			data = append(data, bf)
		}
		doAckEarthSucceed(s, pkt.AckHandle, data)
	} else if pkt.DataType == 0 {
		bf := byteframe.NewByteFrame()
		bf.WriteUint16(uint16(len(paperMissions.Timetables)))
		bf.WriteUint16(uint16(len(paperMissions.Data)))
		for _, timetable := range paperMissions.Timetables {
			bf.WriteUint32(uint32(timetable.Start.Unix()))
			bf.WriteUint32(uint32(timetable.End.Unix()))
		}
		for _, mdata := range paperMissions.Data {
			bf.WriteUint8(mdata.Unk0)
			bf.WriteUint8(mdata.Unk1)
			bf.WriteInt16(mdata.Unk2)
			bf.WriteUint16(mdata.Reward1ID)
			bf.WriteUint8(mdata.Reward1Quantity)
			bf.WriteUint16(mdata.Reward2ID)
			bf.WriteUint8(mdata.Reward2Quantity)
		}
		doAckBufSucceed(s, pkt.AckHandle, bf.Data())
	} else {
		for _, pdata := range paperData {
			bf := byteframe.NewByteFrame()
			bf.WriteUint16(pdata.Unk0)
			bf.WriteInt16(pdata.Unk1)
			bf.WriteInt16(pdata.Unk2)
			bf.WriteInt16(pdata.Unk3)
			bf.WriteInt16(pdata.Unk4)
			bf.WriteInt16(pdata.Unk5)
			bf.WriteInt16(pdata.Unk6)
			data = append(data, bf)
		}
		doAckEarthSucceed(s, pkt.AckHandle, data)
	}
}
