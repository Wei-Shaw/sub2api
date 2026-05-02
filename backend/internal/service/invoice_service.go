package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/mail"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"

	entsql "entgo.io/ent/dialect/sql"
	"github.com/lib/pq"
)

const (
	InvoiceStatusPending   = "pending"
	InvoiceStatusCompleted = "completed"
	InvoiceStatusRejected  = "rejected"

	maxInvoiceOrdersPerRequest = 100
)

type InvoiceProfile struct {
	ID          int64     `json:"id"`
	UserID      int64     `json:"user_id,omitempty"`
	Title       string    `json:"title"`
	TaxNumber   string    `json:"tax_number"`
	Email       string    `json:"email"`
	Address     *string   `json:"address,omitempty"`
	Phone       *string   `json:"phone,omitempty"`
	BankName    *string   `json:"bank_name,omitempty"`
	BankAccount *string   `json:"bank_account,omitempty"`
	IsDefault   bool      `json:"is_default"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type InvoiceProfileInput struct {
	Title       string  `json:"title"`
	TaxNumber   string  `json:"tax_number"`
	Email       string  `json:"email"`
	Address     *string `json:"address"`
	Phone       *string `json:"phone"`
	BankName    *string `json:"bank_name"`
	BankAccount *string `json:"bank_account"`
}

type InvoiceProfileSnapshot struct {
	Title       string  `json:"title"`
	TaxNumber   string  `json:"tax_number"`
	Email       string  `json:"email"`
	Address     *string `json:"address,omitempty"`
	Phone       *string `json:"phone,omitempty"`
	BankName    *string `json:"bank_name,omitempty"`
	BankAccount *string `json:"bank_account,omitempty"`
}

type InvoiceRequest struct {
	ID              int64                  `json:"id"`
	UserID          int64                  `json:"user_id,omitempty"`
	ProfileID       *int64                 `json:"profile_id,omitempty"`
	SerialNo        string                 `json:"serial_no"`
	Status          string                 `json:"status"`
	ProfileSnapshot InvoiceProfileSnapshot `json:"profile_snapshot"`
	TotalAmount     float64                `json:"total_amount"`
	RejectReason    *string                `json:"reject_reason,omitempty"`
	CreatedAt       time.Time              `json:"created_at"`
	UpdatedAt       time.Time              `json:"updated_at"`
	CompletedAt     *time.Time             `json:"completed_at,omitempty"`
	Orders          []InvoiceRequestOrder  `json:"orders"`
}

type InvoiceRequestOrder struct {
	ID          int64      `json:"id"`
	OutTradeNo  string     `json:"out_trade_no"`
	PayAmount   float64    `json:"pay_amount"`
	PaymentType string     `json:"payment_type"`
	OrderType   string     `json:"order_type"`
	Status      string     `json:"status"`
	CreatedAt   time.Time  `json:"created_at"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
}

type InvoiceRequestListParams struct {
	Page      int
	PageSize  int
	Status    string
	StartTime string
	EndTime   string
}

type CreateInvoiceRequestInput struct {
	ProfileID int64   `json:"profile_id"`
	OrderIDs  []int64 `json:"order_ids"`
}

func (s *PaymentService) invoiceDB() (*sql.DB, error) {
	if s == nil || s.entClient == nil {
		return nil, infraerrors.InternalServer("INVOICE_DB_UNAVAILABLE", "invoice database is unavailable")
	}
	drv, ok := s.entClient.Driver().(*entsql.Driver)
	if !ok {
		return nil, infraerrors.InternalServer("INVOICE_DB_UNAVAILABLE", "invoice database driver is unavailable")
	}
	return drv.DB(), nil
}

