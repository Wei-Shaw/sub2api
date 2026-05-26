package routes

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/plugin"
	"github.com/Wei-Shaw/sub2api/internal/web"
	"github.com/gin-gonic/gin"
)

// 静态 mime 推断, 不依赖 stdlib 的 net/http.DetectContentType (它对 ESM JS
// 经常误识别成 text/plain). 仅覆盖插件 frontend bundle 实际会用到的扩展名.
var pluginAssetMimeByExt = map[string]string{
	".js":    "application/javascript; charset=utf-8",
	".mjs":   "application/javascript; charset=utf-8",
	".css":   "text/css; charset=utf-8",
	".map":   "application/json; charset=utf-8",
	".json":  "application/json; charset=utf-8",
	".svg":   "image/svg+xml",
	".png":   "image/png",
	".jpg":   "image/jpeg",
	".jpeg":  "image/jpeg",
	".gif":   "image/gif",
	".webp":  "image/webp",
	".woff":  "font/woff",
	".woff2": "font/woff2",
	".html":  "text/html; charset=utf-8",
}

const (
	// pluginAssetCacheControl 给 5 分钟的可缓存窗口, 加 must-revalidate 让浏览器
	// 在 ETag 命中时仍走 304. 用于无 cache-busting 版本号的请求 (importmap
	// 共享 runtime, plugin 启动后 prefetch 还没完成时拿到旧 manifest 的
	// 客户端). 这条路径仍依赖 ETag 校验来避免长时间持有旧 bundle.
	pluginAssetCacheControl = "public, max-age=300, must-revalidate"

	// pluginAssetVersionedCacheControl 用于 URL 上带 ?v=<content-hash> 的请求.
	// 因为 hash 变 → URL 变 → 拉新资源, 老 URL 永远对应同一份字节, 所以可以
	// 安全地 immutable 缓存一年, 节省 304 验证带宽与延迟. 触发条件: query 里
	// 有非空 v 参数 (manager_frontend.go 在插件 bundle 已知 hash 时拼上).
	pluginAssetVersionedCacheControl = "public, max-age=31536000, immutable"

	// pluginAssetVersionQueryKey 是 entry_js_url / entry_css_url 上 cache-busting
	// query 参数名. 与 manager_frontend.go 的 withVersionQuery 对齐.
	pluginAssetVersionQueryKey = "v"
)

// pickPluginAssetCacheControl 按当前请求是否带 ?v=<hash> 选择对应的
// Cache-Control 值. 命名常量在一处, 调用方只看动词. 复用同一份决策, 让
// importmap shared / plugin entry / plugin-sdk 三条 serve 路径行为一致.
func pickPluginAssetCacheControl(c *gin.Context) string {
	if c.Query(pluginAssetVersionQueryKey) != "" {
		return pluginAssetVersionedCacheControl
	}
	return pluginAssetCacheControl
}

