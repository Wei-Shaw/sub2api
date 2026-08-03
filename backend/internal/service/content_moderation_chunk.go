package service

import (
	"context"
	"fmt"
	"sync"

	"github.com/tiktoken-go/tokenizer"
)

const (
	// defaultContentModerationChunkTokens 是单个审核分片的 token 上限。
	//
	// 背景：OpenAI moderation 接口在输入超过约 4000 token 时不再真正打分，
	// 而是返回 HTTP 200 + 与内容无关的固定分数向量（flagged=false），没有任何
	// 错误信号。实测最早失效点出现在中文约 3870 token，因此这里取 3000 留出余量。
	defaultContentModerationChunkTokens = 3000
	// maxContentModerationChunkTokens 限制管理员可配置的上限，避免把分片调到
	// 静默失效区间内。实测最早失效点约 3870 token，故封顶 3500。
	maxContentModerationChunkTokens = 3500
	minContentModerationChunkTokens = 200

	defaultContentModerationChunkConcurrency = 2
	maxContentModerationChunkConcurrency     = 8

	// defaultContentModerationChunkMaxChunks 限制单次审核最多送多少片。
	//
	// 分片数直接决定一次审核的调用量：实测 166KB 的会话按 3000 token 会切成 21 片、
	// 494KB 的会切成 36 片。前置审核是同步阻塞在用户请求上的，不设上限时延迟和
	// 成本都不可控，因此默认只审前 2 片（约 6000 token，已覆盖单次调用静默失效的
	// ~4000 token 阈值）；超出部分丢弃并记入日志，不静默截断。
	defaultContentModerationChunkMaxChunks = 2
	maxContentModerationChunkMaxChunks     = 64
	minContentModerationChunkMaxChunks     = 1

	// contentModerationChunkMinTailDivisor 决定尾片丢弃阈值：尾片 token 数小于
	// chunkTokens/8 时跳过，避免为极短残片多发一次请求。固定值，不开放配置。
	contentModerationChunkMinTailDivisor = 8
)

// limitChunks 把分片数收敛到 maxChunks，返回保留的分片和被丢弃的片数。
//
// 超限时取前 N 片而非采样：前 N 片可预测、可复现，便于对照排查；实测风险内容
// 也更常出现在会话前段（工具输出注入除外，那属于提取阶段的问题）。
func limitChunks(chunks []string, maxChunks int) ([]string, int) {
	if maxChunks <= 0 {
		maxChunks = defaultContentModerationChunkMaxChunks
	}
	if len(chunks) <= maxChunks {
		return chunks, 0
	}
	return chunks[:maxChunks], len(chunks) - maxChunks
}

// contentModerationChunkCodec 返回用于分片的分词器。moderation 属于新模型系列，
// 统一使用 o200k_base；与 openAIInputTokensCodecForModel 的默认分支一致。
func contentModerationChunkCodec() (tokenizer.Codec, error) {
	return tokenizer.Get(tokenizer.O200kBase)
}

// splitTextByTokens 按 token 上限切分文本。
//
// 当切分出多于 1 片且最后一片的 token 数小于 chunkTokens/8 时，丢弃该尾片：
// 为几十个 token 的残片多发一次审核请求不划算。
// 只有一片时永不丢弃，否则会把全部内容丢掉。
func splitTextByTokens(text string, chunkTokens int) ([]string, error) {
	if text == "" {
		return nil, nil
	}
	if chunkTokens <= 0 {
		chunkTokens = defaultContentModerationChunkTokens
	}
	minTailDivisor := contentModerationChunkMinTailDivisor
	codec, err := contentModerationChunkCodec()
	if err != nil {
		return nil, fmt.Errorf("load moderation tokenizer: %w", err)
	}
	ids, _, err := codec.Encode(text)
	if err != nil {
		return nil, fmt.Errorf("encode moderation input: %w", err)
	}
	if len(ids) == 0 {
		return nil, nil
	}
	if len(ids) <= chunkTokens {
		return []string{text}, nil
	}

	chunks := make([]string, 0, (len(ids)+chunkTokens-1)/chunkTokens)
	for start := 0; start < len(ids); start += chunkTokens {
		end := start + chunkTokens
		if end > len(ids) {
			end = len(ids)
		}
		// 尾片过短则整体丢弃（此时 chunks 已至少有一片，不会丢光内容）。
		if end-start < chunkTokens/minTailDivisor && len(chunks) > 0 {
			break
		}
		part, err := codec.Decode(ids[start:end])
		if err != nil {
			return nil, fmt.Errorf("decode moderation chunk: %w", err)
		}
		chunks = append(chunks, part)
	}
	return chunks, nil
}

// mergeMaxCategoryScores 取各分片同一分类的最高分。
func mergeMaxCategoryScores(all []map[string]float64) map[string]float64 {
	merged := map[string]float64{}
	for _, scores := range all {
		for category, score := range scores {
			if current, ok := merged[category]; !ok || score > current {
				merged[category] = score
			}
		}
	}
	return merged
}

