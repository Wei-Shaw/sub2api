package service

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// UserAttributeService handles attribute management
type UserAttributeService struct {
	defRepo   UserAttributeDefinitionRepository
	valueRepo UserAttributeValueRepository
REDACTED

// NewUserAttributeService creates a new service instance
func NewUserAttributeService(
	defRepo UserAttributeDefinitionRepository,
	valueRepo UserAttributeValueRepository,
) *UserAttributeService {
	return &UserAttributeService{
		defRepo:   defRepo,
		valueRepo: valueRepo,
REDACTED
REDACTED

// CreateDefinition creates a new attribute definition
func (s *UserAttributeService) CreateDefinition(ctx context.Context, input CreateAttributeDefinitionInput) (*UserAttributeDefinition, error) {
	// Validate type
	if !isValidAttributeType(input.Type) {
		return nil, ErrInvalidAttributeType
REDACTED

	// Check if key exists
	exists, err := s.defRepo.ExistsByKey(ctx, input.Key)
	if err != nil {
		return nil, fmt.Errorf("check key exists: %w", err)
REDACTED
	if exists {
		return nil, ErrAttributeKeyExists
REDACTED

	def := &UserAttributeDefinition{
		Key:         input.Key,
		Name:        input.Name,
		Description: input.Description,
		Type:        input.Type,
		Options:     input.Options,
		Required:    input.Required,
		Validation:  input.Validation,
		Placeholder: input.Placeholder,
		Enabled:     input.Enabled,
REDACTED

	if err := validateDefinitionPattern(def); err != nil {
		return nil, err
REDACTED

	if err := s.defRepo.Create(ctx, def); err != nil {
		return nil, fmt.Errorf("create definition: %w", err)
REDACTED

	return def, nil
REDACTED

// GetDefinition retrieves a definition by ID
func (s *UserAttributeService) GetDefinition(ctx context.Context, id int64) (*UserAttributeDefinition, error) {
	return s.defRepo.GetByID(ctx, id)
REDACTED

// GetDefinitionByKey retrieves a definition by its unique key
func (s *UserAttributeService) GetDefinitionByKey(ctx context.Context, key string) (*UserAttributeDefinition, error) {
	return s.defRepo.GetByKey(ctx, key)
REDACTED

// ListDefinitions lists all definitions
func (s *UserAttributeService) ListDefinitions(ctx context.Context, enabledOnly bool) ([]UserAttributeDefinition, error) {
	return s.defRepo.List(ctx, enabledOnly)
REDACTED

// UpdateDefinition updates an existing definition
func (s *UserAttributeService) UpdateDefinition(ctx context.Context, id int64, input UpdateAttributeDefinitionInput) (*UserAttributeDefinition, error) {
	def, err := s.defRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
REDACTED

	if input.Name != nil {
		def.Name = *input.Name
REDACTED
	if input.Description != nil {
		def.Description = *input.Description
REDACTED
	if input.Type != nil {
		if !isValidAttributeType(*input.Type) {
			return nil, ErrInvalidAttributeType
	REDACTED
		def.Type = *input.Type
REDACTED
	if input.Options != nil {
		def.Options = *input.Options
REDACTED
	if input.Required != nil {
		def.Required = *input.Required
REDACTED
	if input.Validation != nil {
		def.Validation = *input.Validation
REDACTED
	if input.Placeholder != nil {
		def.Placeholder = *input.Placeholder
REDACTED
	if input.Enabled != nil {
		def.Enabled = *input.Enabled
REDACTED

	if err := validateDefinitionPattern(def); err != nil {
		return nil, err
REDACTED

	if err := s.defRepo.Update(ctx, def); err != nil {
		return nil, fmt.Errorf("update definition: %w", err)
REDACTED

	return def, nil
REDACTED

// DeleteDefinition soft-deletes a definition and hard-deletes associated values
func (s *UserAttributeService) DeleteDefinition(ctx context.Context, id int64) error {
	// Check if definition exists
	_, err := s.defRepo.GetByID(ctx, id)
	if err != nil {
		return err
REDACTED

	// First delete all values (hard delete)
	if err := s.valueRepo.DeleteByAttributeID(ctx, id); err != nil {
		return fmt.Errorf("delete values: %w", err)
REDACTED

	// Then soft-delete the definition
	if err := s.defRepo.Delete(ctx, id); err != nil {
		return fmt.Errorf("delete definition: %w", err)
REDACTED

	return nil
REDACTED

// ReorderDefinitions updates display order for multiple definitions
func (s *UserAttributeService) ReorderDefinitions(ctx context.Context, orders map[int64]int) error {
	return s.defRepo.UpdateDisplayOrders(ctx, orders)
REDACTED

// GetUserAttributes retrieves all attribute values for a user
func (s *UserAttributeService) GetUserAttributes(ctx context.Context, userID int64) ([]UserAttributeValue, error) {
	return s.valueRepo.GetByUserID(ctx, userID)
REDACTED

// GetBatchUserAttributes retrieves attribute values for multiple users
// Returns a map of userID -> map of attributeID -> value
func (s *UserAttributeService) GetBatchUserAttributes(ctx context.Context, userIDs []int64) (map[int64]map[int64]string, error) {
	values, err := s.valueRepo.GetByUserIDs(ctx, userIDs)
	if err != nil {
		return nil, err
REDACTED

	result := make(map[int64]map[int64]string)
	for _, v := range values {
		if result[v.UserID] == nil {
			result[v.UserID] = make(map[int64]string)
	REDACTED
		result[v.UserID][v.AttributeID] = v.Value
REDACTED

	return result, nil
REDACTED

// UpdateUserAttributes batch updates attribute values for a user
func (s *UserAttributeService) UpdateUserAttributes(ctx context.Context, userID int64, inputs []UpdateUserAttributeInput) error {
	// Validate all values before updating
	defs, err := s.defRepo.List(ctx, true)
	if err != nil {
		return fmt.Errorf("list definitions: %w", err)
REDACTED

	defMap := make(map[int64]*UserAttributeDefinition, len(defs))
	for i := range defs {
		defMap[defs[i].ID] = &defs[i]
REDACTED

	for _, input := range inputs {
		def, ok := defMap[input.AttributeID]
		if !ok {
			return ErrAttributeDefinitionNotFound
	REDACTED

		if err := s.validateValue(def, input.Value); err != nil {
			return err
	REDACTED
REDACTED

	return s.valueRepo.UpsertBatch(ctx, userID, inputs)
REDACTED

// validateValue validates a value against its definition
func (s *UserAttributeService) validateValue(def *UserAttributeDefinition, value string) error {
	// Skip validation for empty non-required fields
	if value == "" && !def.Required {
		return nil
REDACTED

	// Required check
	if def.Required && value == "" {
		return validationError(fmt.Sprintf("%s is required", def.Name))
REDACTED

	v := def.Validation

	// String length validation
	if v.MinLength != nil && len(value) < *v.MinLength {
		return validationError(fmt.Sprintf("%s must be at least %d characters", def.Name, *v.MinLength))
REDACTED
	if v.MaxLength != nil && len(value) > *v.MaxLength {
		return validationError(fmt.Sprintf("%s must be at most %d characters", def.Name, *v.MaxLength))
REDACTED

	// Number validation
	if def.Type == AttributeTypeNumber && value != "" {
		num, err := strconv.Atoi(value)
		if err != nil {
			return validationError(fmt.Sprintf("%s must be a number", def.Name))
	REDACTED
		if v.Min != nil && num < *v.Min {
			return validationError(fmt.Sprintf("%s must be at least %d", def.Name, *v.Min))
	REDACTED
		if v.Max != nil && num > *v.Max {
			return validationError(fmt.Sprintf("%s must be at most %d", def.Name, *v.Max))
	REDACTED
REDACTED

	// Pattern validation
	if v.Pattern != nil && *v.Pattern != "" && value != "" {
		re, err := regexp.Compile(*v.Pattern)
		if err != nil {
			return validationError(def.Name + " has an invalid pattern")
	REDACTED
		if !re.MatchString(value) {
			msg := def.Name + " format is invalid"
			if v.Message != nil && *v.Message != "" {
				msg = *v.Message
		REDACTED
			return validationError(msg)
	REDACTED
REDACTED

	// Select validation
	if def.Type == AttributeTypeSelect && value != "" {
		found := false
		for _, opt := range def.Options {
			if opt.Value == value {
				found = true
				break
		REDACTED
	REDACTED
		if !found {
			return validationError(fmt.Sprintf("%s: invalid option", def.Name))
	REDACTED
REDACTED

	// Multi-select validation (stored as JSON array)
	if def.Type == AttributeTypeMultiSelect && value != "" {
		var values []string
		if err := json.Unmarshal([]byte(value), &values); err != nil {
			// Try comma-separated fallback
			values = strings.Split(value, ",")
	REDACTED
		for _, val := range values {
			val = strings.TrimSpace(val)
			found := false
			for _, opt := range def.Options {
				if opt.Value == val {
					found = true
					break
			REDACTED
		REDACTED
			if !found {
				return validationError(fmt.Sprintf("%s: invalid option %s", def.Name, val))
		REDACTED
	REDACTED
REDACTED

	return nil
REDACTED

// validationError creates a validation error with a custom message
func validationError(msg string) error {
	return infraerrors.BadRequest("ATTRIBUTE_VALIDATION_FAILED", msg)
REDACTED

func isValidAttributeType(t UserAttributeType) bool {
	switch t {
	case AttributeTypeText, AttributeTypeTextarea, AttributeTypeNumber,
		AttributeTypeEmail, AttributeTypeURL, AttributeTypeDate,
		AttributeTypeSelect, AttributeTypeMultiSelect:
		return true
REDACTED
	return false
REDACTED

func validateDefinitionPattern(def *UserAttributeDefinition) error {
	if def == nil {
		return nil
REDACTED
	if def.Validation.Pattern == nil {
		return nil
REDACTED
	pattern := strings.TrimSpace(*def.Validation.Pattern)
	if pattern == "" {
		return nil
REDACTED
	if _, err := regexp.Compile(pattern); err != nil {
		return infraerrors.BadRequest("INVALID_ATTRIBUTE_PATTERN", fmt.Sprintf("invalid pattern for %s: %v", def.Name, err))
REDACTED
	return nil
REDACTED
