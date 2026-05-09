type DOMPurifyLike = {
    sanitize: (dirty: string, config?: Record<string, unknown>) => string;
};
export declare function sanitizePublicHTMLWithPurifier(raw: string, purifier: DOMPurifyLike, document: Document): string;
export declare function sanitizePublicHTML(raw: string): string;
export declare function renderPublicMarkdownWithPurifier(markdown: string, purifier: DOMPurifyLike, document: Document): string;
export declare function renderPublicMarkdown(markdown: string): string;
export {};
