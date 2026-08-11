// Package service ...
package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// ModelIntroService 管理"模型介绍"表的 CRUD。
//
// 该表面向 admin 配置，用户端可选择性展示（本期只做管理端）。表以对外模型名
// （如 "bytedance/seedance-2.5/text-to-video"、"gpt-4o"）作为主键，与定价条目按名字
// 松耦合关联，不建立强外键，便于历史清理。
//
// 因 ent schema 需要走代码生成流程，这里采用底层 *sql.DB 直接读写，
// 避免生成产物膨胀。
type ModelIntroService struct {
	db *sql.DB
}

// NewModelIntroService 构造函数。
func NewModelIntroService(db *sql.DB) *ModelIntroService {
	return &ModelIntroService{db: db}
}

// OutputFieldSpec 描述模型完成后可展示的一个输出字段。管理员在
// "模型介绍"里配置该数组；每个模型可声明多个输出字段。
//
//   - Key：从 fal 原生 result payload 中提取该字段的路径。
//     支持 "video.url"、"images[0].url"、"images[*]"、"images" 等；
//     若字段值本身是标量（如 seed / request_id），可直接写 "seed"。
//   - Label：可选展示名。当前编辑器已不再暴露该列，仅为向后兼容旧数据保留；
//     前端读取时若为空退化到 Key。
//   - Type：JSON Schema 标准类型，取值 "string" / "number" / "boolean" /
//     "object" / "array"。演练台按此渲染：string/number/boolean 走文本；
//     object/array 走预格式化 JSON 展示。
//   - Description：字段说明，可为空。
//   - Default：预留的默认值/示例（仅供文档/复制场景，不参与渲染）。
//
// 主输出字段由 ModelIntro.ResultField 指示，不再在 OutputFieldSpec 上标记 primary；
// 主结果的媒体渲染类型由 ModelIntro.ResultType（video / image）决定。
//
// 为了让"输出参数"支持与"输入参数"完全一致的填写方式，OutputFieldSpec 额外
// 携带三个可选的字段声明位：
//   - Required：输出字段是否被视为必然存在（仅用于文档语义，前端展示）。
//   - Enum：字段是否枚举；Options 声明枚举候选值集合。
//   - Options：与 Enum 配套的候选值列表；Enum=false 时应保持为空/nil。
//
// 这三项在存储层通过 jsonb 无痛扩展，向后兼容旧数据（旧记录读出时为零值）。
//
// 为了支持"嵌套 schema"（object 展开子字段列表 / array 展开元素 schema）：
//   - Properties：Type=="object" 时使用，键为子字段名，值为一份递归 schema。
//     采用 map[string]any 表达而非再声明一个递归 Go 结构，避免后端
//     写死每一层字段；前端已用同一套 rowToSchema/schemaToRow 递归序列化。
//   - Items：Type=="array" 时使用，值为一份递归 schema（数组同构）。同样
//     采用 any 承载任意 shape。
//
// 存储层无需迁移：jsonb 天然支持任意嵌套；旧记录没有这两个键，读出时为
// nil，前端反解时按"空容器"处理，不会破坏历史数据。
type OutputFieldSpec struct {
	Key         string         `json:"key"`
	Label       string         `json:"label,omitempty"`
	Type        string         `json:"type"`
	Description string         `json:"description"`
	Default     string         `json:"default,omitempty"`
	Required    bool           `json:"required,omitempty"`
	Enum        bool           `json:"enum,omitempty"`
	Options     []any          `json:"options,omitempty"`
	Properties  map[string]any `json:"properties,omitempty"`
	Items       any            `json:"items,omitempty"`
}

