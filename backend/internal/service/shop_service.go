package service

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/lib/pq"
)

const (
	ShopProductActive      = "active"
	ShopProductDisabled    = "disabled"
	ShopInventoryAvailable = "available"
	ShopInventorySold      = "sold"
	ShopInventoryDisabled  = "disabled"
	ShopOrderPaid          = "paid"
)

var (
	ErrShopProductNotFound     = infraerrors.New(404, "SHOP_PRODUCT_NOT_FOUND", "shop product not found")
	ErrShopProductDisabled     = infraerrors.New(409, "SHOP_PRODUCT_DISABLED", "shop product is not available")
	ErrShopOutOfStock          = infraerrors.New(409, "SHOP_OUT_OF_STOCK", "shop product is out of stock")
	ErrShopInsufficientBalance = infraerrors.New(400, "SHOP_INSUFFICIENT_BALANCE", "insufficient balance")
	ErrShopCodeExists          = errors.New("shop code already exists")
	ErrShopPurchaseLimit       = infraerrors.New(409, "SHOP_PURCHASE_LIMIT_REACHED", "purchase limit reached")
	ErrShopInventorySold       = infraerrors.New(409, "SHOP_INVENTORY_ALREADY_SOLD", "sold inventory cannot be changed")
)

type ShopProduct struct {
	ID             int64     `json:"id"`
	Name           string    `json:"name"`
	Description    string    `json:"description"`
	Price          float64   `json:"price"`
	LimitPerUser   *int      `json:"limit_per_user,omitempty"`
	Status         string    `json:"status"`
	AvailableStock int64     `json:"available_stock"`
	SoldCount      int64     `json:"sold_count"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type ShopOrder struct {
	ID          int64     `json:"id"`
	OrderNo     string    `json:"order_no"`
	ProductID   int64     `json:"product_id"`
	ProductName string    `json:"product_name"`
	Price       float64   `json:"price"`
	Code        string    `json:"code"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
}

// ShopOverview is the administrator-facing aggregate for the shop dashboard.
type ShopOverview struct {
	ProductCount   int64   `json:"product_count"`
	ActiveProducts int64   `json:"active_products"`
	AvailableStock int64   `json:"available_stock"`
	SoldCount      int64   `json:"sold_count"`
	Revenue        float64 `json:"revenue"`
}

type ShopInventoryCode struct {
	ID        int64      `json:"id"`
	Code      string     `json:"code"`
	Status    string     `json:"status"`
	Buyer     string     `json:"buyer"`
	SoldAt    *time.Time `json:"sold_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
}

type ShopAdminOrder struct {
	ID          int64     `json:"id"`
	OrderNo     string    `json:"order_no"`
	ProductID   int64     `json:"product_id"`
	ProductName string    `json:"product_name"`
	Price       float64   `json:"price"`
	Buyer       string    `json:"buyer"`
	Code        string    `json:"code"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
}

type ShopService struct {
	db              *sql.DB
	authInvalidator APIKeyAuthCacheInvalidator
}

func NewShopService(db *sql.DB, authInvalidator APIKeyAuthCacheInvalidator) *ShopService {
	return &ShopService{db: db, authInvalidator: authInvalidator}
}

