package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const (
	defaultInvoiceUploadDir = "./data/invoices"
	maxInvoiceFileBytes     = 10 * 1024 * 1024 // 10 MB
)

// invoiceNoFormat validates an issued invoice number.
// Accepts:
//   - 8-digit traditional Chinese invoice numbers
//   - 20-digit electronic (数电) invoice numbers
//   - any alphanumeric+hyphen string of 6-64 chars (international fallback)
//
// Whitespace inside the input is rejected — caller must trim before validating.
var invoiceNoFormat = regexp.MustCompile(`^[A-Za-z0-9-]{6,64}$`)

// allowedInvoiceMimeTypes whitelists upload MIME types for invoices.
var allowedInvoiceMimeTypes = map[string]bool{
	"application/pdf":               true,
	"image/png":                     true,
	"image/jpeg":                    true,
	"image/jpg":                     true,
	"application/zip":               true,
	"application/x-zip-compressed":  true,
	"application/octet-stream":      true,
	"application/vnd.ms-excel":      true,
	"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet": true,
}

// InvoiceUploadDir returns the directory used to store invoice files.
// Override via INVOICE_UPLOAD_DIR environment variable.
func InvoiceUploadDir() string {
	if dir := strings.TrimSpace(os.Getenv("INVOICE_UPLOAD_DIR")); dir != "" {
		return dir
	}
	return defaultInvoiceUploadDir
}

// AdminInvoiceListParams filters AdminListInvoiceRequests results.
type AdminInvoiceListParams struct {
	Page      int
	PageSize  int
	Status    string
	Keyword   string // Match against serial_no, profile title, tax_number, username, user email
	UserID    int64
	StartTime string
	EndTime   string
}

