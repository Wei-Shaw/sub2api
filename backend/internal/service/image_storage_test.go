package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// pngBytes is a minimal payload whose signature makes http.DetectContentType
// report image/png.
var pngBytes = []byte("\x89PNG\r\n\x1a\nfake-png-payload")

type savedImage struct {
	key         string
	contentType string
	data        []byte
}

type fakeImageStorage struct {
	saved []savedImage
	url   string
	err   error
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func (f *fakeImageStorage) Save(_ context.Context, key, contentType string, data []byte) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	f.saved = append(f.saved, savedImage{key: key, contentType: contentType, data: append([]byte(nil), data...)})
	if f.url != "" {
		return f.url, nil
	}
	return "https://cdn.test/" + key, nil
}

func TestImageResultUploaderRewritesB64JSON(t *testing.T) {
	storage := &fakeImageStorage{}
	uploader := NewImageResultUploader(storage, "images/", 0, nil)

	b64 := base64.StdEncoding.EncodeToString(pngBytes)
	result := json.RawMessage(`{"created":1,"data":[{"b64_json":"` + b64 + `","revised_prompt":"a cat"}]}`)

	out, err := uploader.Rewrite(context.Background(), "imgtask_abc", result)
	require.NoError(t, err)

	require.Len(t, storage.saved, 1)
	require.Equal(t, "images/imgtask_abc-0.png", storage.saved[0].key)
	require.Equal(t, "image/png", storage.saved[0].contentType)
	require.Equal(t, pngBytes, storage.saved[0].data)

	var parsed struct {
		Data []map[string]json.RawMessage `json:"data"`
	}
	require.NoError(t, json.Unmarshal(out, &parsed))
	require.Len(t, parsed.Data, 1)
	require.JSONEq(t, `"https://cdn.test/images/imgtask_abc-0.png"`, string(parsed.Data[0]["url"]))
	_, hasB64 := parsed.Data[0]["b64_json"]
	require.False(t, hasB64, "b64_json must be stripped after offload")
	require.JSONEq(t, `"a cat"`, string(parsed.Data[0]["revised_prompt"]), "unrelated fields preserved")
}

func TestImageResultUploaderRewritesURL(t *testing.T) {
	var requested string
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		requested = req.URL.String()
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"image/png"}},
			Body:       io.NopCloser(bytes.NewReader(pngBytes)),
			Request:    req,
		}, nil
	})}

	storage := &fakeImageStorage{}
	uploader := NewImageResultUploader(storage, "images/", 0, client)

	result := json.RawMessage(`{"created":1,"data":[{"url":"https://images.example.test/pic.png"}]}`)
	out, err := uploader.Rewrite(context.Background(), "imgtask_xyz", result)
	require.NoError(t, err)
	require.Equal(t, "https://images.example.test/pic.png", requested)

	require.Len(t, storage.saved, 1)
	require.Equal(t, pngBytes, storage.saved[0].data)
	require.Equal(t, "image/png", storage.saved[0].contentType)

	var parsed struct {
		Data []map[string]json.RawMessage `json:"data"`
	}
	require.NoError(t, json.Unmarshal(out, &parsed))
	require.JSONEq(t, `"https://cdn.test/images/imgtask_xyz-0.png"`, string(parsed.Data[0]["url"]))
}

func TestImageResultUploaderRewritesImageDataURLWithoutHTTP(t *testing.T) {
	httpCalls := 0
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		httpCalls++
		return nil, errors.New("HTTP must not be called for data URLs")
	})}
	storage := &fakeImageStorage{}
	uploader := NewImageResultUploader(storage, "images/", 0, client)
	b64 := base64.StdEncoding.EncodeToString(pngBytes)
	result := json.RawMessage(`{"data":[{"url":"DATA:image/jpeg;name=photo.jpg;BaSe64,` + b64 + `","revised_prompt":"kept"}]}`)

	out, err := uploader.Rewrite(context.Background(), "imgtask_data", result)
	require.NoError(t, err)
	require.Zero(t, httpCalls)
	require.Len(t, storage.saved, 1)
	require.Equal(t, pngBytes, storage.saved[0].data)
	require.Equal(t, "image/png", storage.saved[0].contentType, "detected bytes take precedence over a conflicting declaration")
	require.Equal(t, "images/imgtask_data-0.png", storage.saved[0].key)

	var parsed struct {
		Data []map[string]json.RawMessage `json:"data"`
	}
	require.NoError(t, json.Unmarshal(out, &parsed))
	require.JSONEq(t, `"https://cdn.test/images/imgtask_data-0.png"`, string(parsed.Data[0]["url"]))
	require.JSONEq(t, `"kept"`, string(parsed.Data[0]["revised_prompt"]))
}

