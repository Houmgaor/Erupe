package channelserver

import (
	"encoding/binary"
	"erupe-ce/common/mhfcourse"
	cfg "erupe-ce/config"
	"erupe-ce/server/channelserver/compression/nullcomp"
	"testing"
	"time"
)

type rpSaveCaptureRepo struct {
	*mockCharacterRepo
	saved *SaveAtomicParams
}

func (r *rpSaveCaptureRepo) SaveCharacterDataAtomic(p SaveAtomicParams) error {
	r.saved = &p
	return nil
}

func (r *rpSaveCaptureRepo) UpdateTimePlayed(_ uint32, seconds int) error {
	r.ints["time_played"] = seconds
	return nil
}

func TestLogoutRPConfigurationAndPersistence(t *testing.T) {
	for _, cafe := range []bool{false, true} {
		server, charRepo, _, _ := setupLogoutServer()
		server.erupeConfig.RealClientMode = cfg.ZZ
		server.erupeConfig.GameplayOptions.RPAccrualNormalSeconds = 600
		server.erupeConfig.GameplayOptions.RPAccrualCafeSeconds = 300
		server.erupeConfig.GameplayOptions.MaximumRP = 50000
		charRepo.loadSaveDataData = make([]byte, 160000)
		charRepo.ints["time_played"] = 1000
		offset := getPointers(cfg.ZZ)[pRP]
		binary.LittleEndian.PutUint16(charRepo.loadSaveDataData[offset:], 10)
		capture := &rpSaveCaptureRepo{mockCharacterRepo: charRepo}
		server.charRepo = capture
		session, _ := setupLogoutSession(42, server)
		session.sessionStart = TimeAdjusted().Unix()
		if cafe {
			session.courses = []mhfcourse.Course{{ID: 30}}
		}
		start := TimeAdjusted().Unix()
		logoutPlayer(session)
		elapsed := int(TimeAdjusted().Unix() - start)
		if elapsed >= 100 {
			t.Fatal("test crossed RP boundary")
		}
		wantRP, wantRemainder := uint16(11), 400
		if cafe {
			wantRP, wantRemainder = 13, 100
		}
		if capture.saved == nil {
			t.Fatal("no savedata persisted")
		}
		raw, err := nullcomp.Decompress(capture.saved.CompSave)
		if err != nil {
			t.Fatal(err)
		}
		if got := binary.LittleEndian.Uint16(raw[offset:]); got != wantRP {
			t.Fatalf("cafe=%v: saved RP=%d want %d", cafe, got, wantRP)
		}
		if rem := charRepo.ints["time_played"]; rem < wantRemainder || rem > wantRemainder+elapsed+1 {
			t.Fatalf("cafe=%v: remainder=%d", cafe, rem)
		}
	}
}

func TestRPOverflowCapIsPersisted(t *testing.T) {
	server, repo, _, _ := setupLogoutServer()
	server.erupeConfig.RealClientMode = cfg.ZZ
	server.erupeConfig.GameplayOptions.MaximumRP = 50000
	repo.loadSaveDataData = make([]byte, 160000)
	offset := getPointers(cfg.ZZ)[pRP]
	binary.LittleEndian.PutUint16(repo.loadSaveDataData[offset:], 49999)
	capture := &rpSaveCaptureRepo{mockCharacterRepo: repo}
	server.charRepo = capture
	session, _ := setupLogoutSession(42, server)
	session.playtimeTime = time.Time{}
	if err := saveAllCharacterData(session, 65536); err != nil {
		t.Fatal(err)
	}
	if capture.saved == nil {
		t.Fatal("save missing")
	}
	raw, err := nullcomp.Decompress(capture.saved.CompSave)
	if err != nil {
		t.Fatal(err)
	}
	if got := binary.LittleEndian.Uint16(raw[offset:]); got != 50000 {
		t.Fatalf("persisted RP=%d", got)
	}
}

func TestRPAccrualIntervals(t *testing.T) {
	defaults := cfg.GameplayOptions{RPAccrualNormalSeconds: 1800, RPAccrualCafeSeconds: 900}
	for _, tt := range []struct {
		seconds         int
		cafe            bool
		gain, remainder int
	}{
		{0, false, 0, 0}, {1799, false, 0, 1799}, {1800, false, 1, 0}, {3601, false, 2, 1},
		{899, true, 0, 899}, {900, true, 1, 0}, {1801, true, 2, 1},
	} {
		gain, rem := accrueRP(tt.seconds, tt.cafe, defaults)
		if gain != tt.gain || rem != tt.remainder {
			t.Fatalf("%+v: got %d,%d", tt, gain, rem)
		}
	}
	custom := cfg.GameplayOptions{RPAccrualNormalSeconds: 600, RPAccrualCafeSeconds: 300}
	for _, cafe := range []bool{false, true} {
		gain, rem := accrueRP(1000, cafe, custom)
		want := 1
		if cafe {
			want = 3
		}
		if gain != want || rem != 1000-want*map[bool]int{false: 600, true: 300}[cafe] {
			t.Fatalf("custom: %d,%d", gain, rem)
		}
	}
	first, remainder := accrueRP(1200, false, defaults)
	second, remainder := accrueRP(remainder+600, false, defaults)
	if first != 0 || second != 1 || remainder != 0 {
		t.Fatal("carry-over lost")
	}
}

func TestRPAdditionDoesNotOverflow(t *testing.T) {
	for _, tt := range []struct {
		current   uint16
		gain      int
		cap, want uint16
	}{
		{10, 1, 50000, 11}, {49999, 1, 50000, 50000}, {49999, 2, 50000, 50000},
		{50000, 20000, 50000, 50000}, {1, 65536, 50000, 50000},
		{65534, 10, 65535, 65535}, {1, int(^uint(0) >> 1), 50000, 50000},
		{110, 1, 100, 100}, {110, 0, 100, 110}, {10, -1, 100, 10}, {1, 1, 0, 0},
	} {
		if got := addRPWithCap(tt.current, tt.gain, tt.cap); got != tt.want {
			t.Fatalf("%+v: got %d", tt, got)
		}
	}
}