// AdminListInvoiceRequests returns paginated invoice requests across all users (admin view).
func (s *PaymentService) AdminListInvoiceRequests(ctx context.Context, params AdminInvoiceListParams) ([]AdminInvoiceRequest, int, error) {
	db, err := s.invoiceDB()
	if err != nil {
		return nil, 0, err
	}
	params.Page, params.PageSize = normalizeInvoicePagination(params.Page, params.PageSize)

	args := []any{}
	conds := []string{"1=1"}

	if status := strings.TrimSpace(params.Status); status != "" {
		if !isValidInvoiceStatus(status) {
			return nil, 0, infraerrors.BadRequest("INVOICE_STATUS_INVALID", "invoice status is invalid")
		}
		args = append(args, status)
		conds = append(conds, fmt.Sprintf("ir.status = $%d", len(args)))
	}
	if params.UserID > 0 {
		args = append(args, params.UserID)
		conds = append(conds, fmt.Sprintf("ir.user_id = $%d", len(args)))
	}
	if start := strings.TrimSpace(params.StartTime); start != "" {
		t, err := parseInvoiceDate(start)
		if err != nil {
			return nil, 0, infraerrors.BadRequest("INVOICE_DATE_INVALID", "invoice start date is invalid")
		}
		args = append(args, t)
		conds = append(conds, fmt.Sprintf("ir.created_at >= $%d", len(args)))
	}
	if end := strings.TrimSpace(params.EndTime); end != "" {
		t, err := parseInvoiceDate(end)
		if err != nil {
			return nil, 0, infraerrors.BadRequest("INVOICE_DATE_INVALID", "invoice end date is invalid")
		}
		args = append(args, t.Add(24*time.Hour))
		conds = append(conds, fmt.Sprintf("ir.created_at < $%d", len(args)))
	}
	if kw := strings.TrimSpace(params.Keyword); kw != "" {
		args = append(args, "%"+kw+"%")
		conds = append(conds, fmt.Sprintf(`(
			ir.serial_no ILIKE $%d OR
			ir.profile_snapshot->>'title' ILIKE $%d OR
			ir.profile_snapshot->>'tax_number' ILIKE $%d OR
			u.username ILIKE $%d OR
			u.email ILIKE $%d
		)`, len(args), len(args), len(args), len(args), len(args)))
	}

	where := "WHERE " + strings.Join(conds, " AND ")

	var total int
	if err := db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM invoice_requests ir
		LEFT JOIN users u ON u.id = ir.user_id
		`+where, args...).Scan(&total); err != nil {
		return nil, 0, infraerrors.InternalServer("INVOICE_REQUEST_LIST_FAILED", "failed to count invoice requests").WithCause(err)
	}

	queryArgs := append([]any{}, args...)
	queryArgs = append(queryArgs, params.PageSize, (params.Page-1)*params.PageSize)
	rows, err := db.QueryContext(ctx, `
		SELECT `+prefixedInvoiceColumns("ir")+`,
		       u.username, u.email
		FROM invoice_requests ir
		LEFT JOIN users u ON u.id = ir.user_id
		`+where+`
		ORDER BY ir.created_at DESC, ir.id DESC
		LIMIT $`+strconv.Itoa(len(args)+1)+` OFFSET $`+strconv.Itoa(len(args)+2), queryArgs...)
	if err != nil {
		return nil, 0, infraerrors.InternalServer("INVOICE_REQUEST_LIST_FAILED", "failed to list invoice requests").WithCause(err)
	}
	defer rows.Close()

	results := make([]AdminInvoiceRequest, 0)
	requestIDs := make([]int64, 0)
	for rows.Next() {
		req, username, email, err := scanAdminInvoiceRequest(rows)
		if err != nil {
			return nil, 0, err
		}
		requestIDs = append(requestIDs, req.ID)
		results = append(results, AdminInvoiceRequest{
			InvoiceRequest: req,
			Username:       username,
			UserEmail:      email,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, 0, infraerrors.InternalServer("INVOICE_REQUEST_LIST_FAILED", "failed to list invoice requests").WithCause(err)
	}

	if len(requestIDs) > 0 {
		ordersByRequest, err := queryInvoiceRequestOrders(ctx, db, requestIDs)
		if err != nil {
			return nil, 0, err
		}
		for i := range results {
			results[i].Orders = ordersByRequest[results[i].ID]
		}
	}
	return results, total, nil
}

// GetInvoiceRequest fetches a single invoice request scoped to the user.
func (s *PaymentService) GetInvoiceRequest(ctx context.Context, userID, reqID int64) (*InvoiceRequest, error) {
	return s.loadInvoiceRequest(ctx, userID, reqID)
}

// GetInvoiceRequestForAdmin fetches an invoice request without user scoping.
func (s *PaymentService) GetInvoiceRequestForAdmin(ctx context.Context, reqID int64) (*InvoiceRequest, error) {
	return s.loadInvoiceRequest(ctx, 0, reqID)
}

// loadInvoiceRequest loads a single invoice request. Pass userID=0 for admin scope.
func (s *PaymentService) loadInvoiceRequest(ctx context.Context, userID, reqID int64) (*InvoiceRequest, error) {
	db, err := s.invoiceDB()
	if err != nil {
		return nil, err
	}
	args := []any{reqID}
	cond := "id = $1"
	if userID > 0 {
		args = append(args, userID)
		cond += " AND user_id = $2"
	}
	row := db.QueryRowContext(ctx, `
		SELECT `+invoiceRequestColumns+`
		FROM invoice_requests
		WHERE `+cond, args...)
	req, err := scanInvoiceRequest(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, infraerrors.NotFound("INVOICE_REQUEST_NOT_FOUND", "invoice request not found")
		}
		return nil, err
	}

	orders, err := queryInvoiceRequestOrders(ctx, db, []int64{req.ID})
	if err != nil {
		return nil, err
	}
	req.Orders = orders[req.ID]
	return &req, nil
}

// CancelInvoiceRequest hard-deletes a pending invoice request owned by the user.
// Associated orders return to the invoiceable pool automatically (FK cascades on invoice_request_orders).
func (s *PaymentService) CancelInvoiceRequest(ctx context.Context, userID, reqID int64) error {
	db, err := s.invoiceDB()
	if err != nil {
		return err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return infraerrors.InternalServer("INVOICE_REQUEST_CANCEL_FAILED", "failed to begin invoice cancel transaction").WithCause(err)
	}
	defer rollbackIfActive(tx)

	var status string
	if err := tx.QueryRowContext(ctx, `
		SELECT status
		FROM invoice_requests
		WHERE id = $1 AND user_id = $2
		FOR UPDATE
	`, reqID, userID).Scan(&status); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return infraerrors.NotFound("INVOICE_REQUEST_NOT_FOUND", "invoice request not found")
		}
		return infraerrors.InternalServer("INVOICE_REQUEST_CANCEL_FAILED", "failed to load invoice request").WithCause(err)
	}
	if status != InvoiceStatusPending {
		return infraerrors.Conflict("INVOICE_REQUEST_NOT_CANCELLABLE", "only pending invoice requests can be cancelled")
	}

	if _, err := tx.ExecContext(ctx, `DELETE FROM invoice_request_orders WHERE invoice_request_id = $1`, reqID); err != nil {
		return infraerrors.InternalServer("INVOICE_REQUEST_CANCEL_FAILED", "failed to detach invoice orders").WithCause(err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM invoice_requests WHERE id = $1 AND user_id = $2`, reqID, userID); err != nil {
		return infraerrors.InternalServer("INVOICE_REQUEST_CANCEL_FAILED", "failed to delete invoice request").WithCause(err)
	}
	if err := tx.Commit(); err != nil {
		return infraerrors.InternalServer("INVOICE_REQUEST_CANCEL_FAILED", "failed to commit invoice cancel").WithCause(err)
	}

	return nil
}

