package keywordmatcher

import "strings"

// Normalize trims, deduplicates, and caps a keyword list using the same
// rules for every caller. Deduplication is case-insensitive and keeps the
// first configured spelling.
func Normalize(keywords []string, maxKeywords, maxKeywordRunes int) []string {
	if len(keywords) == 0 || maxKeywords <= 0 || maxKeywordRunes <= 0 {
		return []string{}
	}
	result := make([]string, 0, minInt(len(keywords), maxKeywords))
	seen := make(map[string]struct{}, len(keywords))
	for _, raw := range keywords {
		keyword := strings.TrimSpace(raw)
		if keyword == "" {
			continue
		}
		runes := []rune(keyword)
		if len(runes) > maxKeywordRunes {
			keyword = string(runes[:maxKeywordRunes])
		}
		key := strings.ToLower(keyword)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, keyword)
		if len(result) >= maxKeywords {
			break
		}
	}
	return result
}

func minInt(left, right int) int {
	if left < right {
		return left
	}
	return right
}

// Matcher performs case-insensitive substring matching while preserving the
// configured keyword order when more than one keyword matches.
type Matcher struct {
	nodes           []node
	edges           []edge
	rootTransitions [256]int32
	keywords        []string
}

type node struct {
	failure     int32
	bestKeyword int32
	edgeStart   uint32
	edgeCount   uint16
}

type edge struct {
	target int32
	label  byte
}

type buildEdge struct {
	target      int32
	nextSibling int32
	label       byte
}

// New builds an immutable matcher. Empty keyword entries are ignored, but the
// original configured spelling is returned when a keyword matches.
func New(keywords []string) *Matcher {
	if len(keywords) == 0 {
		return nil
	}

	buildNodes := []node{newNode()}
	buildEdges := make([]buildEdge, 0)
	originalKeywords := append([]string(nil), keywords...)

	for keywordIndex, keyword := range keywords {
		if keyword == "" {
			continue
		}
		state := int32(0)
		for _, label := range []byte(strings.ToLower(keyword)) {
			next := buildTransition(buildNodes, buildEdges, state, label)
			if next < 0 {
				next = int32(len(buildNodes))
				buildNodes = append(buildNodes, newNode())
				buildEdges = append(buildEdges, buildEdge{
					target:      next,
					nextSibling: firstBuildEdge(buildNodes[state]),
					label:       label,
				})
				buildNodes[state].edgeStart = uint32(len(buildEdges))
			}
			state = next
		}
		if current := buildNodes[state].bestKeyword; current < 0 || int32(keywordIndex) < current {
			buildNodes[state].bestKeyword = int32(keywordIndex)
		}
	}

	if len(buildNodes) == 1 {
		return nil
	}

	queue := make([]int32, 0, len(buildNodes)-1)
	var rootTransitions [256]int32
	for edgeIndex := firstBuildEdge(buildNodes[0]); edgeIndex >= 0; edgeIndex = buildEdges[edgeIndex].nextSibling {
		edge := buildEdges[edgeIndex]
		rootTransitions[edge.label] = edge.target
		queue = append(queue, edge.target)
	}

	for queueIndex := 0; queueIndex < len(queue); queueIndex++ {
		state := queue[queueIndex]
		for edgeIndex := firstBuildEdge(buildNodes[state]); edgeIndex >= 0; edgeIndex = buildEdges[edgeIndex].nextSibling {
			edge := buildEdges[edgeIndex]
			failure := buildNodes[state].failure
			fallback := buildTransition(buildNodes, buildEdges, failure, edge.label)
			for fallback < 0 && failure != 0 {
				failure = buildNodes[failure].failure
				fallback = buildTransition(buildNodes, buildEdges, failure, edge.label)
			}
			if fallback >= 0 {
				buildNodes[edge.target].failure = fallback
			}
			buildNodes[edge.target].bestKeyword = minKeywordIndex(
				buildNodes[edge.target].bestKeyword,
				buildNodes[buildNodes[edge.target].failure].bestKeyword,
			)
			queue = append(queue, edge.target)
		}
	}

	edges := make([]edge, 0, len(buildEdges))
	var outgoing [256]edge
	for nodeIndex := range buildNodes {
		count := 0
		for edgeIndex := firstBuildEdge(buildNodes[nodeIndex]); edgeIndex >= 0; edgeIndex = buildEdges[edgeIndex].nextSibling {
			built := buildEdges[edgeIndex]
			outgoing[count] = edge{target: built.target, label: built.label}
			count++
		}
		for index := 1; index < count; index++ {
			current := outgoing[index]
			insertAt := index
			for insertAt > 0 && current.label < outgoing[insertAt-1].label {
				outgoing[insertAt] = outgoing[insertAt-1]
				insertAt--
			}
			outgoing[insertAt] = current
		}
		buildNodes[nodeIndex].edgeStart = uint32(len(edges))
		buildNodes[nodeIndex].edgeCount = uint16(count)
		edges = append(edges, outgoing[:count]...)
	}

	return &Matcher{nodes: buildNodes, edges: edges, rootTransitions: rootTransitions, keywords: originalKeywords}
}

func newNode() node { return node{bestKeyword: -1} }

func firstBuildEdge(value node) int32 {
	if value.edgeStart == 0 {
		return -1
	}
	return int32(value.edgeStart - 1)
}

func buildTransition(nodes []node, edges []buildEdge, state int32, label byte) int32 {
	if state < 0 || int(state) >= len(nodes) {
		return -1
	}
	for edgeIndex := firstBuildEdge(nodes[state]); edgeIndex >= 0; edgeIndex = edges[edgeIndex].nextSibling {
		if edges[edgeIndex].label == label {
			return edges[edgeIndex].target
		}
	}
	return -1
}

func minKeywordIndex(left, right int32) int32 {
	if left < 0 {
		return right
	}
	if right < 0 || left < right {
		return left
	}
	return right
}

// Match returns the first matching keyword according to configured order.
func (m *Matcher) Match(text string) (string, bool) {
	if m == nil || text == "" || len(m.nodes) == 0 || len(m.keywords) == 0 {
		return "", false
	}
	lower := strings.ToLower(text)
	state := int32(0)
	bestKeyword := int32(-1)
	for index := 0; index < len(lower); index++ {
		label := lower[index]
		for {
			next := m.next(state, label)
			if next != 0 {
				state = next
				break
			}
			if state == 0 {
				break
			}
			state = m.nodes[state].failure
		}
		bestKeyword = minKeywordIndex(bestKeyword, m.nodes[state].bestKeyword)
		if bestKeyword == 0 {
			return m.keywords[0], true
		}
	}
	if bestKeyword < 0 || int(bestKeyword) >= len(m.keywords) {
		return "", false
	}
	return m.keywords[bestKeyword], true
}

func (m *Matcher) next(state int32, label byte) int32 {
	if state == 0 {
		return m.rootTransitions[label]
	}
	if state < 0 || int(state) >= len(m.nodes) {
		return 0
	}
	node := m.nodes[state]
	left := int(node.edgeStart)
	right := left + int(node.edgeCount)
	for left < right {
		middle := left + (right-left)/2
		edge := m.edges[middle]
		if edge.label < label {
			left = middle + 1
			continue
		}
		right = middle
	}
	end := int(node.edgeStart) + int(node.edgeCount)
	if left < end && m.edges[left].label == label {
		return m.edges[left].target
	}
	return 0
}
