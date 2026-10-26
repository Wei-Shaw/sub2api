package service

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func testOpenAICompactionSummaryKey() []byte {
	return []byte("0123456789abcdef0123456789abcdef")
}

func TestOpenAICompactionSummaryCodecRoundTripAndBinding(t *testing.T) {
	key := testOpenAICompactionSummaryKey()
	groupID := int64(17)
	scope, err := NewOpenAICompactionOwnerScope(42, &groupID)
	require.NoError(t, err)

	sealed, err := SealOpenAICompactionSummary(key, scope, "durable summary", time.Unix(1_700_000_000, 0))
	require.NoError(t, err)
	require.True(t, IsOpenAICompactionSummaryCiphertext(sealed))
	require.NotContains(t, sealed, "durable summary")

	opened, err := OpenOpenAICompactionSummary(key, scope, sealed)
	require.NoError(t, err)
	require.Equal(t, "durable summary", opened)

	otherUser, err := NewOpenAICompactionOwnerScope(43, &groupID)
	require.NoError(t, err)
	_, err = OpenOpenAICompactionSummary(key, otherUser, sealed)
	require.ErrorIs(t, err, ErrOpenAICompactionSummaryInvalid)

	otherGroupID := int64(18)
	otherGroup, err := NewOpenAICompactionOwnerScope(42, &otherGroupID)
	require.NoError(t, err)
	_, err = OpenOpenAICompactionSummary(key, otherGroup, sealed)
	require.ErrorIs(t, err, ErrOpenAICompactionSummaryInvalid)
}

type failingCompactionSummarySecretRepo struct {
	AccountRepository
	err error
}

func (r failingCompactionSummarySecretRepo) GetSecuritySecret(context.Context, string) (string, bool, error) {
	return "", false, r.err
}

func (r failingCompactionSummarySecretRepo) GetOrCreateGeneratedSecuritySecret(context.Context, string, int) (string, bool, error) {
	return "", false, r.err
}

func TestOpenAICompactionSummaryKeyErrorsDoNotExposeStoreDetails(t *testing.T) {
	svc := &OpenAIGatewayService{accountRepo: failingCompactionSummarySecretRepo{
		err: errors.New("database DSN contains fixture-secret"),
	}}

	_, err := svc.openAICompactionSummaryCipherKey(context.Background(), true)
	require.ErrorIs(t, err, ErrOpenAICompactionSummaryKey)
	require.NotContains(t, err.Error(), "fixture-secret")
}

func TestOpenAICompactionSummaryCodecRejectsTamperAndUnknownVersion(t *testing.T) {
	key := testOpenAICompactionSummaryKey()
	scope, err := NewOpenAICompactionOwnerScope(42, nil)
	require.NoError(t, err)
	sealed, err := SealOpenAICompactionSummary(key, scope, "summary", time.Now())
	require.NoError(t, err)

	encoded := strings.TrimPrefix(sealed, openAICompactionSummaryCiphertextPrefix)
	wire, err := base64.RawURLEncoding.DecodeString(encoded)
	require.NoError(t, err)
	wire[len(wire)-1] ^= 0xff
	tampered := openAICompactionSummaryCiphertextPrefix + base64.RawURLEncoding.EncodeToString(wire)
	_, err = OpenOpenAICompactionSummary(key, scope, tampered)
	require.ErrorIs(t, err, ErrOpenAICompactionSummaryInvalid)

	_, err = OpenOpenAICompactionSummary(key, scope, "s2cmp1.v2."+encoded)
	require.ErrorIs(t, err, ErrOpenAICompactionSummaryInvalid)

	_, err = newOpenAICompactionSummaryAEAD([]byte("short"))
	require.ErrorIs(t, err, ErrOpenAICompactionSummaryKey)
}