// sharedRuntimeReExport 表名 -> 浏览器侧 ESM proxy 源码模板.
// 每个文件输出 4-12 行 ESM, named exports 直接读取 window 上 host 已注入的
// 单例 (frontend/src/plugins/sdk/expose-runtime.ts 在 main.ts 启动时挂上去),
// default export 直接是整个 module 对象.
//
// 浏览器通过 <script type="importmap"> 把 'vue' / 'pinia' / 'vue-router' /
// 'vue-i18n' / 'axios' 这五个 bare specifier 映射到这些文件的 URL 后,
// plugin bundle 中所有 `import { defineComponent } from 'vue'` 会通过
// 该 proxy 拿到 host 的 vue 单例, 与 host 自身使用同一份, 保证 reactive
// 状态 / Pinia store / vue-router 实例可以跨 host & plugin 边界共享.
var sharedRuntimeReExport = map[string]string{
	"vue.js": `const m = window.__SUB2API_HOST_VUE__;
if (!m) { throw new Error('host did not expose Vue runtime via __SUB2API_HOST_VUE__'); }
export const h = m.h;
export const defineComponent = m.defineComponent;
export const ref = m.ref;
export const computed = m.computed;
export const watch = m.watch;
export const watchEffect = m.watchEffect;
export const watchPostEffect = m.watchPostEffect;
export const watchSyncEffect = m.watchSyncEffect;
export const onMounted = m.onMounted;
export const onUnmounted = m.onUnmounted;
export const onBeforeMount = m.onBeforeMount;
export const onBeforeUnmount = m.onBeforeUnmount;
export const onUpdated = m.onUpdated;
export const onBeforeUpdate = m.onBeforeUpdate;
export const onActivated = m.onActivated;
export const onDeactivated = m.onDeactivated;
export const onErrorCaptured = m.onErrorCaptured;
export const onRenderTracked = m.onRenderTracked;
export const onRenderTriggered = m.onRenderTriggered;
export const onWatcherCleanup = m.onWatcherCleanup;
export const reactive = m.reactive;
export const readonly = m.readonly;
export const shallowRef = m.shallowRef;
export const shallowReactive = m.shallowReactive;
export const shallowReadonly = m.shallowReadonly;
export const toRefs = m.toRefs;
export const toRef = m.toRef;
export const toRaw = m.toRaw;
export const toValue = m.toValue;
export const isRef = m.isRef;
export const isReactive = m.isReactive;
export const isReadonly = m.isReadonly;
export const isShallow = m.isShallow;
export const isProxy = m.isProxy;
export const markRaw = m.markRaw;
export const customRef = m.customRef;
export const effectScope = m.effectScope;
export const provide = m.provide;
export const inject = m.inject;
export const hasInjectionContext = m.hasInjectionContext;
export const nextTick = m.nextTick;
export const createApp = m.createApp;
export const defineAsyncComponent = m.defineAsyncComponent;
export const useAttrs = m.useAttrs;
export const useSlots = m.useSlots;
export const useModel = m.useModel;
export const useId = m.useId;
export const Transition = m.Transition;
export const TransitionGroup = m.TransitionGroup;
export const Teleport = m.Teleport;
export const KeepAlive = m.KeepAlive;
export const Fragment = m.Fragment;
export const Comment = m.Comment;
export const Text = m.Text;
export const Static = m.Static;
export const Suspense = m.Suspense;
export const openBlock = m.openBlock;
export const createElementBlock = m.createElementBlock;
export const createBlock = m.createBlock;
export const createElementVNode = m.createElementVNode;
export const createVNode = m.createVNode;
export const createTextVNode = m.createTextVNode;
export const createCommentVNode = m.createCommentVNode;
export const createStaticVNode = m.createStaticVNode;
export const createSlots = m.createSlots;
export const cloneVNode = m.cloneVNode;
export const isVNode = m.isVNode;
export const mergeProps = m.mergeProps;
export const guardReactiveProps = m.guardReactiveProps;
export const renderList = m.renderList;
export const renderSlot = m.renderSlot;
export const resolveComponent = m.resolveComponent;
export const resolveDirective = m.resolveDirective;
export const resolveDynamicComponent = m.resolveDynamicComponent;
export const withCtx = m.withCtx;
export const withModifiers = m.withModifiers;
export const withDirectives = m.withDirectives;
export const withKeys = m.withKeys;
export const withDefaults = m.withDefaults;
export const withMemo = m.withMemo;
export const mergeModels = m.mergeModels;
export const mergeDefaults = m.mergeDefaults;
export const createPropsRestProxy = m.createPropsRestProxy;
export const normalizeClass = m.normalizeClass;
export const normalizeStyle = m.normalizeStyle;
export const normalizeProps = m.normalizeProps;
export const toDisplayString = m.toDisplayString;
export const toHandlerKey = m.toHandlerKey;
export const camelize = m.camelize;
export const capitalize = m.capitalize;
export const useTemplateRef = m.useTemplateRef;
export const getCurrentInstance = m.getCurrentInstance;
export const getCurrentScope = m.getCurrentScope;
export const onScopeDispose = m.onScopeDispose;
export const triggerRef = m.triggerRef;
export const unref = m.unref;
export const proxyRefs = m.proxyRefs;
export const version = m.version;
export const pushScopeId = m.pushScopeId;
export const popScopeId = m.popScopeId;
export const setBlockTracking = m.setBlockTracking;
export const Fragment_ = m.Fragment;
export const vModelText = m.vModelText;
export const vModelCheckbox = m.vModelCheckbox;
export const vModelDynamic = m.vModelDynamic;
export const vModelRadio = m.vModelRadio;
export const vModelSelect = m.vModelSelect;
export const vShow = m.vShow;
export default m;
`,
	"vue-router.js": `const m = window.__SUB2API_HOST_VUE_ROUTER__;
if (!m) { throw new Error('host did not expose vue-router via __SUB2API_HOST_VUE_ROUTER__'); }
export const useRouter = m.useRouter;
export const useRoute = m.useRoute;
export const useLink = m.useLink;
export const RouterLink = m.RouterLink;
export const RouterView = m.RouterView;
export const createRouter = m.createRouter;
export const createWebHistory = m.createWebHistory;
export const createWebHashHistory = m.createWebHashHistory;
export const createMemoryHistory = m.createMemoryHistory;
export const onBeforeRouteLeave = m.onBeforeRouteLeave;
export const onBeforeRouteUpdate = m.onBeforeRouteUpdate;
export default m;
`,
	"pinia.js": `const m = window.__SUB2API_HOST_PINIA__;
if (!m) { throw new Error('host did not expose pinia via __SUB2API_HOST_PINIA__'); }
export const createPinia = m.createPinia;
export const setActivePinia = m.setActivePinia;
export const getActivePinia = m.getActivePinia;
export const defineStore = m.defineStore;
export const storeToRefs = m.storeToRefs;
export const acceptHMRUpdate = m.acceptHMRUpdate;
export const mapStores = m.mapStores;
export const mapState = m.mapState;
export const mapActions = m.mapActions;
export const mapGetters = m.mapGetters;
export const PiniaVuePlugin = m.PiniaVuePlugin;
export default m;
`,
	"vue-i18n.js": `const m = window.__SUB2API_HOST_VUE_I18N__;
if (!m) { throw new Error('host did not expose vue-i18n via __SUB2API_HOST_VUE_I18N__'); }
export const createI18n = m.createI18n;
export const useI18n = m.useI18n;
export const I18nInjectionKey = m.I18nInjectionKey;
export const VERSION = m.VERSION;
export default m;
`,
	"axios.js": `const m = window.__SUB2API_HOST_AXIOS__;
if (!m) { throw new Error('host did not expose axios via __SUB2API_HOST_AXIOS__'); }
// axios 主接口本身是个可调用对象 + 一组静态方法; ESM consumer 通常做
//   import axios from 'axios'   => 用 default
//   import { isAxiosError } from 'axios' => 用 named
// 我们两侧都暴露.
export const get = m.get?.bind(m);
export const post = m.post?.bind(m);
export const put = m.put?.bind(m);
export const patch = m.patch?.bind(m);
export const _delete = m.delete?.bind(m);
export const request = m.request?.bind(m);
export const create = m.create?.bind(m);
export const all = m.all?.bind(m);
export const spread = m.spread;
export const isAxiosError = m.isAxiosError;
export const isCancel = m.isCancel;
export const CancelToken = m.CancelToken;
export const Axios = m.Axios;
export const AxiosError = m.AxiosError;
export const AxiosHeaders = m.AxiosHeaders;
export const HttpStatusCode = m.HttpStatusCode;
export const VERSION = m.VERSION;
export default m;
`,
}