// ModelIntro 是 service 层返回的模型介绍实体。
//
// OutputFields 用于用户端演练台正确渲染任务完成后的结果：数组顺序即展示顺序，
// 每个字段声明可独立控制 label / type / description。
//
// ResultField / ResultType 表示演练台展示时的"主结果字段"：
//   - ResultField 指向 OutputFields 中的某个 key。若为空，演练台按 OutputFields
//     顺序取第一个 video / image 字段作为主结果；若未匹配到，主结果区留空。
//   - ResultType 仅取 "video" | "image"（默认 "video"），决定用 <video> 还是
//     <img> 渲染主结果媒体。
type ModelIntro struct {
	ModelKey string `json:"model_key"`
	Title    string `json:"title"`
	// Description 中文文案（兼容历史数据；旧记录只有该字段）。
	Description string `json:"description"`
	// DescriptionEn 英文文案。前端按当前 locale 挑选：
	// 中文界面优先展示 Description，缺失回落到 DescriptionEn；
	// 英文界面优先展示 DescriptionEn，缺失回落到 Description。
	DescriptionEn string            `json:"description_en"`
	CoverURL      string            `json:"cover_url"`
	DefaultParams map[string]any    `json:"default_params"`
	SortOrder     int               `json:"sort_order"`
	Enabled       bool              `json:"enabled"`
	OutputFields  []OutputFieldSpec `json:"output_fields"`
	ResultField   string            `json:"result_field"`
	ResultType    string            `json:"result_type"`
	CreatedAt     time.Time         `json:"created_at"`
	UpdatedAt     time.Time         `json:"updated_at"`
}

// UpsertModelIntroInput 是 Create / Update 共享的输入。model_key 为空时 Create
// 会报错；Update 时 model_key 从 URL 取。
type UpsertModelIntroInput struct {
	ModelKey    string
	Title       string
	Description string
	// DescriptionEn 英文文案；允许为空（前端展示时按 locale 兜底）。
	DescriptionEn string
	CoverURL      string
	DefaultParams map[string]any
	SortOrder     int
	Enabled       bool
	OutputFields  []OutputFieldSpec
	ResultField   string
	ResultType    string
}

const (
	modelIntroMaxKeyLen         = 255
	modelIntroMaxTitleLen       = 255
	modelIntroMaxCoverLen       = 1024
	modelIntroDefaultPageSize   = 20
	modelIntroMaxPageSize       = 200
	modelIntroErrCodeInvalid    = "INVALID_MODEL_INTRO"
	modelIntroErrCodeNotFound   = "MODEL_INTRO_NOT_FOUND"
	modelIntroErrCodeDuplicated = "MODEL_INTRO_DUPLICATED"
)

// List 分页查询模型介绍，按 sort_order asc、model_key asc 排序。
// keyword 非空时按 model_key / title 做 ILIKE 模糊匹配。
func (s *ModelIntroService) List(
	ctx context.Context,
	page, pageSize int,
	keyword string,
) ([]*ModelIntro, int, error) {
	if s == nil || s.db == nil {
		return nil, 0, fmt.Errorf("model intro service not initialized")
	}
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = modelIntroDefaultPageSize
	}
	if pageSize > modelIntroMaxPageSize {
		pageSize = modelIntroMaxPageSize
	}

	whereSQL := ""
	args := []any{}
	kw := strings.TrimSpace(keyword)
	if kw != "" {
		whereSQL = " WHERE model_key ILIKE $1 OR title ILIKE $1"
		args = append(args, "%"+kw+"%")
	}

	countSQL := "SELECT COUNT(*) FROM model_intros" + whereSQL
	var total int
	if err := s.db.QueryRowContext(ctx, countSQL, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count model_intros: %w", err)
	}

	listSQL := `SELECT model_key, title, description, description_en, cover_url, default_params,
	                   sort_order, enabled, output_fields,
	                   result_field, result_type,
	                   created_at, updated_at
	            FROM model_intros` + whereSQL +
		fmt.Sprintf(" ORDER BY sort_order ASC, model_key ASC LIMIT %d OFFSET %d",
			pageSize, (page-1)*pageSize)

	rows, err := s.db.QueryContext(ctx, listSQL, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list model_intros: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]*ModelIntro, 0, pageSize)
	for rows.Next() {
		item, err := scanModelIntroRow(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate model_intros: %w", err)
	}
	return out, total, nil
}