// RejectInvoiceRequest marks a pending invoice request as rejected.
func (s *PaymentService) RejectInvoiceRequest(ctx context.Context, adminID, reqID int64, reason string) (*InvoiceRequest, error) {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return nil, infraerrors.BadRequest("INVOICE_REJECT_REASON_REQUIRED", "rejection reason is required")
	}
	if len(reason) > 1000 {
		return nil, infraerrors.BadRequest("INVOICE_REJECT_REASON_TOO_LONG", "rejection reason is too long")
	}

	db, err := s.invoiceDB()
	if err != nil {
		return nil, err
	}
	row := db.QueryRowContext(ctx, `
		UPDATE invoice_requests
		SET status = $2,
		    reject_reason = $3,
		    processed_by = $4,
		    processed_at = NOW(),
		    updated_at = NOW()
		WHERE id = $1 AND status = $5
		RETURNING `+invoiceRequestColumns,
		reqID, InvoiceStatusRejected, reason, adminID, InvoiceStatusPending)
	req, err := scanInvoiceRequest(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, invoiceRequestStateError(ctx, db, reqID)
		}
		return nil, err
	}

	orders, err := queryInvoiceRequestOrders(ctx, db, []int64{req.ID})
	if err != nil {
		return nil, err
	}
	req.Orders = orders[req.ID]
	return &req, nil
}

// CompleteInvoiceRequestInput is the input for completing an invoice request.
type CompleteInvoiceRequestInput struct {
	InvoiceNo string
	File      *multipart.FileHeader
}

// CompleteInvoiceRequest sends the uploaded invoice file as an email attachment
// to the requester, and on successful send marks the request as completed.
// No file is persisted to disk; the file metadata columns remain NULL.
// Failure to send the email leaves the request in its current pending state.
func (s *PaymentService) CompleteInvoiceRequest(ctx context.Context, adminID, reqID int64, input CompleteInvoiceRequestInput) (*InvoiceRequest, error) {
	invoiceNo := strings.TrimSpace(input.InvoiceNo)
	if invoiceNo == "" {
		return nil, infraerrors.BadRequest("INVOICE_NO_REQUIRED", "invoice number is required")
	}
	if len(invoiceNo) > 64 {
		return nil, infraerrors.BadRequest("INVOICE_NO_TOO_LONG", "invoice number is too long")
	}
	if !invoiceNoFormat.MatchString(invoiceNo) {
		return nil, infraerrors.BadRequest("INVOICE_NO_FORMAT_INVALID", "invoice number must be 6-64 alphanumeric characters or hyphens")
	}
	if input.File == nil {
		return nil, infraerrors.BadRequest("INVOICE_FILE_REQUIRED", "invoice file is required")
	}
	if input.File.Size <= 0 {
		return nil, infraerrors.BadRequest("INVOICE_FILE_EMPTY", "invoice file is empty")
	}
	if input.File.Size > maxInvoiceFileBytes {
		return nil, infraerrors.BadRequest("INVOICE_FILE_TOO_LARGE", "invoice file exceeds 10 MB")
	}
	mimeType := strings.TrimSpace(input.File.Header.Get("Content-Type"))
	if mimeType != "" && !allowedInvoiceMimeTypes[mimeType] {
		return nil, infraerrors.BadRequest("INVOICE_FILE_TYPE_INVALID", "invoice file type is not allowed")
	}

	if s.invoiceEmailSender == nil {
		return nil, infraerrors.ServiceUnavailable("INVOICE_EMAIL_NOT_CONFIGURED", "invoice email sender is not configured")
	}

	// Load the request to confirm it is still pending and to capture the recipient.
	loaded, err := s.loadInvoiceRequest(ctx, 0, reqID)
	if err != nil {
		return nil, err
	}
	if loaded.Status != InvoiceStatusPending {
		return nil, infraerrors.Conflict("INVOICE_REQUEST_NOT_PENDING", "invoice request is not pending")
	}
	to := strings.TrimSpace(loaded.ProfileSnapshot.Email)
	if to == "" {
		return nil, infraerrors.BadRequest("INVOICE_EMAIL_REQUIRED", "profile email is required")
	}

	// Read the uploaded file into memory (bounded).
	src, err := input.File.Open()
	if err != nil {
		return nil, infraerrors.BadRequest("INVOICE_FILE_OPEN_FAILED", "failed to read uploaded file")
	}
	defer src.Close()
	content, err := io.ReadAll(io.LimitReader(src, maxInvoiceFileBytes+1))
	if err != nil {
		return nil, infraerrors.InternalServer("INVOICE_FILE_READ_FAILED", "failed to read invoice file").WithCause(err)
	}
	if int64(len(content)) > maxInvoiceFileBytes {
		return nil, infraerrors.BadRequest("INVOICE_FILE_TOO_LARGE", "invoice file exceeds 10 MB")
	}

	filename := strings.TrimSpace(input.File.Filename)
	if filename == "" {
		filename = "invoice.pdf"
	}

	// Build the email payload.
	siteName := s.invoiceEmailSiteName(ctx)
	subject := fmt.Sprintf("[%s] 您的发票 %s / Your Invoice %s", siteName, invoiceNo, invoiceNo)
	body := buildInvoiceAttachmentEmailBody(loaded, invoiceNo, siteName)

	att := EmailAttachment{Filename: filename, MimeType: mimeType, Content: content}
	if err := s.invoiceEmailSender.SendEmailWithAttachment(ctx, to, subject, body, []EmailAttachment{att}); err != nil {
		return nil, infraerrors.ServiceUnavailable("INVOICE_EMAIL_SEND_FAILED", "failed to send invoice email").WithCause(err)
	}

	// Email succeeded — promote the request to completed.
	db, err := s.invoiceDB()
	if err != nil {
		return nil, err
	}
	row := db.QueryRowContext(ctx, `
		UPDATE invoice_requests
		SET status = $2,
		    invoice_no = $3,
		    processed_by = $4,
		    processed_at = NOW(),
		    completed_at = NOW(),
		    updated_at = NOW(),
		    reject_reason = NULL
		WHERE id = $1 AND status = $5
		RETURNING `+invoiceRequestColumns,
		reqID,
		InvoiceStatusCompleted,
		invoiceNo,
		adminID,
		InvoiceStatusPending,
	)
	req, err := scanInvoiceRequest(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, invoiceRequestStateError(ctx, db, reqID)
		}
		return nil, err
	}

	orders, err := queryInvoiceRequestOrders(ctx, db, []int64{req.ID})
	if err != nil {
		return nil, err
	}
	req.Orders = orders[req.ID]
	return &req, nil
}