// chunkedCount 返回分块结果的实际送审片数，未走分块时返回 0，供日志字段使用。
func chunkedCount(res *chunkedModerationResult) int {
	if res == nil {
		return 0
	}
	return res.ChunkCount
}

// moderateForCheck 按配置决定审核走单次调用还是分块，返回合并后的分类分数。
//
// 分块结果一并返回（未启用时为 nil），让调用方能把片数、丢弃片数记进日志——
// 分块的收益和代价都必须可观测，否则无法判断上限设得是否合理。
func (s *ContentModerationService) moderateForCheck(
	ctx context.Context,
	cfg *ContentModerationConfig,
	content ContentModerationInput,
	trackKeyLoad bool,
) (map[string]float64, *chunkedModerationResult, error) {
	if cfg == nil || !cfg.ChunkModerationEnabled {
		result, err := s.callModeration(ctx, cfg, content.ModerationInput(), trackKeyLoad)
		if err != nil {
			return nil, nil, err
		}
		return result.CategoryScores, nil, nil
	}
	chunked, err := s.moderateChunked(ctx, cfg, content, trackKeyLoad)
	if err != nil {
		return nil, nil, err
	}
	return chunked.CategoryScores, chunked, nil
}

// chunkedModerationResult 汇总一次分片审核的结果。
type chunkedModerationResult struct {
	CategoryScores map[string]float64
	ChunkCount     int
	TokenCount     int
	// DroppedChunks 是因超过 ChunkMaxChunks 而未送审的片数。大于 0 意味着本次
	// 审核并未覆盖全文——调用方需要把它记进日志，否则又成了一次静默的不完全审核。
	DroppedChunks int
}

// moderateChunked 把输入按 token 分片后并发送审，返回各分类的最高分。
//
// 图片只随第一片发送：limitContentModerationImages 本身限制图片数量，
// 重复发送只会放大成本而不增加信息。
//
// trackKeyLoad 透传给 callModeration：前置审核分块时每一片都真实占用 key 容量，
// 必须计入负载统计；重放是管理员旁路操作，传 false 以免污染运行指标。
func (s *ContentModerationService) moderateChunked(
	ctx context.Context,
	cfg *ContentModerationConfig,
	content ContentModerationInput,
	trackKeyLoad bool,
) (*chunkedModerationResult, error) {
	// 用未截断的全文：分片的意义就是覆盖 maxModerationInputRunes 之外的内容。
	sourceText := content.chunkSourceText()
	chunks, err := splitTextByTokens(sourceText, cfg.ChunkTokens)
	if err != nil {
		return nil, err
	}
	chunks, dropped := limitChunks(chunks, cfg.ChunkMaxChunks)
	if len(chunks) == 0 && len(content.Images) == 0 {
		return nil, fmt.Errorf("no moderation input after chunking")
	}

	tokenCount := 0
	if codec, cerr := contentModerationChunkCodec(); cerr == nil {
		if n, nerr := codec.Count(sourceText); nerr == nil {
			tokenCount = n
		}
	}

	// 没有文本但有图片时，仍需发一次纯图片审核。
	inputs := make([]ContentModerationInput, 0, max(len(chunks), 1))
	if len(chunks) == 0 {
		inputs = append(inputs, ContentModerationInput{Images: content.Images})
	} else {
		for i, chunk := range chunks {
			item := ContentModerationInput{Text: chunk}
			if i == 0 {
				item.Images = content.Images
			}
			inputs = append(inputs, item)
		}
	}

	concurrency := cfg.ChunkConcurrency
	if concurrency <= 0 {
		concurrency = defaultContentModerationChunkConcurrency
	}
	if concurrency > len(inputs) {
		concurrency = len(inputs)
	}

	var (
		mu       sync.Mutex
		allScore = make([]map[string]float64, len(inputs))
		firstErr error
		wg       sync.WaitGroup
	)
	sem := make(chan struct{}, concurrency)
	for i := range inputs {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			// 分片审核是否计入 key 负载由调用方决定：前置审核计入，重放不计。
			res, callErr := s.callModeration(ctx, cfg, inputs[idx].ModerationInput(), trackKeyLoad)
			mu.Lock()
			defer mu.Unlock()
			if callErr != nil {
				if firstErr == nil {
					firstErr = callErr
				}
				return
			}
			allScore[idx] = res.CategoryScores
		}(i)
	}
	wg.Wait()
	if firstErr != nil {
		return nil, firstErr
	}

	return &chunkedModerationResult{
		CategoryScores: mergeMaxCategoryScores(allScore),
		ChunkCount:     len(inputs),
		TokenCount:     tokenCount,
		DroppedChunks:  dropped,
	}, nil
}