// Get 根据 model_key 读取一条介绍；不存在返回 (nil, nil)。
func (s *ModelIntroService) Get(ctx context.Context, modelKey string) (*ModelIntro, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("model intro service not initialized")
	}
	key := strings.TrimSpace(modelKey)
	if key == "" {
		return nil, infraerrors.BadRequest(modelIntroErrCodeInvalid, "model_key is required")
	}
	rows, err := s.db.QueryContext(ctx, `SELECT model_key, title, description, description_en, cover_url,
		default_params, sort_order, enabled, output_fields,
		result_field, result_type,
		created_at, updated_at
		FROM model_intros WHERE model_key = $1`, key)
	if err != nil {
		return nil, fmt.Errorf("get model_intro %s: %w", key, err)
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		return nil, nil
	}
	return scanModelIntroRow(rows)
}

// Create 新增一条模型介绍。model_key 已存在时返回 duplicated 错误。
func (s *ModelIntroService) Create(ctx context.Context, in UpsertModelIntroInput) (*ModelIntro, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("model intro service not initialized")
	}
	if err := validateModelIntroInput(&in, true); err != nil {
		return nil, err
	}
	paramsJSON, err := marshalDefaultParams(in.DefaultParams)
	if err != nil {
		return nil, err
	}
	outputJSON, err := marshalOutputFields(in.OutputFields)
	if err != nil {
		return nil, err
	}

	// 冲突检测。避免依赖 unique 约束错误消息文本，先手工查一次。
	existing, err := s.Get(ctx, in.ModelKey)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return nil, infraerrors.BadRequest(modelIntroErrCodeDuplicated,
			fmt.Sprintf("model_intro with model_key %q already exists", in.ModelKey))
	}

	now := time.Now()
	_, err = s.db.ExecContext(ctx, `INSERT INTO model_intros
		(model_key, title, description, description_en, cover_url, default_params, sort_order, enabled,
		 output_fields, result_field, result_type, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $12)`,
		in.ModelKey, in.Title, in.Description, in.DescriptionEn, in.CoverURL, paramsJSON,
		in.SortOrder, in.Enabled, outputJSON, in.ResultField, in.ResultType, now,
	)
	if err != nil {
		return nil, fmt.Errorf("insert model_intro: %w", err)
	}
	return s.Get(ctx, in.ModelKey)
}

// Update 覆盖更新一条介绍。model_key 不允许改（如需改就删了重建）。
func (s *ModelIntroService) Update(ctx context.Context, modelKey string, in UpsertModelIntroInput) (*ModelIntro, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("model intro service not initialized")
	}
	key := strings.TrimSpace(modelKey)
	if key == "" {
		return nil, infraerrors.BadRequest(modelIntroErrCodeInvalid, "model_key is required")
	}
	// 复用校验；这里 model_key 已由 URL 提供，不再要求 in.ModelKey。
	in.ModelKey = key
	if err := validateModelIntroInput(&in, false); err != nil {
		return nil, err
	}
	paramsJSON, err := marshalDefaultParams(in.DefaultParams)
	if err != nil {
		return nil, err
	}
	outputJSON, err := marshalOutputFields(in.OutputFields)
	if err != nil {
		return nil, err
	}

	res, err := s.db.ExecContext(ctx, `UPDATE model_intros SET
		title = $2,
		description = $3,
		description_en = $4,
		cover_url = $5,
		default_params = $6,
		sort_order = $7,
		enabled = $8,
		output_fields = $9,
		result_field = $10,
		result_type = $11,
		updated_at = NOW()
		WHERE model_key = $1`,
		key, in.Title, in.Description, in.DescriptionEn, in.CoverURL, paramsJSON,
		in.SortOrder, in.Enabled, outputJSON, in.ResultField, in.ResultType,
	)
	if err != nil {
		return nil, fmt.Errorf("update model_intro %s: %w", key, err)
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return nil, infraerrors.NotFound(modelIntroErrCodeNotFound, "model intro not found")
	}
	return s.Get(ctx, key)
}