func TestImageResultUploaderDataURLValidation(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		wantErr string
	}{
		{name: "missing comma", url: "data:image/png;base64", wantErr: "missing comma"},
		{name: "non image", url: "data:text/plain;base64,aGVsbG8=", wantErr: "is not an image"},
		{name: "non base64", url: "data:image/png,raw", wantErr: "not base64"},
		{name: "invalid base64", url: "data:image/png;base64,%%%", wantErr: "base64 payload"},
		{name: "invalid media type", url: "data:image/png;bad parameter;base64,aGVsbG8=", wantErr: "invalid media type"},
		{name: "parameter after base64", url: "data:image/png;base64;name=photo.png,aGVsbG8=", wantErr: "base64 marker must be the final header token"},
		{name: "duplicate base64 marker", url: "data:image/png;base64;base64,aGVsbG8=", wantErr: "duplicate base64 marker"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			httpCalls := 0
			client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				httpCalls++
				return nil, errors.New("HTTP must not be called for data URLs")
			})}
			uploader := NewImageResultUploader(&fakeImageStorage{}, "images/", 0, client)
			result, err := json.Marshal(map[string]any{"data": []map[string]string{{"url": tt.url}}})
			require.NoError(t, err)

			_, err = uploader.Rewrite(context.Background(), "imgtask_bad", result)
			require.ErrorContains(t, err, tt.wantErr)
			require.Zero(t, httpCalls)
		})
	}
}

func TestImageResultUploaderRejectsOversizedImageDataURL(t *testing.T) {
	storage := &fakeImageStorage{}
	uploader := NewImageResultUploader(storage, "images/", 3, nil)
	payload := base64.StdEncoding.EncodeToString([]byte("four"))
	result := json.RawMessage(`{"data":[{"url":"data:image/png;base64,` + payload + `"}]}`)

	_, err := uploader.Rewrite(context.Background(), "imgtask_large", result)
	require.ErrorContains(t, err, "decoded image data URL exceeds 3 bytes")
	require.Empty(t, storage.saved)
}

func TestImageResultUploaderB64JSONTakesPrecedenceOverDataURL(t *testing.T) {
	storage := &fakeImageStorage{}
	uploader := NewImageResultUploader(storage, "images/", 0, nil)
	b64 := base64.StdEncoding.EncodeToString(pngBytes)
	result := json.RawMessage(`{"data":[{"b64_json":"` + b64 + `","url":"data:text/plain,not-an-image"}]}`)

	_, err := uploader.Rewrite(context.Background(), "imgtask_precedence", result)
	require.NoError(t, err)
	require.Len(t, storage.saved, 1)
	require.Equal(t, pngBytes, storage.saved[0].data)
}

func TestImageResultUploaderPropagatesStorageError(t *testing.T) {
	storage := &fakeImageStorage{err: errors.New("bucket unreachable")}
	uploader := NewImageResultUploader(storage, "images/", 0, nil)

	b64 := base64.StdEncoding.EncodeToString(pngBytes)
	result := json.RawMessage(`{"data":[{"b64_json":"` + b64 + `"}]}`)

	_, err := uploader.Rewrite(context.Background(), "imgtask_err", result)
	require.Error(t, err)
	require.Contains(t, err.Error(), "bucket unreachable")
}

func TestImageResultUploaderNilStoragePassthrough(t *testing.T) {
	var uploader *ImageResultUploader
	result := json.RawMessage(`{"data":[{"url":"https://example.test/x.png"}]}`)
	out, err := uploader.Rewrite(context.Background(), "imgtask_nil", result)
	require.NoError(t, err)
	require.JSONEq(t, string(result), string(out))
}

