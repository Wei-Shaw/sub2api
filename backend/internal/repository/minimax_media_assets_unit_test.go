//go:build unit

package repository

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/pkg/minimax"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestMiniMaxPrivateVoiceFeeOnlyFirstActivation(t *testing.T) {
	for _, activated := range []bool{false, true} {
		t.Run(map[bool]string{false: "first use", true: "concurrent receipt already settled"}[activated], func(t *testing.T) {
			db, m, e := sqlmock.New()
			require.NoError(t, e)
			defer db.Close()
			m.ExpectBegin()
			tx, e := db.Begin()
			require.NoError(t, e)
			var prior any
			if activated {
				prior = "mm_prior"
			}
			m.ExpectQuery("SELECT activated_task_id FROM minimax_media_assets").WithArgs(int64(4), "privateVoice", int64(1), int64(2)).WillReturnRows(sqlmock.NewRows([]string{"activated_task_id"}).AddRow(prior))
			if !activated {
				m.ExpectExec("UPDATE minimax_media_assets SET activated_task_id").WithArgs("mm_test", int64(4), "privateVoice").WillReturnResult(sqlmock.NewResult(0, 1))
			}
			one, chars := 1.0, 60.0
			task := &service.MiniMaxMediaTask{ID: "mm_test", UserID: 1, APIKeyID: 2, AccountID: 4, Model: minimax.SpeechTurbo, Pricing: service.MiniMaxMediaPricing{Version: 2, CostCurrency: "USD", Multiplier: &one, Retail: service.MiniMaxMediaRates{CharactersPerMillion: &chars}, Cost: service.MiniMaxMediaRates{CharactersPerMillion: &chars}, CustomVoices: []service.MiniMaxVoiceCharge{{ID: "privateVoice", Retail: 1.5, Cost: 1.5}}}}
			bill := &service.MiniMaxMediaBill{Usage: minimax.Usage{Characters: 1000, VoiceCount: 1}, RetailUSD: 1.56, ChargedUSD: 1.56, UpstreamUSD: 1.56}
			task.Bill = bill
			bill, e = reconcileMiniMaxVoiceFees(context.Background(), tx, task)
			require.NoError(t, e)
			if activated {
				require.Equal(t, .06, bill.ChargedUSD)
				require.Empty(t, bill.BilledVoices)
			} else {
				require.Equal(t, 1.56, bill.ChargedUSD)
				require.Equal(t, []string{"privateVoice"}, bill.BilledVoices)
			}
			require.Equal(t, 1.56, bill.UpstreamUSD, "provider receipt is retained independently of retail deduplication")
			m.ExpectRollback()
			require.NoError(t, tx.Rollback())
			require.NoError(t, m.ExpectationsWereMet())
		})
	}
}

func TestMiniMaxPartialVoiceCountRequiresUnambiguousActivation(t *testing.T) {
	for _, priorActivated := range []bool{false, true} {
		t.Run(map[bool]string{false: "ambiguous", true: "one already activated"}[priorActivated], func(t *testing.T) {
			db, m, err := sqlmock.New()
			require.NoError(t, err)
			defer db.Close()
			m.ExpectBegin()
			tx, err := db.Begin()
			require.NoError(t, err)
			one, chars := 1.0, 60.0
			task := &service.MiniMaxMediaTask{ID: "mm_partial", AccountID: 4, UserID: 1, APIKeyID: 2, Model: minimax.SpeechTurbo, Pricing: service.MiniMaxMediaPricing{Version: 2, CostCurrency: "USD", Multiplier: &one, Retail: service.MiniMaxMediaRates{CharactersPerMillion: &chars}, Cost: service.MiniMaxMediaRates{CharactersPerMillion: &chars}, CustomVoices: []service.MiniMaxVoiceCharge{{ID: "voiceB", Retail: 3, Cost: 3}, {ID: "voiceA", Retail: 1.5, Cost: 1.5}}}, Bill: &service.MiniMaxMediaBill{Usage: minimax.Usage{Characters: 1000, VoiceCount: 1}}}
			var prior any
			if priorActivated {
				prior = "mm_prior"
			}
			m.ExpectQuery("SELECT activated_task_id FROM minimax_media_assets").WithArgs(int64(4), "voiceA", int64(1), int64(2)).WillReturnRows(sqlmock.NewRows([]string{"activated_task_id"}).AddRow(prior))
			m.ExpectQuery("SELECT activated_task_id FROM minimax_media_assets").WithArgs(int64(4), "voiceB", int64(1), int64(2)).WillReturnRows(sqlmock.NewRows([]string{"activated_task_id"}).AddRow(nil))
			if priorActivated {
				m.ExpectExec("UPDATE minimax_media_assets SET activated_task_id").WithArgs("mm_partial", int64(4), "voiceB").WillReturnResult(sqlmock.NewResult(0, 1))
			}
			bill, err := reconcileMiniMaxVoiceFees(context.Background(), tx, task)
			if priorActivated {
				require.NoError(t, err)
				require.Equal(t, 3.06, bill.ChargedUSD)
				require.Equal(t, 3.06, bill.UpstreamUSD)
				require.Equal(t, []string{"voiceB"}, bill.BilledVoices)
			} else {
				require.Error(t, err)
				require.Nil(t, bill)
			}
			m.ExpectRollback()
			require.NoError(t, tx.Rollback())
			require.NoError(t, m.ExpectationsWereMet())
		})
	}
}
