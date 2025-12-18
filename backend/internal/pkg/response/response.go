package response

import (
	"math"
	"net/http"

	"github.com/gin-gonic/gin"
)

// Response 标准API响应格式
type Response struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{REDACTED `json:"data,omitempty"`
REDACTED

// PaginatedData 分页数据格式（匹配前端期望）
type PaginatedData struct {
	Items    interface{REDACTED `json:"items"`
	Total    int64       `json:"total"`
	Page     int         `json:"page"`
	PageSize int         `json:"page_size"`
	Pages    int         `json:"pages"`
REDACTED

// Success 返回成功响应
func Success(c *gin.Context, data interface{REDACTED) {
	c.JSON(http.StatusOK, Response{
		Code:    0,
		Message: "success",
		Data:    data,
REDACTED)
REDACTED

// Created 返回创建成功响应
func Created(c *gin.Context, data interface{REDACTED) {
	c.JSON(http.StatusCreated, Response{
		Code:    0,
		Message: "success",
		Data:    data,
REDACTED)
REDACTED

// Error 返回错误响应
func Error(c *gin.Context, statusCode int, message string) {
	c.JSON(statusCode, Response{
		Code:    statusCode,
		Message: message,
REDACTED)
REDACTED

// BadRequest 返回400错误
func BadRequest(c *gin.Context, message string) {
	Error(c, http.StatusBadRequest, message)
REDACTED

// Unauthorized 返回401错误
func Unauthorized(c *gin.Context, message string) {
	Error(c, http.StatusUnauthorized, message)
REDACTED

// Forbidden 返回403错误
func Forbidden(c *gin.Context, message string) {
	Error(c, http.StatusForbidden, message)
REDACTED

// NotFound 返回404错误
func NotFound(c *gin.Context, message string) {
	Error(c, http.StatusNotFound, message)
REDACTED

// InternalError 返回500错误
func InternalError(c *gin.Context, message string) {
	Error(c, http.StatusInternalServerError, message)
REDACTED

// Paginated 返回分页数据
func Paginated(c *gin.Context, items interface{REDACTED, total int64, page, pageSize int) {
	pages := int(math.Ceil(float64(total) / float64(pageSize)))
	if pages < 1 {
		pages = 1
REDACTED

	Success(c, PaginatedData{
		Items:    items,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
		Pages:    pages,
REDACTED)
REDACTED

// PaginationResult 分页结果（与repository.PaginationResult兼容）
type PaginationResult struct {
	Total    int64
	Page     int
	PageSize int
	Pages    int
REDACTED

// PaginatedWithResult 使用PaginationResult返回分页数据
func PaginatedWithResult(c *gin.Context, items interface{REDACTED, pagination *PaginationResult) {
	if pagination == nil {
		Success(c, PaginatedData{
			Items:    items,
			Total:    0,
			Page:     1,
			PageSize: 20,
			Pages:    1,
	REDACTED)
		return
REDACTED

	Success(c, PaginatedData{
		Items:    items,
		Total:    pagination.Total,
		Page:     pagination.Page,
		PageSize: pagination.PageSize,
		Pages:    pagination.Pages,
REDACTED)
REDACTED

// ParsePagination 解析分页参数
func ParsePagination(c *gin.Context) (page, pageSize int) {
	page = 1
	pageSize = 20

	if p := c.Query("page"); p != "" {
		if val, err := parseInt(p); err == nil && val > 0 {
			page = val
	REDACTED
REDACTED

	// 支持 page_size 和 limit 两种参数名
	if ps := c.Query("page_size"); ps != "" {
		if val, err := parseInt(ps); err == nil && val > 0 && val <= 100 {
			pageSize = val
	REDACTED
REDACTED else if l := c.Query("limit"); l != "" {
		if val, err := parseInt(l); err == nil && val > 0 && val <= 100 {
			pageSize = val
	REDACTED
REDACTED

	return page, pageSize
REDACTED

func parseInt(s string) (int, error) {
	var result int
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, nil
	REDACTED
		result = result*10 + int(c-'0')
REDACTED
	return result, nil
REDACTED