func (s *ShopService) ListProducts(ctx context.Context, includeDisabled bool) ([]ShopProduct, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("shop database is not configured")
	}
	statusFilter := "AND p.status = 'active'"
	if includeDisabled {
		statusFilter = ""
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT p.id, p.name, p.description, p.price, p.limit_per_user, p.status, p.created_at, p.updated_at,
		       COUNT(i.id) FILTER (WHERE i.status = 'available'),
		       COUNT(i.id) FILTER (WHERE i.status = 'sold')
		FROM shop_products p LEFT JOIN shop_inventory_codes i ON i.product_id = p.id
		WHERE 1=1 `+statusFilter+`
		GROUP BY p.id ORDER BY p.id DESC`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	products := make([]ShopProduct, 0)
	for rows.Next() {
		var p ShopProduct
		if err := rows.Scan(&p.ID, &p.Name, &p.Description, &p.Price, &p.LimitPerUser, &p.Status, &p.CreatedAt, &p.UpdatedAt, &p.AvailableStock, &p.SoldCount); err != nil {
			return nil, err
		}
		products = append(products, p)
	}
	return products, rows.Err()
}

func (s *ShopService) GetOverview(ctx context.Context) (*ShopOverview, error) {
	var overview ShopOverview
	err := s.db.QueryRowContext(ctx, `
		SELECT
			COUNT(DISTINCT p.id),
			COUNT(DISTINCT p.id) FILTER (WHERE p.status = 'active'),
			COUNT(i.id) FILTER (WHERE i.status = 'available'),
			COUNT(i.id) FILTER (WHERE i.status = 'sold'),
			COALESCE(SUM(o.price), 0)
		FROM shop_products p
		LEFT JOIN shop_inventory_codes i ON i.product_id = p.id
		LEFT JOIN shop_orders o ON o.inventory_code_id = i.id`).
		Scan(&overview.ProductCount, &overview.ActiveProducts, &overview.AvailableStock, &overview.SoldCount, &overview.Revenue)
	if err != nil {
		return nil, err
	}
	return &overview, nil
}

func (s *ShopService) ListInventory(ctx context.Context, productID int64, status string, page, pageSize int) ([]ShopInventoryCode, int64, error) {
	if productID <= 0 {
		return nil, 0, ErrShopProductNotFound
	}
	if status != "" && status != ShopInventoryAvailable && status != ShopInventorySold && status != ShopInventoryDisabled {
		return nil, 0, errors.New("invalid inventory status")
	}
	var total int64
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM shop_inventory_codes WHERE product_id=$1 AND ($2='' OR status=$2)`, productID, status).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT i.id, i.code, i.status, COALESCE(NULLIF(u.username,''), u.email, ''), i.sold_at, i.created_at
		FROM shop_inventory_codes i
		LEFT JOIN users u ON u.id=i.sold_to
		WHERE i.product_id=$1 AND ($2='' OR i.status=$2)
		ORDER BY i.id DESC LIMIT $3 OFFSET $4`, productID, status, pageSize, (page-1)*pageSize)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = rows.Close() }()
	items := make([]ShopInventoryCode, 0)
	for rows.Next() {
		var item ShopInventoryCode
		var code string
		if err := rows.Scan(&item.ID, &code, &item.Status, &item.Buyer, &item.SoldAt, &item.CreatedAt); err != nil {
			return nil, 0, err
		}
		item.Code = code
		items = append(items, item)
	}
	return items, total, rows.Err()
}

func (s *ShopService) ListAdminOrders(ctx context.Context, productID int64, search string, page, pageSize int) ([]ShopAdminOrder, int64, error) {
	if productID <= 0 {
		return nil, 0, ErrShopProductNotFound
	}
	search = strings.TrimSpace(search)
	var total int64
	filter := `o.product_id=$1 AND ($2='' OR o.order_no ILIKE '%' || $2 || '%' OR o.product_name ILIKE '%' || $2 || '%' OR u.email ILIKE '%' || $2 || '%' OR u.username ILIKE '%' || $2 || '%')`
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM shop_orders o JOIN users u ON u.id=o.user_id WHERE `+filter, productID, search).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT o.id,o.order_no,o.product_id,o.product_name,o.price,
		       COALESCE(NULLIF(u.username,''), u.email),i.code,o.status,o.created_at
		FROM shop_orders o
		JOIN users u ON u.id=o.user_id
		JOIN shop_inventory_codes i ON i.id=o.inventory_code_id
		WHERE `+filter+` ORDER BY o.id DESC LIMIT $3 OFFSET $4`, productID, search, pageSize, (page-1)*pageSize)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = rows.Close() }()
	items := make([]ShopAdminOrder, 0)
	for rows.Next() {
		var item ShopAdminOrder
		var code string
		if err := rows.Scan(&item.ID, &item.OrderNo, &item.ProductID, &item.ProductName, &item.Price, &item.Buyer, &code, &item.Status, &item.CreatedAt); err != nil {
			return nil, 0, err
		}
		item.Code = code
		items = append(items, item)
	}
	return items, total, rows.Err()
}

func (s *ShopService) CreateProduct(ctx context.Context, name, description string, price float64, limitPerUser *int) (*ShopProduct, error) {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 200 {
		return nil, errors.New("product name is required")
	}
	if price < 0 {
		return nil, errors.New("product price must be non-negative")
	}
	if limitPerUser != nil && *limitPerUser <= 0 {
		return nil, errors.New("limit per user must be positive")
	}
	var p ShopProduct
	err := s.db.QueryRowContext(ctx, `INSERT INTO shop_products(name, description, price, limit_per_user) VALUES($1,$2,$3,$4) RETURNING id,name,description,price,limit_per_user,status,created_at,updated_at`, name, strings.TrimSpace(description), price, limitPerUser).Scan(&p.ID, &p.Name, &p.Description, &p.Price, &p.LimitPerUser, &p.Status, &p.CreatedAt, &p.UpdatedAt)
	return &p, err
}

func (s *ShopService) UpdateProduct(ctx context.Context, id int64, name, description string, price float64, status string, limitPerUser *int) (*ShopProduct, error) {
	name = strings.TrimSpace(name)
	if id <= 0 || name == "" || price < 0 || (status != ShopProductActive && status != ShopProductDisabled) {
		return nil, errors.New("invalid product")
	}
	if limitPerUser != nil && *limitPerUser <= 0 {
		return nil, errors.New("limit per user must be positive")
	}
	var p ShopProduct
	err := s.db.QueryRowContext(ctx, `UPDATE shop_products SET name=$1,description=$2,price=$3,status=$4,limit_per_user=$5,updated_at=NOW() WHERE id=$6 RETURNING id,name,description,price,limit_per_user,status,created_at,updated_at`, name, strings.TrimSpace(description), price, status, limitPerUser, id).Scan(&p.ID, &p.Name, &p.Description, &p.Price, &p.LimitPerUser, &p.Status, &p.CreatedAt, &p.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrShopProductNotFound
	}
	return &p, err
}

func (s *ShopService) DeleteProduct(ctx context.Context, id int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	res, err := tx.ExecContext(ctx, `DELETE FROM shop_inventory_codes WHERE product_id=$1 AND status <> $2`, id, ShopInventorySold)
	if err != nil {
		return err
	}
	_ = res
	if _, err := tx.ExecContext(ctx, `UPDATE shop_orders SET product_id=NULL WHERE product_id=$1`, id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE shop_inventory_codes SET product_id=NULL WHERE product_id=$1`, id); err != nil {
		return err
	}
	res, err = tx.ExecContext(ctx, `DELETE FROM shop_products WHERE id=$1 AND status=$2`, id, ShopProductDisabled)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrShopProductNotFound
	}
	return tx.Commit()
}