// servePluginSharedAsset 处理 /api/v1/plugin-assets/__shared__/<name>.js 形式
// 的请求, 返回 sharedRuntimeReExport 中预生成的 ESM proxy 或 host frontend
// embed FS 中的编译产物 (如 plugin-sdk.js / plugin-sdk.css). 找不到时 404.
//
// plugin-sdk 特例: 该文件由 host vite (vite.sdk.config.ts) 编译输出到
// frontend dist/, host 二进制 embed FS 内. 通过 importmap 把
// `@sub2api/plugin-sdk` 映射到此端点, plugin frontend 直接 import 即可.
func servePluginSharedAsset(c *gin.Context, asset string) {
	if asset == "plugin-sdk.js" || asset == "plugin-sdk.css" {
		servePluginSdkBundle(c, asset)
		return
	}
	body, ok := sharedRuntimeReExport[asset]
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "message": "shared runtime not registered: " + asset})
		return
	}
	// ETag = "shared-<asset>-<sha256(body)[:8]>": 文件名标识身份, hash 跟随
	// re-export 模板内容变化, 让浏览器在 host 升级 (新增 / 删除 named export)
	// 后能正确触发 200 而不是误命中 304 拿到旧代理.
	sum := sha256.Sum256([]byte(body))
	etag := `"shared-` + asset + `-` + hex.EncodeToString(sum[:8]) + `"`
	if match := c.GetHeader("If-None-Match"); match == etag {
		c.Header("ETag", etag)
		c.Status(http.StatusNotModified)
		return
	}
	c.Header("ETag", etag)
	c.Header("Cache-Control", pickPluginAssetCacheControl(c))
	c.Data(http.StatusOK, "application/javascript; charset=utf-8", []byte(body))
}