func normalizeInvoiceProfileInput(input InvoiceProfileInput) (InvoiceProfileInput, error) {
	input.Title = strings.TrimSpace(input.Title)
	input.TaxNumber = strings.TrimSpace(input.TaxNumber)
	input.Email = strings.TrimSpace(input.Email)
	input.Address = normalizeOptionalString(input.Address)
	input.Phone = normalizeOptionalString(input.Phone)
	input.BankName = normalizeOptionalString(input.BankName)
	input.BankAccount = normalizeOptionalString(input.BankAccount)

	switch {
	case input.Title == "":
		return input, infraerrors.BadRequest("INVOICE_TITLE_REQUIRED", "invoice title is required")
	case len(input.Title) > 255:
		return input, infraerrors.BadRequest("INVOICE_TITLE_TOO_LONG", "invoice title is too long")
	case input.TaxNumber == "":
		return input, infraerrors.BadRequest("INVOICE_TAX_NUMBER_REQUIRED", "invoice tax number is required")
	case len(input.TaxNumber) > 64:
		return input, infraerrors.BadRequest("INVOICE_TAX_NUMBER_TOO_LONG", "invoice tax number is too long")
	case input.Email == "":
		return input, infraerrors.BadRequest("INVOICE_EMAIL_REQUIRED", "invoice email is required")
	case len(input.Email) > 255:
		return input, infraerrors.BadRequest("INVOICE_EMAIL_TOO_LONG", "invoice email is too long")
	}
	if _, err := mail.ParseAddress(input.Email); err != nil {
		return input, infraerrors.BadRequest("INVOICE_EMAIL_INVALID", "invoice email is invalid")
	}
	return input, nil
}

