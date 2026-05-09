export type PrerenderSettingsPayload = {
    code?: number;
    data?: {
        frontend_url?: string;
        site_name?: string;
        site_subtitle?: string;
        seo_default_title?: string;
        seo_home_title?: string;
        seo_default_description?: string;
        seo_home_description?: string;
        seo_default_og_image?: string;
        seo_default_robots?: string;
        seo_home_robots?: string;
        login_agreement_documents?: Array<{
            id?: string;
            title?: string;
            content_md?: string;
        }>;
        custom_menu_items?: Array<{
            id?: string;
            label?: string;
            url?: string;
            page_slug?: string;
            seo_title?: string;
            seo_description?: string;
            seo_og_image?: string;
            seo_robots?: string;
            visibility?: string;
        }>;
    };
};
export type PrerenderRouteEntry = {
    route: string;
    title: string;
    html?: string;
    markdown?: string;
    markdownSlug?: string;
    source?: 'base' | 'legal' | 'custom-markdown' | 'tutorial';
    seoTitle?: string;
    seoDescription?: string;
    seoOGImage?: string;
    seoRobots?: string;
};
export declare const BASE_PUBLIC_PRERENDER_ROUTES: string[];
export declare function collectPrerenderRoutes(payload: PrerenderSettingsPayload | null | undefined): PrerenderRouteEntry[];
export declare function renderSimpleMarkdownHTML(markdown: string): string;
export declare function rewriteRelativeMarkdownImages(markdown: string, pageSlug?: string): string;
export declare function buildPageImageURL(pageSlug: string, src: string): string;
export declare function injectPrerenderContent(indexHTML: string, entry: PrerenderRouteEntry): string;
export declare function buildPrerenderManifest(entries: PrerenderRouteEntry[]): {
    generated_at: string;
    total_routes: number;
    routes: {
        route: string;
        title: string;
        source: "base" | "legal" | "custom-markdown" | "tutorial";
        has_markdown: boolean;
        has_html: boolean;
        markdown_slug: string;
        output: string;
    }[];
};
