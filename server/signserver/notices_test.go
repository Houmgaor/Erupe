package signserver

import (
	"errors"
	"reflect"
	"testing"

	"go.uber.org/zap"

	cfg "erupe-ce/config"
)

type mockSignNoticeRepo struct {
	notices []string
	err     error
}

func (m *mockSignNoticeRepo) ActiveNotices() ([]string, error) { return m.notices, m.err }

func TestLoginNotices(t *testing.T) {
	config := &cfg.Config{LoginNotices: []string{"static"}}

	cases := []struct {
		name string
		repo SignNoticeRepo
		want []string
	}{
		{"no repo (no database)", nil, []string{"static"}},
		{"runtime notices appended", &mockSignNoticeRepo{notices: []string{"maintenance", "event"}}, []string{"static", "maintenance", "event"}},
		{"database error keeps static ones", &mockSignNoticeRepo{err: errors.New("down")}, []string{"static"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := &Server{erupeConfig: config, logger: zap.NewNop(), noticeRepo: tc.repo}
			got := s.loginNotices()
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("got %v, want %v", got, tc.want)
			}
			// The config slice itself must not be appended to.
			if len(config.LoginNotices) != 1 {
				t.Fatalf("config.LoginNotices mutated: %v", config.LoginNotices)
			}
		})
	}
}
