package service

import (
	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/port/userattribute"
)

// User-attribute BC types/errors/consts live in domain; re-exported here for
// existing call sites and test stubs. Repository interfaces live in port/userattribute.
type UserAttributeType = domain.UserAttributeType
type UserAttributeOption = domain.UserAttributeOption
type UserAttributeValidation = domain.UserAttributeValidation
type UserAttributeDefinition = domain.UserAttributeDefinition
type UserAttributeValue = domain.UserAttributeValue
type CreateAttributeDefinitionInput = domain.CreateAttributeDefinitionInput
type UpdateAttributeDefinitionInput = domain.UpdateAttributeDefinitionInput
type UpdateUserAttributeInput = domain.UpdateUserAttributeInput

type UserAttributeDefinitionRepository = userattribute.UserAttributeDefinitionRepository
type UserAttributeValueRepository = userattribute.UserAttributeValueRepository

const (
	AttributeTypeText        = domain.AttributeTypeText
	AttributeTypeTextarea    = domain.AttributeTypeTextarea
	AttributeTypeNumber      = domain.AttributeTypeNumber
	AttributeTypeEmail       = domain.AttributeTypeEmail
	AttributeTypeURL         = domain.AttributeTypeURL
	AttributeTypeDate        = domain.AttributeTypeDate
	AttributeTypeSelect      = domain.AttributeTypeSelect
	AttributeTypeMultiSelect = domain.AttributeTypeMultiSelect
)

var (
	ErrAttributeDefinitionNotFound = domain.ErrAttributeDefinitionNotFound
	ErrAttributeKeyExists          = domain.ErrAttributeKeyExists
	ErrInvalidAttributeType        = domain.ErrInvalidAttributeType
	ErrAttributeValidationFailed   = domain.ErrAttributeValidationFailed
)