func TestImageTaskServiceCompleteOffloadsToStorage(t *testing.T) {
	store := &imageTaskMemoryStore{}
	storage := &fakeImageStorage{}
	uploader := NewImageResultUploader(storage, "images/", 0, nil)
	svc := NewImageTaskServiceWithUploader(store, uploader, time.Hour, time.Minute)
	require.True(t, svc.Enabled())

	owner := ImageTaskOwner{UserID: 1, APIKeyID: 2}
	created, err := svc.Create(context.Background(), owner)
	require.NoError(t, err)

	b64 := base64.StdEncoding.EncodeToString(pngBytes)
	result := json.RawMessage(`{"created":1,"data":[{"b64_json":"` + b64 + `"}]}`)
	require.NoError(t, svc.Complete(context.Background(), created.ID, http.StatusOK, result))

	got, err := svc.Get(context.Background(), owner, created.ID)
	require.NoError(t, err)
	require.Equal(t, ImageTaskStatusCompleted, got.Status)
	require.Equal(t, "https://cdn.test/images/"+created.ID+"-0.png", got.ImageURL)
	require.NotContains(t, string(got.Result), "b64_json", "large base64 must not be persisted to Redis")
	require.Len(t, storage.saved, 1)
}

func TestImageTaskServiceCompleteOffloadFailureMarksFailed(t *testing.T) {
	store := &imageTaskMemoryStore{}
	storage := &fakeImageStorage{err: errors.New("bucket unreachable")}
	uploader := NewImageResultUploader(storage, "images/", 0, nil)
	svc := NewImageTaskServiceWithUploader(store, uploader, time.Hour, time.Minute)

	owner := ImageTaskOwner{UserID: 1, APIKeyID: 2}
	created, err := svc.Create(context.Background(), owner)
	require.NoError(t, err)

	b64 := base64.StdEncoding.EncodeToString(pngBytes)
	result := json.RawMessage(`{"data":[{"b64_json":"` + b64 + `"}]}`)
	require.NoError(t, svc.Complete(context.Background(), created.ID, http.StatusOK, result))

	got, err := svc.Get(context.Background(), owner, created.ID)
	require.NoError(t, err)
	require.Equal(t, ImageTaskStatusFailed, got.Status)
	require.Equal(t, http.StatusBadGateway, got.HTTPStatus)
	require.Contains(t, string(got.Error), "object storage")
	require.NotContains(t, string(got.Result), "b64_json", "failed offload must not persist base64 to Redis")
}

func TestImageResultUploaderDefaultClientRejectsPrivateURL(t *testing.T) {
	var hits atomic.Int32
	internal := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		_, _ = w.Write([]byte("GIF89a-payload"))
	}))
	defer internal.Close()

	storage := &fakeImageStorage{}
	uploader := NewImageResultUploader(storage, "images/", 0, nil)
	result := json.RawMessage(`{"data":[{"url":"` + internal.URL + `/a.gif"}]}`)

	_, err := uploader.Rewrite(context.Background(), "imgtask_private", result)
	require.ErrorContains(t, err, "image url host is not allowed")
	require.Zero(t, hits.Load(), "internal service must not be contacted")
	require.Empty(t, storage.saved)
}

func TestImageTaskServiceCompletePrivateURLMarksFailed(t *testing.T) {
	var hits atomic.Int32
	internal := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		_, _ = w.Write(pngBytes)
	}))
	defer internal.Close()

	store := &imageTaskMemoryStore{}
	storage := &fakeImageStorage{}
	svc := NewImageTaskServiceWithUploader(store, NewImageResultUploader(storage, "images/", 0, nil), time.Hour, time.Minute)
	owner := ImageTaskOwner{UserID: 1, APIKeyID: 2}
	created, err := svc.Create(context.Background(), owner)
	require.NoError(t, err)

	result := json.RawMessage(`{"data":[{"url":"` + internal.URL + `/a.gif"}]}`)
	require.NoError(t, svc.Complete(context.Background(), created.ID, http.StatusOK, result))

	got, err := svc.Get(context.Background(), owner, created.ID)
	require.NoError(t, err)
	require.Equal(t, ImageTaskStatusFailed, got.Status)
	require.Empty(t, got.ImageURL)
	require.NotContains(t, string(got.Result), internal.URL)
	require.Zero(t, hits.Load())
	require.Empty(t, storage.saved)
}

