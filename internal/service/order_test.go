package service

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

type stubOrderRepository struct {
	createOrderIfNotExistsFn func(ctx context.Context, userID int64, number string) (created bool, ownerID int64, err error)
}

func (s stubOrderRepository) CreateOrderIfNotExists(
	ctx context.Context,
	userID int64,
	number string,
) (bool, int64, error) {
	return s.createOrderIfNotExistsFn(ctx, userID, number)
}

func TestOrderService_SubmitOrder(t *testing.T) {
	t.Parallel()

	dbErr := errors.New("db unavailable")

	tests := []struct {
		name         string
		userID       int64
		rawNumber    string
		repo         stubOrderRepository
		wantRepoUser int64
		wantRepoNum  string
		wantResult   SubmitOrderResult
		wantErr      error
		wantRepoCall bool
	}{
		{
			name:      "422 when contains non digits",
			userID:    11,
			rawNumber: "12ab34",
			repo: stubOrderRepository{
				createOrderIfNotExistsFn: func(ctx context.Context, userID int64, number string) (bool, int64, error) {
					return false, 0, nil
				},
			},
			wantErr:      ErrInvalidOrderNumber,
			wantRepoCall: false,
		},
		{
			name:      "422 when luhn is invalid",
			userID:    11,
			rawNumber: "12345678901",
			repo: stubOrderRepository{
				createOrderIfNotExistsFn: func(ctx context.Context, userID int64, number string) (bool, int64, error) {
					return false, 0, nil
				},
			},
			wantErr:      ErrInvalidOrderNumber,
			wantRepoCall: false,
		},
		{
			name:      "202 when new order accepted",
			userID:    11,
			rawNumber: "79927398713",
			repo: stubOrderRepository{
				createOrderIfNotExistsFn: func(ctx context.Context, userID int64, number string) (bool, int64, error) {
					return true, 11, nil
				},
			},
			wantRepoUser: 11,
			wantRepoNum:  "79927398713",
			wantResult:   SubmitOrderAccepted,
			wantRepoCall: true,
		},
		{
			name:      "200 when already uploaded by same user",
			userID:    11,
			rawNumber: "79927398713",
			repo: stubOrderRepository{
				createOrderIfNotExistsFn: func(ctx context.Context, userID int64, number string) (bool, int64, error) {
					return false, 11, nil
				},
			},
			wantRepoUser: 11,
			wantRepoNum:  "79927398713",
			wantResult:   SubmitOrderAlreadyUploadedByUser,
			wantRepoCall: true,
		},
		{
			name:      "409 when uploaded by another user",
			userID:    11,
			rawNumber: "79927398713",
			repo: stubOrderRepository{
				createOrderIfNotExistsFn: func(ctx context.Context, userID int64, number string) (bool, int64, error) {
					return false, 77, nil
				},
			},
			wantRepoUser: 11,
			wantRepoNum:  "79927398713",
			wantErr:      ErrOrderUploadedByAnotherUser,
			wantRepoCall: true,
		},
		{
			name:      "500 when repository returns error",
			userID:    11,
			rawNumber: "79927398713",
			repo: stubOrderRepository{
				createOrderIfNotExistsFn: func(ctx context.Context, userID int64, number string) (bool, int64, error) {
					return false, 0, dbErr
				},
			},
			wantRepoUser: 11,
			wantRepoNum:  "79927398713",
			wantErr:      dbErr,
			wantRepoCall: true,
		},
		{
			name:      "trim spaces before validation",
			userID:    11,
			rawNumber: " 79927398713 \n",
			repo: stubOrderRepository{
				createOrderIfNotExistsFn: func(ctx context.Context, userID int64, number string) (bool, int64, error) {
					return true, 11, nil
				},
			},
			wantRepoUser: 11,
			wantRepoNum:  "79927398713",
			wantResult:   SubmitOrderAccepted,
			wantRepoCall: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			repoCalled := false
			var repoUserID int64
			var repoNumber string
			repo := stubOrderRepository{
				createOrderIfNotExistsFn: func(ctx context.Context, userID int64, number string) (bool, int64, error) {
					repoCalled = true
					repoUserID = userID
					repoNumber = number
					return tt.repo.CreateOrderIfNotExists(ctx, userID, number)
				},
			}

			svc := NewOrderService(repo)

			gotResult, err := svc.SubmitOrder(context.Background(), tt.userID, tt.rawNumber)

			if tt.wantErr != nil {
				require.Error(t, err)
				require.ErrorIs(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.wantResult, gotResult)
			}

			require.Equal(t, tt.wantRepoCall, repoCalled)
			if tt.wantRepoCall {
				require.Equal(t, tt.wantRepoUser, repoUserID)
				require.Equal(t, tt.wantRepoNum, repoNumber)
			}
		})
	}
}