func normalizeOptionalString(value *string) *string {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func invoiceSnapshotFromProfile(profile InvoiceProfile) InvoiceProfileSnapshot {
	return InvoiceProfileSnapshot{
		Title:       profile.Title,
		TaxNumber:   profile.TaxNumber,
		Email:       profile.Email,
		Address:     profile.Address,
		Phone:       profile.Phone,
		BankName:    profile.BankName,
		BankAccount: profile.BankAccount,
	}
}

func scanInvoiceProfile(scanner interface {
	Scan(dest ...any) error
}) (InvoiceProfile, error) {
	var profile InvoiceProfile
	if err := scanner.Scan(
		&profile.ID,
		&profile.UserID,
		&profile.Title,
		&profile.TaxNumber,
		&profile.Email,
		&profile.Address,
		&profile.Phone,
		&profile.BankName,
		&profile.BankAccount,
		&profile.IsDefault,
		&profile.CreatedAt,
		&profile.UpdatedAt,
	); err != nil {
		return InvoiceProfile{}, err
	}
	return profile, nil
}

func (s *PaymentService) ListInvoiceProfiles(ctx context.Context, userID int64) ([]InvoiceProfile, error) {
	db, err := s.invoiceDB()
	if err != nil {
		return nil, err
	}
	rows, err := db.QueryContext(ctx, `
		SELECT id, user_id, title, tax_number, email, address, phone, bank_name, bank_account, is_default, created_at, updated_at
		FROM invoice_profiles
		WHERE user_id = $1
		ORDER BY is_default DESC, updated_at DESC, id DESC
	`, userID)
	if err != nil {
		return nil, infraerrors.InternalServer("INVOICE_PROFILE_LIST_FAILED", "failed to list invoice profiles").WithCause(err)
	}
	defer rows.Close()

	profiles := make([]InvoiceProfile, 0)
	for rows.Next() {
		profile, err := scanInvoiceProfile(rows)
		if err != nil {
			return nil, infraerrors.InternalServer("INVOICE_PROFILE_SCAN_FAILED", "failed to scan invoice profile").WithCause(err)
		}
		profiles = append(profiles, profile)
	}
	if err := rows.Err(); err != nil {
		return nil, infraerrors.InternalServer("INVOICE_PROFILE_LIST_FAILED", "failed to list invoice profiles").WithCause(err)
	}
	return profiles, nil
}

func (s *PaymentService) CreateInvoiceProfile(ctx context.Context, userID int64, input InvoiceProfileInput) (*InvoiceProfile, error) {
	input, err := normalizeInvoiceProfileInput(input)
	if err != nil {
		return nil, err
	}
	db, err := s.invoiceDB()
	if err != nil {
		return nil, err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, infraerrors.InternalServer("INVOICE_PROFILE_CREATE_FAILED", "failed to begin invoice profile transaction").WithCause(err)
	}
	defer rollbackIfActive(tx)

	var existingCount int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM invoice_profiles WHERE user_id = $1`, userID).Scan(&existingCount); err != nil {
		return nil, infraerrors.InternalServer("INVOICE_PROFILE_CREATE_FAILED", "failed to inspect invoice profiles").WithCause(err)
	}

	profile, err := scanInvoiceProfile(tx.QueryRowContext(ctx, `
		INSERT INTO invoice_profiles (user_id, title, tax_number, email, address, phone, bank_name, bank_account, is_default)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id, user_id, title, tax_number, email, address, phone, bank_name, bank_account, is_default, created_at, updated_at
	`, userID, input.Title, input.TaxNumber, input.Email, input.Address, input.Phone, input.BankName, input.BankAccount, existingCount == 0))
	if err != nil {
		return nil, infraerrors.InternalServer("INVOICE_PROFILE_CREATE_FAILED", "failed to create invoice profile").WithCause(err)
	}
	if err := tx.Commit(); err != nil {
		return nil, infraerrors.InternalServer("INVOICE_PROFILE_CREATE_FAILED", "failed to commit invoice profile").WithCause(err)
	}
	return &profile, nil
}

func (s *PaymentService) UpdateInvoiceProfile(ctx context.Context, userID, profileID int64, input InvoiceProfileInput) (*InvoiceProfile, error) {
	input, err := normalizeInvoiceProfileInput(input)
	if err != nil {
		return nil, err
	}
	db, err := s.invoiceDB()
	if err != nil {
		return nil, err
	}
	profile, err := scanInvoiceProfile(db.QueryRowContext(ctx, `
		UPDATE invoice_profiles
		SET title = $3,
			tax_number = $4,
			email = $5,
			address = $6,
			phone = $7,
			bank_name = $8,
			bank_account = $9,
			updated_at = NOW()
		WHERE id = $1 AND user_id = $2
		RETURNING id, user_id, title, tax_number, email, address, phone, bank_name, bank_account, is_default, created_at, updated_at
	`, profileID, userID, input.Title, input.TaxNumber, input.Email, input.Address, input.Phone, input.BankName, input.BankAccount))
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, infraerrors.NotFound("INVOICE_PROFILE_NOT_FOUND", "invoice profile not found")
		}
		return nil, infraerrors.InternalServer("INVOICE_PROFILE_UPDATE_FAILED", "failed to update invoice profile").WithCause(err)
	}
	return &profile, nil
}

func (s *PaymentService) DeleteInvoiceProfile(ctx context.Context, userID, profileID int64) error {
	db, err := s.invoiceDB()
	if err != nil {
		return err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return infraerrors.InternalServer("INVOICE_PROFILE_DELETE_FAILED", "failed to begin invoice profile transaction").WithCause(err)
	}
	defer rollbackIfActive(tx)

	var wasDefault bool
	if err := tx.QueryRowContext(ctx, `
		DELETE FROM invoice_profiles
		WHERE id = $1 AND user_id = $2
		RETURNING is_default
	`, profileID, userID).Scan(&wasDefault); err != nil {
		if err == sql.ErrNoRows {
			return infraerrors.NotFound("INVOICE_PROFILE_NOT_FOUND", "invoice profile not found")
		}
		return infraerrors.InternalServer("INVOICE_PROFILE_DELETE_FAILED", "failed to delete invoice profile").WithCause(err)
	}

	if wasDefault {
		if _, err := tx.ExecContext(ctx, `
			UPDATE invoice_profiles
			SET is_default = true, updated_at = NOW()
			WHERE id = (
				SELECT id FROM invoice_profiles
				WHERE user_id = $1
				ORDER BY updated_at DESC, id DESC
				LIMIT 1
			)
		`, userID); err != nil {
			return infraerrors.InternalServer("INVOICE_PROFILE_DELETE_FAILED", "failed to promote default invoice profile").WithCause(err)
		}
	}
	if err := tx.Commit(); err != nil {
		return infraerrors.InternalServer("INVOICE_PROFILE_DELETE_FAILED", "failed to commit invoice profile delete").WithCause(err)
	}
	return nil
}

func (s *PaymentService) SetDefaultInvoiceProfile(ctx context.Context, userID, profileID int64) (*InvoiceProfile, error) {
	db, err := s.invoiceDB()
	if err != nil {
		return nil, err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, infraerrors.InternalServer("INVOICE_PROFILE_DEFAULT_FAILED", "failed to begin invoice profile transaction").WithCause(err)
	}
	defer rollbackIfActive(tx)

	var exists bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM invoice_profiles WHERE id = $1 AND user_id = $2)`, profileID, userID).Scan(&exists); err != nil {
		return nil, infraerrors.InternalServer("INVOICE_PROFILE_DEFAULT_FAILED", "failed to inspect invoice profile").WithCause(err)
	}
	if !exists {
		return nil, infraerrors.NotFound("INVOICE_PROFILE_NOT_FOUND", "invoice profile not found")
	}
	if _, err := tx.ExecContext(ctx, `UPDATE invoice_profiles SET is_default = false, updated_at = NOW() WHERE user_id = $1`, userID); err != nil {
		return nil, infraerrors.InternalServer("INVOICE_PROFILE_DEFAULT_FAILED", "failed to reset default invoice profile").WithCause(err)
	}
	profile, err := scanInvoiceProfile(tx.QueryRowContext(ctx, `
		UPDATE invoice_profiles
		SET is_default = true, updated_at = NOW()
		WHERE id = $1 AND user_id = $2
		RETURNING id, user_id, title, tax_number, email, address, phone, bank_name, bank_account, is_default, created_at, updated_at
	`, profileID, userID))
	if err != nil {
		return nil, infraerrors.InternalServer("INVOICE_PROFILE_DEFAULT_FAILED", "failed to set default invoice profile").WithCause(err)
	}
	if err := tx.Commit(); err != nil {
		return nil, infraerrors.InternalServer("INVOICE_PROFILE_DEFAULT_FAILED", "failed to commit default invoice profile").WithCause(err)
	}
	return &profile, nil
}