func newCountingImageDownloadTransport(proxyURL *url.URL) (*imageDownloadTransport, *atomic.Int32, *atomic.Int32) {
	var directCalls, proxiedCalls atomic.Int32
	respond := func(counter *atomic.Int32) http.RoundTripper {
		return roundTripFunc(func(req *http.Request) (*http.Response, error) {
			counter.Add(1)
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewReader(pngBytes)), Request: req}, nil
		})
	}
	return &imageDownloadTransport{
		direct:  respond(&directCalls),
		proxied: respond(&proxiedCalls),
		proxy:   func(*http.Request) (*url.URL, error) { return proxyURL, nil },
		resolvesPrivate: func(ctx context.Context, host string) (bool, error) {
			if host == "intranet.example.test" {
				return true, nil
			}
			if host == "unresolvable.example.test" {
				return false, errors.New("no such host")
			}
			return isPrivateOrLoopbackHost(ctx, host)
		},
	}, &directCalls, &proxiedCalls
}

func TestImageDownloadTransportRejectsNonPublicHosts(t *testing.T) {
	hosts := []string{
		"http://127.0.0.1:18082/a.gif",
		"http://localhost/a.png",
		"http://LOCALHOST./a.png",
		"http://foo.localhost/a.png",
		"http://10.0.0.8/a.png",
		"http://172.16.3.4/a.png",
		"http://192.168.1.1/a.png",
		"http://100.64.0.1/a.png",
		"http://169.254.169.254/latest/meta-data/",
		"http://metadata.google.internal/computeMetadata/v1/",
		"http://0.0.0.0/a.png",
		"http://[::1]/a.png",
		"http://[::ffff:127.0.0.1]/a.png",
		"http://[fd00::1]/a.png",
		"http://[fe80::1%25eth0]/a.png",
		"http:///a.png",
	}
	for _, proxied := range []bool{false, true} {
		for _, rawURL := range hosts {
			var proxyURL *url.URL
			if proxied {
				proxyURL = &url.URL{Scheme: "http", Host: "127.0.0.1:7890"}
			}
			transport, directCalls, proxiedCalls := newCountingImageDownloadTransport(proxyURL)
			req, err := http.NewRequest(http.MethodGet, rawURL, nil)
			require.NoError(t, err)

			_, err = transport.RoundTrip(req)
			require.Error(t, err, "proxied=%v url=%s", proxied, rawURL)
			require.Zero(t, directCalls.Load(), rawURL)
			require.Zero(t, proxiedCalls.Load(), rawURL)
		}
	}
}

func TestImageDownloadTransportRoutesPublicHosts(t *testing.T) {
	transport, directCalls, proxiedCalls := newCountingImageDownloadTransport(nil)
	req, err := http.NewRequest(http.MethodGet, "https://203.0.113.10/a.png", nil)
	require.NoError(t, err)
	resp, err := transport.RoundTrip(req)
	require.NoError(t, err)
	_ = resp.Body.Close()
	require.EqualValues(t, 1, directCalls.Load())
	require.Zero(t, proxiedCalls.Load())

	transport, directCalls, proxiedCalls = newCountingImageDownloadTransport(&url.URL{Scheme: "http", Host: "127.0.0.1:7890"})
	req, err = http.NewRequest(http.MethodGet, "https://203.0.113.10/a.png", nil)
	require.NoError(t, err)
	resp, err = transport.RoundTrip(req)
	require.NoError(t, err)
	_ = resp.Body.Close()
	require.Zero(t, directCalls.Load())
	require.EqualValues(t, 1, proxiedCalls.Load(), "a loopback env proxy must still be usable for public targets")
}

func TestImageDownloadClientRejectsRedirectToPrivateHost(t *testing.T) {
	var calls atomic.Int32
	transport := &imageDownloadTransport{
		direct: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			calls.Add(1)
			return &http.Response{
				StatusCode: http.StatusFound,
				Header:     http.Header{"Location": []string{"http://169.254.169.254/latest/meta-data/"}},
				Body:       io.NopCloser(bytes.NewReader(nil)),
				Request:    req,
			}, nil
		}),
	}
	uploader := NewImageResultUploader(&fakeImageStorage{}, "images/", 0, &http.Client{Transport: transport})

	_, err := uploader.Rewrite(context.Background(), "imgtask_redirect", json.RawMessage(`{"data":[{"url":"https://images.example.test/a.png"}]}`))
	require.ErrorContains(t, err, "image url host is not allowed: 169.254.169.254")
	require.EqualValues(t, 1, calls.Load(), "the private redirect hop must not be sent")
}

