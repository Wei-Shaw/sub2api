var __awaiter = (this && this.__awaiter) || function (thisArg, _arguments, P, generator) {
    function adopt(value) { return value instanceof P ? value : new P(function (resolve) { resolve(value); }); }
    return new (P || (P = Promise))(function (resolve, reject) {
        function fulfilled(value) { try { step(generator.next(value)); } catch (e) { reject(e); } }
        function rejected(value) { try { step(generator["throw"](value)); } catch (e) { reject(e); } }
        function step(result) { result.done ? resolve(result.value) : adopt(result.value).then(fulfilled, rejected); }
        step((generator = generator.apply(thisArg, _arguments || [])).next());
    });
};
var __generator = (this && this.__generator) || function (thisArg, body) {
    var _ = { label: 0, sent: function() { if (t[0] & 1) throw t[1]; return t[1]; }, trys: [], ops: [] }, f, y, t, g = Object.create((typeof Iterator === "function" ? Iterator : Object).prototype);
    return g.next = verb(0), g["throw"] = verb(1), g["return"] = verb(2), typeof Symbol === "function" && (g[Symbol.iterator] = function() { return this; }), g;
    function verb(n) { return function (v) { return step([n, v]); }; }
    function step(op) {
        if (f) throw new TypeError("Generator is already executing.");
        while (g && (g = 0, op[0] && (_ = 0)), _) try {
            if (f = 1, y && (t = op[0] & 2 ? y["return"] : op[0] ? y["throw"] || ((t = y["return"]) && t.call(y), 0) : y.next) && !(t = t.call(y, op[1])).done) return t;
            if (y = 0, t) op = [op[0] & 2, t.value];
            switch (op[0]) {
                case 0: case 1: t = op; break;
                case 4: _.label++; return { value: op[1], done: false };
                case 5: _.label++; y = op[1]; op = [0]; continue;
                case 7: op = _.ops.pop(); _.trys.pop(); continue;
                default:
                    if (!(t = _.trys, t = t.length > 0 && t[t.length - 1]) && (op[0] === 6 || op[0] === 2)) { _ = 0; continue; }
                    if (op[0] === 3 && (!t || (op[1] > t[0] && op[1] < t[3]))) { _.label = op[1]; break; }
                    if (op[0] === 6 && _.label < t[1]) { _.label = t[1]; t = op; break; }
                    if (t && _.label < t[2]) { _.label = t[2]; _.ops.push(op); break; }
                    if (t[2]) _.ops.pop();
                    _.trys.pop(); continue;
            }
            op = body.call(thisArg, _);
        } catch (e) { op = [6, e]; y = 0; } finally { f = t = 0; }
        if (op[0] & 5) throw op[1]; return { value: op[0] ? op[1] : void 0, done: true };
    }
};
import { defineConfig, loadEnv } from 'vite';
import vue from '@vitejs/plugin-vue';
import checker from 'vite-plugin-checker';
import { resolve } from 'path';
function escapeHtml(value) {
    return value.replace(/[&<>"']/g, function (character) { return ({
        '&': '&amp;',
        '<': '&lt;',
        '>': '&gt;',
        '"': '&quot;',
        "'": '&#39;',
    })[character] || character; });
}
function isSafeImageUrl(value) {
    var trimmed = value.trim();
    if ((trimmed.startsWith('/') && !trimmed.startsWith('//')) || /^data:image\//i.test(trimmed)) {
        return true;
    }
    try {
        var parsed = new URL(trimmed);
        return parsed.protocol === 'http:' || parsed.protocol === 'https:';
    }
    catch (_a) {
        return false;
    }
}
function injectBranding(html, config) {
    var _a, _b;
    var brandedHtml = html;
    var siteName = (_a = config.site_name) === null || _a === void 0 ? void 0 : _a.trim();
    if (siteName) {
        brandedHtml = brandedHtml.replace(/<title>[^<]*<\/title>/i, "<title>".concat(escapeHtml(siteName), " - AI API Gateway</title>"));
    }
    var siteLogo = (_b = config.site_logo) === null || _b === void 0 ? void 0 : _b.trim();
    if (siteLogo && isSafeImageUrl(siteLogo)) {
        brandedHtml = brandedHtml.replace(/<link\s+rel=["']icon["'][^>]*>/i, "<link rel=\"icon\" href=\"".concat(escapeHtml(siteLogo), "\" />"));
    }
    return brandedHtml;
}
/**
 * Vite 插件：开发模式下注入公开配置到 index.html
 * 与生产模式的后端注入行为保持一致，消除闪烁
 */
function injectPublicSettings(backendUrl, enabled) {
    if (enabled === void 0) { enabled = true; }
    return {
        name: 'inject-public-settings',
        apply: 'serve',
        transformIndexHtml: {
            order: 'pre',
            handler: function (html) {
                return __awaiter(this, void 0, void 0, function () {
                    var response, data, script, e_1;
                    return __generator(this, function (_a) {
                        switch (_a.label) {
                            case 0:
                                if (!enabled)
                                    return [2 /*return*/, html];
                                _a.label = 1;
                            case 1:
                                _a.trys.push([1, 5, , 6]);
                                return [4 /*yield*/, fetch("".concat(backendUrl, "/api/v1/settings/public"), {
                                        signal: AbortSignal.timeout(2000)
                                    })];
                            case 2:
                                response = _a.sent();
                                if (!response.ok) return [3 /*break*/, 4];
                                return [4 /*yield*/, response.json()];
                            case 3:
                                data = _a.sent();
                                if (data.code === 0 && data.data) {
                                    script = "<script>window.__APP_CONFIG__=".concat(JSON.stringify(data.data), ";</script>");
                                    return [2 /*return*/, injectBranding(html, data.data).replace('</head>', "".concat(script, "\n</head>"))];
                                }
                                _a.label = 4;
                            case 4: return [3 /*break*/, 6];
                            case 5:
                                e_1 = _a.sent();
                                console.warn('[vite] 无法获取公开配置，将回退到 API 调用:', e_1.message);
                                return [3 /*break*/, 6];
                            case 6: return [2 /*return*/, html];
                        }
                    });
                });
            }
        }
    };
}
export default defineConfig(function (_a) {
    var mode = _a.mode;
    // 加载环境变量
    var env = loadEnv(mode, process.cwd(), '');
    var backendUrl = env.VITE_DEV_PROXY_TARGET || 'http://localhost:8080';
    var devPort = Number(env.VITE_DEV_PORT || 3000);
    return {
        plugins: [
            vue(),
            checker({
                vueTsc: true
            }),
            injectPublicSettings(backendUrl, env.VITE_LOCAL_PREVIEW !== 'true')
        ],
        resolve: {
            alias: {
                '@': resolve(__dirname, 'src'),
                // 使用 vue-i18n 运行时版本，避免 CSP unsafe-eval 问题
                'vue-i18n': 'vue-i18n/dist/vue-i18n.runtime.esm-bundler.js'
            }
        },
        define: {
            // 启用 vue-i18n JIT 编译，在 CSP 环境下处理消息插值
            // JIT 编译器生成 AST 对象而非 JS 代码，无需 unsafe-eval
            __INTLIFY_JIT_COMPILATION__: true
        },
        build: {
            outDir: '../backend/internal/web/dist',
            emptyOutDir: true,
            rollupOptions: {
                output: {
                    /**
                     * 手动分包配置
                     * 分离第三方库并按功能合并应用代码，避免循环依赖
                     */
                    manualChunks: function (id) {
                        if (id.includes('node_modules')) {
                            // Vue 核心库
                            if (id.includes('/vue/') ||
                                id.includes('/vue-router/') ||
                                id.includes('/pinia/') ||
                                id.includes('/@vue/')) {
                                return 'vendor-vue';
                            }
                            // UI 工具库（较大，单独分离）
                            if (id.includes('/@vueuse/') || id.includes('/xlsx/')) {
                                return 'vendor-ui';
                            }
                            // 图表库
                            if (id.includes('/chart.js/') || id.includes('/vue-chartjs/')) {
                                return 'vendor-chart';
                            }
                            // 国际化
                            if (id.includes('/vue-i18n/') || id.includes('/@intlify/')) {
                                return 'vendor-i18n';
                            }
                            // Stripe 仅在支付流程中按需加载，避免进入首页公共依赖。
                            if (id.includes('/@stripe/stripe-js/')) {
                                return 'vendor-stripe';
                            }
                            // 其他小型第三方库合并
                            return 'vendor-misc';
                        }
                        // 应用代码：按入口点自动分包，不手动干预
                        // 这样可以避免循环依赖，同时保持合理的 chunk 数量
                    }
                }
            }
        },
        server: {
            host: '0.0.0.0',
            port: devPort,
            proxy: {
                '/api': {
                    target: backendUrl,
                    changeOrigin: true
                },
                '/v1': {
                    target: backendUrl,
                    changeOrigin: true
                },
                '/setup': {
                    target: backendUrl,
                    changeOrigin: true
                }
            }
        }
    };
});
