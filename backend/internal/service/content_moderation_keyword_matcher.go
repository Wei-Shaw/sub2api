package service

import (
	"strings"
)

type contentModerationKeywordMatcher struct {
	nodes           []contentModerationKeywordNode
	edges           []contentModerationKeywordEdge
	rootTransitions [256]int32
	keywords        []string
REDACTED

type contentModerationKeywordNode struct {
	failure     int32
	bestKeyword int32
	edgeStart   uint32
	edgeCount   uint16
REDACTED

type contentModerationKeywordEdge struct {
	target int32
	label  byte
REDACTED

type contentModerationKeywordBuildEdge struct {
	target      int32
	nextSibling int32
	label       byte
REDACTED

func newContentModerationKeywordMatcher(keywords []string) *contentModerationKeywordMatcher {
	if len(keywords) == 0 {
		return nil
REDACTED

	buildNodes := []contentModerationKeywordNode{newContentModerationKeywordNode()REDACTED
	buildEdges := make([]contentModerationKeywordBuildEdge, 0)
	originalKeywords := append([]string(nil), keywords...)

	for keywordIndex, keyword := range keywords {
		if keyword == "" {
			continue
	REDACTED
		state := int32(0)
		for _, label := range []byte(strings.ToLower(keyword)) {
			next := contentModerationKeywordBuildTransition(buildNodes, buildEdges, state, label)
			if next < 0 {
				next = int32(len(buildNodes))
				buildNodes = append(buildNodes, newContentModerationKeywordNode())
				buildEdges = append(buildEdges, contentModerationKeywordBuildEdge{
					target:      next,
					nextSibling: contentModerationKeywordBuildFirstEdge(buildNodes[state]),
					label:       label,
			REDACTED)
				buildNodes[state].edgeStart = uint32(len(buildEdges))
		REDACTED
			state = next
	REDACTED
		if current := buildNodes[state].bestKeyword; current < 0 || int32(keywordIndex) < current {
			buildNodes[state].bestKeyword = int32(keywordIndex)
	REDACTED
REDACTED

	if len(buildNodes) == 1 {
		return nil
REDACTED

	queue := make([]int32, 0, len(buildNodes)-1)
	var rootTransitions [256]int32
	for edgeIndex := contentModerationKeywordBuildFirstEdge(buildNodes[0]); edgeIndex >= 0; edgeIndex = buildEdges[edgeIndex].nextSibling {
		edge := buildEdges[edgeIndex]
		rootTransitions[edge.label] = edge.target
		queue = append(queue, edge.target)
REDACTED

	for queueIndex := 0; queueIndex < len(queue); queueIndex++ {
		state := queue[queueIndex]
		for edgeIndex := contentModerationKeywordBuildFirstEdge(buildNodes[state]); edgeIndex >= 0; edgeIndex = buildEdges[edgeIndex].nextSibling {
			edge := buildEdges[edgeIndex]
			failure := buildNodes[state].failure
			fallback := contentModerationKeywordBuildTransition(buildNodes, buildEdges, failure, edge.label)
			for fallback < 0 && failure != 0 {
				failure = buildNodes[failure].failure
				fallback = contentModerationKeywordBuildTransition(buildNodes, buildEdges, failure, edge.label)
		REDACTED
			if fallback >= 0 {
				buildNodes[edge.target].failure = fallback
		REDACTED
			buildNodes[edge.target].bestKeyword = minKeywordIndex(
				buildNodes[edge.target].bestKeyword,
				buildNodes[buildNodes[edge.target].failure].bestKeyword,
			)
			queue = append(queue, edge.target)
	REDACTED
REDACTED

	edges := make([]contentModerationKeywordEdge, 0, len(buildEdges))
	var outgoing [256]contentModerationKeywordEdge
	for nodeIndex := range buildNodes {
		count := 0
		for edgeIndex := contentModerationKeywordBuildFirstEdge(buildNodes[nodeIndex]); edgeIndex >= 0; edgeIndex = buildEdges[edgeIndex].nextSibling {
			edge := buildEdges[edgeIndex]
			outgoing[count] = contentModerationKeywordEdge{target: edge.target, label: edge.labelREDACTED
			count++
	REDACTED
		for index := 1; index < count; index++ {
			current := outgoing[index]
			insertAt := index
			for insertAt > 0 && current.label < outgoing[insertAt-1].label {
				outgoing[insertAt] = outgoing[insertAt-1]
				insertAt--
		REDACTED
			outgoing[insertAt] = current
	REDACTED
		buildNodes[nodeIndex].edgeStart = uint32(len(edges))
		buildNodes[nodeIndex].edgeCount = uint16(count)
		edges = append(edges, outgoing[:count]...)
REDACTED

	return &contentModerationKeywordMatcher{
		nodes:           buildNodes,
		edges:           edges,
		rootTransitions: rootTransitions,
		keywords:        originalKeywords,
REDACTED
REDACTED

func newContentModerationKeywordNode() contentModerationKeywordNode {
	return contentModerationKeywordNode{bestKeyword: -1REDACTED
REDACTED

func contentModerationKeywordBuildFirstEdge(node contentModerationKeywordNode) int32 {
	if node.edgeStart == 0 {
		return -1
REDACTED
	return int32(node.edgeStart - 1)
REDACTED

func contentModerationKeywordBuildTransition(
	nodes []contentModerationKeywordNode,
	edges []contentModerationKeywordBuildEdge,
	state int32,
	label byte,
) int32 {
	if state < 0 || int(state) >= len(nodes) {
		return -1
REDACTED
	for edgeIndex := contentModerationKeywordBuildFirstEdge(nodes[state]); edgeIndex >= 0; edgeIndex = edges[edgeIndex].nextSibling {
		if edges[edgeIndex].label == label {
			return edges[edgeIndex].target
	REDACTED
REDACTED
	return -1
REDACTED

func minKeywordIndex(left, right int32) int32 {
	if left < 0 {
		return right
REDACTED
	if right < 0 || left < right {
		return left
REDACTED
	return right
REDACTED

func (m *contentModerationKeywordMatcher) Match(text string) (string, bool) {
	if m == nil || text == "" || len(m.nodes) == 0 || len(m.keywords) == 0 {
		return "", false
REDACTED
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
		REDACTED
			if state == 0 {
				break
		REDACTED
			state = m.nodes[state].failure
	REDACTED
		bestKeyword = minKeywordIndex(bestKeyword, m.nodes[state].bestKeyword)
		if bestKeyword == 0 {
			return m.keywords[0], true
	REDACTED
REDACTED
	if bestKeyword < 0 || int(bestKeyword) >= len(m.keywords) {
		return "", false
REDACTED
	return m.keywords[bestKeyword], true
REDACTED

func (m *contentModerationKeywordMatcher) next(state int32, label byte) int32 {
	if state == 0 {
		return m.rootTransitions[label]
REDACTED
	if state < 0 || int(state) >= len(m.nodes) {
		return 0
REDACTED
	node := m.nodes[state]
	left := int(node.edgeStart)
	right := left + int(node.edgeCount)
	for left < right {
		middle := left + (right-left)/2
		edge := m.edges[middle]
		if edge.label < label {
			left = middle + 1
			continue
	REDACTED
		right = middle
REDACTED
	end := int(node.edgeStart) + int(node.edgeCount)
	if left < end && m.edges[left].label == label {
		return m.edges[left].target
REDACTED
	return 0
REDACTED