func TestImageDownloadClientDialsOnlyPublicAddresses(t *testing.T) {
	var hits atomic.Int32
	internal := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		_, _ = w.Write(pngBytes)
	}))
	defer internal.Close()

	client := newImageDownloadHTTPClient(nil)
	direct, ok := client.Transport.(*imageDownloadTransport).direct.(*http.Transport)
	require.True(t, ok)
	require.Nil(t, direct.Proxy, "the direct transport must not pick up an environment proxy")

	// 绕过 RoundTrip 预检直接 dial：IP 字面量与经 DNS 解析到回环的域名都必须在 socket 层被拒。
	conn, err := direct.DialContext(context.Background(), "tcp", internal.Listener.Addr().String())
	if conn != nil {
		_ = conn.Close()
	}
	require.ErrorContains(t, err, "address not allowed for image download")
	_, port, err := net.SplitHostPort(internal.Listener.Addr().String())
	require.NoError(t, err)
	conn, err = direct.DialContext(context.Background(), "tcp", net.JoinHostPort("localhost", port))
	if conn != nil {
		_ = conn.Close()
	}
	require.ErrorContains(t, err, "address not allowed for image download")
	require.Zero(t, hits.Load())
}

func newStaticImageDownloadClient(contentType string, body []byte) *http.Client {
	return &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		header := http.Header{}
		if contentType != "" {
			header.Set("Content-Type", contentType)
		}
		return &http.Response{StatusCode: http.StatusOK, Header: header, Body: io.NopCloser(bytes.NewReader(body)), Request: req}, nil
	})}
}

func TestImageResultUploaderDownloadContentTypes(t *testing.T) {
	avif := []byte("\x00\x00\x00\x1cftypavif-not-sniffable")
	tests := []struct {
		name        string
		contentType string
		body        []byte
		wantType    string
		wantErr     string
	}{
		{name: "declared png", contentType: "image/png", body: pngBytes, wantType: "image/png"},
		{name: "octet-stream sniffed png", contentType: "application/octet-stream", body: pngBytes, wantType: "image/png"},
		{name: "missing header sniffed png", body: pngBytes, wantType: "image/png"},
		{name: "declared avif not sniffable", contentType: "Image/AVIF; charset=binary", body: avif, wantType: "image/avif"},
		{name: "json body", contentType: "application/json", body: []byte(`{"ok":true}`), wantErr: "content is not an image"},
		{name: "html body", contentType: "text/html", body: []byte("<html>admin</html>"), wantErr: "content is not an image"},
		{name: "empty body", body: nil, wantErr: "content is not an image"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			storage := &fakeImageStorage{}
			uploader := NewImageResultUploader(storage, "images/", 0, newStaticImageDownloadClient(tt.contentType, tt.body))
			_, err := uploader.Rewrite(context.Background(), "imgtask_ct", json.RawMessage(`{"data":[{"url":"https://images.example.test/a"}]}`))
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				require.Empty(t, storage.saved)
				return
			}
			require.NoError(t, err)
			require.Len(t, storage.saved, 1)
			require.Equal(t, tt.wantType, storage.saved[0].contentType)
		})
	}
}

func TestImageDownloadDialControl(t *testing.T) {
	for _, address := range []string{"127.0.0.1:443", "10.1.2.3:80", "100.100.100.200:80", "169.254.169.254:80", "[::1]:443", "[fe80::1%eth0]:443", "[::ffff:192.168.0.1]:443", "bad"} {
		require.Error(t, imageDownloadDialControl("tcp", address, nil), address)
	}
	for _, address := range []string{"203.0.113.10:443", "[2001:db8::1]:443"} {
		require.NoError(t, imageDownloadDialControl("tcp", address, nil), address)
	}
}

func TestImageDownloadTransportProxiedChecksResolvedHost(t *testing.T) {
	for _, host := range []string{"intranet.example.test", "unresolvable.example.test"} {
		transport, directCalls, proxiedCalls := newCountingImageDownloadTransport(&url.URL{Scheme: "http", Host: "127.0.0.1:7890"})
		req, err := http.NewRequest(http.MethodGet, "https://"+host+"/a.png", nil)
		require.NoError(t, err)
		_, err = transport.RoundTrip(req)
		require.Error(t, err, host)
		require.Zero(t, directCalls.Load(), host)
		require.Zero(t, proxiedCalls.Load(), "a proxied request must fail closed when the target is private or unresolvable: %s", host)
	}
}
