package repository

import (
	"context"

	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/Wei-Shaw/sub2api/internal/model"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"

	"gorm.io/gorm"
)

type proxyRepository struct {
	db *gorm.DB
REDACTED

func NewProxyRepository(db *gorm.DB) service.ProxyRepository {
	return &proxyRepository{db: dbREDACTED
REDACTED

func (r *proxyRepository) Create(ctx context.Context, proxy *model.Proxy) error {
	return r.db.WithContext(ctx).Create(proxy).Error
REDACTED

func (r *proxyRepository) GetByID(ctx context.Context, id int64) (*model.Proxy, error) {
	var proxy model.Proxy
	err := r.db.WithContext(ctx).First(&proxy, id).Error
	if err != nil {
		return nil, translatePersistenceError(err, service.ErrProxyNotFound, nil)
REDACTED
	return &proxy, nil
REDACTED

func (r *proxyRepository) Update(ctx context.Context, proxy *model.Proxy) error {
	return r.db.WithContext(ctx).Save(proxy).Error
REDACTED

func (r *proxyRepository) Delete(ctx context.Context, id int64) error {
	return r.db.WithContext(ctx).Delete(&model.Proxy{REDACTED, id).Error
REDACTED

func (r *proxyRepository) List(ctx context.Context, params pagination.PaginationParams) ([]model.Proxy, *pagination.PaginationResult, error) {
	return r.ListWithFilters(ctx, params, "", "", "")
REDACTED

// ListWithFilters lists proxies with optional filtering by protocol, status, and search query
func (r *proxyRepository) ListWithFilters(ctx context.Context, params pagination.PaginationParams, protocol, status, search string) ([]model.Proxy, *pagination.PaginationResult, error) {
	var proxies []model.Proxy
	var total int64

	db := r.db.WithContext(ctx).Model(&model.Proxy{REDACTED)

	// Apply filters
	if protocol != "" {
		db = db.Where("protocol = ?", protocol)
REDACTED
	if status != "" {
		db = db.Where("status = ?", status)
REDACTED
	if search != "" {
		searchPattern := "%" + search + "%"
		db = db.Where("name ILIKE ?", searchPattern)
REDACTED

	if err := db.Count(&total).Error; err != nil {
		return nil, nil, err
REDACTED

	if err := db.Offset(params.Offset()).Limit(params.Limit()).Order("id DESC").Find(&proxies).Error; err != nil {
		return nil, nil, err
REDACTED

	pages := int(total) / params.Limit()
	if int(total)%params.Limit() > 0 {
		pages++
REDACTED

	return proxies, &pagination.PaginationResult{
		Total:    total,
		Page:     params.Page,
		PageSize: params.Limit(),
		Pages:    pages,
REDACTED, nil
REDACTED

func (r *proxyRepository) ListActive(ctx context.Context) ([]model.Proxy, error) {
	var proxies []model.Proxy
	err := r.db.WithContext(ctx).Where("status = ?", model.StatusActive).Find(&proxies).Error
	return proxies, err
REDACTED

// ExistsByHostPortAuth checks if a proxy with the same host, port, username, and password exists
func (r *proxyRepository) ExistsByHostPortAuth(ctx context.Context, host string, port int, username, password string) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&model.Proxy{REDACTED).
		Where("host = ? AND port = ? AND username = ? AND password = ?", host, port, username, password).
		Count(&count).Error
	if err != nil {
		return false, err
REDACTED
	return count > 0, nil
REDACTED

// CountAccountsByProxyID returns the number of accounts using a specific proxy
func (r *proxyRepository) CountAccountsByProxyID(ctx context.Context, proxyID int64) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&model.Account{REDACTED).
		Where("proxy_id = ?", proxyID).
		Count(&count).Error
	return count, err
REDACTED

// GetAccountCountsForProxies returns a map of proxy ID to account count for all proxies
func (r *proxyRepository) GetAccountCountsForProxies(ctx context.Context) (map[int64]int64, error) {
	type result struct {
		ProxyID int64 `gorm:"column:proxy_id"`
		Count   int64 `gorm:"column:count"`
REDACTED
	var results []result
	err := r.db.WithContext(ctx).
		Model(&model.Account{REDACTED).
		Select("proxy_id, COUNT(*) as count").
		Where("proxy_id IS NOT NULL").
		Group("proxy_id").
		Scan(&results).Error
	if err != nil {
		return nil, err
REDACTED

	counts := make(map[int64]int64)
	for _, r := range results {
		counts[r.ProxyID] = r.Count
REDACTED
	return counts, nil
REDACTED

// ListActiveWithAccountCount returns all active proxies with account count, sorted by creation time descending
func (r *proxyRepository) ListActiveWithAccountCount(ctx context.Context) ([]model.ProxyWithAccountCount, error) {
	var proxies []model.Proxy
	err := r.db.WithContext(ctx).
		Where("status = ?", model.StatusActive).
		Order("created_at DESC").
		Find(&proxies).Error
	if err != nil {
		return nil, err
REDACTED

	// Get account counts
	counts, err := r.GetAccountCountsForProxies(ctx)
	if err != nil {
		return nil, err
REDACTED

	// Build result with account counts
	result := make([]model.ProxyWithAccountCount, len(proxies))
	for i, proxy := range proxies {
		result[i] = model.ProxyWithAccountCount{
			Proxy:        proxy,
			AccountCount: counts[proxy.ID],
	REDACTED
REDACTED

	return result, nil
REDACTED