func (s *ShopService) AddCodes(ctx context.Context, productID int64, rawCodes []string) (int, error) {
	if productID <= 0 || len(rawCodes) == 0 {
		return 0, errors.New("product and codes are required")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()
	var exists bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM shop_products WHERE id=$1)`, productID).Scan(&exists); err != nil {
		return 0, err
	}
	if !exists {
		return 0, ErrShopProductNotFound
	}
	added := 0
	seen := map[string]struct{}{}
	for _, raw := range rawCodes {
		code := strings.TrimSpace(raw)
		if code == "" {
			continue
		}
		if _, ok := seen[code]; ok {
			continue
		}
		seen[code] = struct{}{}
		res, err := tx.ExecContext(ctx, `INSERT INTO shop_inventory_codes(product_id,code) VALUES($1,$2) ON CONFLICT(code) DO NOTHING`, productID, code)
		if err != nil {
			return 0, err
		}
		n, _ := res.RowsAffected()
		added += int(n)
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return added, nil
}

func (s *ShopService) Purchase(ctx context.Context, userID, productID int64, idempotencyKey string) (*ShopOrder, error) {
	if userID <= 0 || productID <= 0 {
		return nil, errors.New("invalid purchase request")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	if len(idempotencyKey) > 128 {
		return nil, errors.New("idempotency key is too long")
	}
	if idempotencyKey != "" {
		var existing ShopOrder
		lookupErr := tx.QueryRowContext(ctx, `
			SELECT o.id,o.order_no,COALESCE(o.product_id,0),o.product_name,o.price,i.code,o.status,o.created_at
			FROM shop_orders o JOIN shop_inventory_codes i ON i.id=o.inventory_code_id
			WHERE o.user_id=$1 AND o.idempotency_key=$2`, userID, idempotencyKey).
			Scan(&existing.ID, &existing.OrderNo, &existing.ProductID, &existing.ProductName, &existing.Price, &existing.Code, &existing.Status, &existing.CreatedAt)
		if lookupErr == nil {
			return &existing, nil
		}
		if !errors.Is(lookupErr, sql.ErrNoRows) {
			return nil, lookupErr
		}
	}
	var name, status string
	var price float64
	var limitPerUser *int
	err = tx.QueryRowContext(ctx, `SELECT name,price,status,limit_per_user FROM shop_products WHERE id=$1 FOR UPDATE`, productID).Scan(&name, &price, &status, &limitPerUser)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrShopProductNotFound
	}
	if err != nil {
		return nil, err
	}
	if status != ShopProductActive {
		return nil, ErrShopProductDisabled
	}
	if limitPerUser != nil {
		var purchased int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM shop_orders WHERE user_id=$1 AND product_id=$2 AND status=$3`, userID, productID, ShopOrderPaid).Scan(&purchased); err != nil {
			return nil, err
		}
		if purchased >= *limitPerUser {
			return nil, ErrShopPurchaseLimit
		}
	}
	var balance float64
	if err := tx.QueryRowContext(ctx, `SELECT balance FROM users WHERE id=$1 AND deleted_at IS NULL FOR UPDATE`, userID).Scan(&balance); err != nil {
		return nil, err
	}
	var inventoryID int64
	var code string
	err = tx.QueryRowContext(ctx, `SELECT id,code FROM shop_inventory_codes WHERE product_id=$1 AND status='available' ORDER BY id FOR UPDATE SKIP LOCKED LIMIT 1`, productID).Scan(&inventoryID, &code)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrShopOutOfStock
	}
	if err != nil {
		return nil, err
	}
	if balance < price {
		return nil, ErrShopInsufficientBalance
	}
	orderNo, err := newShopOrderNo()
	if err != nil {
		return nil, err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE users SET balance=balance-$1,updated_at=NOW() WHERE id=$2`, price, userID); err != nil {
		return nil, err
	}
	var order ShopOrder
	err = tx.QueryRowContext(ctx, `INSERT INTO shop_orders(order_no,user_id,product_id,product_name,price,inventory_code_id,idempotency_key) VALUES($1,$2,$3,$4,$5,$6,$7) RETURNING id,order_no,product_id,product_name,price,status,created_at`, orderNo, userID, productID, name, price, inventoryID, nullableShopIdempotencyKey(idempotencyKey)).Scan(&order.ID, &order.OrderNo, &order.ProductID, &order.ProductName, &order.Price, &order.Status, &order.CreatedAt)
	if err != nil {
		return nil, err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE shop_inventory_codes SET status='sold',sold_to=$1,sold_at=NOW() WHERE id=$2 AND status='available'`, userID, inventoryID); err != nil {
		return nil, err
	}
	order.Code = code
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	if s.authInvalidator != nil {
		s.authInvalidator.InvalidateAuthCacheByUserID(ctx, userID)
	}
	return &order, nil
}

