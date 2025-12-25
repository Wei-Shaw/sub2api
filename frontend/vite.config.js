import { defineConfig REDACTED from 'vite';
import vue from '@vitejs/plugin-vue';
import checker from 'vite-plugin-checker';
import { resolve REDACTED from 'path';
export default defineConfig({
    plugins: [
        vue(),
        checker({
            typescript: true,
            vueTsc: true
        REDACTED)
    ],
    resolve: {
        alias: {
            '@': resolve(__dirname, 'src')
        REDACTED
    REDACTED,
    build: {
        outDir: '../backend/internal/web/dist',
        emptyOutDir: true
    REDACTED,
    server: {
        host: '0.0.0.0',
        port: 3000,
        proxy: {
            '/api': {
                target: 'http://localhost:8080',
                changeOrigin: true
            REDACTED,
            '/setup': {
                target: 'http://localhost:8080',
                changeOrigin: true
            REDACTED
        REDACTED
    REDACTED
REDACTED);