func (s *PaymentService) CreateInvoiceRequest(ctx context.Context, userID int64, input CreateInvoiceRequestInput) (*InvoiceRequest, error) {
	if input.ProfileID <= 0 {
		return nil, infraerrors.BadRequest("INVOICE_PROFILE_REQUIRED", "invoice profile is required")
	}
	orderIDs := uniquePositiveInt64s(input.OrderIDs)
	if len(orderIDs) == 0 {
		return nil, infraerrors.BadRequest("INVOICE_ORDER_REQUIRED", "at least one order is required")
	}
	if len(orderIDs) > maxInvoiceOrdersPerRequest {
		return nil, infraerrors.BadRequest("INVOICE_ORDER_LIMIT_EXCEEDED", "too many orders in one invoice request")
	}

	db, err := s.invoiceDB()
	if err != nil {
		return nil, err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, infraerrors.InternalServer("INVOICE_REQUEST_CREATE_FAILED", "failed to begin invoice request transaction").WithCause(err)
	}
	defer rollbackIfActive(tx)

	profile, err := scanInvoiceProfile(tx.QueryRowContext(ctx, `
		SELECT id, user_id, title, tax_number, email, address, phone, bank_name, bank_account, is_default, created_at, updated_at
		FROM invoice_profiles
		WHERE id = $1 AND user_id = $2
	`, input.ProfileID, userID))
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, infraerrors.NotFound("INVOICE_PROFILE_NOT_FOUND", "invoice profile not found")
		}
		return nil, infraerrors.InternalServer("INVOICE_REQUEST_CREATE_FAILED", "failed to load invoice profile").WithCause(err)
	}

	orders, err := queryInvoiceableOrdersByIDs(ctx, tx, userID, orderIDs)
	if err != nil {
		return nil, err
	}
	if len(orders) != len(orderIDs) {
		return nil, infraerrors.BadRequest("INVOICE_ORDER_INVALID", "some orders cannot be invoiced")
	}

	var alreadyInvoiced int
	if err := tx.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM invoice_request_orders iro
		JOIN invoice_requests ir ON ir.id = iro.invoice_request_id
		WHERE iro.payment_order_id = ANY($1) AND ir.status <> $2
	`, pq.Array(orderIDs), InvoiceStatusRejected).Scan(&alreadyInvoiced); err != nil {
		return nil, infraerrors.InternalServer("INVOICE_REQUEST_CREATE_FAILED", "failed to inspect invoiced orders").WithCause(err)
	}
	if alreadyInvoiced > 0 {
		return nil, infraerrors.Conflict("INVOICE_ORDER_ALREADY_REQUESTED", "some orders already have invoice requests")
	}

	snapshot := invoiceSnapshotFromProfile(profile)
	snapshotBytes, err := json.Marshal(snapshot)
	if err != nil {
		return nil, infraerrors.InternalServer("INVOICE_REQUEST_CREATE_FAILED", "failed to encode invoice profile snapshot").WithCause(err)
	}
	totalAmount := 0.0
	for _, order := range orders {
		totalAmount += order.PayAmount
	}

	serialNo := generateInvoiceSerialNo()
	var req InvoiceRequest
	var profileSnapshotRaw []byte
	err = tx.QueryRowContext(ctx, `
		INSERT INTO invoice_requests (user_id, profile_id, serial_no, status, profile_snapshot, total_amount)
		VALUES ($1, $2, $3, $4, $5::jsonb, $6)
		RETURNING id, user_id, profile_id, serial_no, status, profile_snapshot, total_amount::float8, reject_reason, created_at, updated_at, completed_at
	`, userID, input.ProfileID, serialNo, InvoiceStatusPending, string(snapshotBytes), totalAmount).Scan(
		&req.ID,
		&req.UserID,
		&req.ProfileID,
		&req.SerialNo,
		&req.Status,
		&profileSnapshotRaw,
		&req.TotalAmount,
		&req.RejectReason,
		&req.CreatedAt,
		&req.UpdatedAt,
		&req.CompletedAt,
	)
	if err != nil {
		return nil, infraerrors.InternalServer("INVOICE_REQUEST_CREATE_FAILED", "failed to create invoice request").WithCause(err)
	}
	if err := json.Unmarshal(profileSnapshotRaw, &req.ProfileSnapshot); err != nil {
		return nil, infraerrors.InternalServer("INVOICE_REQUEST_CREATE_FAILED", "failed to decode invoice profile snapshot").WithCause(err)
	}
	for _, order := range orders {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO invoice_request_orders (invoice_request_id, payment_order_id)
			VALUES ($1, $2)
		`, req.ID, order.ID); err != nil {
			return nil, infraerrors.InternalServer("INVOICE_REQUEST_CREATE_FAILED", "failed to attach invoice order").WithCause(err)
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, infraerrors.InternalServer("INVOICE_REQUEST_CREATE_FAILED", "failed to commit invoice request").WithCause(err)
	}
	req.Orders = orders
	return &req, nil
}