// servePluginSdkBundle 从 host frontend embed FS 读取 plugin-sdk.{js,css}
// 并返回. ETag 基于文件 SHA-256, 让浏览器在 host 重新编译后能正确 304/200.
func servePluginSdkBundle(c *gin.Context, asset string) {
	body, err := web.ReadEmbeddedAsset(asset)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "message": "plugin-sdk bundle not embedded: " + err.Error()})
		return
	}
	sum := sha256.Sum256(body)
	etag := `"sdk-` + hex.EncodeToString(sum[:8]) + `"`
	if match := c.GetHeader("If-None-Match"); match == etag {
		c.Header("ETag", etag)
		c.Status(http.StatusNotModified)
		return
	}
	c.Header("ETag", etag)
	c.Header("Cache-Control", pickPluginAssetCacheControl(c))
	mime := "application/javascript; charset=utf-8"
	if strings.HasSuffix(asset, ".css") {
		mime = "text/css; charset=utf-8"
	}
	c.Data(http.StatusOK, mime, body)
}

// RegisterPluginAssetRoutes 挂载 /api/v1/plugin-assets/:plugin/*path,
// 把请求转换为 PluginManager.FetchFrontendAsset 拉到的字节流并返回给浏览器.
//
// 特殊保留 plugin 名 `__shared__` 用于浏览器 importmap 共享 vue/pinia/...
// 五个 ESM proxy 模块 (见 sharedRuntimeReExport). 这条分支不依赖 PluginManager,
// 所以即使插件系统初始化失败也仍可工作.
//
// pm 为 nil 时不注册任何路由; 这种场景出现在 PluginManager 初始化失败的降级路径,
// 此时插件功能整体不可用, 路由也不应存在以免误导客户端.
func RegisterPluginAssetRoutes(r *gin.Engine, pm *plugin.PluginManager) {
	if pm == nil {
		// 即使插件系统未启用, importmap 共享端点仍要可用——否则 host 注入的
		// importmap 会指向 404, 浏览器在加载首屏 vue/pinia 时直接报错.
		// 但实际不会走到这里: importmap 注入与 plugin manifest 注入互相独立,
		// 注入侧只在有 plugin manifest 时才插 importmap, 这里不注册等同于约定.
		return
	}
	r.GET("/api/v1/plugin-assets/:plugin/*path", func(c *gin.Context) {
		pluginName := c.Param("plugin")
		assetPath := strings.TrimPrefix(c.Param("path"), "/")

		// 共享运行时模块特例: /api/v1/plugin-assets/__shared__/<name>.js
		if pluginName == "__shared__" {
			servePluginSharedAsset(c, assetPath)
			return
		}

		asset, err := pm.FetchFrontendAsset(c.Request.Context(), pluginName, assetPath)
		if err != nil {
			if errors.Is(err, plugin.ErrPluginNotRunning) {
				c.JSON(http.StatusNotFound, gin.H{"code": 404, "message": "plugin not running"})
				return
			}
			c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
			return
		}

		// 304 Not Modified
		if match := c.GetHeader("If-None-Match"); match != "" && match == asset.ETag {
			c.Header("ETag", asset.ETag)
			c.Status(http.StatusNotModified)
			return
		}

		ext := lowerExt(assetPath)
		mime := pluginAssetMimeByExt[ext]
		if mime == "" {
			mime = "application/octet-stream"
		}
		c.Header("ETag", asset.ETag)
		c.Header("Cache-Control", pickPluginAssetCacheControl(c))
		c.Data(http.StatusOK, mime, asset.Data)
	})
}

// lowerExt 抽取最后一个 "." 之后的扩展名, 包含点号. 没有扩展名时返回空字符串.
func lowerExt(p string) string {
	idx := strings.LastIndex(p, ".")
	if idx < 0 {
		return ""
	}
	return strings.ToLower(p[idx:])
}