func (s *ShopService) UpdateInventoryStatus(ctx context.Context, productID, inventoryID int64, status string) error {
	if status != ShopInventoryAvailable && status != ShopInventoryDisabled {
		return errors.New("invalid inventory status")
	}
	res, err := s.db.ExecContext(ctx, `UPDATE shop_inventory_codes SET status=$1 WHERE id=$2 AND product_id=$3 AND status <> $4`, status, inventoryID, productID, ShopInventorySold)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrShopInventorySold
	}
	return nil
}

func (s *ShopService) DeleteInventory(ctx context.Context, productID int64, inventoryIDs []int64) (int64, error) {
	if len(inventoryIDs) == 0 {
		return 0, errors.New("inventory ids are required")
	}
	res, err := s.db.ExecContext(ctx, `DELETE FROM shop_inventory_codes WHERE product_id=$1 AND status <> $2 AND id = ANY($3)`, productID, ShopInventorySold, pq.Array(inventoryIDs))
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, nil
}

func nullableShopIdempotencyKey(key string) any {
	if key == "" {
		return nil
	}
	return key
}

func (s *ShopService) ListOrders(ctx context.Context, userID int64) ([]ShopOrder, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT o.id,o.order_no,COALESCE(o.product_id,0),o.product_name,o.price,i.code,o.status,o.created_at FROM shop_orders o JOIN shop_inventory_codes i ON i.id=o.inventory_code_id WHERE o.user_id=$1 ORDER BY o.id DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	orders := make([]ShopOrder, 0)
	for rows.Next() {
		var o ShopOrder
		if err := rows.Scan(&o.ID, &o.OrderNo, &o.ProductID, &o.ProductName, &o.Price, &o.Code, &o.Status, &o.CreatedAt); err != nil {
			return nil, err
		}
		orders = append(orders, o)
	}
	return orders, rows.Err()
}

func newShopOrderNo() (string, error) {
	b := make([]byte, 10)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return fmt.Sprintf("SHOP-%d-%s", time.Now().UnixNano(), hex.EncodeToString(b)), nil
}