func (s *PaymentService) ListInvoiceRequests(ctx context.Context, userID int64, params InvoiceRequestListParams) ([]InvoiceRequest, int, error) {
	db, err := s.invoiceDB()
	if err != nil {
		return nil, 0, err
	}
	params.Page, params.PageSize = normalizeInvoicePagination(params.Page, params.PageSize)
	where, args, err := buildInvoiceRequestWhere(userID, params)
	if err != nil {
		return nil, 0, err
	}

	var total int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM invoice_requests `+where, args...).Scan(&total); err != nil {
		return nil, 0, infraerrors.InternalServer("INVOICE_REQUEST_LIST_FAILED", "failed to count invoice requests").WithCause(err)
	}

	queryArgs := append([]any{}, args...)
	queryArgs = append(queryArgs, params.PageSize, (params.Page-1)*params.PageSize)
	rows, err := db.QueryContext(ctx, `
		SELECT id, user_id, profile_id, serial_no, status, profile_snapshot, total_amount::float8, reject_reason, created_at, updated_at, completed_at
		FROM invoice_requests
		`+where+`
		ORDER BY created_at DESC, id DESC
		LIMIT $`+fmt.Sprint(len(args)+1)+` OFFSET $`+fmt.Sprint(len(args)+2), queryArgs...)
	if err != nil {
		return nil, 0, infraerrors.InternalServer("INVOICE_REQUEST_LIST_FAILED", "failed to list invoice requests").WithCause(err)
	}
	defer rows.Close()

	requests := make([]InvoiceRequest, 0)
	requestIDs := make([]int64, 0)
	for rows.Next() {
		req, err := scanInvoiceRequest(rows)
		if err != nil {
			return nil, 0, err
		}
		requestIDs = append(requestIDs, req.ID)
		requests = append(requests, req)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, infraerrors.InternalServer("INVOICE_REQUEST_LIST_FAILED", "failed to list invoice requests").WithCause(err)
	}
	if len(requestIDs) > 0 {
		ordersByRequest, err := queryInvoiceRequestOrders(ctx, db, requestIDs)
		if err != nil {
			return nil, 0, err
		}
		for i := range requests {
			requests[i].Orders = ordersByRequest[requests[i].ID]
		}
	}
	return requests, total, nil
}

func (s *PaymentService) ListInvoiceableOrders(ctx context.Context, userID int64, page, pageSize int) ([]InvoiceRequestOrder, int, error) {
	db, err := s.invoiceDB()
	if err != nil {
		return nil, 0, err
	}
	page, pageSize = normalizeInvoicePagination(page, pageSize)
	where := `
		WHERE po.user_id = $1
			AND po.status = $2
			AND NOT EXISTS (
				SELECT 1
				FROM invoice_request_orders iro
				JOIN invoice_requests ir ON ir.id = iro.invoice_request_id
				WHERE iro.payment_order_id = po.id AND ir.status <> $3
			)
	`
	var total int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM payment_orders po `+where, userID, OrderStatusCompleted, InvoiceStatusRejected).Scan(&total); err != nil {
		return nil, 0, infraerrors.InternalServer("INVOICE_ORDER_LIST_FAILED", "failed to count invoiceable orders").WithCause(err)
	}
	rows, err := db.QueryContext(ctx, `
		SELECT po.id, po.out_trade_no, po.pay_amount::float8, po.payment_type, po.order_type, po.status, po.created_at, po.completed_at
		FROM payment_orders po
		`+where+`
		ORDER BY po.completed_at DESC NULLS LAST, po.created_at DESC, po.id DESC
		LIMIT $4 OFFSET $5
	`, userID, OrderStatusCompleted, InvoiceStatusRejected, pageSize, (page-1)*pageSize)
	if err != nil {
		return nil, 0, infraerrors.InternalServer("INVOICE_ORDER_LIST_FAILED", "failed to list invoiceable orders").WithCause(err)
	}
	defer rows.Close()

	orders := make([]InvoiceRequestOrder, 0)
	for rows.Next() {
		order, err := scanInvoiceRequestOrder(rows)
		if err != nil {
			return nil, 0, infraerrors.InternalServer("INVOICE_ORDER_LIST_FAILED", "failed to scan invoiceable order").WithCause(err)
		}
		orders = append(orders, order)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, infraerrors.InternalServer("INVOICE_ORDER_LIST_FAILED", "failed to list invoiceable orders").WithCause(err)
	}
	return orders, total, nil
}