func TestOpenAICompactionSummaryCodecSealAlwaysProducesReadableBoundary(t *testing.T) {
	key := testOpenAICompactionSummaryKey()
	scope, err := NewOpenAICompactionOwnerScope(42, nil)
	require.NoError(t, err)

	for _, summary := range []string{
		strings.Repeat("<", 64*1024),
		strings.Repeat("&", 64*1024),
		"\x00\x01\x02 control and <html> & entities",
		strings.Repeat("x", openAICompactionSummaryMaxSummaryBytes),
	} {
		sealed, err := SealOpenAICompactionSummary(key, scope, summary, time.Now())
		require.NoError(t, err)
		opened, err := OpenOpenAICompactionSummary(key, scope, sealed)
		require.NoError(t, err)
		require.Equal(t, strings.TrimSpace(summary), opened)
	}

	_, err = SealOpenAICompactionSummary(key, scope, strings.Repeat("x", openAICompactionSummaryMaxSummaryBytes+1), time.Now())
	require.ErrorIs(t, err, ErrOpenAICompactionSummaryInvalid)
}

func TestRestoreOpenAICompactionSummaryInputRestoresGatewayCiphertextOnly(t *testing.T) {
	key := testOpenAICompactionSummaryKey()
	groupID := int64(17)
	scope, err := NewOpenAICompactionOwnerScope(42, &groupID)
	require.NoError(t, err)
	sealed, err := SealOpenAICompactionSummary(key, scope, "gateway summary", time.Now())
	require.NoError(t, err)

	body := []byte(`{"model":"deepseek-flash","input":[` +
		`{"type":"message","role":"user","content":"before"},` +
		`{"type":"reasoning","encrypted_content":"native-reasoning"},` +
		`{"type":"compaction","encrypted_content":"` + sealed + `"},` +
		`{"type":"compaction","encrypted_content":"native-compaction"},` +
		`{"type":"compaction_trigger"}]}`)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Set("api_key", &APIKey{UserID: 42, GroupID: &groupID})
	svc := &OpenAIGatewayService{compactionSummaryKey: append([]byte(nil), key...)}

	restored, changed, err := svc.RestoreOpenAICompactionSummaryInput(context.Background(), c, body)
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, "native-reasoning", gjson.GetBytes(restored, `input.#(type=="reasoning").encrypted_content`).String())
	require.Equal(t, "native-compaction", gjson.GetBytes(restored, `input.#(type=="compaction").encrypted_content`).String())
	require.Contains(t, gjson.GetBytes(restored, "input.2.content.0.text").String(), "gateway summary")
	require.Equal(t, "user", gjson.GetBytes(restored, "input.2.role").String())
	require.Equal(t, "compaction_trigger", gjson.GetBytes(restored, "input.4.type").String())
	require.Contains(t, string(restored), `{"type":"compaction","encrypted_content":"native-compaction"}`)
}

func TestRestoreOpenAICompactionSummaryInputValidatesStateShapeBeforeKeyRead(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set("api_key", &APIKey{UserID: 42})
	svc := &OpenAIGatewayService{}

	cases := []string{
		`{"input":{"type":"compaction","encrypted_content":"s2cmp1.v9.invalid"}}`,
		`{"input":[{"type":"reasoning","encrypted_content":"s2cmp1.v1.invalid"}]}`,
		`{"input":[{"type":"compaction","encrypted_content":"s2cmp2.v1.invalid"}]}`,
	}
	for _, body := range cases {
		_, _, err := svc.RestoreOpenAICompactionSummaryInput(context.Background(), c, []byte(body))
		require.ErrorIs(t, err, ErrOpenAICompactionSummaryInvalid)
		require.False(t, errors.Is(err, ErrOpenAICompactionSummaryKey))
	}
}

func TestRestoreOpenAICompactionSummaryInputSupportsObjectAndEscapedPrefix(t *testing.T) {
	key := testOpenAICompactionSummaryKey()
	scope, err := NewOpenAICompactionOwnerScope(42, nil)
	require.NoError(t, err)
	sealed, err := SealOpenAICompactionSummary(key, scope, "object summary", time.Now())
	require.NoError(t, err)

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set("api_key", &APIKey{UserID: 42})
	svc := &OpenAIGatewayService{compactionSummaryKey: append([]byte(nil), key...)}

	objectBody := []byte(`{"input":{"type":"compaction","encrypted_content":"` + sealed + `"}}`)
	restored, changed, err := svc.RestoreOpenAICompactionSummaryInput(context.Background(), c, objectBody)
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, "message", gjson.GetBytes(restored, "input.type").String())
	require.Contains(t, gjson.GetBytes(restored, "input.content.0.text").String(), "object summary")

	escaped := `\u0073` + strings.TrimPrefix(sealed, "s")
	arrayBody := []byte(`{"input":[{"type":"compaction","encrypted_content":"` + escaped + `"}]}`)
	restored, changed, err = svc.RestoreOpenAICompactionSummaryInput(context.Background(), c, arrayBody)
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, "message", gjson.GetBytes(restored, "input.0.type").String())

	textOnly := []byte(`{"input":[{"type":"message","role":"user","content":"s2cmp1.v1. is only text"}]}`)
	_, changed, err = svc.RestoreOpenAICompactionSummaryInput(context.Background(), c, textOnly)
	require.NoError(t, err)
	require.False(t, changed)
}