// Delete 硬删除。不存在返回 NotFound。
func (s *ModelIntroService) Delete(ctx context.Context, modelKey string) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("model intro service not initialized")
	}
	key := strings.TrimSpace(modelKey)
	if key == "" {
		return infraerrors.BadRequest(modelIntroErrCodeInvalid, "model_key is required")
	}
	res, err := s.db.ExecContext(ctx, `DELETE FROM model_intros WHERE model_key = $1`, key)
	if err != nil {
		return fmt.Errorf("delete model_intro %s: %w", key, err)
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return infraerrors.NotFound(modelIntroErrCodeNotFound, "model intro not found")
	}
	return nil
}

// ---------- helpers ----------

// scanModelIntroRow 从游标当前行读一条记录。调用方需保证 rows.Next() 已返回 true。
func scanModelIntroRow(rows *sql.Rows) (*ModelIntro, error) {
	var (
		item      ModelIntro
		paramsRaw []byte
		outputRaw []byte
	)
	if err := rows.Scan(
		&item.ModelKey,
		&item.Title,
		&item.Description,
		&item.DescriptionEn,
		&item.CoverURL,
		&paramsRaw,
		&item.SortOrder,
		&item.Enabled,
		&outputRaw,
		&item.ResultField,
		&item.ResultType,
		&item.CreatedAt,
		&item.UpdatedAt,
	); err != nil {
		return nil, fmt.Errorf("scan model_intros row: %w", err)
	}
	if len(paramsRaw) == 0 {
		item.DefaultParams = map[string]any{}
	} else {
		params := map[string]any{}
		if err := json.Unmarshal(paramsRaw, &params); err != nil {
			// 兜底：读出错时给个空对象，避免整条查询挂掉。
			item.DefaultParams = map[string]any{}
		} else {
			item.DefaultParams = params
		}
	}
	if len(outputRaw) == 0 {
		item.OutputFields = []OutputFieldSpec{}
	} else {
		var fields []OutputFieldSpec
		if err := json.Unmarshal(outputRaw, &fields); err != nil || fields == nil {
			// 兜底：解析失败退化为空数组，不阻断整条查询。
			item.OutputFields = []OutputFieldSpec{}
		} else {
			item.OutputFields = fields
		}
	}
	return &item, nil
}

// validateModelIntroInput 校验字段基本约束。requireKey=true 时会校验 in.ModelKey。
func validateModelIntroInput(in *UpsertModelIntroInput, requireKey bool) error {
	if in == nil {
		return infraerrors.BadRequest(modelIntroErrCodeInvalid, "payload is required")
	}
	if requireKey {
		in.ModelKey = strings.TrimSpace(in.ModelKey)
		if in.ModelKey == "" {
			return infraerrors.BadRequest(modelIntroErrCodeInvalid, "model_key is required")
		}
	}
	if len(in.ModelKey) > modelIntroMaxKeyLen {
		return infraerrors.BadRequest(modelIntroErrCodeInvalid,
			fmt.Sprintf("model_key exceeds %d chars", modelIntroMaxKeyLen))
	}
	in.Title = strings.TrimSpace(in.Title)
	if len(in.Title) > modelIntroMaxTitleLen {
		return infraerrors.BadRequest(modelIntroErrCodeInvalid,
			fmt.Sprintf("title exceeds %d chars", modelIntroMaxTitleLen))
	}
	in.CoverURL = strings.TrimSpace(in.CoverURL)
	if len(in.CoverURL) > modelIntroMaxCoverLen {
		return infraerrors.BadRequest(modelIntroErrCodeInvalid,
			fmt.Sprintf("cover_url exceeds %d chars", modelIntroMaxCoverLen))
	}
	// description 无长度上限（PG text 型）；这里也不裁剪。
	if in.DefaultParams == nil {
		in.DefaultParams = map[string]any{}
	}
	if in.OutputFields == nil {
		in.OutputFields = []OutputFieldSpec{}
	}
	if err := normalizeOutputFields(in.OutputFields); err != nil {
		return err
	}
	if err := normalizeResultRef(in); err != nil {
		return err
	}
	return nil
}

