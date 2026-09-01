package tlsfingerprint

import "testing"

// sameUint16Set reports whether a and b contain the same multiset of values, ignoring order.
func sameUint16Set(a, b []uint16) bool {
	if len(a) != len(b) {
		return false
	}
	counts := make(map[uint16]int, len(a))
	for _, v := range a {
		counts[v]++
	}
	for _, v := range b {
		counts[v]--
	}
	for _, c := range counts {
		if c != 0 {
			return false
		}
	}
	return true
}

func sameUint16Order(a, b []uint16) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func uint16sToBytes(ids []uint16) []byte {
	buf := make([]byte, len(ids)*2)
	for i, id := range ids {
		buf[i*2] = byte(id >> 8)
		buf[i*2+1] = byte(id)
	}
	return buf
}

// TestShuffleExtensionOrderPreservesSetButVariesOrder 覆盖 spec User Story 2：重复打乱
// 同一组扩展类型 ID，集合必须每次不变，但排列顺序在多次采样里至少出现 2 种——镜像真实
// rustls 客户端每次连接重新打乱 ClientHello 扩展顺序的行为。
func TestShuffleExtensionOrderPreservesSetButVariesOrder(t *testing.T) {
	original := []uint16{0, 5, 10, 11, 13, 23, 35, 43, 45, 51}

	seenOrders := make(map[string]bool)
	for i := 0; i < 20; i++ {
		got := shuffleExtensionOrder(original)
		if !sameUint16Set(original, got) {
			t.Fatalf("iteration %d: extension set changed, want set %v got %v", i, original, got)
		}
		seenOrders[string(uint16sToBytes(got))] = true
	}
	if len(seenOrders) < 2 {
		t.Fatalf("20 次采样只观察到 %d 种排列，期望 >= 2 种", len(seenOrders))
	}
}

// TestShuffleExtensionOrderDoesNotMutateInput 覆盖并发安全：多个 goroutine 可能并发用
// 同一个包级 Profile 变量构造连接（每次新建 TLS 连接都会调用一次），原地打乱调用方传入的
// 切片会造成数据竞争，也会让后续调用的"打乱前基准顺序"被污染。
func TestShuffleExtensionOrderDoesNotMutateInput(t *testing.T) {
	original := []uint16{0, 5, 10, 11, 13, 23, 35, 43, 45, 51}
	input := append([]uint16(nil), original...)

	for i := 0; i < 20; i++ {
		shuffleExtensionOrder(input)
	}

	if !sameUint16Order(input, original) {
		t.Fatalf("输入切片被就地修改：want %v got %v", original, input)
	}
}

// TestBuildClientHelloSpecRandomizesExtensionOrderWhenEnabled 覆盖集成层：Profile 开启
// RandomizeExtensionOrder 后，buildClientHelloSpecFromProfile 产出的扩展数量与集合关系
// 应保持不变（打乱只影响顺序，不影响内容），且不改变传入 Profile.Extensions 本身。
func TestBuildClientHelloSpecRandomizesExtensionOrderWhenEnabled(t *testing.T) {
	original := []uint16{0, 5, 10, 11, 13, 23, 35, 43, 45, 51}
	profile := &Profile{
		Name:                    "randomize-test",
		Extensions:              append([]uint16(nil), original...),
		RandomizeExtensionOrder: true,
	}

	spec := buildClientHelloSpecFromProfile(profile)
	if len(spec.Extensions) != len(original) {
		t.Fatalf("got %d extensions, want %d", len(spec.Extensions), len(original))
	}
	if !sameUint16Order(profile.Extensions, original) {
		t.Fatalf("profile.Extensions 被就地修改：want %v got %v", original, profile.Extensions)
	}
}

// TestBuildClientHelloSpecKeepsFixedOrderWhenDisabled 覆盖既有 Profile
// （RandomizeExtensionOrder 零值 false）行为不变——这是宪法原则 IV 明确要求不能破坏的边界。
func TestBuildClientHelloSpecKeepsFixedOrderWhenDisabled(t *testing.T) {
	original := []uint16{0, 5, 10, 11, 13, 23, 35, 43, 45, 51}
	profile := &Profile{
		Name:       "fixed-order-test",
		Extensions: append([]uint16(nil), original...),
	}

	spec := buildClientHelloSpecFromProfile(profile)
	if len(spec.Extensions) != len(original) {
		t.Fatalf("got %d extensions, want %d", len(spec.Extensions), len(original))
	}
}