func TestRestoreOpenAICompactionSummaryInputRejectsDuplicateMembers(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set("api_key", &APIKey{UserID: 42})
	svc := &OpenAIGatewayService{}

	cases := []string{
		`{"input":[{"type":"message","role":"user","content":"visible"}],"input":[{"type":"compaction","encrypted_content":"s2cmp1.v9.invalid"}]}`,
		`{"input":[{"type":"message","type":"compaction","encrypted_content":"s2cmp1.v1.invalid"}]}`,
		`{"input":[{"type":"compaction","encrypted_content":"native","encrypted_content":"s2cmp1.v9.invalid"}]}`,
		`{"input":[{"t\u0079pe":"message","type":"compaction","encrypted_content":"s2cmp1.v1.invalid"}]}`,
		`{"input":[{"type":"compaction","encrypted_content":"native","encrypted_\u0063ontent":"s2cmp1.v1.invalid"}]}`,
		`{"Input":[{"type":"compaction","encrypted_content":"s2cmp1.v9.invalid"}]}`,
		`{"input":[],"Input":[{"type":"compaction","encrypted_content":"s2cmp1.v9.invalid"}]}`,
		`{"input":[{"Type":"compaction","encrypted_content":"s2cmp1.v9.invalid"}]}`,
		`{"input":[{"type":"compaction","Encrypted_Content":"s2cmp1.v9.invalid"}]}`,
	}
	for _, body := range cases {
		_, _, err := svc.RestoreOpenAICompactionSummaryInput(context.Background(), c, []byte(body))
		require.ErrorIs(t, err, ErrOpenAICompactionSummaryInvalid)
		require.False(t, errors.Is(err, ErrOpenAICompactionSummaryKey))
	}
}

func TestRestoreOpenAICompactionSummaryInputFailsClosedForGatewayCiphertext(t *testing.T) {
	key := testOpenAICompactionSummaryKey()
	groupID := int64(17)
	scope, err := NewOpenAICompactionOwnerScope(42, &groupID)
	require.NoError(t, err)
	sealed, err := SealOpenAICompactionSummary(key, scope, "gateway summary", time.Now())
	require.NoError(t, err)

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set("api_key", &APIKey{UserID: 42, GroupID: &groupID})
	svc := &OpenAIGatewayService{compactionSummaryKey: append([]byte(nil), key...)}
	body := []byte(`{"input":[{"type":"compaction","encrypted_content":"` + sealed + `"},{"type":"message","role":"user","content":"keep me"}]}`)

	wrongScopeBody := body
	c.Set("api_key", &APIKey{UserID: 99, GroupID: &groupID})
	_, _, err = svc.RestoreOpenAICompactionSummaryInput(context.Background(), c, wrongScopeBody)
	require.ErrorIs(t, err, ErrOpenAICompactionSummaryInvalid)

	c.Set("api_key", &APIKey{UserID: 42, GroupID: &groupID})
	unknownVersion := []byte(`{"input":[{"type":"compaction","encrypted_content":"s2cmp1.v9.invalid"}]}`)
	_, _, err = svc.RestoreOpenAICompactionSummaryInput(context.Background(), c, unknownVersion)
	require.ErrorIs(t, err, ErrOpenAICompactionSummaryInvalid)

	noKey := &OpenAIGatewayService{}
	_, _, err = noKey.RestoreOpenAICompactionSummaryInput(context.Background(), c, body)
	require.ErrorIs(t, err, ErrOpenAICompactionSummaryKey)
	require.False(t, errors.Is(err, ErrOpenAICompactionSummaryInvalid))
}