func scanInvoiceRequest(scanner interface {
	Scan(dest ...any) error
}) (InvoiceRequest, error) {
	var req InvoiceRequest
	var profileSnapshotRaw []byte
	if err := scanner.Scan(
		&req.ID,
		&req.UserID,
		&req.ProfileID,
		&req.SerialNo,
		&req.Status,
		&profileSnapshotRaw,
		&req.TotalAmount,
		&req.RejectReason,
		&req.CreatedAt,
		&req.UpdatedAt,
		&req.CompletedAt,
	); err != nil {
		return InvoiceRequest{}, infraerrors.InternalServer("INVOICE_REQUEST_SCAN_FAILED", "failed to scan invoice request").WithCause(err)
	}
	if len(profileSnapshotRaw) > 0 {
		if err := json.Unmarshal(profileSnapshotRaw, &req.ProfileSnapshot); err != nil {
			return InvoiceRequest{}, infraerrors.InternalServer("INVOICE_REQUEST_SCAN_FAILED", "failed to decode invoice profile snapshot").WithCause(err)
		}
	}
	return req, nil
}

func queryInvoiceableOrdersByIDs(ctx context.Context, tx *sql.Tx, userID int64, orderIDs []int64) ([]InvoiceRequestOrder, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT id, out_trade_no, pay_amount::float8, payment_type, order_type, status, created_at, completed_at
		FROM payment_orders
		WHERE user_id = $1 AND status = $2 AND id = ANY($3)
		ORDER BY completed_at DESC NULLS LAST, created_at DESC, id DESC
	`, userID, OrderStatusCompleted, pq.Array(orderIDs))
	if err != nil {
		return nil, infraerrors.InternalServer("INVOICE_ORDER_LOAD_FAILED", "failed to load invoice orders").WithCause(err)
	}
	defer rows.Close()

	orders := make([]InvoiceRequestOrder, 0)
	for rows.Next() {
		order, err := scanInvoiceRequestOrder(rows)
		if err != nil {
			return nil, infraerrors.InternalServer("INVOICE_ORDER_SCAN_FAILED", "failed to scan invoice order").WithCause(err)
		}
		orders = append(orders, order)
	}
	if err := rows.Err(); err != nil {
		return nil, infraerrors.InternalServer("INVOICE_ORDER_LOAD_FAILED", "failed to load invoice orders").WithCause(err)
	}
	return orders, nil
}

func queryInvoiceRequestOrders(ctx context.Context, db *sql.DB, requestIDs []int64) (map[int64][]InvoiceRequestOrder, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT iro.invoice_request_id, po.id, po.out_trade_no, po.pay_amount::float8, po.payment_type, po.order_type, po.status, po.created_at, po.completed_at
		FROM invoice_request_orders iro
		JOIN payment_orders po ON po.id = iro.payment_order_id
		WHERE iro.invoice_request_id = ANY($1)
		ORDER BY po.created_at DESC, po.id DESC
	`, pq.Array(requestIDs))
	if err != nil {
		return nil, infraerrors.InternalServer("INVOICE_REQUEST_ORDER_LIST_FAILED", "failed to list invoice request orders").WithCause(err)
	}
	defer rows.Close()

	out := make(map[int64][]InvoiceRequestOrder, len(requestIDs))
	for rows.Next() {
		var requestID int64
		var order InvoiceRequestOrder
		if err := rows.Scan(&requestID, &order.ID, &order.OutTradeNo, &order.PayAmount, &order.PaymentType, &order.OrderType, &order.Status, &order.CreatedAt, &order.CompletedAt); err != nil {
			return nil, infraerrors.InternalServer("INVOICE_REQUEST_ORDER_SCAN_FAILED", "failed to scan invoice request order").WithCause(err)
		}
		out[requestID] = append(out[requestID], order)
	}
	if err := rows.Err(); err != nil {
		return nil, infraerrors.InternalServer("INVOICE_REQUEST_ORDER_LIST_FAILED", "failed to list invoice request orders").WithCause(err)
	}
	return out, nil
}

