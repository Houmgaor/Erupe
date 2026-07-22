package channelserver

import "erupe-ce/common/mhfmon"

// Static data tables for handleMsgMhfEnumeratePrice.

type enumeratePriceLBEntry struct {
	Unk0 uint16
	Unk1 uint16
	Unk2 uint32
}

type enumeratePriceWantedEntry struct {
	Unk0 uint32
	Unk1 uint32
	Unk2 uint32
	Unk3 uint16
	Unk4 uint16
	Unk5 uint16
	Unk6 uint16
	Unk7 uint16
	Unk8 uint16
	Unk9 uint16
}

type enumeratePriceGZEntry struct {
	Unk0  uint16
	Gz    uint16
	Unk1  uint16
	Unk2  uint16
	MonID uint16
	Unk3  uint16
	Unk4  uint8
}

// enumeratePriceLB is the LB price list (currently empty/unused).
var enumeratePriceLB []enumeratePriceLBEntry

// enumeratePriceWanted is the wanted monster list (currently empty/unused).
var enumeratePriceWanted []enumeratePriceWantedEntry

// enumeratePriceGZ is the GZ price table mapping monsters to their GZ costs.
var enumeratePriceGZ = []enumeratePriceGZEntry{
	{0, 1000, 0, 0, mhfmon.Pokaradon, 100, 1},
	{0, 800, 0, 0, mhfmon.YianKutKu, 100, 1},
	{0, 800, 0, 0, mhfmon.DaimyoHermitaur, 100, 1},
	{0, 1100, 0, 0, mhfmon.Farunokku, 100, 1},
	{0, 900, 0, 0, mhfmon.Congalala, 100, 1},
	{0, 900, 0, 0, mhfmon.Gypceros, 100, 1},
	{0, 1300, 0, 0, mhfmon.Hyujikiki, 100, 1},
	{0, 1000, 0, 0, mhfmon.Basarios, 100, 1},
	{0, 1000, 0, 0, mhfmon.Rathian, 100, 1},
	{0, 800, 0, 0, mhfmon.ShogunCeanataur, 100, 1},
	{0, 1400, 0, 0, mhfmon.Midogaron, 100, 1},
	{0, 900, 0, 0, mhfmon.Blangonga, 100, 1},
	{0, 1100, 0, 0, mhfmon.Rathalos, 100, 1},
	{0, 1000, 0, 0, mhfmon.Khezu, 100, 1},
	{0, 1600, 0, 0, mhfmon.Giaorugu, 100, 1},
	{0, 1100, 0, 0, mhfmon.Gravios, 100, 1},
	{0, 1400, 0, 0, mhfmon.Tigrex, 100, 1},
	{0, 1000, 0, 0, mhfmon.Pariapuria, 100, 1},
	{0, 1700, 0, 0, mhfmon.Anorupatisu, 100, 1},
	{0, 1500, 0, 0, mhfmon.Lavasioth, 100, 1},
	{0, 1500, 0, 0, mhfmon.Espinas, 100, 1},
	{0, 1600, 0, 0, mhfmon.Rajang, 100, 1},
	{0, 1800, 0, 0, mhfmon.Rebidiora, 100, 1},
	{0, 1100, 0, 0, mhfmon.YianGaruga, 100, 1},
	{0, 1500, 0, 0, mhfmon.AqraVashimu, 100, 1},
	{0, 1600, 0, 0, mhfmon.Gurenzeburu, 100, 1},
	{0, 1500, 0, 0, mhfmon.Dyuragaua, 100, 1},
	{0, 1300, 0, 0, mhfmon.Gougarf, 100, 1},
	{0, 1000, 0, 0, mhfmon.Shantien, 100, 1},
	{0, 1800, 0, 0, mhfmon.Disufiroa, 100, 1},
	{0, 600, 0, 0, mhfmon.Velocidrome, 100, 1},
	{0, 600, 0, 0, mhfmon.Gendrome, 100, 1},
	{0, 700, 0, 0, mhfmon.Iodrome, 100, 1},
	{0, 1700, 0, 0, mhfmon.Baruragaru, 100, 1},
	{0, 800, 0, 0, mhfmon.Cephadrome, 100, 1},
	{0, 1000, 0, 0, mhfmon.Plesioth, 100, 1},
	{0, 1800, 0, 0, mhfmon.Zerureusu, 100, 1},
	{0, 1100, 0, 0, mhfmon.Diablos, 100, 1},
	{0, 1600, 0, 0, mhfmon.Berukyurosu, 100, 1},
	{0, 2000, 0, 0, mhfmon.Fatalis, 100, 1},
	{0, 1500, 0, 0, mhfmon.BlackGravios, 100, 1},
	{0, 1600, 0, 0, mhfmon.GoldRathian, 100, 1},
	{0, 1900, 0, 0, mhfmon.Meraginasu, 100, 1},
	{0, 700, 0, 0, mhfmon.Bulldrome, 100, 1},
	{0, 900, 0, 0, mhfmon.NonoOrugaron, 100, 1},
	{0, 1600, 0, 0, mhfmon.KamuOrugaron, 100, 1},
	{0, 1700, 0, 0, mhfmon.Forokururu, 100, 1},
	{0, 1900, 0, 0, mhfmon.Diorex, 100, 1},
	{0, 1500, 0, 0, mhfmon.AqraJebia, 100, 1},
	{0, 1600, 0, 0, mhfmon.SilverRathalos, 100, 1},
	{0, 2400, 0, 0, mhfmon.CrimsonFatalis, 100, 1},
	{0, 2000, 0, 0, mhfmon.Inagami, 100, 1},
	{0, 2100, 0, 0, mhfmon.GarubaDaora, 100, 1},
	{0, 900, 0, 0, mhfmon.Monoblos, 100, 1},
	{0, 1000, 0, 0, mhfmon.RedKhezu, 100, 1},
	{0, 900, 0, 0, mhfmon.Hypnocatrice, 100, 1},
	{0, 1700, 0, 0, mhfmon.PearlEspinas, 100, 1},
	{0, 900, 0, 0, mhfmon.PurpleGypceros, 100, 1},
	{0, 1800, 0, 0, mhfmon.Poborubarumu, 100, 1},
	{0, 1900, 0, 0, mhfmon.Lunastra, 100, 1},
	{0, 1600, 0, 0, mhfmon.Kuarusepusu, 100, 1},
	{0, 1100, 0, 0, mhfmon.PinkRathian, 100, 1},
	{0, 1200, 0, 0, mhfmon.AzureRathalos, 100, 1},
	{0, 1800, 0, 0, mhfmon.Varusaburosu, 100, 1},
	{0, 1000, 0, 0, mhfmon.Gogomoa, 100, 1},
	{0, 1600, 0, 0, mhfmon.BurningEspinas, 100, 1},
	{0, 2000, 0, 0, mhfmon.Harudomerugu, 100, 1},
	{0, 1800, 0, 0, mhfmon.Akantor, 100, 1},
	{0, 900, 0, 0, mhfmon.BrightHypnoc, 100, 1},
	{0, 2200, 0, 0, mhfmon.Gureadomosu, 100, 1},
	{0, 1200, 0, 0, mhfmon.GreenPlesioth, 100, 1},
	{0, 2400, 0, 0, mhfmon.Zinogre, 100, 1},
	{0, 1900, 0, 0, mhfmon.Gasurabazura, 100, 1},
	{0, 1300, 0, 0, mhfmon.Abiorugu, 100, 1},
	{0, 1200, 0, 0, mhfmon.BlackDiablos, 100, 1},
	{0, 1000, 0, 0, mhfmon.WhiteMonoblos, 100, 1},
	{0, 3000, 0, 0, mhfmon.Deviljho, 100, 1},
	{0, 2300, 0, 0, mhfmon.YamaKurai, 100, 1},
	{0, 2800, 0, 0, mhfmon.Brachydios, 100, 1},
	{0, 1700, 0, 0, mhfmon.Toridcless, 100, 1},
	{0, 1100, 0, 0, mhfmon.WhiteHypnoc, 100, 1},
	{0, 1500, 0, 0, mhfmon.RedLavasioth, 100, 1},
	{0, 2200, 0, 0, mhfmon.Barioth, 100, 1},
	{0, 1800, 0, 0, mhfmon.Odibatorasu, 100, 1},
	{0, 1600, 0, 0, mhfmon.Doragyurosu, 100, 1},
	{0, 900, 0, 0, mhfmon.BlueYianKutKu, 100, 1},
	{0, 2300, 0, 0, mhfmon.ToaTesukatora, 100, 1},
	{0, 2000, 0, 0, mhfmon.Uragaan, 100, 1},
	{0, 1900, 0, 0, mhfmon.Teostra, 100, 1},
	{0, 1700, 0, 0, mhfmon.Chameleos, 100, 1},
	{0, 1800, 0, 0, mhfmon.KushalaDaora, 100, 1},
	{0, 2100, 0, 0, mhfmon.Nargacuga, 100, 1},
	{0, 2600, 0, 0, mhfmon.Guanzorumu, 100, 1},
	{0, 1900, 0, 0, mhfmon.Kirin, 100, 1},
	{0, 2000, 0, 0, mhfmon.Rukodiora, 100, 1},
	{0, 2700, 0, 0, mhfmon.StygianZinogre, 100, 1},
	{0, 2200, 0, 0, mhfmon.Voljang, 100, 1},
	{0, 1800, 0, 0, mhfmon.Zenaserisu, 100, 1},
	{0, 3100, 0, 0, mhfmon.GoreMagala, 100, 1},
	{0, 3200, 0, 0, mhfmon.ShagaruMagala, 100, 1},
	{0, 3500, 0, 0, mhfmon.Eruzerion, 100, 1},
	{0, 3200, 0, 0, mhfmon.Amatsu, 100, 1},
}