// normalizeResultRef 校验主结果指示器（result_field / result_type）并做默认值处理：
//   - result_type 为空时默认为 "video"；只允许 "video" | "image" 两个值。
//   - result_field 允许为空；非空时必须存在于 in.OutputFields[].key 中（
//     避免写错 key 导致前端无法定位主结果）。
//   - result_field 会取 TrimSpace，并限制长度不超过 modelIntroMaxKeyLen。
func normalizeResultRef(in *UpsertModelIntroInput) error {
	in.ResultField = strings.TrimSpace(in.ResultField)
	if len(in.ResultField) > modelIntroMaxKeyLen {
		return infraerrors.BadRequest(modelIntroErrCodeInvalid,
			fmt.Sprintf("result_field exceeds %d chars", modelIntroMaxKeyLen))
	}
	in.ResultType = strings.TrimSpace(strings.ToLower(in.ResultType))
	if in.ResultType == "" {
		in.ResultType = "video"
	}
	if in.ResultType != "video" && in.ResultType != "image" {
		return infraerrors.BadRequest(modelIntroErrCodeInvalid,
			fmt.Sprintf("result_type must be one of video/image, got %q", in.ResultType))
	}
	if in.ResultField != "" {
		// result_field 支持多级路径（如 "data.video.url"、"images[*].url"），
		// 需要递归展开 OutputFields 树，收集所有可选路径后再匹配。
		// 路径语法与前端 paramSpec.pickByPath / collectResultFieldPaths 完全一致：
		//   - object 节点 k     → "k"、"k.child"、"k.child.grand"
		//   - array 节点 k     → "k"、"k[*]"、"k[*].sub"
		//   - 叶子节点 k        → "k"
		paths := collectOutputFieldPaths(in.OutputFields)
		found := false
		for _, p := range paths {
			if p == in.ResultField {
				found = true
				break
			}
		}
		if !found {
			return infraerrors.BadRequest(modelIntroErrCodeInvalid,
				fmt.Sprintf("result_field %q does not match any output_fields[].key", in.ResultField))
		}
	}
	return nil
}

// collectOutputFieldPaths 递归展开 OutputFields 树，把所有可选路径都产出一份。
//
// 与 normalizeOutputFields 保持一致的类型集合（string/number/boolean/object/array）：
//   - Type == "object"：读 Properties 的每个键，把子 schema 视为子节点递归；
//     子节点的 key 就是 Properties 的 map key。
//   - Type == "array"：把 Items 视为一份匿名子节点（key 为空）——路径拼接时
//     直接给当前 prefix 追加 "[*]"，然后进入 Items 继续展开。
//   - 其它类型（叶子）：只产出当前 prefix。
//
// 之所以在服务层重新实现一次遍历，而不是复用 normalizeOutputFields 的循环，
// 是因为校验路径与规范化逻辑关注点不同——前者只需要"哪些 key 合法"，
// 后者要修改字段内容；分开写更清晰、也不会带来意外的副作用。
func collectOutputFieldPaths(fields []OutputFieldSpec) []string {
	out := make([]string, 0, len(fields))
	// walkSchema 处理"某个子层 schema"，raw 直接是从 Properties / Items
	// 反解出来的 any；因为顶层是强类型 OutputFieldSpec，所以分两个入口。
	var walkSchema func(raw any, prefix string)
	walkSchema = func(raw any, prefix string) {
		if prefix != "" {
			out = append(out, prefix)
		}
		m, ok := raw.(map[string]any)
		if !ok {
			return
		}
		typ, _ := m["type"].(string)
		typ = strings.ToLower(strings.TrimSpace(typ))
		switch typ {
		case "object":
			// Properties 反解出来是 map[string]any；每个 value 又是嵌套 schema。
			props, _ := m["properties"].(map[string]any)
			for childKey, childSchema := range props {
				ck := strings.TrimSpace(childKey)
				if ck == "" {
					continue
				}
				nextPrefix := ck
				if prefix != "" {
					nextPrefix = prefix + "." + ck
				}
				walkSchema(childSchema, nextPrefix)
			}
		case "array":
			// Items 是匿名子节点，key 为空；路径拼接直接在当前 prefix 后加 "[*]"。
			nextPrefix := prefix + "[*]"
			walkSchema(m["items"], nextPrefix)
		}
	}

	for i := range fields {
		f := &fields[i]
		key := strings.TrimSpace(f.Key)
		if key == "" {
			continue
		}
		// 顶层节点自身作为一条候选路径。
		out = append(out, key)
		switch strings.ToLower(strings.TrimSpace(f.Type)) {
		case "object":
			for childKey, childSchema := range f.Properties {
				ck := strings.TrimSpace(childKey)
				if ck == "" {
					continue
				}
				walkSchema(childSchema, key+"."+ck)
			}
		case "array":
			walkSchema(f.Items, key+"[*]")
		}
	}
	return out
}