func scanInvoiceRequestOrder(scanner interface {
	Scan(dest ...any) error
}) (InvoiceRequestOrder, error) {
	var order InvoiceRequestOrder
	err := scanner.Scan(
		&order.ID,
		&order.OutTradeNo,
		&order.PayAmount,
		&order.PaymentType,
		&order.OrderType,
		&order.Status,
		&order.CreatedAt,
		&order.CompletedAt,
	)
	return order, err
}

func buildInvoiceRequestWhere(userID int64, params InvoiceRequestListParams) (string, []any, error) {
	args := []any{userID}
	conditions := []string{"user_id = $1"}

	status := strings.TrimSpace(params.Status)
	if status != "" {
		if !isValidInvoiceStatus(status) {
			return "", nil, infraerrors.BadRequest("INVOICE_STATUS_INVALID", "invoice status is invalid")
		}
		args = append(args, status)
		conditions = append(conditions, fmt.Sprintf("status = $%d", len(args)))
	}
	if strings.TrimSpace(params.StartTime) != "" {
		start, err := parseInvoiceDate(params.StartTime)
		if err != nil {
			return "", nil, infraerrors.BadRequest("INVOICE_DATE_INVALID", "invoice start date is invalid")
		}
		args = append(args, start)
		conditions = append(conditions, fmt.Sprintf("created_at >= $%d", len(args)))
	}
	if strings.TrimSpace(params.EndTime) != "" {
		end, err := parseInvoiceDate(params.EndTime)
		if err != nil {
			return "", nil, infraerrors.BadRequest("INVOICE_DATE_INVALID", "invoice end date is invalid")
		}
		args = append(args, end.Add(24*time.Hour))
		conditions = append(conditions, fmt.Sprintf("created_at < $%d", len(args)))
	}
	return "WHERE " + strings.Join(conditions, " AND "), args, nil
}

func isValidInvoiceStatus(status string) bool {
	switch status {
	case InvoiceStatusPending, InvoiceStatusCompleted, InvoiceStatusRejected:
		return true
	default:
		return false
	}
}

func parseInvoiceDate(raw string) (time.Time, error) {
	return time.ParseInLocation("2006-01-02", strings.TrimSpace(raw), time.Local)
}

func normalizeInvoicePagination(page, pageSize int) (int, int) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = defaultPageSize
	}
	if pageSize > maxPageSize {
		pageSize = maxPageSize
	}
	return page, pageSize
}

func uniquePositiveInt64s(values []int64) []int64 {
	seen := make(map[int64]struct{}, len(values))
	out := make([]int64, 0, len(values))
	for _, value := range values {
		if value <= 0 {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func generateInvoiceSerialNo() string {
	return "INV" + time.Now().Format("20060102") + strings.ToUpper(generateRandomString(8))
}

func rollbackIfActive(tx *sql.Tx) {
	if tx != nil {
		_ = tx.Rollback()
	}
}
