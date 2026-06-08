package service

import (
	"context"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// Error definitions for user attribute operations
var (
	ErrAttributeDefinitionNotFound = infraerrors.NotFound("ATTRIBUTE_DEFINITION_NOT_FOUND", "attribute definition not found")
	ErrAttributeKeyExists          = infraerrors.Conflict("ATTRIBUTE_KEY_EXISTS", "attribute key already exists")
	ErrInvalidAttributeType        = infraerrors.BadRequest("INVALID_ATTRIBUTE_TYPE", "invalid attribute type")
	ErrAttributeValidationFailed   = infraerrors.BadRequest("ATTRIBUTE_VALIDATION_FAILED", "attribute value validation failed")
)

// UserAttributeType represents supported attribute types
type UserAttributeType string

const (
	AttributeTypeText        UserAttributeType = "text"
	AttributeTypeTextarea    UserAttributeType = "textarea"
	AttributeTypeNumber      UserAttributeType = "number"
	AttributeTypeEmail       UserAttributeType = "email"
	AttributeTypeURL         UserAttributeType = "url"
	AttributeTypeDate        UserAttributeType = "date"
	AttributeTypeSelect      UserAttributeType = "select"
	AttributeTypeMultiSelect UserAttributeType = "multi_select"
)

// UserAttributeOption represents a select option for select/multi_select types
type UserAttributeOption struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// UserAttributeValidation represents validation rules for an attribute
type UserAttributeValidation struct {
	MinLength *int    `json:"min_length,omitempty"`
	MaxLength *int    `json:"max_length,omitempty"`
	Min       *int    `json:"min,omitempty"`
	Max       *int    `json:"max,omitempty"`
	Pattern   *string `json:"pattern,omitempty"`
	Message   *string `json:"message,omitempty"`
}

// UserAttributeDefinition represents a custom attribute definition
type UserAttributeDefinition struct {
	ID           int64
	Key          string
	Name         string
	Description  string
	Type         UserAttributeType
	Options      []UserAttributeOption
	Required     bool
	Validation   UserAttributeValidation
	Placeholder  string
	DisplayOrder int
	Enabled      bool
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// UserAttributeValue represents a user's attribute value
type UserAttributeValue struct {
	ID          int64
	UserID      int64
	AttributeID int64
	Value       string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// CreateAttributeDefinitionInput for creating new definition
type CreateAttributeDefinitionInput struct {
	Key         string
	Name        string
	Description string
	Type        UserAttributeType
	Options     []UserAttributeOption
	Required    bool
	Validation  UserAttributeValidation
	Placeholder string
	Enabled     bool
}

// UpdateAttributeDefinitionInput for updating definition
type UpdateAttributeDefinitionInput struct {
	Name        *string
	Description *string
	Type        *UserAttributeType
	Options     *[]UserAttributeOption
	Required    *bool
	Validation  *UserAttributeValidation
	Placeholder *string
	Enabled     *bool
}

// UpdateUserAttributeInput for updating a single attribute value
type UpdateUserAttributeInput struct {
	AttributeID int64
	Value       string
}

// UserAttributeDefinitionRepository interface for attribute definition persistence
type UserAttributeDefinitionRepository interface {
	Create(ctx context.Context, def *UserAttributeDefinition) error
	GetByID(ctx context.Context, id int64) (*UserAttributeDefinition, error)
	GetByKey(ctx context.Context, key string) (*UserAttributeDefinition, error)
	Update(ctx context.Context, def *UserAttributeDefinition) error
	Delete(ctx context.Context, id int64) error
	List(ctx context.Context, enabledOnly bool) ([]UserAttributeDefinition, error)
	UpdateDisplayOrders(ctx context.Context, orders map[int64]int) error
	ExistsByKey(ctx context.Context, key string) (bool, error)
}

// UserAttributeValueRepository interface for user attribute value persistence
type UserAttributeValueRepository interface {
	GetByUserID(ctx context.Context, userID int64) ([]UserAttributeValue, error)
	GetByUserIDs(ctx context.Context, userIDs []int64) ([]UserAttributeValue, error)
	UpsertBatch(ctx context.Context, userID int64, values []UpdateUserAttributeInput) error
	DeleteByAttributeID(ctx context.Context, attributeID int64) error
	DeleteByUserID(ctx context.Context, userID int64) error
}
