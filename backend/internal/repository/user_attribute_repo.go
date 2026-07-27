package repository

import (
	"context"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/userattributedefinition"
	"github.com/Wei-Shaw/sub2api/ent/userattributevalue"
	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/port/userattribute"
)

// UserAttributeDefinitionRepository implementation
type userAttributeDefinitionRepository struct {
	client *dbent.Client
}

// NewUserAttributeDefinitionRepository creates a new repository instance
func NewUserAttributeDefinitionRepository(client *dbent.Client) userattribute.UserAttributeDefinitionRepository {
	return &userAttributeDefinitionRepository{client: client}
}

func (r *userAttributeDefinitionRepository) Create(ctx context.Context, def *domain.UserAttributeDefinition) error {
	client := clientFromContext(ctx, r.client)

	created, err := client.UserAttributeDefinition.Create().
		SetKey(def.Key).
		SetName(def.Name).
		SetDescription(def.Description).
		SetType(string(def.Type)).
		SetOptions(toEntOptions(def.Options)).
		SetRequired(def.Required).
		SetValidation(toEntValidation(def.Validation)).
		SetPlaceholder(def.Placeholder).
		SetEnabled(def.Enabled).
		Save(ctx)

	if err != nil {
		return translatePersistenceError(err, nil, domain.ErrAttributeKeyExists)
	}

	def.ID = created.ID
	def.DisplayOrder = created.DisplayOrder
	def.CreatedAt = created.CreatedAt
	def.UpdatedAt = created.UpdatedAt
	return nil
}

func (r *userAttributeDefinitionRepository) GetByID(ctx context.Context, id int64) (*domain.UserAttributeDefinition, error) {
	client := clientFromContext(ctx, r.client)

	e, err := client.UserAttributeDefinition.Query().
		Where(userattributedefinition.IDEQ(id)).
		Only(ctx)
	if err != nil {
		return nil, translatePersistenceError(err, domain.ErrAttributeDefinitionNotFound, nil)
	}
	return defEntityToService(e), nil
}

func (r *userAttributeDefinitionRepository) GetByKey(ctx context.Context, key string) (*domain.UserAttributeDefinition, error) {
	client := clientFromContext(ctx, r.client)

	e, err := client.UserAttributeDefinition.Query().
		Where(userattributedefinition.KeyEQ(key)).
		Only(ctx)
	if err != nil {
		return nil, translatePersistenceError(err, domain.ErrAttributeDefinitionNotFound, nil)
	}
	return defEntityToService(e), nil
}

func (r *userAttributeDefinitionRepository) Update(ctx context.Context, def *domain.UserAttributeDefinition) error {
	client := clientFromContext(ctx, r.client)

	updated, err := client.UserAttributeDefinition.UpdateOneID(def.ID).
		SetName(def.Name).
		SetDescription(def.Description).
		SetType(string(def.Type)).
		SetOptions(toEntOptions(def.Options)).
		SetRequired(def.Required).
		SetValidation(toEntValidation(def.Validation)).
		SetPlaceholder(def.Placeholder).
		SetDisplayOrder(def.DisplayOrder).
		SetEnabled(def.Enabled).
		Save(ctx)

	if err != nil {
		return translatePersistenceError(err, domain.ErrAttributeDefinitionNotFound, domain.ErrAttributeKeyExists)
	}

	def.UpdatedAt = updated.UpdatedAt
	return nil
}

func (r *userAttributeDefinitionRepository) Delete(ctx context.Context, id int64) error {
	client := clientFromContext(ctx, r.client)

	_, err := client.UserAttributeDefinition.Delete().
		Where(userattributedefinition.IDEQ(id)).
		Exec(ctx)
	return translatePersistenceError(err, domain.ErrAttributeDefinitionNotFound, nil)
}

