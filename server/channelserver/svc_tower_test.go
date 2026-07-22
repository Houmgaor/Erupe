package channelserver

import (
	"errors"
	"testing"

	"go.uber.org/zap"
)

func newTestTowerService(mock *mockTowerRepo) *TowerService {
	logger, _ := zap.NewDevelopment()
	return NewTowerService(mock, logger)
}

// --- AddGem tests ---

func TestTowerService_AddGem_Success(t *testing.T) {
	mock := &mockTowerRepo{gems: "0,0,5,0,0"}
	svc := newTestTowerService(mock)

	err := svc.AddGem(1, 2, 3)
	if err != nil {
		t.Fatalf("AddGem returned error: %v", err)
	}
	// Gem at index 2 was 5, added 3, so should be 8
	if mock.updatedGems != "0,0,8,0,0" {
		t.Errorf("updatedGems = %q, want %q", mock.updatedGems, "0,0,8,0,0")
	}
}

func TestTowerService_AddGem_GetGemsError(t *testing.T) {
	mock := &mockTowerRepo{gemsErr: errors.New("db error")}
	svc := newTestTowerService(mock)

	err := svc.AddGem(1, 0, 1)
	if err == nil {
		t.Fatal("AddGem should return error when GetGems fails")
	}
}

// --- GetTenrouiraiProgressCapped tests ---

func TestTowerService_GetTenrouiraiProgressCapped_CapsToGoals(t *testing.T) {
	// Page 1 missions have goals: 80, 16, 50 (from tenrouiraiData indices 0,1,2)
	mock := &mockTowerRepo{
		progress: TenrouiraiProgressData{
			Page:     1,
			Mission1: 9999,
			Mission2: 9999,
			Mission3: 9999,
		},
	}
	svc := newTestTowerService(mock)

	result, err := svc.GetTenrouiraiProgressCapped(10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Mission1 != tenrouiraiData[0].Goal {
		t.Errorf("Mission1 = %d, want %d", result.Mission1, tenrouiraiData[0].Goal)
	}
	if result.Mission2 != tenrouiraiData[1].Goal {
		t.Errorf("Mission2 = %d, want %d", result.Mission2, tenrouiraiData[1].Goal)
	}
	if result.Mission3 != tenrouiraiData[2].Goal {
		t.Errorf("Mission3 = %d, want %d", result.Mission3, tenrouiraiData[2].Goal)
	}
}

func TestTowerService_GetTenrouiraiProgressCapped_BelowGoals(t *testing.T) {
	mock := &mockTowerRepo{
		progress: TenrouiraiProgressData{
			Page:     1,
			Mission1: 10,
			Mission2: 5,
			Mission3: 20,
		},
	}
	svc := newTestTowerService(mock)

	result, err := svc.GetTenrouiraiProgressCapped(10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Mission1 != 10 {
		t.Errorf("Mission1 = %d, want 10", result.Mission1)
	}
	if result.Mission2 != 5 {
		t.Errorf("Mission2 = %d, want 5", result.Mission2)
	}
	if result.Mission3 != 20 {
		t.Errorf("Mission3 = %d, want 20", result.Mission3)
	}
}

func TestTowerService_GetTenrouiraiProgressCapped_MinPage1(t *testing.T) {
	mock := &mockTowerRepo{
		progress: TenrouiraiProgressData{Page: 0},
	}
	svc := newTestTowerService(mock)

	result, err := svc.GetTenrouiraiProgressCapped(10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Page != 1 {
		t.Errorf("Page = %d, want 1", result.Page)
	}
}

func TestTowerService_GetTenrouiraiProgressCapped_DBError(t *testing.T) {
	mock := &mockTowerRepo{progressErr: errors.New("db error")}
	svc := newTestTowerService(mock)

	_, err := svc.GetTenrouiraiProgressCapped(10)
	if err == nil {
		t.Fatal("expected error from DB failure")
	}
}

// --- DonateGuildTowerRP tests ---

func TestTowerService_DonateGuildTowerRP_NoAdvance(t *testing.T) {
	mock := &mockTowerRepo{
		page:    1,
		donated: 0,
	}
	svc := newTestTowerService(mock)

	result, err := svc.DonateGuildTowerRP(10, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Advanced {
		t.Error("should not advance when donation < requirement")
	}
	if result.ActualDonated != 1 {
		t.Errorf("ActualDonated = %d, want 1", result.ActualDonated)
	}
	if mock.advanceCalled {
		t.Error("AdvanceTenrouiraiPage should not be called")
	}
	if mock.donatedRP != 1 {
		t.Errorf("donatedRP = %d, want 1", mock.donatedRP)
	}
}

func TestTowerService_DonateGuildTowerRP_AdvancesPage(t *testing.T) {
	// Compute the requirement for page 1: sum of Cost for indices 0..3
	var requirement int
	for i := 0; i < 4; i++ {
		requirement += int(tenrouiraiData[i].Cost)
	}

	mock := &mockTowerRepo{
		page:    1,
		donated: requirement - 10, // 10 short of requirement
	}
	svc := newTestTowerService(mock)

	result, err := svc.DonateGuildTowerRP(10, 100) // donating 100, but only 10 needed
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Advanced {
		t.Error("should advance when donation meets requirement")
	}
	if result.ActualDonated != 10 {
		t.Errorf("ActualDonated = %d, want 10 (capped to remaining)", result.ActualDonated)
	}
	if !mock.advanceCalled {
		t.Error("AdvanceTenrouiraiPage should be called")
	}
}

func TestTowerService_DonateGuildTowerRP_DBError(t *testing.T) {
	mock := &mockTowerRepo{pageRPErr: errors.New("db error")}
	svc := newTestTowerService(mock)

	_, err := svc.DonateGuildTowerRP(10, 100)
	if err == nil {
		t.Fatal("expected error from DB failure")
	}
}