// normalizeOutputFields 就地清洗输出字段声明：
//   - key 必填（不含 key 直接报错，避免脏数据）；
//   - type 未指定时默认 "text"；只接受一组白名单值，其它拒绝；
//   - 其它文本字段做 TrimSpace，避免前端手抖带上前后空格；
//   - Enum=false 时强制清空 Options，避免噪声写库；
//   - Options 为 nil 时归一到 nil（json 序列化后不出现该字段）。
func normalizeOutputFields(fields []OutputFieldSpec) error {
	for i := range fields {
		f := &fields[i]
		f.Key = strings.TrimSpace(f.Key)
		if f.Key == "" {
			return infraerrors.BadRequest(modelIntroErrCodeInvalid,
				"output_fields[].key is required")
		}
		if len(f.Key) > modelIntroMaxKeyLen {
			return infraerrors.BadRequest(modelIntroErrCodeInvalid,
				fmt.Sprintf("output_fields[].key exceeds %d chars", modelIntroMaxKeyLen))
		}
		f.Label = strings.TrimSpace(f.Label)
		f.Description = strings.TrimSpace(f.Description)
		f.Type = strings.TrimSpace(strings.ToLower(f.Type))
		if f.Type == "" {
			f.Type = "string"
		}
		switch f.Type {
		case "string", "number", "boolean", "object", "array":
			// ok
		default:
			return infraerrors.BadRequest(modelIntroErrCodeInvalid,
				fmt.Sprintf("output_fields[].type must be one of string/number/boolean/object/array, got %q", f.Type))
		}
		// Enum=false 时保证 Options 为空；Enum=true 但没给候选也允许（前端可能后补）。
		if !f.Enum {
			f.Options = nil
		}
		if f.Options != nil && len(f.Options) == 0 {
			f.Options = nil
		}
		// 嵌套 schema 归一：
		//   - object：Properties 至少给空 map（json 层面存 {} 而非 null），
		//     便于前端读回后直接递归遍历；同时清空 Items 避免残留。
		//   - array：Items 若缺省则不硬造（允许保存尚未填的空 array 声明），
		//     由前端在展示时按空 items 兜底；同时清空 Properties。
		//   - 其他类型：Properties/Items 都强制清空。
		switch f.Type {
		case "object":
			if f.Properties == nil {
				f.Properties = map[string]any{}
			}
			f.Items = nil
		case "array":
			f.Properties = nil
			// f.Items 保留，nil 也允许（前端反解时给一份默认 string items）。
		default:
			f.Properties = nil
			f.Items = nil
		}
	}
	return nil
}

// marshalOutputFields 把切片编码成 JSON 字节串写入 jsonb 列。nil 切片会序列化
// 为 "[]"，避免 PG 收到 "null" 后再往下游传时前端错拿到 null。
func marshalOutputFields(fields []OutputFieldSpec) ([]byte, error) {
	if fields == nil {
		fields = []OutputFieldSpec{}
	}
	b, err := json.Marshal(fields)
	if err != nil {
		return nil, infraerrors.BadRequest(modelIntroErrCodeInvalid,
			fmt.Sprintf("output_fields encode failed: %s", err.Error()))
	}
	return b, nil
}

// marshalDefaultParams 把 map 编码成 JSON 字节串写入 jsonb 列。
func marshalDefaultParams(m map[string]any) ([]byte, error) {
	if m == nil {
		m = map[string]any{}
	}
	b, err := json.Marshal(m)
	if err != nil {
		return nil, infraerrors.BadRequest(modelIntroErrCodeInvalid,
			fmt.Sprintf("default_params encode failed: %s", err.Error()))
	}
	return b, nil
}