func (r *userAttributeDefinitionRepository) List(ctx context.Context, enabledOnly bool) ([]domain.UserAttributeDefinition, error) {
	client := clientFromContext(ctx, r.client)

	q := client.UserAttributeDefinition.Query()
	if enabledOnly {
		q = q.Where(userattributedefinition.EnabledEQ(true))
	}

	entities, err := q.Order(dbent.Asc(userattributedefinition.FieldDisplayOrder)).All(ctx)
	if err != nil {
		return nil, err
	}

	result := make([]domain.UserAttributeDefinition, 0, len(entities))
	for _, e := range entities {
		result = append(result, *defEntityToService(e))
	}
	return result, nil
}

func (r *userAttributeDefinitionRepository) UpdateDisplayOrders(ctx context.Context, orders map[int64]int) error {
	tx, err := r.client.Tx(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	for id, order := range orders {
		if _, err := tx.UserAttributeDefinition.UpdateOneID(id).
			SetDisplayOrder(order).
			Save(ctx); err != nil {
			return translatePersistenceError(err, domain.ErrAttributeDefinitionNotFound, nil)
		}
	}

	return tx.Commit()
}

func (r *userAttributeDefinitionRepository) ExistsByKey(ctx context.Context, key string) (bool, error) {
	client := clientFromContext(ctx, r.client)
	return client.UserAttributeDefinition.Query().
		Where(userattributedefinition.KeyEQ(key)).
		Exist(ctx)
}

// UserAttributeValueRepository implementation
type userAttributeValueRepository struct {
	client *dbent.Client
}

// NewUserAttributeValueRepository creates a new repository instance
func NewUserAttributeValueRepository(client *dbent.Client) userattribute.UserAttributeValueRepository {
	return &userAttributeValueRepository{client: client}
}

func (r *userAttributeValueRepository) GetByUserID(ctx context.Context, userID int64) ([]domain.UserAttributeValue, error) {
	client := clientFromContext(ctx, r.client)

	entities, err := client.UserAttributeValue.Query().
		Where(userattributevalue.UserIDEQ(userID)).
		All(ctx)
	if err != nil {
		return nil, err
	}

	result := make([]domain.UserAttributeValue, 0, len(entities))
	for _, e := range entities {
		result = append(result, domain.UserAttributeValue{
			ID:          e.ID,
			UserID:      e.UserID,
			AttributeID: e.AttributeID,
			Value:       e.Value,
			CreatedAt:   e.CreatedAt,
			UpdatedAt:   e.UpdatedAt,
		})
	}
	return result, nil
}

func (r *userAttributeValueRepository) GetByUserIDs(ctx context.Context, userIDs []int64) ([]domain.UserAttributeValue, error) {
	if len(userIDs) == 0 {
		return []domain.UserAttributeValue{}, nil
	}

	client := clientFromContext(ctx, r.client)

	entities, err := client.UserAttributeValue.Query().
		Where(userattributevalue.UserIDIn(userIDs...)).
		All(ctx)
	if err != nil {
		return nil, err
	}

	result := make([]domain.UserAttributeValue, 0, len(entities))
	for _, e := range entities {
		result = append(result, domain.UserAttributeValue{
			ID:          e.ID,
			UserID:      e.UserID,
			AttributeID: e.AttributeID,
			Value:       e.Value,
			CreatedAt:   e.CreatedAt,
			UpdatedAt:   e.UpdatedAt,
		})
	}
	return result, nil
}

func (r *userAttributeValueRepository) UpsertBatch(ctx context.Context, userID int64, inputs []domain.UpdateUserAttributeInput) error {
	if len(inputs) == 0 {
		return nil
	}

	tx, err := r.client.Tx(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	for _, input := range inputs {
		// Use upsert (ON CONFLICT DO UPDATE)
		err := tx.UserAttributeValue.Create().
			SetUserID(userID).
			SetAttributeID(input.AttributeID).
			SetValue(input.Value).
			OnConflictColumns(userattributevalue.FieldUserID, userattributevalue.FieldAttributeID).
			UpdateValue().
			UpdateUpdatedAt().
			Exec(ctx)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (r *userAttributeValueRepository) DeleteByAttributeID(ctx context.Context, attributeID int64) error {
	client := clientFromContext(ctx, r.client)

	_, err := client.UserAttributeValue.Delete().
		Where(userattributevalue.AttributeIDEQ(attributeID)).
		Exec(ctx)
	return err
}

func (r *userAttributeValueRepository) DeleteByUserID(ctx context.Context, userID int64) error {
	client := clientFromContext(ctx, r.client)

	_, err := client.UserAttributeValue.Delete().
		Where(userattributevalue.UserIDEQ(userID)).
		Exec(ctx)
	return err
}

// Helper functions for entity to service conversion
func defEntityToService(e *dbent.UserAttributeDefinition) *domain.UserAttributeDefinition {
	if e == nil {
		return nil
	}
	return &domain.UserAttributeDefinition{
		ID:           e.ID,
		Key:          e.Key,
		Name:         e.Name,
		Description:  e.Description,
		Type:         domain.UserAttributeType(e.Type),
		Options:      toServiceOptions(e.Options),
		Required:     e.Required,
		Validation:   toServiceValidation(e.Validation),
		Placeholder:  e.Placeholder,
		DisplayOrder: e.DisplayOrder,
		Enabled:      e.Enabled,
		CreatedAt:    e.CreatedAt,
		UpdatedAt:    e.UpdatedAt,
	}
}

// Type conversion helpers (map types <-> service types)
func toEntOptions(opts []domain.UserAttributeOption) []map[string]any {
	if opts == nil {
		return []map[string]any{}
	}
	result := make([]map[string]any, len(opts))
	for i, o := range opts {
		result[i] = map[string]any{"value": o.Value, "label": o.Label}
	}
	return result
}

func toServiceOptions(opts []map[string]any) []domain.UserAttributeOption {
	if opts == nil {
		return []domain.UserAttributeOption{}
	}
	result := make([]domain.UserAttributeOption, len(opts))
	for i, o := range opts {
		result[i] = domain.UserAttributeOption{
			Value: getString(o, "value"),
			Label: getString(o, "label"),
		}
	}
	return result
}

func toEntValidation(v domain.UserAttributeValidation) map[string]any {
	result := map[string]any{}
	if v.MinLength != nil {
		result["min_length"] = *v.MinLength
	}
	if v.MaxLength != nil {
		result["max_length"] = *v.MaxLength
	}
	if v.Min != nil {
		result["min"] = *v.Min
	}
	if v.Max != nil {
		result["max"] = *v.Max
	}
	if v.Pattern != nil {
		result["pattern"] = *v.Pattern
	}
	if v.Message != nil {
		result["message"] = *v.Message
	}
	return result
}

func toServiceValidation(v map[string]any) domain.UserAttributeValidation {
	result := domain.UserAttributeValidation{}
	if val := getInt(v, "min_length"); val != nil {
		result.MinLength = val
	}
	if val := getInt(v, "max_length"); val != nil {
		result.MaxLength = val
	}
	if val := getInt(v, "min"); val != nil {
		result.Min = val
	}
	if val := getInt(v, "max"); val != nil {
		result.Max = val
	}
	if val := getStringPtr(v, "pattern"); val != nil {
		result.Pattern = val
	}
	if val := getStringPtr(v, "message"); val != nil {
		result.Message = val
	}
	return result
}

// Helper functions for type conversion
func getString(m map[string]any, key string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func getStringPtr(m map[string]any, key string) *string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return &s
		}
	}
	return nil
}

func getInt(m map[string]any, key string) *int {
	if v, ok := m[key]; ok {
		switch n := v.(type) {
		case int:
			return &n
		case int64:
			i := int(n)
			return &i
		case float64:
			i := int(n)
			return &i
		}
	}
	return nil
}