// invoiceRequestStateError returns an appropriate error for state-conflict failures.
func invoiceRequestStateError(ctx context.Context, db *sql.DB, reqID int64) error {
	var status string
	if err := db.QueryRowContext(ctx, `SELECT status FROM invoice_requests WHERE id = $1`, reqID).Scan(&status); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return infraerrors.NotFound("INVOICE_REQUEST_NOT_FOUND", "invoice request not found")
		}
		return infraerrors.InternalServer("INVOICE_REQUEST_LOAD_FAILED", "failed to load invoice request").WithCause(err)
	}
	return infraerrors.Conflict("INVOICE_REQUEST_NOT_PENDING", "invoice request is not pending (current status: "+status+")")
}

// scanAdminInvoiceRequest scans columns from invoice_requests + (username, email).
func scanAdminInvoiceRequest(scanner interface {
	Scan(dest ...any) error
}) (InvoiceRequest, string, string, error) {
	var req InvoiceRequest
	var profileSnapshotRaw []byte
	var filePath, username, email sql.NullString
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
		&req.InvoiceNo,
		&filePath,
		&req.InvoiceFileSize,
		&req.InvoiceFileName,
		&req.InvoiceFileMime,
		&req.ProcessedBy,
		&req.ProcessedAt,
		&req.HasRefundedOrders,
		&req.VoidedAt,
		&req.VoidedReason,
		&username,
		&email,
	); err != nil {
		return InvoiceRequest{}, "", "", infraerrors.InternalServer("INVOICE_REQUEST_SCAN_FAILED", "failed to scan invoice request").WithCause(err)
	}
	req.HasFile = filePath.Valid && filePath.String != ""
	if len(profileSnapshotRaw) > 0 {
		if err := json.Unmarshal(profileSnapshotRaw, &req.ProfileSnapshot); err != nil {
			return InvoiceRequest{}, "", "", infraerrors.InternalServer("INVOICE_REQUEST_SCAN_FAILED", "failed to decode invoice profile snapshot").WithCause(err)
		}
	}
	return req, username.String, email.String, nil
}

func prefixedInvoiceColumns(alias string) string {
	cols := strings.Split(invoiceRequestColumns, ",")
	out := make([]string, 0, len(cols))
	for _, c := range cols {
		c = strings.TrimSpace(c)
		out = append(out, alias+"."+c)
	}
	return strings.Join(out, ", ")
}

